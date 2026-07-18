//go:build jmap || !custom

package main

import "github.com/ehlo-pl/gomailtesttool/internal/protocols/jmap"

func init() {
	rootCmd.AddCommand(jmap.NewCmd())
}
