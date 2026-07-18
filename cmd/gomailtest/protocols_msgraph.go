//go:build msgraph || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/msgraph"

func init() {
	rootCmd.AddCommand(msgraph.NewCmd())
}
