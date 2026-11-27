//go:build !integration

package imap

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/ehlo-pl/gomailtesttool/internal/common/mailbox"
)

// startJunkServer runs go-imap's in-memory server with INBOX and a Spam folder.
func startJunkServer(t *testing.T) (*Config, *imapmemserver.User) {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser("u@example.com", "pw")
	for _, name := range []string{"INBOX", "Spam"} {
		if err := user.Create(name, nil); err != nil {
			t.Fatalf("Create(%s): %v", name, err)
		}
	}
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true,
		Caps:         imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	cfg := NewConfig()
	cfg.Host, cfg.Port = "127.0.0.1", ln.Addr().(*net.TCPAddr).Port
	cfg.Username, cfg.Password = "u@example.com", "pw"
	return cfg, user
}

type literal struct{ *strings.Reader }

func (l literal) Size() int64 { return int64(l.Len()) }

func TestMailboxSession(t *testing.T) {
	cfg, user := startJunkServer(t)
	msg := "Message-ID: <abc123@example.com>\r\nSubject: hello\r\nAuthentication-Results: mx; spf=pass\r\n\r\nbody\r\n"
	if _, err := user.Append("Spam", literal{strings.NewReader(msg)}, &imap.AppendOptions{}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	ctx := context.Background()

	if _, err := OpenMailbox(ctx, cfg, ""); err == nil || !strings.Contains(err.Error(), "junk:") {
		t.Fatalf("OpenMailbox() without \\Junk error = %v, want a hint to set junk:", err)
	}
	s, err := OpenMailbox(ctx, cfg, "Spam")
	if err != nil {
		t.Fatalf("OpenMailbox: %v", err)
	}
	defer func() { _ = s.Close() }()

	m, err := s.Locate(ctx, "abc123@example.com")
	if err != nil || m == nil {
		t.Fatalf("Locate() = %v, %v", m, err)
	}
	if m.Placement != mailbox.Junk || !strings.Contains(string(m.Header), "spf=pass") || strings.Contains(string(m.Header), "body") {
		t.Errorf("located %s, header %q", m.Placement, m.Header)
	}
	if unseen := status(t, s, "Spam"); unseen != 1 {
		t.Errorf("unseen after Locate = %d, want 1 (headers must be peeked)", unseen)
	}

	if err := s.MarkRead(ctx, "abc123@example.com"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := s.Rescue(ctx, "abc123@example.com"); err != nil {
		t.Fatalf("Rescue: %v", err)
	}
	m, err = s.Locate(ctx, "abc123@example.com")
	if err != nil || m == nil || m.Placement != mailbox.Inbox {
		t.Fatalf("after Rescue, Locate() = %+v, %v; want inbox", m, err)
	}
	if unseen := status(t, s, "INBOX"); unseen != 0 {
		t.Errorf("unseen in INBOX = %d, want 0", unseen)
	}

	if m, err := s.Locate(ctx, "missing@example.com"); m != nil || err != nil {
		t.Errorf("Locate(missing) = %v, %v; want nil, nil", m, err)
	}
	if _, err := s.Locate(ctx, "bad\r\nid@example.com"); err == nil {
		t.Error("Locate accepted a Message-ID with CRLF")
	}
}

func status(t *testing.T, s mailbox.Session, folder string) uint32 {
	t.Helper()
	data, err := s.(*mailboxSession).client.client.Status(folder, &imap.StatusOptions{NumUnseen: true}).Wait()
	if err != nil || data.NumUnseen == nil {
		t.Fatalf("STATUS %s: %v", folder, err)
	}
	return *data.NumUnseen
}
