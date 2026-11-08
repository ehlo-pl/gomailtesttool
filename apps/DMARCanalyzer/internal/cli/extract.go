package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/extract"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/pipeline"
	"github.com/ehlo-pl/gomailtesttool/internal/common/bootstrap"
)

func newExtractCmd(v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extract",
		Short: "Extract DMARC reports from .eml / .xml / .gz / .zip files into the database",
		Long: `Scan files and directories for DMARC aggregate reports and store them in the database.

Inputs may be .eml messages (for example from 'gomailtest <protocol> exportmessages'
or from 'dmarcanalyzer collect') and loose report files (.xml, .xml.gz, .gz, .zip).
Files already processed with identical content are skipped unless --reprocess is set.
The same report (reporter, report ID, domain) is stored only once.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := bootstrap.SetupSignalContext()
			defer cancel()
			e, err := setup(ctx, cmd, v)
			if err != nil {
				return err
			}
			defer e.Close()

			inputs, _ := cmd.Flags().GetStringSlice("input")
			if len(inputs) == 0 {
				inputs = []string{e.cfg.EMLDir()}
				if _, err := os.Stat(inputs[0]); os.IsNotExist(err) {
					return fmt.Errorf("nothing to extract: %s does not exist (use --input or run 'collect' first)", inputs[0])
				}
			}
			reprocess, _ := cmd.Flags().GetBool("reprocess")
			x := &pipeline.Extractor{Store: e.store, Limits: extract.DefaultLimits, Reprocess: reprocess, Log: e.log}
			st, err := x.Paths(ctx, inputs)
			printExtractStats(cmd.OutOrStdout(), st)
			return err
		},
	}
	cmd.Flags().StringSlice("input", nil, "Files or directories to scan (repeatable; default: <datadir>/eml)")
	cmd.Flags().Bool("reprocess", false, "Re-read files even if already processed with the same content")
	return cmd
}

func printExtractStats(w io.Writer, st *pipeline.ExtractStats) {
	if st == nil {
		return
	}
	fmt.Fprintf(w, "Files: %d (unchanged %d)  New reports: %d  Duplicates: %d  No report: %d  Forensic skipped: %d  Errors: %d\n",
		st.Files, st.Unchanged, st.Reports, st.Duplicates, st.NoReport, st.Forensic, st.Errors)
	for i, e := range st.ErrorList {
		if i == 20 {
			fmt.Fprintf(w, "  ... %d more errors\n", len(st.ErrorList)-20)
			break
		}
		fmt.Fprintf(w, "  error: %s\n", e)
	}
}
