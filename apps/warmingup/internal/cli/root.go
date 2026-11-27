// Package cli implements the warmingup command tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/engine"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// exitError carries a specific process exit code (status / once results).
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

type globals struct {
	configFile, dataDir, logLevel, domain string
}

// NewRootCmd builds the warmingup command tree.
func NewRootCmd() *cobra.Command {
	g := &globals{}
	root := &cobra.Command{
		Use:   config.AppName,
		Short: "Warm up sending domains slowly and watch where the mail lands",
		Long: `warmingup sends a slowly growing number of ordinary looking messages from your
domains, through your own mail server, to mailboxes you own. It records whether
each one landed in the inbox or in junk, marks it read, moves it out of junk,
collects bounce reports and stops increasing the volume when results turn bad.

Typical use:
  warmingup validate --config /etc/warmingup/warmingup.yaml --connect
  warmingup plan     --config /etc/warmingup/warmingup.yaml
  warmingup once     --config /etc/warmingup/warmingup.yaml --target gmail1
  warmingup run      --config /etc/warmingup/warmingup.yaml     (the service)

Environment: WARMUPCONFIG, WARMUPDATADIR, WARMUPLOGLEVEL.`,
		Version:       version.Get(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := root.PersistentFlags()
	f.StringVar(&g.configFile, "config", os.Getenv("WARMUPCONFIG"), "global YAML config file (env: WARMUPCONFIG)")
	f.StringVar(&g.dataDir, "datadir", os.Getenv("WARMUPDATADIR"), "data directory for the database and tokens (env: WARMUPDATADIR)")
	f.StringVar(&g.logLevel, "loglevel", os.Getenv("WARMUPLOGLEVEL"), "DEBUG, INFO, WARN, ERROR (env: WARMUPLOGLEVEL)")
	f.StringVar(&g.domain, "domain", "", "limit to one warmed domain")

	root.AddCommand(newRunCmd(g), newOnceCmd(g), newPlanCmd(g), newStatusCmd(g), newReportCmd(g),
		newValidateCmd(g), newAuthCmd(g), newResumeCmd(g))
	return root
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := NewRootCmd().ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	return 1
}

// env is the per-invocation runtime environment.
type env struct {
	cfg   *config.Config
	log   *slog.Logger
	store *store.Store
	open  *opener
}

func (e *env) Close() {
	if e.store != nil {
		_ = e.store.Close()
	}
}

func (e *env) engine() *engine.Engine { return engine.New(e.cfg, e.store, e.open, e.log) }

// secrets says whether a command signs in to mail servers and therefore
// needs every ${NAME} of the configuration.
type secrets bool

const (
	needSecrets secrets = true
	noSecrets   secrets = false
)

// lookup returns the environment lookup. Commands that open no session work
// without the secrets: an unset variable becomes a placeholder.
func (s secrets) lookup() func(string) (string, bool) {
	if s == needSecrets {
		return os.LookupEnv
	}
	return func(name string) (string, bool) {
		if v, ok := os.LookupEnv(name); ok {
			return v, true
		}
		return "unset", true
	}
}

// loadConfig reads and validates the configuration without opening anything.
func loadConfig(g *globals, s secrets) (*env, error) {
	cfg, err := config.Load(g.configFile, s.lookup())
	if err != nil {
		return nil, err
	}
	if err := checkEndpoints(cfg); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	if g.dataDir != "" {
		cfg.DataDir = g.dataDir
	}
	if cfg.DataDir == "" {
		cfg.DataDir = config.DefaultDataDir()
	}
	if g.logLevel != "" {
		cfg.LogLevel = g.logLevel
	}
	log := logger.SetupLogger(false, cfg.LogLevel)
	return &env{cfg: cfg, log: log, open: &opener{cfg: cfg, log: log}}, nil
}

// setup also opens the database.
func setup(ctx context.Context, g *globals, s secrets) (*env, error) {
	e, err := loadConfig(g, s)
	if err != nil {
		return nil, err
	}
	if e.store, err = store.Open(ctx, filepath.Join(e.cfg.DataDir, config.AppName+".db")); err != nil {
		return nil, err
	}
	return e, nil
}

// domains returns the domains selected by --domain.
func (e *env) domains(name string) ([]*config.Domain, error) {
	if name == "" {
		return e.cfg.Domains, nil
	}
	if d := e.cfg.Domain(name); d != nil {
		return []*config.Domain{d}, nil
	}
	var known []string
	for _, d := range e.cfg.Domains {
		known = append(known, d.Name)
	}
	return nil, fmt.Errorf("domain %q is not configured (%s)", name, strings.Join(known, ", "))
}
