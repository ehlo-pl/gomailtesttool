//go:build windows

package eventlog

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/svc/eventlog"
)

type winSink struct{ log *eventlog.Log }

// Open returns a sink writing to the Windows Application log under source.
// Writing works without registration, but Event Viewer then shows a
// "description cannot be found" preamble; Install registers the source.
func Open(source string) (Sink, error) {
	l, err := eventlog.Open(source)
	if err != nil {
		return Nop{}, fmt.Errorf("open event log source %s: %w", source, err)
	}
	return &winSink{log: l}, nil
}

func (s *winSink) Report(level Level, id uint32, msg string) {
	switch level {
	case Error:
		_ = s.log.Error(id, msg)
	case Warning:
		_ = s.log.Warning(id, msg)
	default:
		_ = s.log.Info(id, msg)
	}
}

func (s *winSink) Close() error { return s.log.Close() }

// Install registers the event source (requires administrator rights).
// An already registered source is not an error.
func Install(source string) error {
	err := eventlog.InstallAsEventCreate(source, eventlog.Error|eventlog.Warning|eventlog.Info)
	if err != nil && !isExists(err) {
		return fmt.Errorf("register event source %s: %w", source, err)
	}
	return nil
}

// Remove unregisters the event source.
func Remove(source string) error { return eventlog.Remove(source) }

// isExists recognizes x/sys's plain "... registry key already exists" error.
func isExists(err error) bool {
	return strings.Contains(err.Error(), "already exists")
}
