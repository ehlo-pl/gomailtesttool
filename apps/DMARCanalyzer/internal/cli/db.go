package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
)

func newDBCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database maintenance and statistics",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "stats",
		Short: "Show database statistics",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v)
			if err != nil {
				return err
			}
			defer e.Close()
			st, err := e.store.Stats(ctx)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(w, "Database:         %s\n", e.cfg.DBPath); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Data directory:   %s\n", e.cfg.DataDir); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Sources:          %d\n", st.Sources); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Fetched messages: %d\n", st.FetchedMessages); err != nil {
				return err
			}
			statuses := make([]string, 0, len(st.Files))
			for k := range st.Files {
				statuses = append(statuses, k)
			}
			sort.Strings(statuses)
			for _, k := range statuses {
				if _, err := fmt.Fprintf(w, "Files %-11s %d\n", k+":", st.Files[k]); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w, "Reports:          %d from %d reporters, %d domains\n", st.Reports, st.Reporters, st.Domains); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(w, "Records:          %d covering %d messages\n", st.Records, st.Messages); err != nil {
				return err
			}
			if !st.FirstReport.IsZero() {
				if _, err := fmt.Fprintf(w, "Report range:     %s → %s\n", st.FirstReport.Format("2006-01-02"), st.LastReport.Format("2006-01-02")); err != nil {
					return err
				}
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "vacuum",
		Short: "Compact the database file",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v)
			if err != nil {
				return err
			}
			defer e.Close()
			if _, err := e.store.DB().ExecContext(ctx, "VACUUM"); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Database compacted:", e.cfg.DBPath)
			return err
		},
	})
	return cmd
}
