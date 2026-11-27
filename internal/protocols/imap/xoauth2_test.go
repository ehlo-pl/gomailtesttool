//go:build !integration

package imap

import "testing"

func TestXOAUTH2Start(t *testing.T) {
	mech, ir, err := (&xoauth2Client{username: "u@example.com", token: "tok"}).Start()
	if err != nil || mech != "XOAUTH2" {
		t.Fatalf("Start() = %q, %v; want XOAUTH2", mech, err)
	}
	if want := "user=u@example.com\x01auth=Bearer tok\x01\x01"; string(ir) != want {
		t.Errorf("initial response = %q, want %q", ir, want)
	}
}
