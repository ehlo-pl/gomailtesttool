package wikicontent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

const (
	// DefaultRFCURL is the RFC Editor site, the IETF's RFC archive.
	DefaultRFCURL = "https://www.rfc-editor.org"
	// RFCSubjectPrefix starts the subject of a message carrying an RFC.
	RFCSubjectPrefix = "RFC "

	// maxTextBytes caps the document text put in a message. The body is sent as
	// plain text and as HTML, and APIs such as Graph limit the request size.
	maxTextBytes = 1 << 20
	// maxRFCAttempts bounds how many RFCs are tried when documents are missing.
	maxRFCAttempts = 5
)

var (
	// rfcEntryStart matches the first line of an rfc-index.txt entry.
	rfcEntryStart = regexp.MustCompile(`^(\d+) `)
	// rfcFormats matches the format list of an index entry.
	rfcFormats = regexp.MustCompile(`\(Format: ([^)]*)\)`)
)

// RFC is one document fetched from the RFC archive.
type RFC struct {
	Number int
	Title  string
	URL    string
	Text   string // plain text, cut at maxTextBytes
	Reused bool   // every RFC had been used before; this one repeats
}

// RFCSubjectFor is the subject used for an RFC when no subject was given.
// History parses the number back.
func RFCSubjectFor(number int, title string) string {
	return fmt.Sprintf("%s%d: %s", RFCSubjectPrefix, number, title)
}

// RandomRFC picks a random RFC that has a plain-text version and is not in
// used. When all of them are in used, one is repeated and marked Reused.
func (f *Fetcher) RandomRFC(ctx context.Context, used map[int]bool) (*RFC, error) {
	index, err := f.getBytes(ctx, f.RFCURL+"/rfc-index.txt", "text/plain")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the RFC index: %w", err)
	}
	all := parseRFCIndex(string(index))
	if len(all) == 0 {
		return nil, errors.New("the RFC index lists no plain-text RFCs")
	}
	candidates := slices.DeleteFunc(slices.Clone(all), func(n int) bool { return used[n] })
	reused := len(candidates) == 0
	if reused {
		candidates = all
	}
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	var lastErr error
	for _, n := range candidates[:min(len(candidates), maxRFCAttempts)] {
		doc, err := f.rfc(ctx, n)
		if err == nil {
			doc.Reused = reused
			return doc, nil
		}
		var status *statusError
		if !errors.As(err, &status) || status.code != http.StatusNotFound {
			return nil, err
		}
		lastErr = err
	}
	return nil, lastErr
}

// parseRFCIndex returns the numbers of the RFCs in rfc-index.txt that are
// available as plain text. Entries start in the first column and continue on
// indented lines; the numbers of "Not Issued." entries have no format list.
func parseRFCIndex(index string) []int {
	var numbers []int
	var number int
	var entry strings.Builder
	flush := func() {
		if m := rfcFormats.FindStringSubmatch(entry.String()); m != nil && number > 0 {
			for _, format := range strings.Split(m[1], ",") {
				switch strings.TrimSpace(format) {
				case "TXT", "ASCII", "TEXT":
					numbers = append(numbers, number)
					number = 0
					return
				}
			}
		}
		number = 0
	}
	for _, line := range strings.Split(index, "\n") {
		if m := rfcEntryStart.FindStringSubmatch(line); m != nil {
			flush()
			number, _ = strconv.Atoi(m[1])
			entry.Reset()
		}
		// Joined with single spaces: "(Format:" may wrap onto the next line.
		entry.WriteString(strings.TrimSpace(line))
		entry.WriteByte(' ')
	}
	flush()
	return numbers
}

// rfc fetches the title and text of one RFC.
func (f *Fetcher) rfc(ctx context.Context, number int) (*RFC, error) {
	base := fmt.Sprintf("%s/rfc/rfc%d", f.RFCURL, number)

	data, err := f.getBytes(ctx, base+".json", "application/json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RFC %d metadata: %w", number, err)
	}
	var meta struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("invalid metadata for RFC %d: %w", number, err)
	}
	title := strings.Join(strings.Fields(meta.Title), " ")

	data, err = f.getBytes(ctx, base+".txt", "text/plain")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch RFC %d: %w", number, err)
	}
	text := cleanText(string(data))
	if title == "" || text == "" {
		return nil, fmt.Errorf("RFC %d has no title or text", number)
	}
	return &RFC{Number: number, Title: title, URL: base + ".txt", Text: truncateText(text, maxTextBytes, base+".txt")}, nil
}

// cleanText drops the byte order mark, form feeds and other control
// characters (not allowed in the XML that EWS sends) and normalizes line
// endings.
func cleanText(text string) string {
	text = strings.ToValidUTF8(text, "?")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r == '\uFEFF' || unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
	return strings.Trim(text, "\n")
}

// truncateText cuts text after the last full line within limit bytes and says
// where the rest is.
func truncateText(text string, limit int, url string) string {
	if len(text) <= limit {
		return text
	}
	cut := strings.LastIndexByte(text[:limit], '\n')
	if cut < 0 {
		cut = limit
	}
	return strings.ToValidUTF8(text[:cut], "") + "\n\n[Truncated. Full text: " + url + "]"
}

// fillRFC is Fill for BodySourceRFC.
func fillRFC(ctx context.Context, f *Fetcher, msg Message, defaultSubject string, history History, slogLogger *slog.Logger) {
	used := history.UsedRFCs()
	logger.LogDebug(slogLogger, "RFCs already used in previous runs", "count", len(used), "log_pattern", history.pattern())

	doc, err := f.RandomRFC(ctx, used)
	if err != nil {
		logger.LogWarn(slogLogger, "Could not fetch an RFC; using the static body", "error", err)
		return
	}
	if doc.Reused {
		logger.LogWarn(slogLogger, "All RFCs were already used; repeating one", "used", len(used))
	}
	logger.LogInfo(slogLogger, "Using RFC as message body", "number", doc.Number, "title", doc.Title, "url", doc.URL)
	applyRFC(doc, msg, defaultSubject)
}

// applyRFC writes doc into msg, keeping the original body as the first line.
func applyRFC(doc *RFC, msg Message, defaultSubject string) {
	if *msg.Subject == defaultSubject {
		*msg.Subject = RFCSubjectFor(doc.Number, doc.Title)
	}
	const notice = "Text from the RFC Editor archive, subject to the copyright notice in the document (BCP 78)."
	intro := *msg.Body
	*msg.Body = fmt.Sprintf("%s\n\nRandom RFC %d: %s\n%s\n%s\n\n%s", intro, doc.Number, doc.Title, doc.URL, notice, doc.Text)
	*msg.BodyHTML = fmt.Sprintf(
		"<p>%s</p>\n<p>Random RFC %d: <a href=\"%s\">%s</a><br>%s</p>\n<hr>\n<pre>%s</pre>",
		html.EscapeString(intro), doc.Number, html.EscapeString(doc.URL), html.EscapeString(doc.Title),
		notice, html.EscapeString(doc.Text))
}
