//go:build !integration

package imap

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/ehlo-pl/gomailtesttool/internal/common/fetch"
)

// startMemServer runs an in-memory IMAP server with one user and returns its port.
func startMemServer(t *testing.T) int {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("dmarc", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		InsecureAuth: true,
		Logger:       nopLogger{},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return ln.Addr().(*net.TCPAddr).Port
}

type nopLogger struct{}

func (nopLogger) Printf(string, ...any) {}

func appendMsg(t *testing.T, port int, subject string, date time.Time) {
	t.Helper()
	c, err := imapclient.DialInsecure(net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Login("dmarc", "secret").Wait(); err != nil {
		t.Fatal(err)
	}
	body := "From: a@reporter.example\r\nSubject: " + subject + "\r\n\r\nbody\r\n"
	cmd := c.Append("INBOX", int64(len(body)), &imap.AppendOptions{Time: date})
	_, _ = io.WriteString(cmd, body)
	_ = cmd.Close()
	if _, err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestFetchSessionIMAP(t *testing.T) {
	port := startMemServer(t)
	appendMsg(t, port, "Report domain: example.com old", time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	appendMsg(t, port, "Report domain: example.com new", time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC))
	appendMsg(t, port, "Hello", time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC))

	cfg := NewConfig()
	cfg.Host, cfg.Port = "127.0.0.1", port
	cfg.Username, cfg.Password, cfg.AuthMethod = "dmarc", "secret", "LOGIN"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	sess, err := OpenFetchSession(ctx, cfg, log)
	if err != nil {
		t.Fatalf("OpenFetchSession: %v", err)
	}
	defer func() { _ = sess.Close() }()

	var keys []string
	err = sess.Fetch(ctx, fetch.Criteria{Since: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Subject: "Report domain"},
		func(_ context.Context, m fetch.RawMessage) error {
			keys = append(keys, m.Key)
			if !strings.Contains(string(m.Raw), "new") {
				t.Errorf("unexpected message %q", m.Raw)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(keys) != 1 || !strings.HasSuffix(keys[0], ":2") {
		t.Fatalf("keys = %v, want [<validity>:2]", keys)
	}

	// Skipping known keys avoids the download.
	n := 0
	_ = sess.Fetch(ctx, fetch.Criteria{SkipKey: func(k string) bool { return k == keys[0] }},
		func(context.Context, fetch.RawMessage) error { n++; return nil })
	if n != 2 {
		t.Errorf("fetched %d with skip, want 2", n)
	}

	if err := sess.Delete(ctx, keys[0]); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := sess.Delete(ctx, "1:abc"); err == nil {
		t.Error("expected invalid key error")
	}
	if err := sess.Delete(ctx, "999999:1"); err == nil || !strings.Contains(err.Error(), "UIDVALIDITY") {
		t.Errorf("expected UIDVALIDITY error, got %v", err)
	}
	n = 0
	_ = sess.Fetch(ctx, fetch.Criteria{}, func(context.Context, fetch.RawMessage) error { n++; return nil })
	if n != 2 {
		t.Errorf("messages after delete = %d, want 2", n)
	}
}
