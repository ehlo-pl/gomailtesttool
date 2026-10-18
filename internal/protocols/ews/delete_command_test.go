package ews

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeleteMessagesCommandAndExportFlag(t *testing.T) {
	root := NewCmd()
	deleteCmd, _, err := root.Find([]string{"deletemessages"})
	if err != nil || deleteCmd.Name() != "deletemessages" {
		t.Fatalf("deletemessages command = %v, err = %v", deleteCmd, err)
	}
	exportCmd, _, err := root.Find([]string{"exportmessages"})
	if err != nil {
		t.Fatal(err)
	}
	if exportCmd.Flags().Lookup("delete") == nil {
		t.Fatal("exportmessages is missing --delete")
	}
}

func TestDeleteEWSMessageMovesToDeletedItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `DeleteType="MoveToDeletedItems"`) ||
			!strings.Contains(string(body), `Id="item&amp;1"`) {
			t.Errorf("unexpected DeleteItem request: %s", body)
		}
		_, _ = io.WriteString(w, `<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" xmlns:m="http://schemas.microsoft.com/exchange/services/2006/messages"><s:Body><m:DeleteItemResponse><m:ResponseMessages><m:DeleteItemResponseMessage ResponseClass="Success"><m:ResponseCode>NoError</m:ResponseCode></m:DeleteItemResponseMessage></m:ResponseMessages></m:DeleteItemResponse></s:Body></s:Envelope>`)
	}))
	defer server.Close()

	client := &EWSClient{httpClient: server.Client(), config: NewConfig(), ewsURL: server.URL}
	if err := deleteEWSMessage(context.Background(), client, "item&1"); err != nil {
		t.Fatalf("deleteEWSMessage() error = %v", err)
	}
}
