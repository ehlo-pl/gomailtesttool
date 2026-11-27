//go:build !integration

package smtp

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/ehlo-pl/gomailtesttool/internal/common/send"
)

// greetings runs a minimal server and returns every EHLO/HELO line it saw.
func greetings(t *testing.T, change func(*Config)) []string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	seen := make(chan []string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			seen <- nil
			return
		}
		defer func() { _ = conn.Close() }()
		var lines []string
		defer func() { seen <- lines }()
		r := bufio.NewReader(conn)
		reply := func(s string) { _, _ = io.WriteString(conn, s+"\r\n") }
		reply("220 test")
		data := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			verb := strings.ToUpper(line)
			switch {
			case data:
				if line == "." {
					data = false
					reply("250 queued")
				}
			case strings.HasPrefix(verb, "EHLO"), strings.HasPrefix(verb, "HELO"):
				lines = append(lines, line)
				reply("250-test")
				reply("250 AUTH PLAIN")
			case strings.HasPrefix(verb, "AUTH"):
				reply("235 ok")
			case verb == "DATA":
				data = true
				reply("354 go")
			case verb == "QUIT":
				reply("221 bye")
				return
			default:
				reply("250 ok")
			}
		}
	}()

	cfg := NewConfig()
	cfg.Host, cfg.Port, cfg.NoStartTLS = "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, true
	change(cfg)
	s, err := OpenSender(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), send.Message{From: "a@example.com", To: []string{"b@example.com"}, Raw: []byte("Subject: x\r\n\r\ny\r\n")}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	return <-seen
}

func TestSenderEHLOName(t *testing.T) {
	for name, tt := range map[string]struct {
		change func(*Config)
		last   string
	}{
		"anonymous":            {func(c *Config) {}, "EHLO " + ehloName},
		"anonymous configured": {func(c *Config) { c.EHLOName = "mail.example.com" }, "EHLO mail.example.com"},
		"authenticated configured": {func(c *Config) {
			c.EHLOName, c.Username, c.Password = "mail.example.com", "u", "p"
		}, "EHLO mail.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			got := greetings(t, tt.change)
			if len(got) == 0 || got[len(got)-1] != tt.last {
				t.Errorf("greetings = %q, want the last to be %q", got, tt.last)
			}
			for _, g := range got {
				if len(strings.Fields(g)) != 2 {
					t.Errorf("greeting without a name: %q", g)
				}
			}
		})
	}
}
