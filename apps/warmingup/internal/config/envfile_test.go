//go:build !integration

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadEnvFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.env")
	write := func(content string) {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("\xef\xbb\xbf# comment\r\n; another\r\n\r\nA=one\r\n B = \"two words\" \r\nC='q'\r\nD=a=b#c\r\nE=\r\nF=\"\r\n")
	got, err := readEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "one", "B": "two words", "C": "q", "D": "a=b#c", "E": "", "F": `"`}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("got %v", got)
	}
	for _, bad := range []string{"no equals sign", "=value", "export A=1", "\xff\xfeA\x00"} {
		write(bad)
		if _, err := readEnvFile(path); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestLookupOrder(t *testing.T) {
	c := &Config{}
	env := func(name string) (string, bool) {
		v, ok := map[string]string{"BOTH": "env", "ENVONLY": "env"}[name]
		return v, ok
	}
	file := map[string]string{"BOTH": "file", "FILEONLY": "file"}
	for name, want := range map[string]string{"BOTH": "env", "ENVONLY": "env", "FILEONLY": "file"} {
		if got, _ := c.lookup(env, file, false)(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if _, ok := c.lookup(env, file, false)("NONE"); ok {
		t.Error("an unset name must stay unset")
	}
	if got, _ := c.lookup(env, file, true)("NONE"); got != "unset" {
		t.Errorf("placeholder = %q", got)
	}
	if len(c.FromProcessEnv) != 1 || c.FromProcessEnv[0] != "ENVONLY" {
		t.Errorf("FromProcessEnv = %v", c.FromProcessEnv)
	}
}
