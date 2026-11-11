package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/config"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/eventlog"
	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/svc"
)

// servicePasswordEnv supplies the service account password to 'service
// install' so it never appears on a command line.
const servicePasswordEnv = "TMFSERVICEPASSWORD"

func newServiceCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Install and control the Windows service",
		Long: `Manage testmailflow as a native Windows service (administrator rights required).
The service runs 'testmailflow run' with the config file and data directory given at
install time. On Linux, run 'testmailflow run' under systemd instead.`,
	}

	install := &cobra.Command{
		Use:   "install",
		Short: "Register the service (automatic delayed start, restart on failure) and its event source",
		Long: `Register the Windows service and the 'testmailflow' event log source.

The config file is validated first. ${ENV} references in it must be resolvable by the
service account, so define them as machine-level environment variables.

By default the service runs as LocalSystem. To use another account pass --user
(e.g. 'NT AUTHORITY\LocalService', 'DOMAIN\svc-tmf' or a gMSA 'DOMAIN\name$') and, for
ordinary accounts, set the password in the ` + servicePasswordEnv + ` environment variable.
The account needs read access to the config file and write access to the data directory.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = v.BindPFlags(cmd.Flags())
			_ = v.BindPFlags(cmd.InheritedFlags())
			if !svc.Supported {
				return svc.Install(svc.InstallOptions{})
			}
			cfgPath := strings.TrimSpace(v.GetString(config.KeyConfig))
			if cfgPath == "" {
				return errors.New("--config is required")
			}
			cfgPath, err := filepath.Abs(cfgPath)
			if err != nil {
				return err
			}
			plan, err := config.LoadPlan(cfgPath)
			if err != nil {
				return err
			}
			if err := checkEndpoints(plan); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			dataDir := strings.TrimSpace(v.GetString(config.KeyDataDir))
			if dataDir == "" {
				dataDir = config.ServiceDataDir()
			}
			if dataDir, err = filepath.Abs(dataDir); err != nil {
				return err
			}
			if err := os.MkdirAll(dataDir, 0o700); err != nil {
				return fmt.Errorf("create data directory: %w", err)
			}
			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("locate executable: %w", err)
			}
			if err := svc.Install(svc.InstallOptions{
				Name:        config.AppName,
				DisplayName: "TestMailFlow mail flow probe",
				Description: "Sends probe e-mails and verifies their arrival (gomailtesttool testmailflow).",
				ExePath:     exe,
				Args:        []string{"run", "--" + config.KeyConfig, cfgPath, "--" + config.KeyDataDir, dataDir},
				User:        v.GetString("user"),
				Password:    os.Getenv(servicePasswordEnv),
			}); err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if err := eventlog.Install(config.AppName); err != nil {
				fmt.Fprintf(out, "Warning: %v\n", err)
			}
			fmt.Fprintf(out, "Service %s installed.\n  executable: %s\n  config:     %s\n  data dir:   %s\n  flows:      %d\n",
				config.AppName, exe, cfgPath, dataDir, len(plan.Flows))
			fmt.Fprintf(out, "Start it with: %s service start\n", config.AppName)
			return nil
		},
	}
	install.Flags().String("user", "", "Service account (default: LocalSystem); password from env "+servicePasswordEnv)

	simple := func(use, short string, fn func() (string, error)) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short,
			RunE: func(cmd *cobra.Command, _ []string) error {
				msg, err := fn()
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), msg)
				return nil
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
