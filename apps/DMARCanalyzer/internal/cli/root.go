// Package cli implements the dmarcanalyzer command tree.
package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// NewRootCmd builds the dmarcanalyzer command tree.
func NewRootCmd() *cobra.Command {
	v := viper.New()
	root := &cobra.Command{
		Use:   "dmarcanalyzer",
		Short: "Collect, extract and summarize DMARC aggregate reports",
		Long: `dmarcanalyzer downloads DMARC aggregate (rua) report e-mails from a mailbox
(Microsoft Graph, Gmail API, IMAP or POP3) as .eml files, extracts the XML reports
(zip / gzip / xml attachments) into a local SQLite database, and produces period
summaries (day / week / month / quarter / year) as text, Markdown, CSV, JSON or HTML.

Typical flow:
  dmarcanalyzer collect imap --host imap.example.com --imaps --username dmarc@example.com --password ... --since 30d
  dmarcanalyzer report --period week --last 90d
  dmarcanalyzer report --format html --out dmarc.html

Global settings use the DMARC environment prefix (DMARCDATADIR, DMARCDB, DMARCLOGLEVEL);
protocol settings keep their gomailtest prefixes (MSGRAPH*, GMAIL*, IMAP*, POP3*).`,
		Version:       version.Get(),
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	config.RegisterPersistentFlags(root)
	config.BindEnvs(v)

	root.AddCommand(
		newExtractCmd(v),
		newReportCmd(v),
		newDBCmd(v),
		newCollectCmd(v),
		newRunCmd(v),
	)
	return root
}

// Execute runs the root command.
func Execute() error { return NewRootCmd().Execute() }

// env is the per-invocation runtime environment.
type env struct {
	cfg   *config.Config
	log   *slog.Logger
	store *store.Store
}

// setup binds flags, loads the "dmarc" config section, opens the store.
func setup(ctx context.Context, cmd *cobra.Command, v *viper.Viper) (*env, error) {
	_ = v.BindPFlags(cmd.Flags())
	_ = v.BindPFlags(cmd.InheritedFlags())
	if err := bootstrap.LoadConfigFileSection(v, v.GetString(config.KeyConfig), "dmarc"); err != nil {
		return nil, err
	}
	cfg, err := config.FromViper(v)
	if err != nil {
		return nil, fmt.Errorf("configuration: %w", err)
	}
	log := logger.SetupLogger(cfg.Verbose, cfg.LogLevel)
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return nil, err
	}
	log.Debug("database opened", "path", cfg.DBPath)
	return &env{cfg: cfg, log: log, store: st}, nil
}

func (e *env) Close() {
	if e != nil && e.store != nil {
		_ = e.store.Close()
	}
}
