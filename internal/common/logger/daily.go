package logger

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DailyWriter appends to <dir>/<prefix>-YYYY-MM-DD.log, switching files at
// midnight UTC. Services without a console log through it.
type DailyWriter struct {
	dir, prefix string

	mu   sync.Mutex
	day  string
	file *os.File
}

// NewDailyWriter returns a writer; the directory and files are created on the
// first write.
func NewDailyWriter(dir, prefix string) *DailyWriter {
	return &DailyWriter{dir: dir, prefix: prefix}
}

// Write implements io.Writer.
func (w *DailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := time.Now().UTC().Format("2006-01-02")
	if w.file == nil || day != w.day {
		if w.file != nil {
			_ = w.file.Close()
			w.file = nil
		}
		if err := os.MkdirAll(w.dir, 0o700); err != nil {
			return 0, err
		}
		f, err := os.OpenFile(filepath.Join(w.dir, w.prefix+"-"+day+".log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return 0, err
		}
		w.file, w.day = f, day
	}
	return w.file.Write(p)
}

// Close closes the current file.
func (w *DailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
