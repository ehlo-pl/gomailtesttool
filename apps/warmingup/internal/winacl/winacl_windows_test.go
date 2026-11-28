//go:build windows && !integration

package winacl

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestRestrict(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "existing")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The current user stands in for the service account, so the test keeps access.
	if err := Restrict(dir, u.Username, false); err != nil {
		t.Fatal(err)
	}
	sddl := func(path string) string {
		sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		return sd.String()
	}
	got := sddl(dir)
	if !strings.HasPrefix(got, "D:P") || strings.Count(got, "(") != 3 || !strings.Contains(got, ";SY)") || !strings.Contains(got, ";BA)") {
		t.Errorf("directory access list = %s", got)
	}
	created := filepath.Join(dir, "created")
	if err := os.WriteFile(created, []byte("x"), 0o600); err != nil {
		t.Fatalf("service account cannot write: %v", err)
	}
	for _, path := range []string{existing, created} {
		if s := sddl(path); strings.Count(s, "(") != 3 {
			t.Errorf("%s does not inherit only the directory entries: %s", path, s)
		}
	}
	// A single file, readable by the account.
	if err := Restrict(existing, u.Username, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(existing); err != nil {
		t.Errorf("read after restrict: %v", err)
	}
	if err := Restrict(existing, `no-such-domain\no-such-account`, true); err == nil {
		t.Error("an unknown account must be an error")
	}
	// Let the temporary directory be removed.
	if err := Restrict(existing, u.Username, false); err != nil {
		t.Fatal(err)
	}
}
