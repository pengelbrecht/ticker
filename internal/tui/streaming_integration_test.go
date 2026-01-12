package tui

import (
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pengelbrecht/ticker/internal/agent"
)

// -----------------------------------------------------------------------------
// Streaming Pipeline Integration Tests
//
// These tests verify the full streaming pipeline:
// ClaudeAgent stream-json -> StreamParser -> TUI messages -> Model updates
//
// The tests use mock stream data to simulate real Claude CLI output and
// verify that thinking, output, tool use, and metrics all flow correctly.
// -----------------------------------------------------------------------------

// mockTUIProgram simulates tea.Program.Send for collecting messages.
type mockTUIProgram struct {
	mu       sync.Mutex
	messages []tea.Msg
}

func (p *mockTUIProgram) Send(msg tea.Msg) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, msg)
}

func (p *mockTUIProgram) Messages() []tea.Msg {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]tea.Msg{}, p.messages...)
}

// applyMsg is a helper that sends a message to the model and returns the updated model.
func applyMsg(m Model, msg tea.Msg) Model {
	newModel, _ := m.Update(msg)
	return newModel.(Model)
}

// TestStreamingPipeline_BasicTextFlow verifies the basic streaming pipeline:
// stream-json -> StreamParser -> AgentState callback -> TUI messages -> Model
func TestStreamingPipeline_BasicTextFlow(t *testing.T) {
	// Simulate stream-json output from Claude
	streamData := `{"type":"system","subtype":"init","cwd":"/tmp","session_id":"test-123","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello "}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"World!"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":100,"output_tokens":10}}}
{"type":"result","subtype":"success","result":"Hello World!","duration_ms":1500,"num_turns":1,"total_cost_usd":0.02,"usage":{"input_tokens":100,"output_tokens":10}}`

	// Create AgentState and StreamParser
	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	// Track deltas for message conversion (mimics main.go OnAgentState)
	var prevOutput string

	// Create parser with callback that converts to TUI messages
	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		// Convert output delta
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				mockProgram.Send(AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}

		// Send metrics
		mockProgram.Send(AgentMetricsMsg{
			InputTokens:  snap.Metrics.InputTokens,
			OutputTokens: snap.Metrics.OutputTokens,
			CostUSD:      snap.Metrics.CostUSD,
			Model:        snap.Model,
		})

		// Send status
		mockProgram.Send(AgentStatusMsg{
			Status: snap.Status,
			Error:  snap.ErrorMsg,
		})
	})

	// Parse the stream
	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify AgentState
	snap := state.Snapshot()
	if snap.SessionID != "test-123" {
		t.Errorf("SessionID = %q, want %q", snap.SessionID, "test-123")
	}
	if snap.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", snap.Model, "claude-opus-4-5-20251101")
	}
	if snap.Output != "Hello World!" {
		t.Errorf("Output = %q, want %q", snap.Output, "Hello World!")
	}
	if snap.Status != agent.StatusComplete {
		t.Errorf("Status = %q, want %q", snap.Status, agent.StatusComplete)
	}

	// Verify TUI messages were generated
	messages := mockProgram.Messages()
	if len(messages) == 0 {
		t.Fatal("Expected messages to be sent to TUI")
	}

	// Find AgentTextMsg messages and verify deltas
	var textMsgs []AgentTextMsg
	for _, msg := range messages {
		if txt, ok := msg.(AgentTextMsg); ok {
			textMsgs = append(textMsgs, txt)
		}
	}
	if len(textMsgs) == 0 {
		t.Error("Expected AgentTextMsg to be sent")
	}

	// Concatenate all text deltas
	var fullText strings.Builder
	for _, txt := range textMsgs {
		fullText.WriteString(txt.Text)
	}
	if fullText.String() != "Hello World!" {
		t.Errorf("Concatenated text = %q, want %q", fullText.String(), "Hello World!")
	}

	// Now apply messages to TUI Model
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// Start an iteration
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test Task"})

	// Apply all the streaming messages
	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify Model state was updated
	if !strings.Contains(m.output, "Hello World!") {
		t.Errorf("Model output should contain 'Hello World!', got %q", m.output)
	}
	if m.liveModel != "claude-opus-4-5-20251101" {
		t.Errorf("Model liveModel = %q, want %q", m.liveModel, "claude-opus-4-5-20251101")
	}
}

// TestStreamingPipeline_ThinkingAndOutput verifies that thinking and output
// streams are properly separated and flow to the TUI.
func TestStreamingPipeline_ThinkingAndOutput(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"think-test","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me think about this..."}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" I need to analyze the problem."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Here is my answer."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"result","subtype":"success","result":"Here is my answer.","duration_ms":2000,"num_turns":1,"total_cost_usd":0.05}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	var prevOutput, prevThinking string

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		// Convert thinking delta
		if snap.Thinking != prevThinking {
			delta := snap.Thinking[len(prevThinking):]
			if delta != "" {
				mockProgram.Send(AgentThinkingMsg{Text: delta})
			}
			prevThinking = snap.Thinking
		}

		// Convert output delta
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				mockProgram.Send(AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}

		mockProgram.Send(AgentStatusMsg{Status: snap.Status})
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify AgentState has both streams
	snap := state.Snapshot()
	if snap.Thinking != "Let me think about this... I need to analyze the problem." {
		t.Errorf("Thinking = %q, want %q", snap.Thinking, "Let me think about this... I need to analyze the problem.")
	}
	if snap.Output != "Here is my answer." {
		t.Errorf("Output = %q, want %q", snap.Output, "Here is my answer.")
	}

	// Verify separate message types were sent
	messages := mockProgram.Messages()
	var thinkingMsgs []AgentThinkingMsg
	var textMsgs []AgentTextMsg
	for _, msg := range messages {
		switch m := msg.(type) {
		case AgentThinkingMsg:
			thinkingMsgs = append(thinkingMsgs, m)
		case AgentTextMsg:
			textMsgs = append(textMsgs, m)
		}
	}

	if len(thinkingMsgs) == 0 {
		t.Error("Expected AgentThinkingMsg to be sent")
	}
	if len(textMsgs) == 0 {
		t.Error("Expected AgentTextMsg to be sent")
	}

	// Apply to TUI Model
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test"})

	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify Model has both thinking and output
	if !strings.Contains(m.thinking, "Let me think") {
		t.Errorf("Model thinking should contain 'Let me think', got %q", m.thinking)
	}
	if !strings.Contains(m.output, "Here is my answer") {
		t.Errorf("Model output should contain 'Here is my answer', got %q", m.output)
	}
}

// TestStreamingPipeline_ToolUse verifies that tool invocations are properly
// tracked and flow to the TUI with start/end events.
func TestStreamingPipeline_ToolUse(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"tool-test","model":"claude-sonnet-4-20250514"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_abc","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"{\"file_path\":"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"\"/tmp/test.txt\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"File contents: hello"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"result","subtype":"success","result":"File contents: hello","duration_ms":500,"num_turns":1,"total_cost_usd":0.01}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	var prevOutput, prevToolID string

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		// Tool start
		if snap.ActiveTool != nil && snap.ActiveTool.ID != prevToolID {
			mockProgram.Send(AgentToolStartMsg{
				ID:   snap.ActiveTool.ID,
				Name: snap.ActiveTool.Name,
			})
			prevToolID = snap.ActiveTool.ID
		} else if snap.ActiveTool == nil && prevToolID != "" {
			// Tool ended
			for _, tool := range snap.ToolHistory {
				if tool.ID == prevToolID {
					mockProgram.Send(AgentToolEndMsg{
						ID:       tool.ID,
						Name:     tool.Name,
						Duration: tool.Duration,
						IsError:  tool.IsError,
					})
					break
				}
			}
			prevToolID = ""
		}

		// Output delta
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				mockProgram.Send(AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}

		mockProgram.Send(AgentStatusMsg{Status: snap.Status})
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify tool was tracked
	snap := state.Snapshot()
	if len(snap.ToolHistory) != 1 {
		t.Fatalf("ToolHistory len = %d, want 1", len(snap.ToolHistory))
	}
	if snap.ToolHistory[0].Name != "Read" {
		t.Errorf("Tool name = %q, want %q", snap.ToolHistory[0].Name, "Read")
	}
	if snap.ToolHistory[0].Input != `{"file_path":"/tmp/test.txt"}` {
		t.Errorf("Tool input = %q, want %q", snap.ToolHistory[0].Input, `{"file_path":"/tmp/test.txt"}`)
	}

	// Verify tool messages were sent
	messages := mockProgram.Messages()
	var toolStartMsgs []AgentToolStartMsg
	var toolEndMsgs []AgentToolEndMsg
	for _, msg := range messages {
		switch m := msg.(type) {
		case AgentToolStartMsg:
			toolStartMsgs = append(toolStartMsgs, m)
		case AgentToolEndMsg:
			toolEndMsgs = append(toolEndMsgs, m)
		}
	}

	if len(toolStartMsgs) != 1 {
		t.Errorf("Expected 1 AgentToolStartMsg, got %d", len(toolStartMsgs))
	}
	if len(toolEndMsgs) != 1 {
		t.Errorf("Expected 1 AgentToolEndMsg, got %d", len(toolEndMsgs))
	}

	if len(toolStartMsgs) > 0 && toolStartMsgs[0].Name != "Read" {
		t.Errorf("ToolStart name = %q, want %q", toolStartMsgs[0].Name, "Read")
	}
	if len(toolEndMsgs) > 0 && toolEndMsgs[0].Name != "Read" {
		t.Errorf("ToolEnd name = %q, want %q", toolEndMsgs[0].Name, "Read")
	}

	// Apply to TUI Model
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test"})

	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify tool history in Model
	if len(m.toolHistory) != 1 {
		t.Fatalf("Model toolHistory len = %d, want 1", len(m.toolHistory))
	}
	if m.toolHistory[0].Name != "Read" {
		t.Errorf("Model toolHistory[0].Name = %q, want %q", m.toolHistory[0].Name, "Read")
	}
}

// TestStreamingPipeline_MultipleTools verifies multiple sequential tool uses.
func TestStreamingPipeline_MultipleTools(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"multi-tool","model":"claude-sonnet-4-20250514"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tool_1","name":"Glob"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","text":"{}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_2","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","text":"{}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"stream_event","event":{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool_3","name":"Edit"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","text":"{}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":2}}
{"type":"stream_event","event":{"type":"content_block_start","index":3,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":3,"delta":{"type":"text_delta","text":"Done!"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":3}}
{"type":"result","subtype":"success","result":"Done!","duration_ms":1000,"num_turns":1,"total_cost_usd":0.03}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	var prevToolID string

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		if snap.ActiveTool != nil && snap.ActiveTool.ID != prevToolID {
			mockProgram.Send(AgentToolStartMsg{
				ID:   snap.ActiveTool.ID,
				Name: snap.ActiveTool.Name,
			})
			prevToolID = snap.ActiveTool.ID
		} else if snap.ActiveTool == nil && prevToolID != "" {
			for _, tool := range snap.ToolHistory {
				if tool.ID == prevToolID {
					mockProgram.Send(AgentToolEndMsg{
						ID:       tool.ID,
						Name:     tool.Name,
						Duration: tool.Duration,
					})
					break
				}
			}
			prevToolID = ""
		}
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify all tools tracked
	snap := state.Snapshot()
	if len(snap.ToolHistory) != 3 {
		t.Fatalf("ToolHistory len = %d, want 3", len(snap.ToolHistory))
	}

	expectedTools := []string{"Glob", "Read", "Edit"}
	for i, name := range expectedTools {
		if snap.ToolHistory[i].Name != name {
			t.Errorf("ToolHistory[%d].Name = %q, want %q", i, snap.ToolHistory[i].Name, name)
		}
	}

	// Verify message counts
	messages := mockProgram.Messages()
	var startCount, endCount int
	for _, msg := range messages {
		switch msg.(type) {
		case AgentToolStartMsg:
			startCount++
		case AgentToolEndMsg:
			endCount++
		}
	}

	if startCount != 3 {
		t.Errorf("AgentToolStartMsg count = %d, want 3", startCount)
	}
	if endCount != 3 {
		t.Errorf("AgentToolEndMsg count = %d, want 3", endCount)
	}
}

// TestStreamingPipeline_MetricsFlow verifies that metrics updates flow correctly.
func TestStreamingPipeline_MetricsFlow(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"metrics-test","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Test"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":500,"output_tokens":50,"cache_read_input_tokens":1000}}}
{"type":"result","subtype":"success","result":"Test","duration_ms":2000,"num_turns":1,"total_cost_usd":0.08,"usage":{"input_tokens":500,"output_tokens":50,"cache_read_input_tokens":1000}}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()
		mockProgram.Send(AgentMetricsMsg{
			InputTokens:     snap.Metrics.InputTokens,
			OutputTokens:    snap.Metrics.OutputTokens,
			CacheReadTokens: snap.Metrics.CacheReadTokens,
			CostUSD:         snap.Metrics.CostUSD,
			Model:           snap.Model,
		})
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify final metrics
	snap := state.Snapshot()
	if snap.Metrics.InputTokens != 500 {
		t.Errorf("InputTokens = %d, want 500", snap.Metrics.InputTokens)
	}
	if snap.Metrics.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", snap.Metrics.OutputTokens)
	}
	if snap.Metrics.CacheReadTokens != 1000 {
		t.Errorf("CacheReadTokens = %d, want 1000", snap.Metrics.CacheReadTokens)
	}
	if snap.Metrics.CostUSD != 0.08 {
		t.Errorf("CostUSD = %f, want 0.08", snap.Metrics.CostUSD)
	}

	// Verify metrics messages were sent and apply to TUI
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test"})

	messages := mockProgram.Messages()
	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify Model has live metrics
	if m.liveInputTokens != 500 {
		t.Errorf("Model liveInputTokens = %d, want 500", m.liveInputTokens)
	}
	if m.liveOutputTokens != 50 {
		t.Errorf("Model liveOutputTokens = %d, want 50", m.liveOutputTokens)
	}
	if m.liveCacheReadTokens != 1000 {
		t.Errorf("Model liveCacheReadTokens = %d, want 1000", m.liveCacheReadTokens)
	}
	if m.liveModel != "claude-opus-4-5-20251101" {
		t.Errorf("Model liveModel = %q, want %q", m.liveModel, "claude-opus-4-5-20251101")
	}
}

// TestStreamingPipeline_StatusTransitions verifies status changes flow correctly.
func TestStreamingPipeline_StatusTransitions(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"status-test","model":"opus"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"thinking..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","text":"{}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"stream_event","event":{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"done"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":2}}
{"type":"result","subtype":"success","result":"done","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`

	state := &agent.AgentState{}
	var statusHistory []agent.RunStatus

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()
		// Track status changes
		if len(statusHistory) == 0 || statusHistory[len(statusHistory)-1] != snap.Status {
			statusHistory = append(statusHistory, snap.Status)
		}
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify we saw the expected status transitions
	expectedStatuses := []agent.RunStatus{
		agent.StatusStarting,
		agent.StatusThinking,
		agent.StatusToolUse,
		agent.StatusWriting,
		agent.StatusComplete,
	}

	if len(statusHistory) != len(expectedStatuses) {
		t.Errorf("Status history len = %d, want %d", len(statusHistory), len(expectedStatuses))
		t.Logf("Got: %v", statusHistory)
	}

	for i, expected := range expectedStatuses {
		if i < len(statusHistory) && statusHistory[i] != expected {
			t.Errorf("Status[%d] = %q, want %q", i, statusHistory[i], expected)
		}
	}
}

// TestStreamingPipeline_ErrorFlow verifies error status propagates correctly.
func TestStreamingPipeline_ErrorFlow(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"error-test","model":"sonnet"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Starting..."}}}
{"type":"result","subtype":"error","result":"API rate limit exceeded","duration_ms":500,"num_turns":0,"total_cost_usd":0.00}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()
		mockProgram.Send(AgentStatusMsg{
			Status: snap.Status,
			Error:  snap.ErrorMsg,
		})
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify error state
	snap := state.Snapshot()
	if snap.Status != agent.StatusError {
		t.Errorf("Status = %q, want %q", snap.Status, agent.StatusError)
	}
	if snap.ErrorMsg != "API rate limit exceeded" {
		t.Errorf("ErrorMsg = %q, want %q", snap.ErrorMsg, "API rate limit exceeded")
	}

	// Verify error was sent to TUI
	messages := mockProgram.Messages()
	var errorFound bool
	for _, msg := range messages {
		if status, ok := msg.(AgentStatusMsg); ok {
			if status.Status == agent.StatusError && status.Error == "API rate limit exceeded" {
				errorFound = true
				break
			}
		}
	}
	if !errorFound {
		t.Error("Expected AgentStatusMsg with error to be sent")
	}
}

// TestStreamingPipeline_IterationReset verifies that iteration start resets delta tracking.
func TestStreamingPipeline_IterationReset(t *testing.T) {
	// Create TUI model and simulate two iterations
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// First iteration
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Task 1"})
	m = applyMsg(m, AgentTextMsg{Text: "Output from iteration 1"})
	m = applyMsg(m, AgentThinkingMsg{Text: "Thinking from iteration 1"})
	m = applyMsg(m, AgentToolStartMsg{ID: "tool1", Name: "Read"})
	m = applyMsg(m, AgentToolEndMsg{ID: "tool1", Name: "Read", Duration: time.Second})
	m = applyMsg(m, IterationEndMsg{Iteration: 1, Cost: 0.10, Tokens: 100})

	// Verify iteration 1 state
	if !strings.Contains(m.output, "iteration 1") {
		t.Error("Expected iteration 1 output")
	}
	if !strings.Contains(m.thinking, "iteration 1") {
		t.Error("Expected iteration 1 thinking")
	}
	if len(m.toolHistory) != 1 {
		t.Error("Expected 1 tool in history")
	}

	// Second iteration should reset
	m = applyMsg(m, IterationStartMsg{Iteration: 2, TaskID: "t2", TaskTitle: "Task 2"})

	// Verify reset
	if strings.Contains(m.output, "iteration 1") {
		t.Error("Output should be cleared on new iteration")
	}
	if strings.Contains(m.thinking, "iteration 1") {
		t.Error("Thinking should be cleared on new iteration")
	}
	if len(m.toolHistory) != 0 {
		t.Error("Tool history should be cleared on new iteration")
	}
	if m.activeTool != nil {
		t.Error("Active tool should be nil on new iteration")
	}

	// Add new iteration data
	m = applyMsg(m, AgentTextMsg{Text: "Output from iteration 2"})
	m = applyMsg(m, AgentThinkingMsg{Text: "Thinking from iteration 2"})

	if !strings.Contains(m.output, "iteration 2") {
		t.Error("Expected iteration 2 output")
	}
	if !strings.Contains(m.thinking, "iteration 2") {
		t.Error("Expected iteration 2 thinking")
	}
}

// TestStreamingPipeline_ToRecord verifies RunRecord generation for persistence.
func TestStreamingPipeline_ToRecord(t *testing.T) {
	streamData := `{"type":"system","subtype":"init","session_id":"record-test","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Thinking..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"t1","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","text":"{\"path\":\"/test\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"stream_event","event":{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Done!"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":2}}
{"type":"result","subtype":"success","result":"Done!","duration_ms":3000,"num_turns":2,"total_cost_usd":0.15,"usage":{"input_tokens":200,"output_tokens":20,"cache_read_input_tokens":500}}`

	state := &agent.AgentState{}
	parser := agent.NewStreamParser(state, nil)
	parser.Parse(strings.NewReader(streamData))

	// Generate RunRecord
	record := state.ToRecord()

	// Verify record contents
	if record.SessionID != "record-test" {
		t.Errorf("SessionID = %q, want %q", record.SessionID, "record-test")
	}
	if record.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", record.Model, "claude-opus-4-5-20251101")
	}
	if !record.Success {
		t.Error("Success should be true")
	}
	if record.NumTurns != 2 {
		t.Errorf("NumTurns = %d, want 2", record.NumTurns)
	}
	if record.Output != "Done!" {
		t.Errorf("Output = %q, want %q", record.Output, "Done!")
	}
	if record.Thinking != "Thinking..." {
		t.Errorf("Thinking = %q, want %q", record.Thinking, "Thinking...")
	}
	if len(record.Tools) != 1 {
		t.Fatalf("Tools len = %d, want 1", len(record.Tools))
	}
	if record.Tools[0].Name != "Read" {
		t.Errorf("Tools[0].Name = %q, want %q", record.Tools[0].Name, "Read")
	}
	if record.Metrics.InputTokens != 200 {
		t.Errorf("Metrics.InputTokens = %d, want 200", record.Metrics.InputTokens)
	}
	if record.Metrics.CostUSD != 0.15 {
		t.Errorf("Metrics.CostUSD = %f, want 0.15", record.Metrics.CostUSD)
	}
}

// TestStreamingPipeline_FullEndToEnd simulates a complete agent run with all features.
func TestStreamingPipeline_FullEndToEnd(t *testing.T) {
	// Comprehensive stream simulating a real agent interaction
	streamData := `{"type":"system","subtype":"init","cwd":"/project","session_id":"e2e-test","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"thinking"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me analyze this task..."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tool_glob","name":"Glob"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","text":"{\"pattern\":\"**/*.go\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":1}}
{"type":"stream_event","event":{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool_read","name":"Read"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","text":"{\"file_path\":\"/project/main.go\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":2}}
{"type":"stream_event","event":{"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"tool_edit","name":"Edit"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","text":"{\"file_path\":\"/project/main.go\",\"old\":\"foo\",\"new\":\"bar\"}"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":3}}
{"type":"stream_event","event":{"type":"content_block_start","index":4,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"I've analyzed the codebase and made the requested changes. "}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":4,"delta":{"type":"text_delta","text":"The task is complete."}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":4}}
{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":1500,"output_tokens":100,"cache_read_input_tokens":5000,"cache_creation_input_tokens":200}}}
{"type":"result","subtype":"success","result":"I've analyzed the codebase and made the requested changes. The task is complete.","duration_ms":8500,"num_turns":3,"total_cost_usd":0.35,"usage":{"input_tokens":1500,"output_tokens":100,"cache_read_input_tokens":5000,"cache_creation_input_tokens":200}}`

	// Setup
	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	var prevOutput, prevThinking, prevToolID string

	// Full callback that mimics main.go OnAgentState
	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		// Thinking deltas
		if snap.Thinking != prevThinking {
			delta := snap.Thinking[len(prevThinking):]
			if delta != "" {
				mockProgram.Send(AgentThinkingMsg{Text: delta})
			}
			prevThinking = snap.Thinking
		}

		// Output deltas
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				mockProgram.Send(AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}

		// Tool activity
		if snap.ActiveTool != nil && snap.ActiveTool.ID != prevToolID {
			mockProgram.Send(AgentToolStartMsg{
				ID:   snap.ActiveTool.ID,
				Name: snap.ActiveTool.Name,
			})
			prevToolID = snap.ActiveTool.ID
		} else if snap.ActiveTool == nil && prevToolID != "" {
			for _, tool := range snap.ToolHistory {
				if tool.ID == prevToolID {
					mockProgram.Send(AgentToolEndMsg{
						ID:       tool.ID,
						Name:     tool.Name,
						Duration: tool.Duration,
						IsError:  tool.IsError,
					})
					break
				}
			}
			prevToolID = ""
		}

		// Metrics
		mockProgram.Send(AgentMetricsMsg{
			InputTokens:         snap.Metrics.InputTokens,
			OutputTokens:        snap.Metrics.OutputTokens,
			CacheReadTokens:     snap.Metrics.CacheReadTokens,
			CacheCreationTokens: snap.Metrics.CacheCreationTokens,
			CostUSD:             snap.Metrics.CostUSD,
			Model:               snap.Model,
		})

		// Status
		mockProgram.Send(AgentStatusMsg{
			Status: snap.Status,
			Error:  snap.ErrorMsg,
		})
	})

	// Parse
	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify AgentState
	snap := state.Snapshot()
	if snap.SessionID != "e2e-test" {
		t.Errorf("SessionID = %q, want %q", snap.SessionID, "e2e-test")
	}
	if snap.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", snap.Model, "claude-opus-4-5-20251101")
	}
	if !strings.HasPrefix(snap.Thinking, "Let me analyze") {
		t.Errorf("Thinking should start with 'Let me analyze', got %q", snap.Thinking)
	}
	if !strings.Contains(snap.Output, "task is complete") {
		t.Errorf("Output should contain 'task is complete', got %q", snap.Output)
	}
	if len(snap.ToolHistory) != 3 {
		t.Errorf("ToolHistory len = %d, want 3", len(snap.ToolHistory))
	}
	if snap.Status != agent.StatusComplete {
		t.Errorf("Status = %q, want %q", snap.Status, agent.StatusComplete)
	}
	if snap.NumTurns != 3 {
		t.Errorf("NumTurns = %d, want 3", snap.NumTurns)
	}
	if snap.Metrics.InputTokens != 1500 {
		t.Errorf("InputTokens = %d, want 1500", snap.Metrics.InputTokens)
	}
	if snap.Metrics.CacheReadTokens != 5000 {
		t.Errorf("CacheReadTokens = %d, want 5000", snap.Metrics.CacheReadTokens)
	}
	if snap.Metrics.CostUSD != 0.35 {
		t.Errorf("CostUSD = %f, want 0.35", snap.Metrics.CostUSD)
	}

	// Apply all messages to TUI Model
	m := New(Config{
		EpicID:       "test-epic",
		EpicTitle:    "End-to-End Test Epic",
		MaxCost:      50.0,
		MaxIteration: 100,
	})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(m, TasksUpdateMsg{Tasks: []TaskInfo{
		{ID: "t1", Title: "Test Task", Status: TaskStatusOpen},
	}})
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test Task"})

	messages := mockProgram.Messages()
	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify final TUI Model state
	if !strings.Contains(m.thinking, "Let me analyze") {
		t.Errorf("Model thinking should contain 'Let me analyze', got %q", m.thinking)
	}
	if !strings.Contains(m.output, "task is complete") {
		t.Errorf("Model output should contain 'task is complete', got %q", m.output)
	}
	if len(m.toolHistory) != 3 {
		t.Errorf("Model toolHistory len = %d, want 3", len(m.toolHistory))
	}
	if m.liveInputTokens != 1500 {
		t.Errorf("Model liveInputTokens = %d, want 1500", m.liveInputTokens)
	}
	if m.liveOutputTokens != 100 {
		t.Errorf("Model liveOutputTokens = %d, want 100", m.liveOutputTokens)
	}
	if m.liveCacheReadTokens != 5000 {
		t.Errorf("Model liveCacheReadTokens = %d, want 5000", m.liveCacheReadTokens)
	}
	if m.liveCacheCreationTokens != 200 {
		t.Errorf("Model liveCacheCreationTokens = %d, want 200", m.liveCacheCreationTokens)
	}
	if m.liveModel != "claude-opus-4-5-20251101" {
		t.Errorf("Model liveModel = %q, want %q", m.liveModel, "claude-opus-4-5-20251101")
	}
	if m.liveStatus != agent.StatusComplete {
		t.Errorf("Model liveStatus = %q, want %q", m.liveStatus, agent.StatusComplete)
	}

	// Verify View renders without panic
	view := m.View()
	if view == "" {
		t.Error("Expected non-empty view")
	}

	// Generate and verify RunRecord
	record := state.ToRecord()
	if !record.Success {
		t.Error("RunRecord.Success should be true")
	}
	if record.NumTurns != 3 {
		t.Errorf("RunRecord.NumTurns = %d, want 3", record.NumTurns)
	}
	if len(record.Tools) != 3 {
		t.Errorf("RunRecord.Tools len = %d, want 3", len(record.Tools))
	}

	// Verify tool names in record
	expectedToolNames := []string{"Glob", "Read", "Edit"}
	for i, name := range expectedToolNames {
		if record.Tools[i].Name != name {
			t.Errorf("RunRecord.Tools[%d].Name = %q, want %q", i, record.Tools[i].Name, name)
		}
	}
}

// TestStreamingPipeline_NewlinePreservation verifies that newlines in streamed output
// are preserved through the full pipeline: stream-json -> StreamParser -> TUI Model.
func TestStreamingPipeline_NewlinePreservation(t *testing.T) {
	// Stream data with newlines in text_delta events (mimics real Claude output)
	streamData := `{"type":"system","subtype":"init","session_id":"newline-test","model":"claude-opus-4-5-20251101"}
{"type":"stream_event","event":{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Line"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 1\nLine 2\nLine"}}}
{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" 3"}}}
{"type":"stream_event","event":{"type":"content_block_stop","index":0}}
{"type":"result","subtype":"success","result":"Line 1\nLine 2\nLine 3","duration_ms":1000,"num_turns":1,"total_cost_usd":0.01}`

	state := &agent.AgentState{}
	mockProgram := &mockTUIProgram{}

	var prevOutput string

	parser := agent.NewStreamParser(state, func() {
		snap := state.Snapshot()

		// Convert output delta (mimics main.go OnAgentState)
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				mockProgram.Send(AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}
	})

	err := parser.Parse(strings.NewReader(streamData))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	// Verify AgentState has preserved newlines
	snap := state.Snapshot()
	expectedOutput := "Line 1\nLine 2\nLine 3"

	if snap.Output != expectedOutput {
		t.Errorf("AgentState.Output = %q, want %q", snap.Output, expectedOutput)
	}

	// Verify newlines in state
	newlineCount := strings.Count(snap.Output, "\n")
	if newlineCount != 2 {
		t.Errorf("AgentState.Output newline count = %d, want 2", newlineCount)
	}

	// Apply messages to TUI Model and verify
	m := New(Config{EpicID: "test", EpicTitle: "Test"})
	m = applyMsg(m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(m, IterationStartMsg{Iteration: 1, TaskID: "t1", TaskTitle: "Test"})

	messages := mockProgram.Messages()
	for _, msg := range messages {
		m = applyMsg(m, msg)
	}

	// Verify TUI Model output has preserved newlines
	if !strings.Contains(m.output, "\n") {
		t.Errorf("Model.output should contain newlines, got %q", m.output)
	}

	// Concatenate all text messages and verify
	var allText strings.Builder
	for _, msg := range messages {
		if txt, ok := msg.(AgentTextMsg); ok {
			allText.WriteString(txt.Text)
		}
	}
	concatenatedText := allText.String()
	if concatenatedText != expectedOutput {
		t.Errorf("Concatenated text = %q, want %q", concatenatedText, expectedOutput)
	}

	modelNewlineCount := strings.Count(m.output, "\n")
	if modelNewlineCount != 2 {
		t.Errorf("Model.output newline count = %d, want 2", modelNewlineCount)
	}
}
