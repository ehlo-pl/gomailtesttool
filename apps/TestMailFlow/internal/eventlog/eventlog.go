// Package eventlog holds testmailflow's event IDs on top of the shared
// system event log.
package eventlog

import common "github.com/ehlo-pl/gomailtesttool/internal/common/eventlog"

// Event IDs.
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

// Shared types.
type (
	Level = common.Level
	Sink  = common.Sink
	Nop   = common.Nop
)

// Severities.
const (
	Info    = common.Info
	Warning = common.Warning
	Error   = common.Error
)

// Event log access.
var (
	Open    = common.Open
	Install = common.Install
	Remove  = common.Remove
)
