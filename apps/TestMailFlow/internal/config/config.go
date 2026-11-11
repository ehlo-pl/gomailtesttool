// Package config holds the testmailflow configuration: global settings
// (data directory, database, logging, HTTP listener) following the
// gomailtesttool conventions, plus the YAML model of endpoints and flows.
//
// Global settings: CLI flag > environment variable (TMF prefix, no
// separator) > config file ("testmailflow:" section) > default.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// AppName is the binary, service and event source name.
const AppName = "testmailflow"

// Flag / key names.
const (
	KeyConfig    = "config"
	KeyDataDir   = "datadir"
	KeyDB        = "db"
	KeyVerbose   = "verbose"
	KeyLogLevel  = "loglevel"
	KeyHTTP      = "http"
	KeyRetention = "retention"
)

// Section is the YAML section with the global settings.
const Section = "testmailflow"

// DefaultRetention is how long finished probes are kept.
const DefaultRetention = 90 * 24 * time.Hour

// Config is the resolved global configuration.
type Config struct {
	ConfigFile string
	DataDir    string
	DBPath     string
	Verbose    bool
	LogLevel   string
	HTTPAddr   string        // "" = HTTP endpoint disabled
	Retention  time.Duration // finished probes older than this are pruned
}

// LogDir is where the service writes its daily log files.
func (c *Config) LogDir() string { return filepath.Join(c.DataDir, "logs") }

// RegisterPersistentFlags adds global flags to the root command.
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.String(KeyConfig, "", "YAML config file with testmailflow:, endpoints: and flows: (env: TMFCONFIG)")
	f.String(KeyDataDir, "", "Data directory for the database and logs (env: TMFDATADIR; default: per-user data dir)")
	f.String(KeyDB, "", "SQLite database path (env: TMFDB; default: <datadir>/testmailflow.db)")
	f.Bool(KeyVerbose, false, "Verbose (debug) logging (env: TMFVERBOSE)")
	f.String(KeyLogLevel, "INFO", "Log level: DEBUG, INFO, WARN, ERROR (env: TMFLOGLEVEL)")
}

// BindEnvs binds TMF-prefixed environment variables.
func BindEnvs(v *viper.Viper) {
	for key, env := range map[string]string{
		KeyConfig:    "TMFCONFIG",
		KeyDataDir:   "TMFDATADIR",
		KeyDB:        "TMFDB",
		KeyVerbose:   "TMFVERBOSE",
		KeyLogLevel:  "TMFLOGLEVEL",
		KeyHTTP:      "TMFHTTP",
		KeyRetention: "TMFRETENTION",
	} {
		_ = v.BindEnv(key, env)
	}
}

// FromViper resolves the global configuration and applies defaults.
func FromViper(v *viper.Viper) (*Config, error) {
	c := &Config{
		ConfigFile: v.GetString(KeyConfig),
		DataDir:    strings.TrimSpace(v.GetString(KeyDataDir)),
		DBPath:     strings.TrimSpace(v.GetString(KeyDB)),
		Verbose:    v.GetBool(KeyVerbose),
		LogLevel:   v.GetString(KeyLogLevel),
		HTTPAddr:   strings.TrimSpace(v.GetString(KeyHTTP)),
		Retention:  DefaultRetention,
	}
	if s := strings.TrimSpace(v.GetString(KeyRetention)); s != "" {
		d, err := ParseDuration(s)
		if err != nil || d <= 0 {
			return nil, errors.New("retention: invalid duration " + s)
		}
		c.Retention = d
	}
	if c.DataDir == "" {
		d, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		c.DataDir = d
	}
	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, AppName+".db")
	}
	if strings.ContainsRune(c.DataDir, 0) || strings.ContainsRune(c.DBPath, 0) {
		return nil, errors.New("invalid path")
	}
	return c, nil
}

// DefaultDataDir returns the per-user data directory:
// Windows %LOCALAPPDATA%\testmailflow, macOS ~/Library/Application Support/testmailflow,
// otherwise $XDG_DATA_HOME/testmailflow or ~/.local/share/testmailflow.
func DefaultDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		d, err := os.UserCacheDir() // %LocalAppData%
		if err != nil {
			return "", err
		}
		return filepath.Join(d, AppName), nil
	case "darwin":
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, AppName), nil
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", AppName), nil
}

// ServiceDataDir is the machine-wide data directory used by the installed
// service: %ProgramData%\testmailflow on Windows, /var/lib/testmailflow elsewhere.
func ServiceDataDir() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return filepath.Join(pd, AppName)
	}
	return filepath.Join("/var/lib", AppName)
}
