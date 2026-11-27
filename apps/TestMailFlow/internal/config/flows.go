package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/internal/common/endpoint"
	"github.com/ehlo-pl/gomailtesttool/internal/common/validation"
)

// Protocols usable for each role.
var (
	SendProtocols  = []string{"smtp", "msgraph"}
	CheckProtocols = []string{"msgraph", "gmail", "imap", "pop3"}
)

// DefaultChecks is the check schedule from the original requirement:
// first look after 5 minutes, then after a further 2.5, 5, 10 and 15 minutes.
var DefaultChecks = []time.Duration{5 * time.Minute, 150 * time.Second, 5 * time.Minute, 10 * time.Minute, 15 * time.Minute}

// DefaultInterval is the default time between probes of one flow.
const DefaultInterval = 15 * time.Minute

// Endpoint is one named connection; see package endpoint.
type Endpoint = endpoint.Endpoint

// Flow is one tested mail route.
type Flow struct {
	Name string

	SendEndpoint string
	From         string
	To           []string

	CheckEndpoint string
	Folder        string
	Delete        bool // delete matched probe messages from the checked mailbox

	Interval  time.Duration
	Checks    []time.Duration // delays between check attempts
	WarnAfter time.Duration   // delivery slower than this is "late"
}

// Deadline is the time after sending at which the probe is declared lost.
func (f *Flow) Deadline() time.Duration {
	var d time.Duration
	for _, c := range f.Checks {
		d += c
	}
	return d
}

// CheckOffset returns the delay after sending of check attempt n (0-based).
func (f *Flow) CheckOffset(n int) time.Duration {
	var d time.Duration
	for i := 0; i <= n && i < len(f.Checks); i++ {
		d += f.Checks[i]
	}
	return d
}

// Plan is the parsed endpoints and flows of a config file.
type Plan struct {
	Endpoints map[string]*Endpoint
	Flows     []*Flow
}

// Flow returns the flow by name, or nil.
func (p *Plan) Flow(name string) *Flow {
	for _, f := range p.Flows {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// EndpointNames returns the endpoint names sorted.
func (p *Plan) EndpointNames() []string {
	names := make([]string, 0, len(p.Endpoints))
	for n := range p.Endpoints {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// LoadPlan reads endpoints: and flows: from the YAML file at path, expands
// ${ENV} references in endpoint settings and validates everything before
// any network work.
func LoadPlan(path string) (*Plan, error) {
	if path == "" {
		return nil, errors.New("a config file is required (--config, env TMFCONFIG)")
	}
	fv := viper.New()
	fv.SetConfigFile(path)
	fv.SetConfigType("yaml")
	if err := fv.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}
	return planFromMaps(fv.Get("endpoints"), fv.Get("flows"), os.LookupEnv)
}

func planFromMaps(rawEndpoints, rawFlows any, lookupEnv func(string) (string, bool)) (*Plan, error) {
	p := &Plan{Endpoints: map[string]*Endpoint{}}

	eps, ok := rawEndpoints.(map[string]any)
	if !ok || len(eps) == 0 {
		return nil, errors.New("config: 'endpoints:' must define at least one endpoint")
	}
	for name, raw := range eps {
		settings, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("endpoint %q: must be a mapping", name)
		}
		ep, err := endpoint.Parse(name, settings, lookupEnv)
		if err != nil {
			return nil, err
		}
		if !contains(SendProtocols, ep.Protocol) && !contains(CheckProtocols, ep.Protocol) {
			return nil, fmt.Errorf("endpoint %q: unsupported protocol %q (send: %s; check: %s)", name, ep.Protocol,
				strings.Join(SendProtocols, ", "), strings.Join(CheckProtocols, ", "))
		}
		p.Endpoints[ep.Name] = ep
	}

	list, ok := rawFlows.([]any)
	if !ok || len(list) == 0 {
		return nil, errors.New("config: 'flows:' must list at least one flow")
	}
	seen := map[string]bool{}
	for i, raw := range list {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("flow #%d: must be a mapping", i+1)
		}
		f, err := flowFromMap(lower(m), p)
		if err != nil {
			name := str(lower(m)["name"])
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			return nil, fmt.Errorf("flow %s: %w", name, err)
		}
		if seen[strings.ToLower(f.Name)] {
			return nil, fmt.Errorf("flow %s: duplicate flow name", f.Name)
		}
		seen[strings.ToLower(f.Name)] = true
		p.Flows = append(p.Flows, f)
	}
	return p, nil
}

var flowNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func flowFromMap(m map[string]any, p *Plan) (*Flow, error) {
	f := &Flow{Name: str(m["name"])}
	if !flowNamePattern.MatchString(f.Name) {
		return nil, errors.New("'name' is required: letters, digits, '.', '_' and '-' (max 63), starting with a letter or digit")
	}

	snd, ok := m["send"].(map[string]any)
	if !ok {
		return nil, errors.New("'send:' section is required")
	}
	snd = lower(snd)
	f.SendEndpoint = strings.ToLower(str(snd["endpoint"]))
	sep, ok := p.Endpoints[f.SendEndpoint]
	if !ok {
		return nil, fmt.Errorf("send.endpoint %q is not defined under endpoints", f.SendEndpoint)
	}
	if !contains(SendProtocols, sep.Protocol) {
		return nil, fmt.Errorf("send.endpoint %q uses %s; sending supports: %s", f.SendEndpoint, sep.Protocol, strings.Join(SendProtocols, ", "))
	}
	f.From = str(snd["from"])
	if f.From == "" && sep.Protocol == "msgraph" {
		f.From = sep.String("mailbox")
	}
	if err := validation.ValidateEmail(f.From); err != nil {
		return nil, fmt.Errorf("send.from: %w", err)
	}
	f.To = strList(snd["to"])
	if len(f.To) == 0 {
		return nil, errors.New("send.to is required")
	}
	if err := validation.ValidateEmails(f.To, "send.to"); err != nil {
		return nil, err
	}

	chk, ok := m["check"].(map[string]any)
	if !ok {
		return nil, errors.New("'check:' section is required")
	}
	chk = lower(chk)
	f.CheckEndpoint = strings.ToLower(str(chk["endpoint"]))
	cep, ok := p.Endpoints[f.CheckEndpoint]
	if !ok {
		return nil, fmt.Errorf("check.endpoint %q is not defined under endpoints", f.CheckEndpoint)
	}
	if !contains(CheckProtocols, cep.Protocol) {
		return nil, fmt.Errorf("check.endpoint %q uses %s; checking supports: %s", f.CheckEndpoint, cep.Protocol, strings.Join(CheckProtocols, ", "))
	}
	f.Folder = str(chk["folder"])
	f.Delete, _ = toBool(chk["delete"])

	var err error
	f.Interval = DefaultInterval
	if s := str(m["interval"]); s != "" {
		if f.Interval, err = ParseDuration(s); err != nil {
			return nil, fmt.Errorf("interval: %w", err)
		}
	}
	if f.Interval < 30*time.Second {
		return nil, errors.New("interval must be at least 30s")
	}

	f.Checks = DefaultChecks
	if raw, present := m["checks"]; present && raw != nil {
		f.Checks = nil
		for _, s := range strList(raw) {
			d, err := ParseDuration(s)
			if err != nil {
				return nil, fmt.Errorf("checks: %w", err)
			}
			if d <= 0 {
				return nil, fmt.Errorf("checks: %q must be positive", s)
			}
			f.Checks = append(f.Checks, d)
		}
		if len(f.Checks) == 0 {
			return nil, errors.New("checks: at least one delay is required")
		}
	}

	f.WarnAfter = f.Checks[0]
	if s := str(m["warn_after"]); s != "" {
		if f.WarnAfter, err = ParseDuration(s); err != nil {
			return nil, fmt.Errorf("warn_after: %w", err)
		}
		if f.WarnAfter <= 0 {
			return nil, errors.New("warn_after must be positive")
		}
	}
	return f, nil
}

// ParseDuration accepts Go durations plus whole days and weeks.
func ParseDuration(s string) (time.Duration, error) { return endpoint.ParseDuration(s) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func lower(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[strings.ToLower(k)] = v
	}
	return out
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// strList accepts a YAML list or a comma-separated string.
func strList(v any) []string {
	var out []string
	add := func(s string) {
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	switch t := v.(type) {
	case nil:
	case []any:
		for _, x := range t {
			add(fmt.Sprint(x))
		}
	case []string:
		for _, x := range t {
			add(x)
		}
	default:
		add(fmt.Sprint(t))
	}
	return out
}

func toBool(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(t))
		return b, err == nil
	}
	return false, false
}
