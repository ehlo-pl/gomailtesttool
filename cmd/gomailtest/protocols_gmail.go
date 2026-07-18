//go:build gmail || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/gmail"

func init() {
	rootCmd.AddCommand(gmail.NewCmd())
}
