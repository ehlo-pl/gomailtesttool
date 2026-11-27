// Package content turns a random Wikipedia article, RFC or poem (the same
// sources as "gomailtest ... sendmail --body-source") into an ordinary
// looking mail.
package content

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand/v2"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/wikicontent"
)

// Item is the content of one message.
type Item struct {
	Key         string // "<source>:<id>", used to avoid repeats
	Source      string
	Subject     string
	Text        string // excerpt
	Attribution string
}

// Source supplies content not named in used (a set of Item keys).
type Source interface {
	Next(ctx context.Context, sources, categories []string, used map[string]bool, rng *rand.Rand) (Item, error)
}

// Web fetches content from the Internet through package wikicontent.
type Web struct {
	// NewFetcher may be replaced in tests.
	NewFetcher func(categories []string) *wikicontent.Fetcher
}

// Excerpt size bounds in bytes.
const (
	minExcerpt = 2 << 10
	maxExcerpt = 6 << 10
)

// Next implements Source. The sources are tried in random order. A fetch
// that fails is an error: there is no static fallback, because identical
// placeholder messages would harm the reputation being built.
func (w Web) Next(ctx context.Context, sources, categories []string, used map[string]bool, rng *rand.Rand) (Item, error) {
	newFetcher := w.NewFetcher
	if newFetcher == nil {
		newFetcher = wikicontent.NewFetcher
	}
	f := newFetcher(categories)
	var errs []error
	for _, i := range rng.Perm(len(sources)) {
		item, err := fetch(ctx, f, sources[i], used, minExcerpt+rng.IntN(maxExcerpt-minExcerpt))
		if err == nil {
			return item, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", sources[i], err))
	}
	return Item{}, errors.Join(errs...)
}

func fetch(ctx context.Context, f *wikicontent.Fetcher, source string, used map[string]bool, size int) (Item, error) {
	ctx, cancel := context.WithTimeout(ctx, wikicontent.DefaultTimeout)
	defer cancel()

	names := map[string]bool{}
	numbers := map[int]bool{}
	for key := range used {
		src, id, _ := strings.Cut(key, ":")
		if src != source {
			continue
		}
		names[id] = true
		if n, err := strconv.Atoi(id); err == nil {
			numbers[n] = true
		}
	}

	switch source {
	case wikicontent.BodySourceWikipedia:
		a, err := f.Random(ctx, names)
		if err != nil {
			return Item{}, err
		}
		if a.Reused {
			return Item{}, errors.New("every article of the configured categories was already sent")
		}
		return Item{Source: source, Key: source + ":" + a.Title, Subject: a.Title, Text: excerpt(a.Text, size),
			Attribution: "From the Wikipedia article \"" + a.Title + "\" (" + a.URL + "), licensed under CC BY-SA 4.0."}, nil
	case wikicontent.BodySourceRFC:
		doc, err := f.RandomRFC(ctx, numbers)
		if err != nil {
			return Item{}, err
		}
		if doc.Reused {
			return Item{}, errors.New("every RFC was already sent")
		}
		text := doc.Text
		if i := strings.Index(text, "\nAbstract\n"); i >= 0 {
			text = text[i+1:]
		}
		return Item{Source: source, Key: source + ":" + strconv.Itoa(doc.Number), Subject: doc.Title, Text: excerpt(text, size),
			Attribution: fmt.Sprintf("Excerpt of RFC %d (%s), subject to the copyright notice in the document (BCP 78).", doc.Number, doc.URL)}, nil
	case wikicontent.BodySourcePoetry:
		p, err := f.RandomPoem(ctx, names)
		if err != nil {
			return Item{}, err
		}
		if p.Reused {
			return Item{}, errors.New("every offered poem was already sent")
		}
		return Item{Source: source, Key: source + ":" + p.Name(), Subject: p.Name(), Text: excerpt(p.Text, size),
			Attribution: "Poetry data from PoetryDB (" + f.PoetryURL + "), licensed under CC BY 4.0."}, nil
	}
	return Item{}, fmt.Errorf("unknown content source %q", source)
}

// excerpt returns whole lines of text up to about limit bytes.
func excerpt(text string, limit int) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if len(text) <= limit {
		return text
	}
	cut := strings.LastIndexByte(text[:limit], '\n')
	if cut < limit/2 {
		cut = strings.LastIndexByte(text[:limit], ' ')
	}
	if cut <= 0 {
		cut = limit
	}
	return strings.TrimSpace(strings.ToValidUTF8(text[:cut], ""))
}

// Message is an item addressed to one recipient.
type Message struct {
	From      string // formatted address, may carry a display name
	To        string
	MessageID string // without angle brackets
	Date      time.Time
	Item      Item
}

// Build assembles a multipart/alternative message with quoted-printable
// parts, so long paragraphs stay within SMTP line limits. It adds nothing
// that marks the message as automated.
func Build(m Message) ([]byte, error) {
	for _, v := range []string{m.From, m.To, m.MessageID} {
		if v == "" || strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("invalid header value %q", v)
		}
	}
	text := m.Item.Text + "\n\n-- \n" + m.Item.Attribution + "\n"

	var paragraphs strings.Builder
	for _, p := range strings.Split(m.Item.Text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			paragraphs.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(p), "\n", "<br>\n") + "</p>\n")
		}
	}
	htmlBody := "<!DOCTYPE html>\n<html>\n<head><meta charset=\"utf-8\"><title>" + html.EscapeString(m.Item.Subject) +
		"</title></head>\n<body>\n" + paragraphs.String() + "<hr>\n<p><small>" + html.EscapeString(m.Item.Attribution) +
		"</small></p>\n</body>\n</html>\n"

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, part := range []struct{ kind, text string }{{"text/plain", text}, {"text/html", htmlBody}} {
		w, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {part.kind + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		if err := writeQP(w, part.text); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	subject := strings.Join(strings.Fields(m.Item.Subject), " ")
	var out bytes.Buffer
	fmt.Fprintf(&out, "Message-ID: <%s>\r\n", m.MessageID)
	fmt.Fprintf(&out, "Date: %s\r\n", m.Date.Format(time.RFC1123Z))
	fmt.Fprintf(&out, "From: %s\r\n", m.From)
	fmt.Fprintf(&out, "To: %s\r\n", m.To)
	fmt.Fprintf(&out, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	out.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&out, "Content-Type: multipart/alternative; boundary=%s\r\n\r\n", mw.Boundary())
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

func writeQP(w io.Writer, s string) error {
	qp := quotedprintable.NewWriter(w) // turns "\n" into CRLF
	if _, err := io.WriteString(qp, s); err != nil {
		return err
	}
	return qp.Close()
}
