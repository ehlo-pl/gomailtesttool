// Package endpoint holds the named-connection model shared by the apps built
// on gomailtest (testmailflow, warmingup): YAML endpoints whose settings use
// the flag names of "gomailtest <protocol>", ${ENV} expansion, secret masking
// and durations with day and week units.
package endpoint

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/internal/common/security"
)

// Endpoint is one named connection. Settings use the flag names of
// "gomailtest <protocol>" (host, port, username, tenantid, mailbox, ...).
type Endpoint struct {
	Name     string
	Protocol string
	Settings map[string]any
}

// Parse builds an endpoint from its YAML mapping. Names, keys and the
// protocol are lower-cased and ${NAME} references in string values are
// expanded with lookupEnv.
func Parse(name string, settings map[string]any, lookupEnv func(string) (string, bool)) (*Endpoint, error) {
	ep := &Endpoint{Name: strings.ToLower(name), Settings: map[string]any{}}
	for k, v := range settings {
		k = strings.ToLower(k)
		if s, isStr := v.(string); isStr {
			expanded, err := ExpandEnv(s, lookupEnv)
			if err != nil {
				return nil, fmt.Errorf("endpoint %q, %s: %w", name, k, err)
			}
			v = expanded
		}
		if k == "protocol" {
			ep.Protocol = strings.ToLower(strings.TrimSpace(fmt.Sprint(v)))
			continue
		}
		ep.Settings[k] = v
	}
	if ep.Protocol == "" {
		return nil, fmt.Errorf("endpoint %q: 'protocol' is required", name)
	}
	return ep, nil
}

// Viper returns a fresh viper holding the endpoint settings, suitable for
// the protocol package's ConfigFromViper.
func (e *Endpoint) Viper() *viper.Viper {
	v := viper.New()
	_ = v.MergeConfigMap(e.Settings)
	return v
}

// String returns the setting as a string ("" when absent).
func (e *Endpoint) String(key string) string {
	if v, ok := e.Settings[key]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// secretKeys are endpoint settings never shown in clear text.
var secretKeys = map[string]bool{
	"password": true, "secret": true, "accesstoken": true, "bearertoken": true, "pfxpass": true,
	"token": true, "clientsecret": true, "refreshtoken": true,
}

// MaskedSettings returns "key=value" pairs of an endpoint with secrets
// masked, sorted by key, for display.
func (e *Endpoint) MaskedSettings() []string {
	keys := make([]string, 0, len(e.Settings))
	for k := range e.Settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		val := fmt.Sprint(e.Settings[k])
		switch {
		case secretKeys[k]:
			val = security.MaskSecret(val)
		case k == "username":
			val = security.MaskUsername(val)
		case k == "tenantid" || k == "clientid":
			val = security.MaskGUID(val)
		}
		out = append(out, k+"="+val)
	}
	return out
}

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnv replaces ${NAME} with the environment value. Only the braced
// form is expanded so passwords containing '$' are left alone. A reference
// to an unset variable is an error rather than a silent empty secret.
func ExpandEnv(s string, lookup func(string) (string, bool)) (string, error) {
	var missing []string
	out := envRef.ReplaceAllStringFunc(s, func(ref string) string {
		name := envRef.FindStringSubmatch(ref)[1]
		val, ok := lookup(name)
		if !ok {
			missing = append(missing, name)
		}
		return val
	})
	if len(missing) > 0 {
		return "", errors.New("environment variable " + strings.Join(missing, ", ") + " is not set")
	}
	return out, nil
}

var relDuration = regexp.MustCompile(`^(\d+)([dw])$`)

// ParseDuration accepts Go durations ("2m30s", "1h") plus whole days and
// weeks ("90d", "2w").
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if m := relDuration.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		unit := 24 * time.Hour
		if m[2] == "w" {
			unit *= 7
		}
		return time.Duration(n) * unit, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (examples: 30s, 2m30s, 1h, 90d)", s)
	}
	return d, nil
}
