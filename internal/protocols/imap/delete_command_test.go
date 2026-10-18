package imap

import "testing"

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
