// Package wikicontent fills an otherwise empty test message with a random
// featured Wikipedia article, so sendmail tests carry realistic, varied
// content instead of a one-line placeholder. It can use a random RFC from the
// RFC Editor archive or a poem from PoetryDB instead.
//
// Articles are picked from a Wikipedia category through the MediaWiki API.
// Fetching is best effort: callers fall back to their static body when it
// fails, so a missing Internet connection never breaks a send.
package wikicontent

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/ehlo-pl/gomailtesttool/internal/common/logger"
	"github.com/ehlo-pl/gomailtesttool/internal/common/version"
)

// Body sources selectable with the sendmail --body-source flag.
const (
	// BodySourceWikipedia fills a message without any body with a random
	// featured Wikipedia article (opt-in with --body-source wikipedia).
	BodySourceWikipedia = "wikipedia"
	// BodySourceRFC fills a message without any body with a random RFC from
	// the IETF's RFC archive (opt-in with --body-source rfc).
	BodySourceRFC = "rfc"
	// BodySourcePoetry fills a message without any body with a random public
	// domain poem from PoetryDB (opt-in with --body-source poetry).
	BodySourcePoetry = "poetry"
	// BodySourceStatic keeps the built-in one-line placeholder body (the default).
	BodySourceStatic = "static"
)

const (
	// DefaultAPIURL is the English Wikipedia MediaWiki API endpoint.
	DefaultAPIURL = "https://en.wikipedia.org/w/api.php"
	// DefaultTimeout bounds the whole fetch (category listings plus article).
	DefaultTimeout = 15 * time.Second

	maxCategoryPages = 10
	maxResponseBytes = 8 << 20
)

// DefaultCategories lists the candidate articles: featured (FA-Class)
// articles of technology and science WikiProjects. Their members are the
// articles' talk pages, which carry the quality assessment.
var DefaultCategories = []string{
	"Category:FA-Class Computer science articles",
	"Category:FA-Class Computing articles",
	"Category:FA-Class Technology articles",
	"Category:FA-Class Engineering articles",
	"Category:FA-Class Systems articles",
	"Category:FA-Class Internet articles",
	"Category:FA-Class Telecommunications articles",
	"Category:FA-Class Cryptography articles",
	"Category:FA-Class Robotics articles",
	"Category:FA-Class Statistics articles",
	"Category:FA-Class Chemistry articles",
	"Category:FA-Class Astronomy articles",
}

// ParseCategories splits a comma-separated --body-categories value. Names
// without a "Category:" prefix get one. An empty value yields nil, meaning
// DefaultCategories.
func ParseCategories(value string) []string {
	var cats []string
	for _, c := range strings.Split(value, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(c), "category:") {
			c = "Category:" + c
		}
		cats = append(cats, c)
	}
	return cats
}

// Article is one fetched Wikipedia article.
type Article struct {
	Title    string
	URL      string
	Text     string // plain-text extract
	HTML     string // HTML extract
	Category string // category the article was picked from
	Reused   bool   // every candidate had been used before; this one repeats
}

// Fetcher retrieves random articles from Wikipedia categories, random RFCs or random poems.
type Fetcher struct {
	APIURL     string
	RFCURL     string // base URL of the RFC archive
	PoetryURL  string // base URL of PoetryDB
	Categories []string
	Client     *http.Client
}

// NewFetcher returns a Fetcher for categories (DefaultCategories when empty)
// on English Wikipedia. It honours the HTTP(S)_PROXY environment variables.
func NewFetcher(categories []string) *Fetcher {
	if len(categories) == 0 {
		categories = DefaultCategories
	}
	return &Fetcher{
		APIURL:     DefaultAPIURL,
		RFCURL:     DefaultRFCURL,
		PoetryURL:  DefaultPoetryURL,
		Categories: categories,
		Client: &http.Client{
			Timeout:   DefaultTimeout,
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		},
	}
}

// ValidateBodySource checks a --body-source value.
func ValidateBodySource(source string) error {
	switch source {
	case BodySourceWikipedia, BodySourceRFC, BodySourcePoetry, BodySourceStatic:
		return nil
	}
	return fmt.Errorf("invalid body source %q: must be %q, %q, %q or %q", source, BodySourceStatic, BodySourceWikipedia, BodySourceRFC, BodySourcePoetry)
}

// NoBodyGiven reports whether a message was configured without any body:
// body still equals the protocol's default (so it was not set by flag, env
// var or config file), there is no HTML body, and no template was used.
func NoBodyGiven(body, defaultBody, bodyHTML, template string) bool {
	return body == defaultBody && bodyHTML == "" && template == ""
}

// Message is the part of a message configuration that Fill may rewrite.
type Message struct {
	Subject  *string
	Body     *string
	BodyHTML *string
}

// Fill replaces the message body with a random article when source is
// BodySourceWikipedia, or with a random RFC when it is BodySourceRFC (subject
// "RFC <number>: <title>", logged RFC numbers are skipped), or with a random
// poem when it is BodySourcePoetry (subject "Poem: <title> by <author>"). Articles already named in the action's default log
// files (see History) are skipped while unused ones remain. When the subject
// still equals defaultSubject (no subject was given) it becomes
// "Wikipedia: <title>"; that subject is what the log records, so it is what later runs skip. The
// original body is kept as the first line. Errors are logged as warnings and
// leave the message unchanged.
func Fill(ctx context.Context, f *Fetcher, source string, msg Message, defaultSubject string, history History, slogLogger *slog.Logger) {
	switch source {
	case BodySourceWikipedia, BodySourceRFC, BodySourcePoetry:
	default:
		return
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultTimeout)
	defer cancel()

	switch source {
	case BodySourceRFC:
		fillRFC(ctx, f, msg, defaultSubject, history, slogLogger)
		return
	case BodySourcePoetry:
		fillPoem(ctx, f, msg, defaultSubject, history, slogLogger)
		return
	}

	used := history.UsedTitles()
	logger.LogDebug(slogLogger, "Wikipedia articles already used in previous runs", "count", len(used), "log_pattern", history.pattern())

	article, err := f.Random(ctx, used)
	if err != nil {
		logger.LogWarn(slogLogger, "Could not fetch Wikipedia article; using the static body", "error", err)
		return
	}
	if article.Reused {
		logger.LogWarn(slogLogger, "All Wikipedia articles in the configured categories were already used; repeating one", "used", len(used))
	}
	logger.LogInfo(slogLogger, "Using Wikipedia article as message body", "title", article.Title, "url", article.URL, "category", article.Category)
	Apply(article, msg, defaultSubject)
}

// Apply writes article into msg. See Fill.
func Apply(article *Article, msg Message, defaultSubject string) {
	if *msg.Subject == defaultSubject {
		*msg.Subject = SubjectFor(article.Title)
	}
	intro := *msg.Body
	attribution := fmt.Sprintf("Random featured Wikipedia article: %s\n%s\nText from Wikipedia, licensed under CC BY-SA 4.0.", article.Title, article.URL)
	*msg.Body = intro + "\n\n" + attribution + "\n\n" + article.Text
	*msg.BodyHTML = fmt.Sprintf(
		"<p>%s</p>\n<p>Random featured Wikipedia article: <a href=\"%s\">%s</a><br>Text from Wikipedia, licensed under <a href=\"https://creativecommons.org/licenses/by-sa/4.0/\">CC BY-SA 4.0</a>.</p>\n<hr>\n<h1>%s</h1>\n%s",
		html.EscapeString(intro), html.EscapeString(article.URL), html.EscapeString(article.Title),
		html.EscapeString(article.Title), article.HTML)
}

// SubjectPrefix starts the subject of a message carrying a Wikipedia article.
const SubjectPrefix = "Wikipedia: "

// SubjectFor is the subject used for an article when no subject was given.
// History parses it back.
func SubjectFor(title string) string {
	return SubjectPrefix + title
}

// Random picks a random article that is not in used. Categories are tried in
// random order and listed one at a time, stopping at the first one with an
// unused article, so a typical call needs a single listing request. When
// every article of every category is in used, a random one is repeated and
// the result is marked Reused.
func (f *Fetcher) Random(ctx context.Context, used map[string]bool) (*Article, error) {
	categories := slices.Clone(f.Categories)
	rand.Shuffle(len(categories), func(i, j int) { categories[i], categories[j] = categories[j], categories[i] })

	type candidate struct{ title, category string }
	var all []candidate
	var lastErr error
	for _, category := range categories {
		titles, err := f.categoryTitles(ctx, category)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			lastErr = err
			continue
		}
		var fresh []string
		for _, t := range titles {
			all = append(all, candidate{t, category})
			if !used[t] {
				fresh = append(fresh, t)
			}
		}
		if len(fresh) > 0 {
			return f.article(ctx, fresh[rand.IntN(len(fresh))], category, false)
		}
	}
	if len(all) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("categories %s have no members", strings.Join(f.Categories, ", "))
	}
	c := all[rand.IntN(len(all))]
	return f.article(ctx, c.title, c.category, true)
}

// article fetches the plain-text and HTML extracts of one article.
func (f *Fetcher) article(ctx context.Context, title, category string, reused bool) (*Article, error) {
	textPage, err := f.extract(ctx, title, true)
	if err != nil {
		return nil, err
	}
	htmlPage, err := f.extract(ctx, title, false)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(textPage.Extract) == "" {
		return nil, fmt.Errorf("article %q has no text", title)
	}
	return &Article{
		Title:    textPage.Title,
		URL:      textPage.FullURL,
		Text:     textPage.Extract,
		HTML:     htmlPage.Extract,
		Category: category,
		Reused:   reused,
	}, nil
}

// categoryTitles lists the article titles in a category. Talk pages are
// mapped to their article ("Talk:Binary search" -> "Binary search").
func (f *Fetcher) categoryTitles(ctx context.Context, category string) ([]string, error) {
	var titles []string
	cont := ""
	for range maxCategoryPages {
		params := url.Values{
			"action":        {"query"},
			"list":          {"categorymembers"},
			"cmtitle":       {category},
			"cmlimit":       {"500"},
			"format":        {"json"},
			"formatversion": {"2"},
		}
		if cont != "" {
			params.Set("cmcontinue", cont)
		}
		var resp struct {
			Continue struct {
				CmContinue string `json:"cmcontinue"`
			} `json:"continue"`
			Query struct {
				CategoryMembers []struct {
					Ns    int    `json:"ns"`
					Title string `json:"title"`
				} `json:"categorymembers"`
			} `json:"query"`
		}
		if err := f.get(ctx, params, &resp); err != nil {
			return nil, fmt.Errorf("failed to list %q: %w", category, err)
		}
		for _, m := range resp.Query.CategoryMembers {
			switch m.Ns {
			case 0:
				titles = append(titles, m.Title)
			case 1:
				titles = append(titles, strings.TrimPrefix(m.Title, "Talk:"))
			}
		}
		if resp.Continue.CmContinue == "" {
			break
		}
		cont = resp.Continue.CmContinue
	}
	return titles, nil
}

type extractPage struct {
	Title   string `json:"title"`
	FullURL string `json:"fullurl"`
	Extract string `json:"extract"`
	Missing bool   `json:"missing"`
}

// extract fetches the article's full extract as plain text or HTML.
func (f *Fetcher) extract(ctx context.Context, title string, plain bool) (*extractPage, error) {
	params := url.Values{
		"action":        {"query"},
		"prop":          {"extracts|info"},
		"inprop":        {"url"},
		"titles":        {title},
		"redirects":     {"1"},
		"format":        {"json"},
		"formatversion": {"2"},
	}
	if plain {
		params.Set("explaintext", "1")
	}
	var resp struct {
		Query struct {
			Pages []extractPage `json:"pages"`
		} `json:"query"`
	}
	if err := f.get(ctx, params, &resp); err != nil {
		return nil, fmt.Errorf("failed to fetch article %q: %w", title, err)
	}
	if len(resp.Query.Pages) == 0 || resp.Query.Pages[0].Missing {
		return nil, fmt.Errorf("article %q not found", title)
	}
	return &resp.Query.Pages[0], nil
}

// get performs one MediaWiki API GET request and decodes the JSON response.
func (f *Fetcher) get(ctx context.Context, params url.Values, out any) error {
	body, err := f.getBytes(ctx, f.APIURL+"?"+params.Encode(), "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("invalid JSON response: %w", err)
	}
	return nil
}

// statusError is a response other than 200 OK.
type statusError struct {
	code int
	url  string
}

func (e *statusError) Error() string { return fmt.Sprintf("HTTP %d from %s", e.code, e.url) }

// getBytes performs one GET request and returns the response body.
func (f *Fetcher) getBytes(ctx context.Context, target, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	// Wikimedia's API etiquette asks for a descriptive User-Agent.
	req.Header.Set("User-Agent", "gomailtest/"+version.Get()+" (+https://github.com/ehlo-pl/gomailtesttool)")
	req.Header.Set("Accept", accept)

	client := f.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		base, _, _ := strings.Cut(target, "?")
		return nil, &statusError{code: resp.StatusCode, url: base}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
}
