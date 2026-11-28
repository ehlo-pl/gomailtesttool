// Package eventlog reports application state changes to the operating
// system's event log (Windows Event Log; elsewhere a no-op, the same
// information is always written to the slog log).
//
// Applications define their own event IDs. Keep them within 1..1000 because
// the event source is registered with the generic EventCreate message file.
package eventlog

// Level is the event severity.
type Level int

// Severities.
const (
	Info Level = iota
	Warning
	Error
)

// Sink receives events.
type Sink interface {
	Report(level Level, id uint32, msg string)
	Close() error
}

// Nop discards events.
type Nop struct{}

// Report implements Sink.
func (Nop) Report(Level, uint32, string) {}

// Close implements Sink.
func (Nop) Close() error { return nil }
