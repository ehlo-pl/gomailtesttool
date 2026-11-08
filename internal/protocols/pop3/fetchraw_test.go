package pop3

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

// fakePOP3 serves a fixed mailbox over conn until QUIT and records commands.
func fakePOP3(t *testing.T, conn net.Conn, msgs map[int][2]string, cmds chan<- string) {
	t.Helper()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		cmds <- line
		f := strings.Fields(line)
		var n int
		if len(f) > 1 {
			_, _ = fmt.Sscanf(f[1], "%d", &n)
		}
		switch f[0] {
		case "UIDL":
			var b strings.Builder
			b.WriteString("+OK\r\n")
			for i := 1; i <= len(msgs); i++ {
				fmt.Fprintf(&b, "%d %s\r\n", i, msgs[i][0])
			}
			b.WriteString(".\r\n")
			_, _ = io.WriteString(conn, b.String())
		case "TOP":
			head, _, _ := strings.Cut(msgs[n][1], "\r\n\r\n")
			_, _ = io.WriteString(conn, "+OK\r\n"+head+"\r\n.\r\n")
		case "RETR":
			_, _ = io.WriteString(conn, "+OK\r\n"+msgs[n][1]+"\r\n.\r\n")
		case "DELE":
			_, _ = io.WriteString(conn, "+OK marked\r\n")
		case "QUIT":
			_, _ = io.WriteString(conn, "+OK bye\r\n")
			return
		default:
			_, _ = io.WriteString(conn, "-ERR unknown\r\n")
		}
	}
}

func msg(subject, date string) string {
	return "From: a@reporter.example\r\nSubject: " + subject + "\r\nDate: " + date + "\r\n\r\nbody"
}

func TestFetchSessionPOP3(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = serverConn.Close() }()
	msgs := map[int][2]string{
		1: {"u-old", msg("Report domain: example.com", "Mon, 01 Sep 2026 10:00:00 +0000")},
		2: {"u-new", msg("Report domain: example.com", "Mon, 02 Nov 2026 10:00:00 +0000")},
		3: {"u-other", msg("Hello", "Mon, 02 Nov 2026 11:00:00 +0000")},
		4: {"u-known", msg("Report domain: example.com", "Mon, 02 Nov 2026 12:00:00 +0000")},
	}
	cmds := make(chan string, 100)
	go fakePOP3(t, serverConn, msgs, cmds)

	s := &fetchSession{
		client: &POP3Client{conn: clientConn, reader: bufio.NewReader(clientConn)},
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		byKey:  map[string]int{},
	}
	var got []string
	err := s.Fetch(context.Background(), fetch.Criteria{
		Since:   time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Subject: "report domain",
		SkipKey: func(k string) bool { return k == "u-known" },
	}, func(_ context.Context, m fetch.RawMessage) error {
		got = append(got, m.Key)
		if !strings.Contains(string(m.Raw), "body") || m.Received.IsZero() {
			t.Errorf("message %s: raw=%q received=%v", m.Key, m.Raw, m.Received)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(got) != 1 || got[0] != "u-new" {
		t.Fatalf("fetched = %v, want [u-new]", got)
	}
	if err := s.Delete(context.Background(), "u-new"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete(context.Background(), "nope"); err == nil {
		t.Error("expected error for unknown key")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(cmds)
	var all []string
	for c := range cmds {
		all = append(all, c)
	}
	joined := strings.Join(all, "|")
	if strings.Contains(joined, "TOP 4") || strings.Contains(joined, "RETR 1") || !strings.Contains(joined, "DELE 2") {
		t.Errorf("unexpected command sequence: %s", joined)
	}
}
