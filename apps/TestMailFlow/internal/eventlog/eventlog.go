// Package eventlog reports testmailflow state changes to the operating
// system's event log (Windows Event Log; elsewhere a no-op, the same
// information is always written to the slog log).
package eventlog

// Event IDs. They stay within 1..1000 because the event source is
// registered with the generic EventCreate message file.
const (
	IDStarted          uint32 = 100
	IDStopped          uint32 = 101
	IDFlowRecovered    uint32 = 102
	IDCheckerRecovered uint32 = 103
	IDProbeLate        uint32 = 200
	IDProbeLost        uint32 = 300
	IDSendFailed       uint32 = 301
	IDCheckerFailing   uint32 = 302
)

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
