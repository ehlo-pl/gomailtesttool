// Command dmarcanalyzer collects DMARC aggregate report e-mails from a mailbox
// (Microsoft Graph, Gmail API, IMAP, POP3), extracts the XML reports into a
// SQLite database and summarizes them over periods.
package main

import (
	"os"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
