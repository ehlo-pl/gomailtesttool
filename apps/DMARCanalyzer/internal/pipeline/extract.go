// Package pipeline wires extraction, parsing and storage together.
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/dmarc"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/extract"
	"github.com/ehlo-pl/gomailtesttool/apps/DMARCanalyzer/internal/store"
)

// ExtractStats counts extraction outcomes.
type ExtractStats struct {
	Files      int // files examined
	Unchanged  int // skipped: already processed with the same content
	Reports    int // new reports stored
	Duplicates int // reports already in the database
	NoReport   int // files without any DMARC aggregate report
	Forensic   int // forensic (ruf) reports skipped
	Errors     int
	ErrorList  []string
}

// Extractor processes input files into the store.
type Extractor struct {
	Store     *store.Store
	Limits    extract.Limits
	Reprocess bool
	Log       *slog.Logger
}

// supported reports whether a path looks like an input file.
func supported(path string) bool {
	l := strings.ToLower(path)
	for _, ext := range []string{".eml", ".xml", ".gz", ".gzip", ".zip"} {
		if strings.HasSuffix(l, ext) {
			return true
		}
	}
	return false
}

// Paths processes files and directories (recursively).
func (e *Extractor) Paths(ctx context.Context, paths []string) (*ExtractStats, error) {
	st := &ExtractStats{}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return st, fmt.Errorf("input %s: %w", p, err)
		}
		if !info.IsDir() {
			if err := e.processPath(ctx, p, st); err != nil {
				return st, err
			}
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !supported(path) {
				return nil
			}
			return e.processPath(ctx, path, st)
		})
		if err != nil {
			return st, err
		}
	}
	return st, nil
}

func (e *Extractor) processPath(ctx context.Context, path string, st *ExtractStats) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		st.Errors++
		st.ErrorList = append(st.ErrorList, fmt.Sprintf("%s: %v", path, err))
		return nil
	}
	_, err = e.File(ctx, path, data, st)
	return err
}

// File processes one input (an .eml message or a loose report file) and
// returns the stored status. Only database errors are returned as errors;
// content problems are recorded in st and in processed_files.
func (e *Extractor) File(ctx context.Context, path string, data []byte, st *ExtractStats) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	st.Files++

	if !e.Reprocess {
		prev, err := e.Store.FileSHA(ctx, abs)
		if err != nil {
			return "", err
		}
		if prev == sha {
			st.Unchanged++
			return "unchanged", nil
		}
	}

	pf := store.ProcessedFile{Path: abs, SHA256: sha}
	var payloads []extract.Payload
	forensic := false
	var problems []string

	if strings.HasSuffix(strings.ToLower(path), ".eml") {
		res, err := extract.FromEML(data, e.Limits)
		if err != nil {
			problems = append(problems, err.Error())
		} else {
			payloads, forensic = res.Payloads, res.Forensic
			problems = append(problems, res.Warnings...)
		}
	} else {
		payloads, err = extract.Unpack(filepath.Base(path), data, e.Limits)
		if err != nil {
			problems = append(problems, err.Error())
		}
	}

	inserted, dups := 0, 0
	for _, p := range payloads {
		f, err := dmarc.Parse(p.Data)
		if errors.Is(err, dmarc.ErrNotDMARC) {
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: %v", p.Name, err))
			continue
		}
		xs := sha256.Sum256(p.Data)
		ok, err := e.Store.InsertReport(ctx, f, hex.EncodeToString(xs[:]), abs)
		if err != nil {
			return "", err
		}
		if ok {
			inserted++
			e.debug("report stored", "org", f.Metadata.OrgName, "report_id", f.Metadata.ReportID,
				"domain", f.PolicyPublished.Domain, "records", len(f.Records))
		} else {
			dups++
		}
		for _, w := range f.Warnings {
			e.debug("report warning", "report_id", f.Metadata.ReportID, "warning", w)
		}
	}
	st.Reports += inserted
	st.Duplicates += dups
	pf.ReportsFound = inserted + dups

	switch {
	case pf.ReportsFound > 0 && inserted == 0:
		pf.Status = store.StatusDuplicate
	case pf.ReportsFound > 0:
		pf.Status = store.StatusOK
	case len(problems) > 0:
		pf.Status = store.StatusError
	case forensic:
		pf.Status = store.StatusForensic
		st.Forensic++
	default:
		pf.Status = store.StatusNoReport
		st.NoReport++
	}
	if len(problems) > 0 {
		pf.Error = strings.Join(problems, "; ")
		if pf.Status == store.StatusError {
			st.Errors++
			st.ErrorList = append(st.ErrorList, fmt.Sprintf("%s: %s", path, pf.Error))
		}
		if e.Log != nil {
			e.Log.Warn("extraction problem", "file", path, "error", pf.Error)
		}
	}
	if err := e.Store.MarkFile(ctx, pf); err != nil {
		return "", err
	}
	return pf.Status, nil
}

func (e *Extractor) debug(msg string, args ...any) {
	if e.Log != nil {
		e.Log.Debug(msg, args...)
	}
}
