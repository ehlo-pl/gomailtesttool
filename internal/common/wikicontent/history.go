package wikicontent

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// History locates the default result logs of one tool action, written by
// internal/common/logger as <Dir>/_<Tool>_<Action>_<date>.csv or .jsonl.
// Every logged sendmail row records the subject, and a subject left at its
// default carries the article title (see SubjectFor) or RFC number (see
// RFCSubjectFor) or poem (PoemSubjectFor), so the logs tell which were already sent.
type History struct {
	Dir    string // directory with the logs; "" means os.TempDir()
	Tool   string // tool name passed to the logger, e.g. "jmaptool"
	Action string // action, e.g. "sendmail"
}

func (h History) pattern() string {
	dir := h.Dir
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "_"+h.Tool+"_"+h.Action+"_*")
}

// UsedTitles returns the article titles found in subjects of the form
// SubjectFor(title) ("Wikipedia: <title>") in all CSV and JSONL log files of the
// action, across all dates. Unreadable files are skipped: the history is a
// best-effort hint, never a reason to fail a send.
func (h History) UsedTitles() map[string]bool {
	return h.usedAfter(SubjectFor(""))
}

// UsedPoems is UsedTitles for the poem names in subjects of the form
// PoemSubjectFor(name) ("Poem: <title> by <author>").
func (h History) UsedPoems() map[string]bool {
	return h.usedAfter(PoemSubjectFor(""))
}

// usedAfter returns what follows prefix in the logged values starting with it.
func (h History) usedAfter(prefix string) map[string]bool {
	used := map[string]bool{}
	h.eachValue(func(value string) {
		if title, ok := strings.CutPrefix(value, prefix); ok && title != "" {
			used[title] = true
		}
	})
	return used
}

// UsedRFCs is UsedTitles for the RFC numbers in subjects of the form
// RFCSubjectFor(number, title) ("RFC <number>: <title>").
func (h History) UsedRFCs() map[int]bool {
	used := map[int]bool{}
	h.eachValue(func(value string) {
		rest, ok := strings.CutPrefix(value, RFCSubjectPrefix)
		if !ok {
			return
		}
		digits, _, found := strings.Cut(rest, ": ")
		if n, err := strconv.Atoi(digits); found && err == nil {
			used[n] = true
		}
	})
	return used
}

// eachValue calls add for every value in the action's log files.
func (h History) eachValue(add func(string)) {
	if h.Tool == "" || h.Action == "" {
		return
	}
	files, _ := filepath.Glob(h.pattern())
	for _, path := range files {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csv":
			readCSVValues(path, add)
		case ".jsonl":
			readJSONLValues(path, add)
		}
	}
}

func readCSVValues(path string, add func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // header and rows may differ after a format change
	r.LazyQuotes = true
	for {
		record, err := r.Read()
		if err == io.EOF {
			return
		}
		if err != nil {
			if _, ok := err.(*csv.ParseError); ok {
				continue
			}
			return
		}
		for _, v := range record {
			add(v)
		}
	}
}

func readJSONLValues(path string, add func(string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		for _, v := range row {
			if s, ok := v.(string); ok {
				add(s)
			}
		}
	}
}
