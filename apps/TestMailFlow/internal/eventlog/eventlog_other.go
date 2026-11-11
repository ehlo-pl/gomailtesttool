//go:build !windows

package eventlog

// Open returns a no-op sink: outside Windows, events are only written to the log.
func Open(string) (Sink, error) { return Nop{}, nil }

// Install is a no-op outside Windows.
func Install(string) error { return nil }

// Remove is a no-op outside Windows.
func Remove(string) error { return nil }
