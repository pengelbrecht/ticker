package agent

import (
	"strings"
	"testing"
)

func TestStreamParser_BasicFlow(t *testing.T) {
	// Real output from: claude --print --output-format stream-json --include-partial-messages --verbose
	input := `{"type":"system","subtype":"init","cwd":"/tmp","session_id":"abc-123","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":5}}}
{"type":"result","subtype":"success","result":"hello world","duration_ms":3000,"num_turns":1,"total_cost_usd":0.05,"usage":{"input_tokens":100,"output_tokens":5,"cache_read_input_tokens":1000}}`

	state := &AgentState{}
	updateCount := 0
	parser := NewStreamParser(state, func() { updateCount++ })

	err := parser.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	snap := state.Snapshot()

	if snap.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q", snap.SessionID, "abc-123")
	}

	if snap.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", snap.Model, "claude-opus-4-5-20251101")
	}

	if snap.Output != "hello world" {
		t.Errorf("Output = %q, want %q", snap.Output, "hello world")
	}

	if snap.Status != StatusComplete {
		t.Errorf("Status = %q, want %q", snap.Status, StatusComplete)
	}

	if snap.Metrics.CostUSD != 0.05 {
		t.Errorf("CostUSD = %v, want %v", snap.Metrics.CostUSD, 0.05)
	}

	if snap.Metrics.CacheReadTokens != 1000 {
		t.Errorf("CacheReadTokens = %d, want %d", snap.Metrics.CacheReadTokens, 1000)
	}

	if snap.NumTurns != 1 {
		t.Errorf("NumTurns = %d, want %d", snap.NumTurns, 1)
	}

	if updateCount == 0 {
		t.Error("OnUpdate was never called")
	}
}

func TestStreamParser_ToolUse(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"xyz","model":"sonnet"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"{\"path\":\"/tmp\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"result","subtype":"success","result":"done","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`

	state := &AgentState{}
	parser := NewStreamParser(state, nil)
	parser.Parse(strings.NewReader(input))

	snap := state.Snapshot()

	if len(snap.ToolHistory) != 1 {
		t.Fatalf("ToolHistory len = %d, want 1", len(snap.ToolHistory))
	}

	tool := snap.ToolHistory[0]
	if tool.Name != "Read" {
		t.Errorf("Tool name = %q, want %q", tool.Name, "Read")
	}

	if tool.Input != `{"path":"/tmp"}` {
		t.Errorf("Tool input = %q, want %q", tool.Input, `{"path":"/tmp"}`)
	}
}

func TestStreamParser_Thinking(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"xyz","model":"opus"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Here's my answer"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"result","subtype":"success","result":"Here's my answer","duration_ms":5000,"num_turns":1,"total_cost_usd":0.10}`

	state := &AgentState{}
	parser := NewStreamParser(state, nil)
	parser.Parse(strings.NewReader(input))

	snap := state.Snapshot()

	if snap.Thinking != "Let me think..." {
		t.Errorf("Thinking = %q, want %q", snap.Thinking, "Let me think...")
	}

	if snap.Output != "Here's my answer" {
		t.Errorf("Output = %q, want %q", snap.Output, "Here's my answer")
	}
}
