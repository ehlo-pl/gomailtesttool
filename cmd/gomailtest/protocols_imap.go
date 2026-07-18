//go:build imap || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/imap"

func init() {
	rootCmd.AddCommand(imap.NewCmd())
}
