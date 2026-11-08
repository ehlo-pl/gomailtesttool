package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/extract"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/pipeline"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/report"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
	"github.com/ehlo-pl/gomailtesttool/internal/common/export"
	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/gmail"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/imap"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"
	"github.com/ehlo-pl/gomailtesttool/internal/protocols/pop3"
)

// defaultLookback is used for the first collection of a source.
const defaultLookback = 30 * 24 * time.Hour

// protocolSource adapts one gomailtesttool protocol package.
type protocolSource struct {
	name  string
	short string
	// register adds the protocol's persistent flags; bind binds its env vars.
	register func(*cobra.Command)
	bind     func(*viper.Viper)
	// open builds the protocol config from v and opens a fetch session.
	// It returns the session and the mailbox identity used for state.
	open func(ctx context.Context, v *viper.Viper, log *slog.Logger, withDelete bool) (fetch.Session, string, error)
}

func protocolSources() []protocolSource {
	return []protocolSource{
		{
			name: "msgraph", short: "Collect from Exchange Online / Microsoft 365 via Microsoft Graph",
			register: msgraph.RegisterPersistentFlags, bind: msgraph.BindEnvs,
			open: func(ctx context.Context, v *viper.Viper, log *slog.Logger, _ bool) (fetch.Session, string, error) {
				cfg := msgraph.ConfigFromViper(v)
				if cfg.Mailbox == "" {
					return nil, "", errors.New("--mailbox is required (env MSGRAPHMAILBOX)")
				}
				s, err := msgraph.OpenFetchSession(ctx, cfg, log)
				return s, cfg.Mailbox, err
			},
		},
		{
			name: "gmail", short: "Collect from Gmail / Google Workspace via the Gmail API",
			register: gmail.RegisterPersistentFlags, bind: gmail.BindEnvs,
			open: func(ctx context.Context, v *viper.Viper, log *slog.Logger, withDelete bool) (fetch.Session, string, error) {
				cfg := gmail.ConfigFromViper(v)
				id := cfg.Mailbox
				if id == "" {
					id = "me"
				}
				s, err := gmail.OpenFetchSession(ctx, cfg, log, withDelete)
				return s, id, err
			},
		},
		{
			name: "imap", short: "Collect from any IMAP server",
			register: imap.RegisterPersistentFlags, bind: imap.BindEnvs,
			open: func(ctx context.Context, v *viper.Viper, log *slog.Logger, _ bool) (fetch.Session, string, error) {
				cfg := imap.ConfigFromViper(v)
				if err := requireServer(cfg.Host, cfg.Username, cfg.Password, cfg.AccessToken, "IMAP"); err != nil {
					return nil, "", err
				}
				s, err := imap.OpenFetchSession(ctx, cfg, log)
				return s, cfg.Username + "@" + cfg.Host, err
			},
		},
		{
			name: "pop3", short: "Collect from a POP3 server (requires UIDL)",
			register: pop3.RegisterPersistentFlags, bind: pop3.BindEnvs,
			open: func(ctx context.Context, v *viper.Viper, log *slog.Logger, _ bool) (fetch.Session, string, error) {
				cfg := pop3.ConfigFromViper(v)
				if err := requireServer(cfg.Host, cfg.Username, cfg.Password, cfg.AccessToken, "POP3"); err != nil {
					return nil, "", err
				}
				s, err := pop3.OpenFetchSession(ctx, cfg, log)
				return s, cfg.Username + "@" + cfg.Host, err
			},
		},
	}
}

func requireServer(host, user, pass, token, prefix string) error {
	switch {
	case host == "":
		return fmt.Errorf("--host is required (env %sHOST)", prefix)
	case user == "":
		return fmt.Errorf("--username is required (env %sUSERNAME)", prefix)
	case pass == "" && token == "":
		return fmt.Errorf("--password or --accesstoken is required (env %sPASSWORD / %sACCESSTOKEN)", prefix, prefix)
	}
	return nil
}

func newCollectCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Download DMARC report e-mails from a mailbox as .eml and extract them",
		Long: `Download messages from a mailbox into <datadir>/eml/<protocol>/<YYYY-MM>/ and
(by default) extract DMARC reports into the database.

Collection is incremental: every downloaded message key is remembered, and without
--since the window starts at the previous successful run minus --overlap (first run:
last 30 days). Re-running never downloads the same message twice.

Protocol flags and environment variables are the same as for 'gomailtest <protocol>'
(e.g. MSGRAPHTENANTID, GMAILCREDENTIALS, IMAPHOST, POP3USERNAME); a --config file may
hold them under msgraph:/gmail:/imap:/pop3: sections.`,
	}
	for _, ps := range protocolSources() {
		cmd.AddCommand(newCollectProtoCmd(v, ps, false))
	}
	return cmd
}

func newRunCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "collect + extract + report in one step (for scheduled runs)",
		Long: `Collect new messages over a protocol, extract the reports and print a report.
Accepts all 'collect <protocol>' and 'report' flags.

Example (Windows Task Scheduler / cron):
  dmarcanalyzer run msgraph --config dmarc.yaml --period week --last 8w --format html --out weekly.html`,
	}
	for _, ps := range protocolSources() {
		cmd.AddCommand(newCollectProtoCmd(v, ps, true))
	}
	return cmd
}

func addCollectFlags(f *pflag.FlagSet) {
	f.String("folder", "", "Folder to read: IMAP mailbox (default INBOX), Graph folder id or well-known name (default inbox), Gmail label")
	f.String("since", "", "Only messages received on/after: YYYY-MM-DD, RFC 3339 or relative (30d). Default: last run - overlap, or 30d")
	f.String("until", "", "Only messages received before (YYYY-MM-DD or RFC 3339)")
	f.String("overlap", "2d", "Re-scan window before the last run (covers late delivery; duplicates are skipped)")
	f.Int("max", 0, "Download at most N messages (0 = no limit)")
	f.String("subject", "", "Only messages whose subject contains this text (case-insensitive)")
	f.String("query", "", "Raw Gmail search query added to the filter (gmail only), e.g. 'has:attachment'")
	f.Bool("extract", true, "Extract DMARC reports right after download")
	f.Bool("delete-after", false, "Delete server copies of messages whose DMARC report was stored (needs --yes)")
	f.Bool("yes", false, "Confirm destructive --delete-after")
}

func newCollectProtoCmd(root *viper.Viper, ps protocolSource, withReport bool) *cobra.Command {
	pv := viper.New()
	cmd := &cobra.Command{
		Use:   ps.name,
		Short: ps.short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, root)
			if err != nil {
				return err
			}
			defer e.Close()

			_ = pv.BindPFlags(cmd.Flags())
			_ = pv.BindPFlags(cmd.InheritedFlags())
			if err := bootstrap.LoadConfigFileSection(pv, root.GetString(config.KeyConfig), ps.name); err != nil {
				return err
			}

			co, err := collectOptsFromViper(root, ps.name, time.Now())
			if err != nil {
				return err
			}
			var ro *reportOpts
			if withReport {
				if ro, err = reportOptionsFromViper(root, time.Now()); err != nil {
					return err
				}
			}
			co.open = func(ctx context.Context, withDelete bool) (fetch.Session, string, error) {
				return ps.open(ctx, pv, e.log, withDelete)
			}
			out := cmd.OutOrStdout()
			if withReport && ro.out == "" {
				out = cmd.ErrOrStderr() // keep stdout clean for the report itself
			}
			if _, err := runCollect(ctx, e, co, out); err != nil {
				return err
			}
			if withReport {
				return runReport(ctx, e, ro, cmd.OutOrStdout())
			}
			return nil
		},
	}
	ps.register(cmd)
	ps.bind(pv)
	addCollectFlags(cmd.Flags())
	if withReport {
		addReportFlags(cmd.Flags())
	}
	return cmd
}

// collectOpts are the parsed collect flags.
type collectOpts struct {
	protocol    string
	crit        fetch.Criteria
	sinceSet    bool
	overlap     time.Duration
	extract     bool
	deleteAfter bool
	open        func(ctx context.Context, withDelete bool) (fetch.Session, string, error)
	now         func() time.Time
}

func collectOptsFromViper(v *viper.Viper, protocol string, now time.Time) (*collectOpts, error) {
	get := func(n string) string { return strings.TrimSpace(v.GetString(n)) }
	co := &collectOpts{protocol: protocol, extract: v.GetBool("extract"), deleteAfter: v.GetBool("delete-after"), now: time.Now}
	co.crit.Folder = get("folder")
	co.crit.Subject = get("subject")
	co.crit.Query = get("query")
	co.crit.Max = v.GetInt("max")
	if co.crit.Max < 0 {
		return nil, errors.New("--max must be >= 0")
	}
	if co.crit.Query != "" && protocol != "gmail" {
		return nil, errors.New("--query is supported for gmail only")
	}
	var err error
	if s := get("since"); s != "" {
		if co.crit.Since, err = report.ParseSince(s, now, time.UTC); err != nil {
			return nil, fmt.Errorf("--since: %w", err)
		}
		co.sinceSet = true
	}
	if s := get("until"); s != "" {
		if co.crit.Before, err = report.ParseDate(s, time.UTC); err != nil {
			return nil, fmt.Errorf("--until: %w", err)
		}
	}
	d, ok, err := report.ParseRelative(get("overlap"))
	if !ok || err != nil {
		return nil, fmt.Errorf("--overlap: invalid duration %q", get("overlap"))
	}
	co.overlap = d
	if co.deleteAfter && !v.GetBool("yes") {
		return nil, errors.New("--delete-after removes messages from the server; add --yes to confirm")
	}
	if co.deleteAfter && !co.extract {
		return nil, errors.New("--delete-after requires --extract (messages are deleted only after their report is stored)")
	}
	return co, nil
}

// collectStats summarizes a collection run.
type collectStats struct {
	Downloaded int
	Deleted    int
	Extract    pipeline.ExtractStats
}

// runCollect downloads new messages, stores them as .eml, optionally extracts
// and deletes, and advances the source's last-collect timestamp on success.
func runCollect(ctx context.Context, e *env, co *collectOpts, out io.Writer) (*collectStats, error) {
	startedAt := co.now().UTC()
	sess, mailboxID, err := co.open(ctx, co.deleteAfter)
	if err != nil {
		return nil, err
	}
	folder := co.crit.Folder
	if folder == "" {
		folder = "(default)"
	}
	src, err := e.store.EnsureSource(ctx, co.protocol, mailboxID, folder)
	if err != nil {
		_ = sess.Close()
		return nil, err
	}

	crit := co.crit
	if !co.sinceSet {
		if src.LastCollectAt.IsZero() {
			crit.Since = startedAt.Add(-defaultLookback)
		} else {
			crit.Since = src.LastCollectAt.Add(-co.overlap)
		}
	}
	crit.SkipKey = func(key string) bool {
		ok, err := e.store.IsFetched(ctx, src.ID, key)
		if err != nil {
			e.log.Warn("state lookup failed; downloading again", "key", key, "error", err)
			return false
		}
		return ok
	}
	e.log.Info("collecting", "protocol", co.protocol, "folder", folder, "since", crit.Since.Format(time.RFC3339))

	stats := &collectStats{}
	x := &pipeline.Extractor{Store: e.store, Limits: extract.DefaultLimits, Log: e.log}
	baseDir := filepath.Join(e.cfg.EMLDir(), co.protocol)
	var toDelete []string

	sink := func(ctx context.Context, m fetch.RawMessage) error {
		received := m.Received
		if received.IsZero() {
			received = headerDate(m.Raw, startedAt)
		}
		dir := filepath.Join(baseDir, received.UTC().Format("2006-01"))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		path := filepath.Join(dir, emlName(m.Key))
		if err := export.WriteFile(path, m.Raw); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		sum := sha256.Sum256(m.Raw)
		if err := e.store.RecordFetched(ctx, src.ID, store.FetchedMessage{
			MsgKey: m.Key, ReceivedAt: received, EMLPath: path, SHA256: hex.EncodeToString(sum[:]),
		}); err != nil {
			return err
		}
		stats.Downloaded++
		if !co.extract {
			return nil
		}
		status, err := x.File(ctx, path, m.Raw, &stats.Extract)
		if err != nil {
			return err
		}
		// Only delete mail whose aggregate report is safely stored. Deletion is
		// deferred until Fetch has finished: deleting while paging (Graph
		// $skip-based nextLink, Gmail pageToken) would shift unseen messages
		// out of the result set.
		if co.deleteAfter && (status == store.StatusOK || status == store.StatusDuplicate) {
			toDelete = append(toDelete, m.Key)
		}
		return nil
	}

	fetchErr := sess.Fetch(ctx, crit, sink)
	// Delete queued messages even if Fetch failed later: their reports are stored.
	for _, key := range toDelete {
		if err := sess.Delete(ctx, key); err != nil {
			e.log.Warn("delete failed", "key", key, "error", err)
			continue
		}
		if err := e.store.MarkDeleted(ctx, src.ID, key); err != nil {
			_ = sess.Close()
			return stats, err
		}
		stats.Deleted++
	}
	closeErr := sess.Close() // POP3 commits deletions here
	if fetchErr != nil {
		printCollectStats(out, stats, co)
		return stats, fmt.Errorf("collect: %w", fetchErr)
	}
	if closeErr != nil && stats.Deleted > 0 {
		e.log.Warn("closing session failed; deletions may not be committed", "error", closeErr)
	}
	if err := e.store.SetLastCollect(ctx, src.ID, startedAt); err != nil {
		return stats, err
	}
	printCollectStats(out, stats, co)
	return stats, nil
}

func printCollectStats(w io.Writer, st *collectStats, co *collectOpts) {
	fmt.Fprintf(w, "Downloaded: %d new messages", st.Downloaded)
	if co.deleteAfter {
		fmt.Fprintf(w, "  Deleted on server: %d", st.Deleted)
	}
	fmt.Fprintln(w)
	if co.extract {
		printExtractStats(w, &st.Extract)
	}
}

// emlName builds a collision-safe file name from a message key: a readable
// sanitized prefix plus a short hash (keys may differ only by case, which
// case-insensitive file systems would otherwise merge).
func emlName(key string) string {
	clean := export.SanitizeFilename(key)
	clean = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return '_'
		}
		return r
	}, clean)
	if len(clean) > 48 {
		clean = clean[:48]
	}
	sum := sha256.Sum256([]byte(key))
	return clean + "-" + hex.EncodeToString(sum[:4]) + ".eml"
}

// headerDate returns the message Date header, or fallback.
func headerDate(raw []byte, fallback time.Time) time.Time {
	end := bytes.Index(raw, []byte("\r\n\r\n"))
	if end < 0 {
		end = bytes.Index(raw, []byte("\n\n"))
	}
	if end < 0 {
		end = len(raw)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(append(append([]byte{}, raw[:end]...), "\r\n\r\n"...)))
	if err != nil {
		return fallback
	}
	if d, err := msg.Header.Date(); err == nil {
		return d
	}
	return fallback
}
