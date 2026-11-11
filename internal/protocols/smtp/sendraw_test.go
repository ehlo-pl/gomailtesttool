//go:build !integration
// +build !integration

package smtp

import (
	"bufio"
	"context"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// fakeSMTP is a minimal plain-text SMTP server recording one transaction.
type fakeSMTP struct {
	ln net.Listener

	mu       sync.Mutex
	from     string
	rcpts    []string
	data     string
	rejectTo string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()
	return f
}

func (f *fakeSMTP) port() int { return f.ln.Addr().(*net.TCPAddr).Port }

func (f *fakeSMTP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	r := bufio.NewReader(conn)
	w := func(s string) { _, _ = conn.Write([]byte(s + "\r\n")) }
	w("220 fake.test ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			w("250-fake.test")
			w("250 8BITMIME")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			f.mu.Lock()
			f.from = strings.Trim(strings.Fields(line[len("MAIL FROM:"):])[0], "<>")
			f.mu.Unlock()
			w("250 2.1.0 Sender OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			addr := strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>")
			f.mu.Lock()
			reject := f.rejectTo != "" && addr == f.rejectTo
			if !reject {
				f.rcpts = append(f.rcpts, addr)
			}
			f.mu.Unlock()
			if reject {
				w("550 5.1.1 User unknown")
			} else {
				w("250 2.1.5 Recipient OK")
			}
		case upper == "DATA":
			w("354 Start mail input")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			f.mu.Lock()
			f.data = b.String()
			f.mu.Unlock()
			w("250 2.0.0 Queued as TEST123")
		case upper == "QUIT":
			w("221 Bye")
			return
		default:
			w("250 OK")
		}
	}
}

func TestOpenSender_SendsRawMessage(t *testing.T) {
	srv := newFakeSMTP(t)
	cfg := NewConfig()
	cfg.Host, cfg.Port, cfg.Timeout = "127.0.0.1", srv.port(), 5*time.Second

	s, err := OpenSender(cfg, nil)
	if err != nil {
		t.Fatalf("OpenSender() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	raw := []byte("Subject: [TMF] flow abc\r\nX-TestMailFlow-Id: abc\r\n\r\nbody\r\n")
	res, err := s.Send(context.Background(), send.Message{From: "a@example.com", To: []string{"b@example.net", "c@example.net"}, Raw: raw})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !strings.Contains(res.ServerResponse, "TEST123") {
		t.Errorf("ServerResponse = %q, want queue id", res.ServerResponse)
	}
	if res.RemoteHost != "127.0.0.1" || res.TLSVersion != "" {
		t.Errorf("RemoteHost/TLS = %q/%q, want 127.0.0.1 and no TLS", res.RemoteHost, res.TLSVersion)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.from != "a@example.com" {
		t.Errorf("MAIL FROM = %q", srv.from)
	}
	if strings.Join(srv.rcpts, ",") != "b@example.net,c@example.net" {
		t.Errorf("RCPT TO = %v", srv.rcpts)
	}
	if srv.data != string(raw) {
		t.Errorf("DATA = %q, want %q", srv.data, raw)
	}
}

func TestOpenSender_Errors(t *testing.T) {
	srv := newFakeSMTP(t)
	srv.rejectTo = "nobody@example.net"

	tests := []struct {
		name      string
		configure func(*Config)
		msg       send.Message
		openErr   string
		sendErr   string
	}{
		{
			name:      "missing host",
			configure: func(c *Config) { c.Host = "" },
			openErr:   "host is required",
		},
		{
			name:      "smtps with starttls",
			configure: func(c *Config) { c.SMTPS, c.StartTLS = true, true },
			openErr:   "mutually exclusive",
		},
		{
			name:    "no recipient",
			msg:     send.Message{From: "a@example.com", Raw: []byte("x")},
			sendErr: "at least one recipient",
		},
		{
			name:    "recipient rejected",
			msg:     send.Message{From: "a@example.com", To: []string{"nobody@example.net"}, Raw: []byte("Subject: x\r\n\r\nx\r\n")},
			sendErr: "RCPT TO failed",
		},
		{
			name:      "starttls required but not offered",
			configure: func(c *Config) { c.StartTLS = true },
			msg:       send.Message{From: "a@example.com", To: []string{"b@example.net"}, Raw: []byte("x")},
			sendErr:   "STARTTLS required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig()
			cfg.Host, cfg.Port, cfg.Timeout = "127.0.0.1", srv.port(), 5*time.Second
			if tt.configure != nil {
				tt.configure(cfg)
			}
			s, err := OpenSender(cfg, nil)
			if tt.openErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.openErr) {
					t.Fatalf("OpenSender() error = %v, want substring %q", err, tt.openErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("OpenSender() error = %v", err)
			}
			_, err = s.Send(context.Background(), tt.msg)
			if err == nil || !strings.Contains(err.Error(), tt.sendErr) {
				t.Fatalf("Send() error = %v, want substring %q", err, tt.sendErr)
			}
		})
	}
}

func TestWantsStartTLS(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"submission port", Config{Port: 587}, true},
		{"relay port", Config{Port: 25}, true},
		{"other port", Config{Port: 10025}, false},
		{"forced on other port", Config{Port: 10025, StartTLS: true}, true},
		{"disabled", Config{Port: 587, NoStartTLS: true}, false},
		{"smtps", Config{Port: 465, SMTPS: true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsStartTLS(&tt.cfg); got != tt.want {
				t.Errorf("wantsStartTLS() = %v, want %v", got, tt.want)
			}
		})
	}
}
