//go:build !integration
// +build !integration

package msgraph

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempAttachment(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestCreateFileAttachments(t *testing.T) {
	dir := t.TempDir()
	docPath := writeTempAttachment(t, dir, "report.pdf", []byte("pdf-bytes"))
	imgPath := writeTempAttachment(t, dir, "logo.png", []byte{0x89, 0x50, 0x4e, 0x47})

	cfg := NewConfig()

	t.Run("regular attachment", func(t *testing.T) {
		attachments, err := createFileAttachments([]string{docPath}, nil, cfg)
		if err != nil {
			t.Fatalf("createFileAttachments() error = %v", err)
		}
		if len(attachments) != 1 {
			t.Fatalf("got %d attachments, want 1", len(attachments))
		}

		fa, ok := attachments[0].(interface {
			GetName() *string
			GetContentType() *string
			GetIsInline() *bool
		})
		if !ok {
			t.Fatal("attachment does not implement expected getters")
		}
		if got := fa.GetName(); got == nil || *got != "report.pdf" {
			t.Errorf("Name = %v, want report.pdf", got)
		}
		if got := fa.GetContentType(); got == nil || *got != "application/pdf" {
			t.Errorf("ContentType = %v, want application/pdf", got)
		}
		if got := fa.GetIsInline(); got != nil && *got {
			t.Errorf("IsInline = %v, want nil/false for regular attachment", *got)
		}
	})

	t.Run("inline attachment sets IsInline and ContentId", func(t *testing.T) {
		attachments, err := createFileAttachments(nil, []string{imgPath}, cfg)
		if err != nil {
			t.Fatalf("createFileAttachments() error = %v", err)
		}
		if len(attachments) != 1 {
			t.Fatalf("got %d attachments, want 1", len(attachments))
		}

		fa, ok := attachments[0].(interface {
			GetName() *string
			GetIsInline() *bool
			GetContentId() *string
		})
		if !ok {
			t.Fatal("attachment does not implement expected getters")
		}
		if got := fa.GetIsInline(); got == nil || !*got {
			t.Errorf("IsInline = %v, want true", got)
		}
		if got := fa.GetContentId(); got == nil || *got != "logo.png" {
			t.Errorf("ContentId = %v, want logo.png", got)
		}
	})

	t.Run("regular and inline combined", func(t *testing.T) {
		attachments, err := createFileAttachments([]string{docPath}, []string{imgPath}, cfg)
		if err != nil {
			t.Fatalf("createFileAttachments() error = %v", err)
		}
		if len(attachments) != 2 {
			t.Fatalf("got %d attachments, want 2", len(attachments))
		}
	})

	t.Run("empty input returns no attachments", func(t *testing.T) {
		attachments, err := createFileAttachments(nil, nil, cfg)
		if err != nil || len(attachments) != 0 {
			t.Errorf("createFileAttachments(nil, nil) = %v, %v, want empty, nil", attachments, err)
		}
	})

	t.Run("missing file returns error when no attachments succeed", func(t *testing.T) {
		missing := filepath.Join(dir, "missing.pdf")
		if _, err := createFileAttachments([]string{missing}, nil, cfg); err == nil {
			t.Error("createFileAttachments() error = nil, want error when all attachments fail")
		}
	})
}
