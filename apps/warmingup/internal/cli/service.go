package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/winacl"
	"github.com/ehlo-pl/gomailtesttool/internal/common/eventlog"
	"github.com/ehlo-pl/gomailtesttool/internal/common/svc"
)

// servicePasswordEnv supplies the service account password to 'service
// install' so it never appears on a command line.
const servicePasswordEnv = "WARMUPSERVICEPASSWORD"

// Windows event IDs.
const (
	eventStarted    uint32 = 100
	eventStopped    uint32 = 101
	eventTargetHeld uint32 = 200
	eventDomainHeld uint32 = 300
)

func newServiceCmd(g *globals) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Install and control the Windows service",
		Long: `Manage warmingup as a native Windows service (administrator rights required).
The service runs 'warmingup run' with the config file and data directory given at
install time. On Linux, run 'warmingup run' under systemd instead.`,
	}

	var user string
	install := &cobra.Command{
		Use:   "install",
		Short: "Register the service (automatic delayed start, restart on failure) and its event source",
		Long: `Register the Windows service and the 'warmingup' event log source, and limit the
data directory to SYSTEM, Administrators and the service account.

The configuration is validated first. The service does not see your environment
variables: put the ${NAME} values in the file named by 'warmingup: envfile:'.

By default the service runs as LocalSystem. To use another account pass --user
(e.g. 'NT AUTHORITY\LocalService', 'DOMAIN\svc-warmup' or a gMSA 'DOMAIN\name$')
and, for ordinary accounts, set the password in the ` + servicePasswordEnv + `
environment variable.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !svc.Supported {
				return svc.Install(svc.InstallOptions{})
			}
			return installService(cmd.OutOrStdout(), g, user)
		},
	}
	install.Flags().StringVar(&user, "user", "", "service account (default: LocalSystem); password from env "+servicePasswordEnv)

	simple := func(use, short string, fn func() (string, error)) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				msg, err := fn()
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), msg)
				return err
			},
		}
	}
	cmd.AddCommand(
		install,
		simple("uninstall", "Stop and remove the service and its event source", func() (string, error) {
			if err := svc.Uninstall(config.AppName); err != nil {
				return "", err
			}
			_ = eventlog.Remove(config.AppName)
			return "Service " + config.AppName + " removed. The data directory was kept.", nil
		}),
		simple("start", "Start the service", func() (string, error) {
			return "Service " + config.AppName + " started.", svc.Start(config.AppName)
		}),
		simple("stop", "Stop the service", func() (string, error) {
			return "Service " + config.AppName + " stopped.", svc.Stop(config.AppName)
		}),
		simple("status", "Show the service state", func() (string, error) {
			state, err := svc.State(config.AppName)
			return "Service " + config.AppName + ": " + state, err
		}),
	)
	return cmd
}

// servicePlan is what 'service install' will register.
type servicePlan struct {
	exe, configFile, dataDir, envFile string
	domains                           int
	warnings                          []string
}

// planService validates the configuration and resolves the paths, without
// changing anything.
func planService(g *globals) (*servicePlan, error) {
	if strings.TrimSpace(g.configFile) == "" {
		return nil, errors.New("--config is required")
	}
	// A service starts in System32: every path must be absolute.
	configFile, err := filepath.Abs(g.configFile)
	if err != nil {
		return nil, err
	}
	local := *g
	local.configFile = configFile
	e, err := loadConfig(&local, needSecrets)
	if err != nil {
		return nil, err
	}
	dataDir, err := filepath.Abs(e.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	p := &servicePlan{exe: exe, configFile: configFile, dataDir: dataDir, envFile: e.cfg.EnvFile, domains: len(e.cfg.Domains)}
	warn := func(format string, args ...any) { p.warnings = append(p.warnings, fmt.Sprintf(format, args...)) }

	if vars := e.cfg.FromProcessEnv; len(vars) > 0 {
		slices.Sort(vars)
		warn("only your environment provides %s; the service needs them in the envfile or as machine-wide variables",
			strings.Join(vars, ", "))
	}
	// What a command without --datadir (and without WARMUPDATADIR) would use.
	plain := e.cfg.FileDataDir
	if plain == "" {
		plain = config.DefaultDataDir()
	}
	if !samePath(plain, dataDir) {
		warn("other commands would use %s: set 'datadir: %s' in the config file, or pass --datadir to them", plain, dataDir)
	}
	if home, err := os.UserHomeDir(); err == nil && within(exe, home) {
		warn("the executable is in your profile; copy it to a folder only administrators can write, "+
			"such as %%ProgramFiles%%\\%s, and install from there", config.AppName)
	}
	for name, ep := range e.cfg.Endpoints {
		if ep.String("oauth") == "" {
			continue
		}
		token := filepath.Join(dataDir, "tokens", name+".json")
		if _, err := os.Stat(token); errors.Is(err, fs.ErrNotExist) {
			warn("endpoint %s is not signed in yet: run '%s auth %s --config %s' from an elevated prompt",
				name, config.AppName, name, configFile)
		}
	}
	return p, nil
}

func installService(out io.Writer, g *globals, user string) error {
	p, err := planService(g)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.dataDir, 0o700); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	if err := svc.Install(svc.InstallOptions{
		Name:        config.AppName,
		DisplayName: "Mail domain warm-up",
		Description: "Sends slowly growing mail volume from warmed domains and checks where it lands (gomailtesttool warmingup).",
		ExePath:     p.exe,
		Args:        []string{"run", "--config", p.configFile, "--datadir", p.dataDir},
		User:        user,
		Password:    os.Getenv(servicePasswordEnv),
	}); err != nil {
		return err
	}
	// After the registration: virtual accounts exist only from then on.
	p.warnings = append(p.warnings, protect(p, user, os.Getenv("ProgramData"), winacl.Restrict)...)
	if err := eventlog.Install(config.AppName); err != nil {
		p.warnings = append(p.warnings, err.Error())
	}
	w := &printer{w: out}
	w.f("Service %s installed.\n  executable: %s\n  config:     %s\n  data dir:   %s\n  domains:    %d\n",
		config.AppName, p.exe, p.configFile, p.dataDir, p.domains)
	for _, text := range p.warnings {
		w.f("WARNING: %s\n", text)
	}
	w.f("Start it with: %s service start\n", config.AppName)
	return w.err
}

// protect limits who can read the secrets, tokens and message data, and who
// can change what the service sends. Only places that are evidently ours get a
// new access list; it returns what was left unprotected.
func protect(p *servicePlan, user, programData string, apply func(path, account string, readOnly bool) error) []string {
	var warnings []string
	restrict := func(path string, readOnly bool) {
		if err := apply(path, user, readOnly); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	inProgramData := func(path string) bool { return programData != "" && within(path, programData) }

	if inProgramData(p.dataDir) || ownDirectory(p.dataDir) {
		restrict(p.dataDir, false)
	} else {
		warnings = append(warnings, fmt.Sprintf("permissions of %s were not changed because it holds other files; "+
			"it will contain tokens and message data, limit who can read it", p.dataDir))
	}
	dir := filepath.Dir(p.configFile)
	switch {
	case samePath(dir, p.dataDir) || within(dir, p.dataDir):
		// Covered by the data directory.
	case inProgramData(dir):
		restrict(dir, true)
	default:
		warnings = append(warnings, fmt.Sprintf("permissions of %s were not changed; "+
			"only administrators should be able to change the configuration", dir))
		if p.envFile != "" && !within(p.envFile, p.dataDir) {
			restrict(p.envFile, true)
		}
	}
	return warnings
}

// ownDirectory reports whether dir is empty or already holds the database.
func ownDirectory(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return len(entries) == 0 || slices.ContainsFunc(entries, func(e fs.DirEntry) bool {
		return strings.EqualFold(e.Name(), config.AppName+".db")
	})
}

// within reports whether path is strictly below dir.
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && rel == "."
}

// runDaemon runs the engine until ctx ends. A service logs to files; start,
// stop and holds also go to the event log.
func runDaemon(ctx context.Context, g *globals, asService bool) (err error) {
	events, evErr := eventlog.Open(config.AppName)
	defer func() { _ = events.Close() }()
	// A service has no console: make start-up failures visible.
	defer func() {
		if err != nil && asService {
			events.Report(eventlog.Error, eventStopped, config.AppName+" stopped with an error: "+err.Error())
		}
	}()
	local := *g
	local.logToFile = asService
	e, err := setup(ctx, &local, needSecrets)
	if err != nil {
		return err
	}
	defer e.Close()
	if evErr != nil {
		e.log.Warn("event log unavailable", "error", evErr)
	}
	for _, d := range e.cfg.Domains {
		e.log.Info("domain loaded", "domain", d.Name, "enabled", d.Enabled, "targets", len(d.Targets),
			"start", d.Start.Format("2006-01-02 15:04"), "peak_per_day", d.Schedule.Peak())
	}
	eng := e.engine()
	eng.OnHold = func(domain, target, reason string) {
		if target == "" {
			events.Report(eventlog.Error, eventDomainHeld, fmt.Sprintf("%s is held: %s", domain, reason))
			return
		}
		events.Report(eventlog.Warning, eventTargetHeld, fmt.Sprintf("%s: target %s is held: %s", domain, target, reason))
	}
	events.Report(eventlog.Info, eventStarted, fmt.Sprintf("%s started with %d domain(s)", config.AppName, len(e.cfg.Domains)))
	if err = eng.Run(ctx); err != nil {
		e.log.Error("stopped", "error", err)
		return err
	}
	e.log.Info("stopped")
	events.Report(eventlog.Info, eventStopped, config.AppName+" stopped")
	return nil
}
