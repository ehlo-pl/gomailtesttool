package pop3

import (
	"bufio"
	"context"
	"net"
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

func TestPOP3ClientDeleteSendsDELE(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	client := &POP3Client{conn: clientConn, reader: bufio.NewReader(clientConn)}
	serverResult := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(serverConn)
		command, _ := reader.ReadString('\n')
		_, _ = serverConn.Write([]byte("+OK message marked\r\n"))
		serverResult <- command
	}()

	if err := client.Delete(context.Background(), 7); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got := <-serverResult; got != "DELE 7\r\n" {
		t.Errorf("command = %q, want %q", got, "DELE 7\r\n")
	}
}

func TestPOP3ClientQuitReportsDeletionCommitFailure(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()

	client := &POP3Client{conn: clientConn, reader: bufio.NewReader(clientConn)}
	go func() {
		_, _ = bufio.NewReader(serverConn).ReadString('\n')
		_, _ = serverConn.Write([]byte("-ERR deletion failed\r\n"))
	}()

	if err := client.Quit(); err == nil {
		t.Fatal("Quit() error = nil, want failed commit error")
	}
}
