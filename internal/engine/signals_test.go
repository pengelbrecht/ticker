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
		{SignalApprovalNeeded, "APPROVAL_NEEDED"},
		{SignalInputNeeded, "INPUT_NEEDED"},
		{SignalReviewRequested, "REVIEW_REQUESTED"},
		{SignalContentReview, "CONTENT_REVIEW"},
		{SignalEscalate, "ESCALATE"},
		{SignalCheckpoint, "CHECKPOINT"},
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
		// New handoff signal types
		{
			name:       "approval needed signal with context",
			output:     "Ready for review. <promise>APPROVAL_NEEDED: Please approve the database migration</promise>",
			wantSignal: SignalApprovalNeeded,
			wantReason: "Please approve the database migration",
		},
		{
			name:       "approval needed signal without context",
			output:     "<promise>APPROVAL_NEEDED</promise>",
			wantSignal: SignalApprovalNeeded,
			wantReason: "",
		},
		{
			name:       "input needed signal with context",
			output:     "<promise>INPUT_NEEDED: What should the default timeout be?</promise>",
			wantSignal: SignalInputNeeded,
			wantReason: "What should the default timeout be?",
		},
		{
			name:       "input needed signal without context",
			output:     "<promise>INPUT_NEEDED</promise>",
			wantSignal: SignalInputNeeded,
			wantReason: "",
		},
		{
			name:       "review requested signal with context",
			output:     "<promise>REVIEW_REQUESTED: Please review the API changes before I continue</promise>",
			wantSignal: SignalReviewRequested,
			wantReason: "Please review the API changes before I continue",
		},
		{
			name:       "review requested signal without context",
			output:     "<promise>REVIEW_REQUESTED</promise>",
			wantSignal: SignalReviewRequested,
			wantReason: "",
		},
		{
			name:       "content review signal with context",
			output:     "<promise>CONTENT_REVIEW: Please verify the generated documentation is accurate</promise>",
			wantSignal: SignalContentReview,
			wantReason: "Please verify the generated documentation is accurate",
		},
		{
			name:       "content review signal without context",
			output:     "<promise>CONTENT_REVIEW</promise>",
			wantSignal: SignalContentReview,
			wantReason: "",
		},
		{
			name:       "escalate signal with context",
			output:     "<promise>ESCALATE: This security issue requires senior engineer review</promise>",
			wantSignal: SignalEscalate,
			wantReason: "This security issue requires senior engineer review",
		},
		{
			name:       "escalate signal without context",
			output:     "<promise>ESCALATE</promise>",
			wantSignal: SignalEscalate,
			wantReason: "",
		},
		{
			name:       "checkpoint signal with context",
			output:     "<promise>CHECKPOINT: Saving state before refactoring</promise>",
			wantSignal: SignalCheckpoint,
			wantReason: "Saving state before refactoring",
		},
		{
			name:       "checkpoint signal without context",
			output:     "<promise>CHECKPOINT</promise>",
			wantSignal: SignalCheckpoint,
			wantReason: "",
		},
		// Priority tests for new signals
		{
			name:       "complete takes priority over approval needed",
			output:     "<promise>APPROVAL_NEEDED: review</promise> <promise>COMPLETE</promise>",
			wantSignal: SignalComplete,
			wantReason: "",
		},
		{
			name:       "eject takes priority over input needed",
			output:     "<promise>INPUT_NEEDED: question</promise> <promise>EJECT: reason</promise>",
			wantSignal: SignalEject,
			wantReason: "reason",
		},
		{
			name:       "blocked takes priority over review requested",
			output:     "<promise>REVIEW_REQUESTED: review</promise> <promise>BLOCKED: blocked</promise>",
			wantSignal: SignalBlocked,
			wantReason: "blocked",
		},
		{
			name:       "approval needed takes priority over input needed",
			output:     "<promise>INPUT_NEEDED: input</promise> <promise>APPROVAL_NEEDED: approval</promise>",
			wantSignal: SignalApprovalNeeded,
			wantReason: "approval",
		},
		{
			name:       "unknown signal returns none",
			output:     "<promise>UNKNOWN_SIGNAL: something</promise>",
			wantSignal: SignalNone,
			wantReason: "",
		},
		{
			name:       "escalate with colon in context",
			output:     "<promise>ESCALATE: Error: critical failure detected</promise>",
			wantSignal: SignalEscalate,
			wantReason: "Error: critical failure detected",
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
