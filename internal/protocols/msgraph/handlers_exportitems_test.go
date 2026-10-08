//go:build !integration
// +build !integration

package msgraph

import "testing"

func TestParseExportItemsResponse(t *testing.T) {
	data, err := parseExportItemsResponse([]byte(`{"value":[{"itemId":"a","data":"aGVsbG8=","error":null}]}`))
	if err != nil || string(data) != "hello" {
		t.Fatalf("got %q, %v", data, err)
	}
	if _, err := parseExportItemsResponse([]byte(`{"value":[{"itemId":"a","error":{"code":"x"}}]}`)); err == nil {
		t.Fatal("expected item error")
	}
	if _, err := parseExportItemsResponse([]byte(`{"value":[]}`)); err == nil {
		t.Fatal("expected empty error")
	}
}

func TestExportItemsURL(t *testing.T) {
	if got := exportItemsURL("a@b.com"); got != "https://graph.microsoft.com/beta/admin/exchange/mailboxes/a@b.com/exportItems" {
		t.Fatal(got)
	}
}

func TestValidateExportMethod(t *testing.T) {
	c := &Config{Action: ActionExportMessages, Folder: "inbox", ExportMethod: "bogus"}
	if err := validateConfiguration(c); err == nil {
		t.Fatal("expected error")
	}
}
