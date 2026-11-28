//go:build !windows

// Package winacl restricts files to administrators and one service account.
// Outside Windows the file modes set at creation do this.
package winacl

// Restrict does nothing outside Windows.
func Restrict(string, string, bool) error { return nil }
