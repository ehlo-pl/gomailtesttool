//go:build smtp || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/smtp"

func init() {
	rootCmd.AddCommand(smtp.NewCmd())
}
