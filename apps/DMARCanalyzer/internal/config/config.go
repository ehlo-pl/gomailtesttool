// Package config holds the dmarcanalyzer global configuration (data
// directory, database, logging) following the gomailtesttool conventions:
// CLI flag > environment variable (DMARC prefix, no separator) > config file
// ("dmarc:" section) > default.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Flag / key names.
const (
	KeyConfig   = "config"
	KeyDataDir  = "datadir"
	KeyDB       = "db"
	KeyVerbose  = "verbose"
	KeyLogLevel = "loglevel"
)

// Config is the resolved global configuration.
type Config struct {
	ConfigFile string
	DataDir    string
	DBPath     string
	Verbose    bool
	LogLevel   string
}

// EMLDir is where collected messages are stored.
func (c *Config) EMLDir() string { return filepath.Join(c.DataDir, "eml") }

// RegisterPersistentFlags adds global flags to the root command.
func RegisterPersistentFlags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.String(KeyConfig, "", "YAML config file (sections: dmarc, msgraph, gmail, imap, pop3)")
	f.String(KeyDataDir, "", "Data directory for .eml files and the database (env DMARCDATADIR; default: per-user data dir)")
	f.String(KeyDB, "", "SQLite database path (env DMARCDB; default: <datadir>/dmarc.db)")
	f.Bool(KeyVerbose, false, "Verbose (debug) logging (env DMARCVERBOSE)")
	f.String(KeyLogLevel, "INFO", "Log level: DEBUG, INFO, WARN, ERROR (env DMARCLOGLEVEL)")
}

// BindEnvs binds DMARC-prefixed environment variables.
func BindEnvs(v *viper.Viper) {
	for key, env := range map[string]string{
		KeyConfig:   "DMARCCONFIG",
		KeyDataDir:  "DMARCDATADIR",
		KeyDB:       "DMARCDB",
		KeyVerbose:  "DMARCVERBOSE",
		KeyLogLevel: "DMARCLOGLEVEL",
	} {
		_ = v.BindEnv(key, env)
	}
}

// FromViper resolves the configuration and applies defaults.
func FromViper(v *viper.Viper) (*Config, error) {
	c := &Config{
		ConfigFile: v.GetString(KeyConfig),
		DataDir:    strings.TrimSpace(v.GetString(KeyDataDir)),
		DBPath:     strings.TrimSpace(v.GetString(KeyDB)),
		Verbose:    v.GetBool(KeyVerbose),
		LogLevel:   v.GetString(KeyLogLevel),
	}
	if c.DataDir == "" {
		d, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		c.DataDir = d
	}
	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, "dmarc.db")
	}
	if strings.ContainsRune(c.DataDir, 0) || strings.ContainsRune(c.DBPath, 0) {
		return nil, errors.New("invalid path")
	}
	return c, nil
}

// DefaultDataDir returns the per-user data directory:
// Windows %LOCALAPPDATA%\dmarcanalyzer, macOS ~/Library/Application Support/dmarcanalyzer,
// otherwise $XDG_DATA_HOME/dmarcanalyzer or ~/.local/share/dmarcanalyzer.
func DefaultDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		d, err := os.UserCacheDir() // %LocalAppData%
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "dmarcanalyzer"), nil
	case "darwin":
		d, err := os.UserConfigDir() // ~/Library/Application Support
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "dmarcanalyzer"), nil
	}
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "dmarcanalyzer"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "dmarcanalyzer"), nil
}
