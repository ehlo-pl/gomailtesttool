// Package cli implements the testmailflow command tree.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// exitError carries a specific process exit code (status / once results).
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

// NewRootCmd builds the testmailflow command tree.
func NewRootCmd() *cobra.Command {
	v := viper.New()
	root := &cobra.Command{
		Use:   config.AppName,
		Short: "Probe mail flow end to end: send over one channel, verify arrival over another",
		Long: `testmailflow sends small probe messages over SMTP or Microsoft Graph and verifies
over Microsoft Graph, Gmail API, IMAP or POP3 that they arrive in the target mailbox.
Each named flow has its own sender, checked mailbox, interval and check schedule
(by default: look after 5 minutes, then after a further 2.5, 5, 10 and 15 minutes).

Results are kept in a local SQLite database and exposed through 'status', 'report',
the Windows Event Log and an optional HTTP endpoint (/healthz, /metrics).

Typical use:
  testmailflow validate --config tmf.yaml
  testmailflow once     --config tmf.yaml --flow onprem-to-exo
  testmailflow run      --config tmf.yaml                 (foreground)
  testmailflow service install --config C:\tmf\tmf.yaml   (Windows service)

Global settings use the TMF environment prefix (TMFCONFIG, TMFDATADIR, TMFDB, TMFLOGLEVEL).`,
		Version:       version.Get(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	config.RegisterPersistentFlags(root)
	config.BindEnvs(v)

	root.AddCommand(
		newRunCmd(v),
		newOnceCmd(v),
		newStatusCmd(v),
		newReportCmd(v),
		newValidateCmd(v),
		newDBCmd(v),
		newServiceCmd(v),
	)
	return root
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	err := NewRootCmd().Execute()
	if err == nil {
		return 0
	}
	var ee *exitError
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, ee.msg)
		}
		return ee.code
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	return 1
}

// env is the per-invocation runtime environment.
type env struct {
	cfg     *config.Config
	log     *slog.Logger
	store   *store.Store
	logFile io.Closer
}

// setupOpts select optional parts of setup.
type setupOpts struct {
	logToFile bool // service mode: daily log files instead of stderr
}

// setup binds flags, loads the "testmailflow" config section, creates the
// logger and opens the store.
func setup(ctx context.Context, cmd *cobra.Command, v *viper.Viper, o setupOpts) (*env, error) {
	_ = v.BindPFlags(cmd.Flags())
	_ = v.BindPFlags(cmd.InheritedFlags())
	if err := bootstrap.LoadConfigFileSection(v, v.GetString(config.KeyConfig), config.Section); err != nil {
		return nil, err
	}
	cfg, err := config.FromViper(v)
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	e := &env{cfg: cfg}
	if o.logToFile {
		w := &dailyWriter{dir: cfg.LogDir(), prefix: config.AppName}
		level := logger.ParseLogLevel(cfg.LogLevel)
		if cfg.Verbose {
			level = slog.LevelDebug
		}
		e.log = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level}))
		e.logFile = w
	} else {
		e.log = logger.SetupLogger(cfg.Verbose, cfg.LogLevel)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		e.Close()
		return nil, err
	}
	e.store = st
	e.log.Debug("database opened", "path", cfg.DBPath)
	return e, nil
}

func (e *env) Close() {
	if e == nil {
		return
	}
	if e.store != nil {
		_ = e.store.Close()
	}
	if e.logFile != nil {
		_ = e.logFile.Close()
	}
}

// loadPlan reads and validates endpoints and flows from the config file.
func loadPlan(cfg *config.Config) (*config.Plan, error) {
	plan, err := config.LoadPlan(cfg.ConfigFile)
	if err != nil {
		return nil, err
	}
	if err := checkEndpoints(plan); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	return plan, nil
}

// dailyWriter appends to <dir>/<prefix>-YYYY-MM-DD.log, switching files at
// midnight UTC. Files are created with owner-only permissions.
type dailyWriter struct {
	dir, prefix string

	mu   sync.Mutex
	day  string
	file *os.File
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := time.Now().UTC().Format("2006-01-02")
	if w.file == nil || day != w.day {
		if w.file != nil {
			_ = w.file.Close()
			w.file = nil
		}
		if err := os.MkdirAll(w.dir, 0o700); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(filepath.Join(w.dir, w.prefix+"-"+day+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.file, w.day = f, day
	}
	return w.file.Write(p)
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
