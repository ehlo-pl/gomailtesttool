//go:build pop3 || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/pop3"

func init() {
	rootCmd.AddCommand(pop3.NewCmd())
}
