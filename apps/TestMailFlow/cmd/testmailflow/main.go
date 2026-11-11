// Command testmailflow probes mail flow end to end: it sends small probe
// messages over SMTP or Microsoft Graph and verifies over Microsoft Graph,
// Gmail API, IMAP or POP3 that they arrive, as a foreground process or a
// native Windows service.
package main

import (
	"os"

	"github.com/ehlo-pl/gomailtesttool/apps/TestMailFlow/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
