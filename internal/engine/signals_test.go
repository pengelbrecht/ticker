package engine

import (
	"testing"
)

func TestSignal_String(t *testing.T) {
	tests := []struct {
		signal Signal
		want   string
	}{
		{SignalNone, "NONE"},
		{SignalComplete, "COMPLETE"},
		{SignalEject, "EJECT"},
		{SignalBlocked, "BLOCKED"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.signal.String(); got != tt.want {
				t.Errorf("Signal.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseSignals(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantSignal Signal
		wantReason string
	}{
		{
			name:       "no signal in empty output",
			output:     "",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "no signal in regular output",
			output:     "I've completed the task. The tests pass.",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "complete signal",
			output:     "All tasks are done. <promise>COMPLETE</promise>",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "complete signal at start",
			output:     "<promise>COMPLETE</promise> and here is some trailing text",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "complete signal in middle of text",
			output:     "Some text before <promise>COMPLETE</promise> and after",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "eject signal with reason",
			output:     "This requires a large install. <promise>EJECT: Need to install Docker (>5GB)</promise>",
			wantSignal: SignalEject,
			wantReason: "Need to install Docker (>5GB)",
		},
		{
			name:       "eject signal with simple reason",
			output:     "<promise>EJECT: Large SDK required</promise>",
			wantSignal: SignalEject,
			wantReason: "Large SDK required",
		},
		{
			name:       "eject signal with spaces after colon",
			output:     "<promise>EJECT:   Multiple spaces</promise>",
			wantSignal: SignalEject,
			wantReason: "Multiple spaces",
		},
		{
			name:       "blocked signal with reason",
			output:     "Cannot proceed. <promise>BLOCKED: Missing API key for external service</promise>",
			wantSignal: SignalBlocked,
			wantReason: "Missing API key for external service",
		},
		{
			name:       "blocked signal with simple reason",
			output:     "<promise>BLOCKED: Unclear requirements</promise>",
			wantSignal: SignalBlocked,
			wantReason: "Unclear requirements",
		},
		{
			name:       "blocked signal with spaces after colon",
			output:     "<promise>BLOCKED:   Need clarification</promise>",
			wantSignal: SignalBlocked,
			wantReason: "Need clarification",
		},
		{
			name:       "complete takes priority over eject",
			output:     "<promise>COMPLETE</promise> <promise>EJECT: some reason</promise>",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "complete takes priority over blocked",
			output:     "<promise>BLOCKED: reason</promise> <promise>COMPLETE</promise>",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "eject takes priority over blocked",
			output:     "<promise>BLOCKED: reason1</promise> <promise>EJECT: reason2</promise>",
			wantSignal: SignalEject,
			wantReason: "reason2",
		},
		{
			name:       "multiline output with complete",
			output:     "Line 1\nLine 2\n<promise>COMPLETE</promise>\nLine 4",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "multiline output with eject",
			output:     "Working...\nFound issue\n<promise>EJECT: Need Xcode</promise>\nDone",
			wantSignal: SignalEject,
			wantReason: "Need Xcode",
		},
		{
			name:       "partial promise tag is not a signal",
			output:     "<promise>COMPLETE",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "incomplete eject is not a signal",
			output:     "<promise>EJECT: reason",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "wrong case is not a signal",
			output:     "<promise>complete</promise>",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "promise in text is not signal",
			output:     "I promise to COMPLETE this task",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "eject with colon in reason",
			output:     "<promise>EJECT: Error: disk full</promise>",
			wantSignal: SignalEject,
			wantReason: "Error: disk full",
		},
		{
			name:       "blocked with special characters",
			output:     "<promise>BLOCKED: Need credentials for https://api.example.com</promise>",
			wantSignal: SignalBlocked,
			wantReason: "Need credentials for https://api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSignal, gotReason := ParseSignals(tt.output)
			if gotSignal != tt.wantSignal {
				t.Errorf("ParseSignals() signal = %v, want %v", gotSignal, tt.wantSignal)
			}
			if gotReason != tt.wantReason {
				t.Errorf("ParseSignals() reason = %q, want %q", gotReason, tt.wantReason)
			}
		})
	}
}
