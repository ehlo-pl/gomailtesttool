//go:build ews || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/ews"

func init() {
	rootCmd.AddCommand(ews.NewCmd())
}
