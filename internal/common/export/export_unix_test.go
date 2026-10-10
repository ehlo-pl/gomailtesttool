//go:build unix

package export

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateExportDirUsesOwnerOnlyPermissions(t *testing.T) {
	baseDir := t.TempDir()

	dir, err := CreateExportDir(baseDir)
	if err != nil {
		t.Fatalf("CreateExportDir() error = %v", err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod(%q) error = %v", dir, err)
	}

	got, err := CreateExportDir(baseDir)
	if err != nil {
		t.Fatalf("CreateExportDir() for existing directory error = %v", err)
	}
	if got != dir {
		t.Fatalf("CreateExportDir() = %q, want %q", got, dir)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", dir, err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Errorf("export directory mode = %#o, want %#o", got, want)
	}
}

func TestWriteFileUsesOwnerOnlyPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message.eml")
	if err := os.WriteFile(path, []byte("previous content"), 0o644); err != nil {
		t.Fatalf("WriteFile() setup error = %v", err)
	}

	const wantContent = "private message"
	if err := WriteFile(path, []byte(wantContent)); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("export file mode = %#o, want %#o", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if got := string(data); got != wantContent {
		t.Errorf("export file contents = %q, want %q", got, wantContent)
	}
}
