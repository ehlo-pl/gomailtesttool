package wikicontent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
)

const (
	// DefaultPoetryURL is PoetryDB, a database of public domain poetry.
	DefaultPoetryURL = "https://poetrydb.org"
	// PoemSubjectPrefix starts the subject of a message carrying a poem.
	PoemSubjectPrefix = "Poem: "

	// poemBatch is how many random poems one request asks for, so that one
	// not used yet is found without further requests.
	poemBatch = 10
)

// Poem is one poem fetched from PoetryDB.
type Poem struct {
	Title  string
	Author string
	Text   string // lines of the poem, cut at maxTextBytes
	Reused bool   // every poem offered had been used before; this one repeats
}

// Name identifies the poem in the subject, and so in the history.
func (p *Poem) Name() string { return p.Title + " by " + p.Author }

// PoemSubjectFor is the subject used for a poem when no subject was given.
// History parses the name back.
func PoemSubjectFor(name string) string {
	return PoemSubjectPrefix + name
}

// RandomPoem picks a random poem whose Name is not in used. It asks for one
// batch of random poems; when all of them are in used, the first one is
// repeated and marked Reused.
func (f *Fetcher) RandomPoem(ctx context.Context, used map[string]bool) (*Poem, error) {
	data, err := f.getBytes(ctx, fmt.Sprintf("%s/random/%d", f.PoetryURL, poemBatch), "application/json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch poems: %w", err)
	}
	// An error answer is an object, not an array, and fails here.
	var batch []struct {
		Title  string   `json:"title"`
		Author string   `json:"author"`
		Lines  []string `json:"lines"`
	}
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, fmt.Errorf("invalid poetry response: %w", err)
	}

	oneLine := func(s string) string { return strings.Join(strings.Fields(cleanText(s)), " ") }
	var repeat *Poem
	for _, p := range batch {
		poem := &Poem{Title: oneLine(p.Title), Author: oneLine(p.Author), Text: cleanText(strings.Join(p.Lines, "\n"))}
		if poem.Title == "" || poem.Author == "" || poem.Text == "" {
			continue
		}
		poem.Text = truncateText(poem.Text, maxTextBytes, f.PoetryURL)
		if !used[poem.Name()] {
			return poem, nil
		}
		if repeat == nil {
			repeat = poem
		}
	}
	if repeat == nil {
		return nil, errors.New("the poetry response has no poems")
	}
	repeat.Reused = true
	return repeat, nil
}

// fillPoem is Fill for BodySourcePoetry.
func fillPoem(ctx context.Context, f *Fetcher, msg Message, defaultSubject string, history History, slogLogger *slog.Logger) {
	used := history.UsedPoems()
	logger.LogDebug(slogLogger, "Poems already used in previous runs", "count", len(used), "log_pattern", history.pattern())

	poem, err := f.RandomPoem(ctx, used)
	if err != nil {
		logger.LogWarn(slogLogger, "Could not fetch a poem; using the static body", "error", err)
		return
	}
	if poem.Reused {
		logger.LogWarn(slogLogger, "All offered poems were already used; repeating one", "used", len(used))
	}
	logger.LogInfo(slogLogger, "Using poem as message body", "title", poem.Title, "author", poem.Author)
	applyPoem(poem, f.PoetryURL, msg, defaultSubject)
}

// applyPoem writes poem into msg, keeping the original body as the first line.
// source is the site credited, as PoetryDB's CC BY 4.0 data license asks.
func applyPoem(poem *Poem, source string, msg Message, defaultSubject string) {
	if *msg.Subject == defaultSubject {
		*msg.Subject = PoemSubjectFor(poem.Name())
	}
	intro := *msg.Body
	*msg.Body = fmt.Sprintf("%s\n\nRandom poem: %s\nPoetry data from PoetryDB (%s), licensed under CC BY 4.0.\n\n%s",
		intro, poem.Name(), source, poem.Text)
	*msg.BodyHTML = fmt.Sprintf(
		"<p>%s</p>\n<p>Random poem: %s<br>Poetry data from <a href=\"%s\">PoetryDB</a>, licensed under <a href=\"https://creativecommons.org/licenses/by/4.0/\">CC BY 4.0</a>.</p>\n<hr>\n<h1>%s</h1>\n<p><em>%s</em></p>\n<pre style=\"white-space:pre-wrap\">%s</pre>",
		html.EscapeString(intro), html.EscapeString(poem.Name()), html.EscapeString(source),
		html.EscapeString(poem.Title), html.EscapeString(poem.Author), html.EscapeString(poem.Text))
}
