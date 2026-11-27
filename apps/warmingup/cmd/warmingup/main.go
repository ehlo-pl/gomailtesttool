// Command warmingup warms up sending domains with slowly growing mail volume.
package main

import (
	"os"
	_ "time/tzdata" // time zones must work on minimal hosts

	"github.com/ehlo-pl/gomailtesttool/apps/warmingup/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
