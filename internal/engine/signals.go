package engine

import (
	"regexp"
)

// Signal represents a Ralph control signal emitted by an agent.
type Signal int

const (
	// SignalNone indicates no signal was detected in the output.
	SignalNone Signal = iota

	// SignalComplete indicates the epic is complete (all tasks done).
	SignalComplete

	// SignalEject indicates the agent needs to exit for a large install or similar.
	SignalEject

	// SignalBlocked indicates the agent is blocked (missing credentials, unclear requirements, etc).
	SignalBlocked
)

// String returns the string representation of the signal.
func (s Signal) String() string {
	switch s {
	case SignalComplete:
		return "COMPLETE"
	case SignalEject:
		return "EJECT"
	case SignalBlocked:
		return "BLOCKED"
	default:
		return "NONE"
	}
}

// Regex patterns for detecting Ralph signals in agent output.
// Signals are enclosed in <promise>...</promise> tags.
var (
	// completePattern matches <promise>COMPLETE</promise>
	completePattern = regexp.MustCompile(`<promise>COMPLETE</promise>`)

	// ejectPattern matches <promise>EJECT: reason</promise>
	// Captures the reason text after the colon.
	ejectPattern = regexp.MustCompile(`<promise>EJECT:\s*(.+?)</promise>`)

	// blockedPattern matches <promise>BLOCKED: reason</promise>
	// Captures the reason text after the colon.
	blockedPattern = regexp.MustCompile(`<promise>BLOCKED:\s*(.+?)</promise>`)
)

// ParseSignals scans the agent output for Ralph control signals.
// It returns the detected signal and any associated reason text.
// If multiple signals are present, the first one encountered is returned.
// Signal priority: Complete > Eject > Blocked (checked in this order).
func ParseSignals(output string) (Signal, string) {
	// Check for COMPLETE signal first
	if completePattern.MatchString(output) {
		return SignalComplete, ""
	}

	// Check for EJECT signal with reason
	if matches := ejectPattern.FindStringSubmatch(output); len(matches) > 1 {
		return SignalEject, matches[1]
	}

	// Check for BLOCKED signal with reason
	if matches := blockedPattern.FindStringSubmatch(output); len(matches) > 1 {
		return SignalBlocked, matches[1]
	}

	return SignalNone, ""
}
