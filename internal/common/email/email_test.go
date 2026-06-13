package email

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestLoadAttachments(t *testing.T) {
	dir := t.TempDir()
	txtPath := writeTempFile(t, dir, "note.txt", []byte("hello world"))
	pngPath := writeTempFile(t, dir, "image.png", []byte{0x89, 0x50, 0x4e, 0x47})
	binPath := writeTempFile(t, dir, "data.unknownext", []byte{0x01, 0x02})

	t.Run("reads files with MIME detection", func(t *testing.T) {
		attachments, err := LoadAttachments([]string{txtPath, pngPath}, nil)
		if err != nil {
			t.Fatalf("LoadAttachments() error = %v", err)
		}
		if len(attachments) != 2 {
			t.Fatalf("got %d attachments, want 2", len(attachments))
		}
		if attachments[0].Name != "note.txt" || attachments[0].ContentType != "text/plain; charset=utf-8" {
			t.Errorf("note.txt: got name=%q contentType=%q", attachments[0].Name, attachments[0].ContentType)
		}
		if string(attachments[0].Data) != "hello world" {
			t.Errorf("note.txt: got data=%q", attachments[0].Data)
		}
		if attachments[0].Inline || attachments[0].ContentID != "" {
			t.Errorf("note.txt: expected non-inline attachment, got Inline=%v ContentID=%q", attachments[0].Inline, attachments[0].ContentID)
		}
	})

	t.Run("unknown extension falls back to octet-stream", func(t *testing.T) {
		attachments, err := LoadAttachments([]string{binPath}, nil)
		if err != nil {
			t.Fatalf("LoadAttachments() error = %v", err)
		}
		if attachments[0].ContentType != "application/octet-stream" {
			t.Errorf("got contentType=%q, want application/octet-stream", attachments[0].ContentType)
		}
	})

	t.Run("skips unreadable files and reports via callback", func(t *testing.T) {
		missing := filepath.Join(dir, "missing.txt")
		var skipped []string
		attachments, err := LoadAttachments([]string{txtPath, missing}, func(path string, _ error) {
			skipped = append(skipped, path)
		})
		if err != nil {
			t.Fatalf("LoadAttachments() error = %v", err)
		}
		if len(attachments) != 1 {
			t.Fatalf("got %d attachments, want 1", len(attachments))
		}
		if len(skipped) != 1 || skipped[0] != missing {
			t.Errorf("got skipped=%v, want [%s]", skipped, missing)
		}
	})

	t.Run("error when all paths fail", func(t *testing.T) {
		missing := filepath.Join(dir, "missing.txt")
		_, err := LoadAttachments([]string{missing}, nil)
		if err == nil {
			t.Error("LoadAttachments() error = nil, want error when no attachments could be loaded")
		}
	})

	t.Run("empty input returns no attachments and no error", func(t *testing.T) {
		attachments, err := LoadAttachments(nil, nil)
		if err != nil || len(attachments) != 0 {
			t.Errorf("LoadAttachments(nil) = %v, %v, want empty, nil", attachments, err)
		}
	})
}

func TestLoadInlineAttachments(t *testing.T) {
	dir := t.TempDir()
	pngPath := writeTempFile(t, dir, "logo.png", []byte{0x89, 0x50, 0x4e, 0x47})

	attachments, err := LoadInlineAttachments([]string{pngPath}, nil)
	if err != nil {
		t.Fatalf("LoadInlineAttachments() error = %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("got %d attachments, want 1", len(attachments))
	}
	a := attachments[0]
	if !a.Inline {
		t.Error("expected Inline = true")
	}
	if a.ContentID != "logo.png" {
		t.Errorf("got ContentID=%q, want logo.png", a.ContentID)
	}
}

func TestParseHeaders(t *testing.T) {
	tests := []struct {
		name    string
		raw     []string
		want    []Header
		wantErr bool
	}{
		{
			name: "valid headers",
			raw:  []string{"X-Custom: value", "X-Other:1"},
			want: []Header{
				{Name: "X-Custom", Value: "value"},
				{Name: "X-Other", Value: "1"},
			},
		},
		{
			name:    "missing colon",
			raw:     []string{"X-Custom value"},
			wantErr: true,
		},
		{
			name:    "empty name",
			raw:     []string{": value"},
			wantErr: true,
		},
		{
			name:    "protected header rejected",
			raw:     []string{"Subject: hijacked"},
			wantErr: true,
		},
		{
			name:    "protected header rejected case-insensitively",
			raw:     []string{"from: hijacked@evil.com"},
			wantErr: true,
		},
		{
			name: "CRLF stripped from value",
			raw:  []string{"X-Custom: value\r\nBcc: attacker@evil.com"},
			want: []Header{
				{Name: "X-Custom", Value: "valueBcc: attacker@evil.com"},
			},
		},
		{
			name: "empty input",
			raw:  nil,
			want: []Header{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseHeaders(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseHeaders() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d headers, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("header[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
