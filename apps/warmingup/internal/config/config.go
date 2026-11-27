// Package config loads the warmingup configuration: one global YAML file with
// the shared endpoints and one file per warmed domain.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/schedule"
	"github.com/ehlo-pl/gomailtesttool/internal/common/endpoint"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
)

// AppName is the binary name and default directory name.
const AppName = "warmingup"

// Protocols usable for each role.
var (
	SendProtocols   = []string{"smtp"}
	TargetProtocols = []string{"imap", "jmap"}
	BounceProtocols = []string{"imap", "pop3"}
)

// OAuthMicrosoft is the value of an IMAP endpoint's "oauth" setting for
// Outlook.com sign-in with a stored refresh token ("warmingup auth").
const OAuthMicrosoft = "microsoft"

// Config is the complete resolved configuration.
type Config struct {
	DataDir   string
	LogLevel  string
	Endpoints map[string]*endpoint.Endpoint
	Domains   []*Domain
}

// Domain is the warm-up of one sending domain.
type Domain struct {
	Name    string
	File    string
	Enabled bool

	SenderEndpoint string
	From           string // header From, may carry a display name
	FromAddress    string

	BounceEndpoint string
	BounceAddress  string // envelope sender; "" means FromAddress

	Targets []Target

	Location   *time.Location
	Start, End time.Time // campaign window; End zero = open ended
	Schedule   schedule.Params

	ContentSources []string
	Categories     []string

	Hold   Hold
	Engage Engage
	// WarnAfter is the delivery latency above which a message is "late".
	WarnAfter    time.Duration
	DKIMSelector string
}

// Target is one mailbox that receives warm-up mail.
type Target struct {
	Endpoint string
	Address  string
	Engage   bool
}

// Target returns the target by endpoint name.
func (d *Domain) Target(name string) (Target, bool) {
	for _, t := range d.Targets {
		if t.Endpoint == name {
			return t, true
		}
	}
	return Target{}, false
}

// EnvelopeFrom is the SMTP MAIL FROM address.
func (d *Domain) EnvelopeFrom() string {
	if d.BounceAddress != "" {
		return d.BounceAddress
	}
	return d.FromAddress
}

// Hold are the thresholds that stop a ramp.
type Hold struct {
	Window       int     // resolved messages examined per target
	MinSamples   int     // fewer resolved messages never trigger
	BadRatio     float64 // junk + lost
	LateRatio    float64
	DSN          int // consecutive messages with a bounce report
	SendFailures int // consecutive submission failures that hold the domain
}

// Engage is the delay window before marking read and rescuing.
type Engage struct{ MinDelay, MaxDelay time.Duration }

// Domain returns the domain by name, or nil.
func (c *Config) Domain(name string) *Domain {
	for _, d := range c.Domains {
		if strings.EqualFold(d.Name, name) {
			return d
		}
	}
	return nil
}

// ------------------------------------------------------------ file formats

type globalFile struct {
	Warmingup struct {
		DataDir    string `yaml:"datadir"`
		DomainsDir string `yaml:"domains_dir"`
		LogLevel   string `yaml:"loglevel"`
	} `yaml:"warmingup"`
	Endpoints map[string]map[string]any `yaml:"endpoints"`
}

type domainFile struct {
	Domain  string `yaml:"domain"`
	Enabled *bool  `yaml:"enabled"`
	Sender  struct {
		Endpoint string `yaml:"endpoint"`
		From     string `yaml:"from"`
		EHLOName string `yaml:"ehloname"`
	} `yaml:"sender"`
	Bounce struct {
		Endpoint string `yaml:"endpoint"`
		Address  string `yaml:"address"`
	} `yaml:"bounce"`
	Targets  []string `yaml:"targets"`
	Campaign struct {
		Start string `yaml:"start"`
		End   string `yaml:"end"`
	} `yaml:"campaign"`
	Timezone      string    `yaml:"timezone"`
	BusinessHours yaml.Node `yaml:"business_hours"`
	Holidays      []string  `yaml:"holidays"`
	Speed         struct {
		RampDays        *int   `yaml:"ramp_days"`
		StartPerDay     *int   `yaml:"start_per_day"`
		Curve           string `yaml:"curve"`
		BusinessPerHour *int   `yaml:"business_per_hour"`
		OffHoursPerDay  []int  `yaml:"off_hours_per_day"`
		PerDay          *int   `yaml:"per_day"`
		MinGap          string `yaml:"min_gap"`
		MaxPerDay       *int   `yaml:"max_per_day"`
	} `yaml:"speed"`
	Content struct {
		Sources    []string `yaml:"sources"`
		Categories string   `yaml:"categories"`
	} `yaml:"content"`
	Hold struct {
		Window       *int     `yaml:"window"`
		MinSamples   *int     `yaml:"min_samples"`
		BadRatio     *float64 `yaml:"bad_ratio"`
		LateRatio    *float64 `yaml:"late_ratio"`
		DSN          *int     `yaml:"dsn"`
		SendFailures *int     `yaml:"send_failures"`
	} `yaml:"hold"`
	Engage struct {
		MinDelay string `yaml:"min_delay"`
		MaxDelay string `yaml:"max_delay"`
	} `yaml:"engage"`
	WarnAfter    string `yaml:"warn_after"`
	DKIMSelector string `yaml:"dkim_selector"`
}

type hoursFile struct {
	Start string   `yaml:"start"`
	End   string   `yaml:"end"`
	Days  []string `yaml:"days"`
}

// decodeStrict rejects unknown keys so a typo never silently keeps a default.
func decodeStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// Load reads the global file at path and every domain file, and validates
// everything before any network work.
func Load(path string, lookupEnv func(string) (string, bool)) (*Config, error) {
	if path == "" {
		return nil, errors.New("a config file is required (--config, env WARMUPCONFIG)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var g globalFile
	if err := decodeStrict(data, &g); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	c := &Config{
		DataDir:   g.Warmingup.DataDir,
		LogLevel:  g.Warmingup.LogLevel,
		Endpoints: map[string]*endpoint.Endpoint{},
	}
	for name, settings := range g.Endpoints {
		ep, err := endpoint.Parse(name, settings, lookupEnv)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(SendProtocols, ep.Protocol) && !slices.Contains(TargetProtocols, ep.Protocol) && !slices.Contains(BounceProtocols, ep.Protocol) {
			return nil, fmt.Errorf("endpoint %q: unsupported protocol %q (smtp, imap, jmap, pop3)", name, ep.Protocol)
		}
		c.Endpoints[ep.Name] = ep
	}
	if len(c.Endpoints) == 0 {
		return nil, errors.New("config: 'endpoints:' must define at least one endpoint")
	}

	dir := g.Warmingup.DomainsDir
	if dir == "" {
		dir = "domains.d"
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(path), dir)
	}
	var files []string
	for _, pattern := range []string{"*.yaml", "*.yml"} {
		m, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, fmt.Errorf("domains_dir %q: %w", dir, err)
		}
		files = append(files, m...)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no domain files (*.yaml) in %s", dir)
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		d, err := parseDomain(data, c.Endpoints)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		d.File = f
		if c.Domain(d.Name) != nil {
			return nil, fmt.Errorf("%s: domain %s is configured twice", f, d.Name)
		}
		c.Domains = append(c.Domains, d)
	}
	return c, nil
}

func parseDomain(data []byte, endpoints map[string]*endpoint.Endpoint) (*Domain, error) {
	var f domainFile
	if err := decodeStrict(data, &f); err != nil {
		return nil, err
	}
	d := &Domain{Name: strings.ToLower(strings.TrimSpace(f.Domain)), Enabled: f.Enabled == nil || *f.Enabled,
		DKIMSelector: f.DKIMSelector, Categories: wikicontent.ParseCategories(f.Content.Categories)}
	if d.Name == "" || !strings.Contains(d.Name, ".") {
		return nil, errors.New("'domain' is required")
	}
	if err := validation.ValidateHostname(d.Name); err != nil {
		return nil, fmt.Errorf("domain: %w", err)
	}

	use := func(role, name string, protocols []string) (*endpoint.Endpoint, error) {
		ep, ok := endpoints[strings.ToLower(name)]
		if !ok {
			return nil, fmt.Errorf("%s: endpoint %q is not defined", role, name)
		}
		if !slices.Contains(protocols, ep.Protocol) {
			return nil, fmt.Errorf("%s: endpoint %q uses %s; supported: %s", role, name, ep.Protocol, strings.Join(protocols, ", "))
		}
		return ep, nil
	}

	sender, err := use("sender", f.Sender.Endpoint, SendProtocols)
	if err != nil {
		return nil, err
	}
	d.SenderEndpoint = sender.Name
	if f.Sender.EHLOName != "" {
		if err := validation.ValidateHostname(f.Sender.EHLOName); err != nil {
			return nil, fmt.Errorf("sender.ehloname: %w", err)
		}
		// A copy: two domains may share one sender endpoint.
		cp := &endpoint.Endpoint{Name: sender.Name + "@" + d.Name, Protocol: sender.Protocol, Settings: map[string]any{}}
		for k, v := range sender.Settings {
			cp.Settings[k] = v
		}
		cp.Settings["ehloname"] = f.Sender.EHLOName
		endpoints[cp.Name] = cp
		d.SenderEndpoint = cp.Name
	}
	from, err := mail.ParseAddress(f.Sender.From)
	if err != nil {
		return nil, fmt.Errorf("sender.from: %w", err)
	}
	d.From, d.FromAddress = from.String(), from.Address
	if err := validation.ValidateEmail(d.FromAddress); err != nil {
		return nil, fmt.Errorf("sender.from: %w", err)
	}

	if f.Bounce.Endpoint != "" {
		ep, err := use("bounce", f.Bounce.Endpoint, BounceProtocols)
		if err != nil {
			return nil, err
		}
		d.BounceEndpoint = ep.Name
		d.BounceAddress = f.Bounce.Address
		if d.BounceAddress == "" {
			d.BounceAddress = addressOf(ep)
		}
		if err := validation.ValidateEmail(d.BounceAddress); err != nil {
			return nil, fmt.Errorf("bounce.address: %w", err)
		}
		// The envelope domain is what SPF checks and DMARC aligns with From.
		if !related(domainOf(d.BounceAddress), domainOf(d.FromAddress)) {
			return nil, fmt.Errorf("bounce.address %s: its domain must be %s or a parent or subdomain of it, "+
				"otherwise SPF does not align with From", d.BounceAddress, domainOf(d.FromAddress))
		}
	}

	if len(f.Targets) == 0 {
		return nil, errors.New("'targets' must list at least one endpoint")
	}
	for _, name := range f.Targets {
		ep, err := use("targets", name, TargetProtocols)
		if err != nil {
			return nil, err
		}
		if _, dup := d.Target(ep.Name); dup {
			return nil, fmt.Errorf("targets: %q is listed twice", name)
		}
		t := Target{Endpoint: ep.Name, Address: addressOf(ep), Engage: true}
		if err := validation.ValidateEmail(t.Address); err != nil {
			return nil, fmt.Errorf("target %q: set 'address:' on the endpoint: %w", name, err)
		}
		if s := ep.String("engage"); s != "" {
			if t.Engage, err = strconv.ParseBool(s); err != nil {
				return nil, fmt.Errorf("target %q: engage must be true or false", name)
			}
		}
		d.Targets = append(d.Targets, t)
	}

	if err := parseTimes(d, &f); err != nil {
		return nil, err
	}
	if err := parseSpeed(d, &f); err != nil {
		return nil, err
	}

	d.ContentSources = []string{wikicontent.BodySourceWikipedia, wikicontent.BodySourcePoetry, wikicontent.BodySourceRFC}
	if len(f.Content.Sources) > 0 {
		d.ContentSources = nil
		for _, s := range f.Content.Sources {
			s = strings.ToLower(strings.TrimSpace(s))
			if s == wikicontent.BodySourceStatic || wikicontent.ValidateBodySource(s) != nil {
				return nil, fmt.Errorf("content.sources: %q must be wikipedia, rfc or poetry", s)
			}
			d.ContentSources = append(d.ContentSources, s)
		}
	}

	d.Hold = Hold{Window: 10, MinSamples: 5, BadRatio: 0.3, LateRatio: 0.5, DSN: 3, SendFailures: 3}
	setInt(&d.Hold.Window, f.Hold.Window)
	setInt(&d.Hold.MinSamples, f.Hold.MinSamples)
	setInt(&d.Hold.DSN, f.Hold.DSN)
	setInt(&d.Hold.SendFailures, f.Hold.SendFailures)
	if f.Hold.BadRatio != nil {
		d.Hold.BadRatio = *f.Hold.BadRatio
	}
	if f.Hold.LateRatio != nil {
		d.Hold.LateRatio = *f.Hold.LateRatio
	}
	h := d.Hold
	if h.Window < 1 || h.MinSamples < 1 || h.MinSamples > h.Window || h.DSN < 1 || h.SendFailures < 1 ||
		h.BadRatio <= 0 || h.BadRatio > 1 || h.LateRatio <= 0 || h.LateRatio > 1 {
		return nil, errors.New("hold: counts must be positive, min_samples <= window and ratios in (0, 1]")
	}

	d.Engage = Engage{MinDelay: 10 * time.Minute, MaxDelay: 4 * time.Hour}
	d.WarnAfter = 5 * time.Minute
	for _, p := range []struct {
		key, val string
		dst      *time.Duration
	}{
		{"engage.min_delay", f.Engage.MinDelay, &d.Engage.MinDelay},
		{"engage.max_delay", f.Engage.MaxDelay, &d.Engage.MaxDelay},
		{"warn_after", f.WarnAfter, &d.WarnAfter},
	} {
		if p.val == "" {
			continue
		}
		v, err := endpoint.ParseDuration(p.val)
		if err != nil || v < 0 {
			return nil, fmt.Errorf("%s: invalid duration %q", p.key, p.val)
		}
		*p.dst = v
	}
	if d.Engage.MaxDelay < d.Engage.MinDelay {
		return nil, errors.New("engage: max_delay is shorter than min_delay")
	}
	return d, nil
}

func domainOf(address string) string {
	_, domain, _ := strings.Cut(address, "@")
	return strings.ToLower(domain)
}

// related reports whether one domain equals or is a subdomain of the other.
func related(a, b string) bool {
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}

// addressOf is the mail address of a mailbox endpoint.
func addressOf(ep *endpoint.Endpoint) string {
	if a := ep.String("address"); a != "" {
		return a
	}
	if u := ep.String("username"); strings.Contains(u, "@") {
		return u
	}
	return ""
}

func setInt(dst *int, v *int) {
	if v != nil {
		*dst = *v
	}
}

func parseTimes(d *Domain, f *domainFile) error {
	tz := f.Timezone
	if tz == "" {
		tz = "Europe/Warsaw"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	d.Location = loc

	parse := func(s string) (time.Time, error) {
		for _, layout := range []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"} {
			if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), loc); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("invalid time %q (use \"2006-01-02 15:04\")", s)
	}
	if f.Campaign.Start == "" {
		return errors.New("campaign.start is required")
	}
	if d.Start, err = parse(f.Campaign.Start); err != nil {
		return fmt.Errorf("campaign.start: %w", err)
	}
	if f.Campaign.End != "" {
		if d.End, err = parse(f.Campaign.End); err != nil {
			return fmt.Errorf("campaign.end: %w", err)
		}
		if !d.End.After(d.Start) {
			return errors.New("campaign.end must be after campaign.start")
		}
	}
	return nil
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday, "thu": time.Thursday,
	"fri": time.Friday, "sat": time.Saturday, "sun": time.Sunday,
}

func clock(s string) (int, error) {
	if s == "24:00" {
		return 24 * 60, nil
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("invalid time of day %q (use \"08:00\")", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func parseBusinessHours(node *yaml.Node) (*schedule.Hours, error) {
	if node.Kind == yaml.ScalarNode && strings.EqualFold(node.Value, "none") {
		return nil, nil
	}
	h := hoursFile{Start: "08:00", End: "17:00", Days: []string{"mon", "tue", "wed", "thu", "fri"}}
	if !node.IsZero() && node.Tag != "!!null" {
		raw, err := yaml.Marshal(node)
		if err != nil {
			return nil, err
		}
		if err := decodeStrict(raw, &h); err != nil {
			return nil, fmt.Errorf("must be \"none\" or start/end/days: %w", err)
		}
	}
	out := &schedule.Hours{Days: map[time.Weekday]bool{}}
	var err error
	if out.Start, err = clock(h.Start); err != nil {
		return nil, err
	}
	if out.End, err = clock(h.End); err != nil {
		return nil, err
	}
	if out.End-out.Start < 60 {
		return nil, errors.New("end must be at least one hour after start")
	}
	for _, name := range h.Days {
		key := strings.ToLower(strings.TrimSpace(name))
		if len(key) > 3 {
			key = key[:3]
		}
		wd, ok := weekdays[key]
		if !ok {
			return nil, fmt.Errorf("unknown day %q", name)
		}
		out.Days[wd] = true
	}
	if len(out.Days) == 0 {
		return nil, errors.New("days must not be empty")
	}
	return out, nil
}

func parseSpeed(d *Domain, f *domainFile) error {
	hours, err := parseBusinessHours(&f.BusinessHours)
	if err != nil {
		return fmt.Errorf("business_hours: %w", err)
	}
	p := schedule.Params{
		Location: d.Location, Business: hours, Holidays: map[string]bool{},
		RampDays: 30, StartPerDay: 1, BusinessPerHour: 3, OffMin: 1, OffMax: 2, MinGap: 10 * time.Minute,
	}
	for _, h := range f.Holidays {
		if _, err := time.Parse("2006-01-02", h); err != nil {
			return fmt.Errorf("holidays: invalid date %q", h)
		}
		p.Holidays[h] = true
	}
	s := f.Speed
	setInt(&p.RampDays, s.RampDays)
	setInt(&p.StartPerDay, s.StartPerDay)
	setInt(&p.BusinessPerHour, s.BusinessPerHour)
	setInt(&p.PerDay, s.PerDay)
	setInt(&p.MaxPerDay, s.MaxPerDay)
	if s.OffHoursPerDay != nil {
		if len(s.OffHoursPerDay) != 2 {
			return errors.New("speed.off_hours_per_day must be [min, max]")
		}
		p.OffMin, p.OffMax = s.OffHoursPerDay[0], s.OffHoursPerDay[1]
	}
	switch strings.ToLower(s.Curve) {
	case "", "geometric":
	case "linear":
		p.Linear = true
	default:
		return fmt.Errorf("speed.curve %q: use geometric or linear", s.Curve)
	}
	if s.MinGap != "" {
		if p.MinGap, err = endpoint.ParseDuration(s.MinGap); err != nil {
			return fmt.Errorf("speed.min_gap: %w", err)
		}
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("speed: %w", err)
	}
	d.Schedule = p
	return nil
}

// DefaultDataDir is used when neither the flag, the environment nor the
// config file names a data directory.
func DefaultDataDir() string {
	if os.PathSeparator == '/' {
		return "/var/lib/" + AppName
	}
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, AppName)
	}
	return AppName + "-data"
}
