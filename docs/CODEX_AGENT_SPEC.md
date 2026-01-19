# Codex Agent Implementation Specification

This document specifies how to add OpenAI Codex CLI support to Ticker.

## Overview

**Codex CLI** is OpenAI's coding agent that runs locally in your terminal. It's written in Rust, open source, and supports non-interactive execution via `codex exec`.

- **Binary**: `codex` (installed via `npm i -g @openai/codex` or `brew install --cask codex`)
- **GitHub**: https://github.com/openai/codex
- **Docs**: https://developers.openai.com/codex/cli/

## CLI Command Mapping

### Claude vs Codex Comparison

| Feature | Claude CLI | Codex CLI |
|---------|-----------|-----------|
| **Command** | `claude` | `codex exec` (non-interactive) |
| **Skip permissions** | `--dangerously-skip-permissions` | `--dangerously-bypass-approvals-and-sandbox` or `--yolo` |
| **JSON streaming** | `--output-format stream-json` | `--json` |
| **Session persistence** | `--no-session-persistence` | N/A (ephemeral by default in exec) |
| **Prompt passing** | Last positional arg | Last positional arg |
| **Working directory** | `cmd.Dir` | `--cd <path>` or `cmd.Dir` |
| **Model selection** | N/A (configured externally) | `--model <model>` |

### Codex Execution Modes

For Ticker's autonomous operation, we need **non-interactive mode**:

```bash
# Full autonomous mode (equivalent to Claude's --dangerously-skip-permissions)
codex exec --json --dangerously-bypass-approvals-and-sandbox "prompt here"

# Safer alternative with sandboxing
codex exec --json --full-auto "prompt here"
```

**Flag meanings:**
- `exec` (or `e`): Non-interactive execution subcommand
- `--json`: Stream JSONL events to stdout (critical for parsing)
- `--dangerously-bypass-approvals-and-sandbox` (or `--yolo`): Skip all approval prompts, no sandbox
- `--full-auto`: Safer preset (`--ask-for-approval on-failure --sandbox workspace-write`)

### Approval Policies (`--ask-for-approval` / `-a`)

| Value | Behavior |
|-------|----------|
| `untrusted` | Only trusted commands (ls, cat, sed) auto-run |
| `on-failure` | Auto-run all, ask on failure |
| `on-request` | Model decides when to ask |
| `never` | Never ask (failures go to model) |

### Sandbox Modes (`--sandbox` / `-s`)

| Value | Behavior |
|-------|----------|
| `read-only` | Can read files, cannot write |
| `workspace-write` | Can write in repo + temp dirs |
| `danger-full-access` | Can write anywhere |

## JSON Output Format

Codex `--json` produces **JSON Lines (JSONL)** similar to Claude's `stream-json`.

### Event Types

```
thread.started    → Session initialized
turn.started      → New turn begins
turn.completed    → Turn finished (includes token usage)
turn.failed       → Turn failed with error
item.started      → Item (tool/message) begins
item.updated      → Item progress update
item.completed    → Item finished
error             → Error event
```

### Event Structure Examples

**Thread Started:**
```json
{"type":"thread.started","thread_id":"0199a213-81c0-7800-8aa1-bbab2a035a53"}
```

**Turn Started:**
```json
{"type":"turn.started"}
```

**Command Execution Item:**
```json
{
  "type":"item.started",
  "item":{
    "id":"item_1",
    "type":"command_execution",
    "command":"bash -lc ls",
    "status":"in_progress"
  }
}
```

```json
{
  "type":"item.completed",
  "item":{
    "id":"item_1",
    "type":"command_execution",
    "command":"bash -lc ls",
    "aggregated_output":"file1.go\nfile2.go\n",
    "exit_code":0,
    "status":"completed"
  }
}
```

**Agent Message Item:**
```json
{
  "type":"item.completed",
  "item":{
    "id":"item_3",
    "type":"agent_message",
    "text":"Repo contains docs, sdk, and examples directories."
  }
}
```

**Turn Completed (with metrics):**
```json
{
  "type":"turn.completed",
  "usage":{
    "input_tokens":24763,
    "cached_input_tokens":24448,
    "output_tokens":122
  }
}
```

### Item Types

| Item Type | Description | Ticker Mapping |
|-----------|-------------|----------------|
| `agent_message` | Text response | → `Output` |
| `reasoning` | Internal reasoning | → `Thinking` |
| `command_execution` | Shell command | → `ToolActivity` |
| `file_change` | File modification | → `ToolActivity` |
| `mcp_tool_call` | MCP tool call | → `ToolActivity` |
| `web_search` | Web search | → `ToolActivity` |
| `todo_list` | Plan updates | → (log only) |

## Implementation

### File: `internal/agent/codex.go`

```go
package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CodexAgent implements the Agent interface for OpenAI Codex CLI.
type CodexAgent struct {
	// Command is the path to the codex binary. Defaults to "codex".
	Command string

	// Model specifies which model to use (e.g., "o4-mini", "gpt-4").
	// Empty string uses Codex's default.
	Model string

	// FullAuto uses the safer --full-auto preset instead of --yolo.
	// Default false = use --dangerously-bypass-approvals-and-sandbox.
	FullAuto bool
}

// NewCodexAgent creates a new Codex agent with default settings.
func NewCodexAgent() *CodexAgent {
	return &CodexAgent{Command: "codex"}
}

// Name returns "codex".
func (a *CodexAgent) Name() string {
	return "codex"
}

// Available checks if the codex CLI is installed and accessible.
func (a *CodexAgent) Available() bool {
	_, err := exec.LookPath(a.command())
	return err == nil
}

// Run executes codex with the given prompt.
func (a *CodexAgent) Run(ctx context.Context, prompt string, opts RunOpts) (*Result, error) {
	start := time.Now()

	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// Build args for non-interactive execution
	args := []string{
		"exec",
		"--json",
	}

	// Permission mode
	if a.FullAuto {
		args = append(args, "--full-auto")
	} else {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}

	// Model selection (if specified)
	if a.Model != "" {
		args = append(args, "--model", a.Model)
	}

	// Working directory (if specified)
	if opts.WorkDir != "" {
		args = append(args, "--cd", opts.WorkDir)
	}

	// Prompt is the final positional argument
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, a.command(), args...)

	var stderr bytes.Buffer

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	// Create state and parser for structured streaming
	state := &AgentState{}
	var prevOutputLen int

	onUpdate := func() {
		snap := state.Snapshot()

		if opts.StateCallback != nil {
			opts.StateCallback(snap)
		}

		if opts.Stream != nil && len(snap.Output) > prevOutputLen {
			delta := snap.Output[prevOutputLen:]
			select {
			case opts.Stream <- delta:
				prevOutputLen = len(snap.Output)
			default:
			}
		}
	}

	parser := NewCodexStreamParser(state, onUpdate)

	parseErr := parser.Parse(stdoutPipe)

	waitErr := cmd.Wait()

	duration := time.Since(start)

	if waitErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			snap := state.Snapshot()
			record := state.ToRecord()
			record.Success = false
			record.ErrorMsg = fmt.Sprintf("timed out after %v", opts.Timeout)
			return &Result{
				Output:    snap.Output,
				TokensIn:  snap.Metrics.InputTokens,
				TokensOut: snap.Metrics.OutputTokens,
				Cost:      snap.Metrics.CostUSD,
				Duration:  duration,
				Record:    &record,
			}, ErrTimeout
		}
		if ctx.Err() == context.Canceled {
			return nil, fmt.Errorf("codex cancelled")
		}
		return nil, fmt.Errorf("codex exited with error: %w\nstderr: %s", waitErr, stderr.String())
	}
	if parseErr != nil {
		return nil, fmt.Errorf("parse stream output: %w", parseErr)
	}

	snap := state.Snapshot()
	record := state.ToRecord()

	return &Result{
		Output:    snap.Output,
		TokensIn:  snap.Metrics.InputTokens,
		TokensOut: snap.Metrics.OutputTokens,
		Cost:      snap.Metrics.CostUSD,
		Duration:  duration,
		Record:    &record,
	}, nil
}

func (a *CodexAgent) command() string {
	if a.Command != "" {
		return a.Command
	}
	return "codex"
}
```

### File: `internal/agent/codex_stream.go`

```go
package agent

import (
	"bufio"
	"encoding/json"
	"io"
	"time"
)

// CodexStreamParser parses Codex's JSONL output and updates AgentState.
type CodexStreamParser struct {
	state    *AgentState
	onUpdate func()

	// Track active items by ID
	activeItems map[string]*ToolActivity
}

// NewCodexStreamParser creates a parser for Codex output.
func NewCodexStreamParser(state *AgentState, onUpdate func()) *CodexStreamParser {
	return &CodexStreamParser{
		state:       state,
		onUpdate:    onUpdate,
		activeItems: make(map[string]*ToolActivity),
	}
}

// Parse reads JSON lines from r and updates state.
func (p *CodexStreamParser) Parse(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		p.parseLine(line)
	}

	return scanner.Err()
}

func (p *CodexStreamParser) parseLine(line []byte) {
	var event struct {
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Item     json.RawMessage `json:"item"`
		Usage    struct {
			InputTokens       int `json:"input_tokens"`
			CachedInputTokens int `json:"cached_input_tokens"`
			OutputTokens      int `json:"output_tokens"`
		} `json:"usage"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(line, &event); err != nil {
		return
	}

	switch event.Type {
	case "thread.started":
		p.handleThreadStarted(event.ThreadID)

	case "turn.started":
		p.state.mu.Lock()
		p.state.Status = StatusThinking
		p.state.NumTurns++
		p.state.mu.Unlock()
		p.notify()

	case "turn.completed":
		p.state.mu.Lock()
		p.state.Status = StatusComplete
		p.state.Metrics.InputTokens += event.Usage.InputTokens
		p.state.Metrics.OutputTokens += event.Usage.OutputTokens
		p.state.Metrics.CacheReadTokens += event.Usage.CachedInputTokens
		p.state.mu.Unlock()
		p.notify()

	case "turn.failed":
		p.state.mu.Lock()
		p.state.Status = StatusError
		p.state.ErrorMsg = event.Error
		p.state.mu.Unlock()
		p.notify()

	case "item.started", "item.updated", "item.completed":
		p.handleItem(event.Type, event.Item)

	case "error":
		p.state.mu.Lock()
		p.state.Status = StatusError
		p.state.ErrorMsg = event.Error
		p.state.mu.Unlock()
		p.notify()
	}
}

func (p *CodexStreamParser) handleThreadStarted(threadID string) {
	p.state.mu.Lock()
	p.state.SessionID = threadID
	p.state.StartedAt = time.Now()
	p.state.Status = StatusStarting
	p.state.mu.Unlock()
	p.notify()
}

func (p *CodexStreamParser) handleItem(eventType string, itemData json.RawMessage) {
	var item struct {
		ID               string `json:"id"`
		Type             string `json:"type"`
		Text             string `json:"text"`              // agent_message
		Command          string `json:"command"`           // command_execution
		AggregatedOutput string `json:"aggregated_output"` // command_execution
		ExitCode         *int   `json:"exit_code"`         // command_execution
		Status           string `json:"status"`
		// For file_change
		Path   string `json:"path"`
		Action string `json:"action"` // create, modify, delete
		// For reasoning
		Reasoning string `json:"reasoning"`
	}

	if err := json.Unmarshal(itemData, &item); err != nil {
		return
	}

	switch item.Type {
	case "agent_message":
		if eventType == "item.completed" {
			p.state.mu.Lock()
			if p.state.Output.Len() > 0 {
				p.state.Output.WriteString("\n\n")
			}
			p.state.Output.WriteString(item.Text)
			p.state.Status = StatusWriting
			p.state.mu.Unlock()
			p.notify()
		}

	case "reasoning":
		if item.Reasoning != "" || item.Text != "" {
			p.state.mu.Lock()
			text := item.Reasoning
			if text == "" {
				text = item.Text
			}
			p.state.Thinking.WriteString(text)
			p.state.Status = StatusThinking
			p.state.mu.Unlock()
			p.notify()
		}

	case "command_execution":
		p.handleToolItem(eventType, item.ID, "command", item.Command, item.AggregatedOutput, item.ExitCode)

	case "file_change":
		input := item.Action + ": " + item.Path
		p.handleToolItem(eventType, item.ID, "file", input, "", nil)

	case "mcp_tool_call":
		p.handleToolItem(eventType, item.ID, "mcp", "", "", nil)

	case "web_search":
		p.handleToolItem(eventType, item.ID, "web_search", "", "", nil)
	}
}

func (p *CodexStreamParser) handleToolItem(eventType, id, name, input, output string, exitCode *int) {
	switch eventType {
	case "item.started":
		activity := &ToolActivity{
			ID:        id,
			Name:      name,
			Input:     input,
			StartedAt: time.Now(),
		}
		p.activeItems[id] = activity

		p.state.mu.Lock()
		p.state.ActiveTool = activity
		p.state.Status = StatusToolUse
		p.state.mu.Unlock()
		p.notify()

	case "item.updated":
		if activity, ok := p.activeItems[id]; ok {
			if output != "" {
				activity.Output = output
			}
			p.state.mu.Lock()
			p.state.ActiveTool = activity
			p.state.mu.Unlock()
			p.notify()
		}

	case "item.completed":
		if activity, ok := p.activeItems[id]; ok {
			activity.Duration = time.Since(activity.StartedAt)
			if output != "" {
				activity.Output = output
			}
			if exitCode != nil && *exitCode != 0 {
				activity.IsError = true
			}

			p.state.mu.Lock()
			p.state.ToolHistory = append(p.state.ToolHistory, *activity)
			p.state.ActiveTool = nil
			p.state.mu.Unlock()

			delete(p.activeItems, id)
			p.notify()
		}
	}
}

func (p *CodexStreamParser) notify() {
	if p.onUpdate != nil {
		p.onUpdate()
	}
}
```

## Configuration

Add to `.ticker/config.json`:

```json
{
  "agent": {
    "type": "codex",
    "codex": {
      "model": "o4-mini",
      "full_auto": false
    }
  }
}
```

Or via CLI flag:

```bash
ticker run <epic-id> --agent codex
ticker run <epic-id> --agent codex --codex-model gpt-4
```

## Testing

### Unit Tests

```go
func TestCodexAgent_Available(t *testing.T) {
	agent := NewCodexAgent()
	// Will be false if codex not installed
	_ = agent.Available()
}

func TestCodexStreamParser_ThreadStarted(t *testing.T) {
	state := &AgentState{}
	parser := NewCodexStreamParser(state, nil)

	input := `{"type":"thread.started","thread_id":"test-123"}`
	parser.parseLine([]byte(input))

	assert.Equal(t, "test-123", state.SessionID)
	assert.Equal(t, StatusStarting, state.Status)
}

func TestCodexStreamParser_CommandExecution(t *testing.T) {
	state := &AgentState{}
	parser := NewCodexStreamParser(state, nil)

	// Start
	parser.parseLine([]byte(`{"type":"item.started","item":{"id":"1","type":"command_execution","command":"ls","status":"in_progress"}}`))
	assert.Equal(t, StatusToolUse, state.Status)
	assert.NotNil(t, state.ActiveTool)

	// Complete
	parser.parseLine([]byte(`{"type":"item.completed","item":{"id":"1","type":"command_execution","command":"ls","aggregated_output":"file.go","exit_code":0,"status":"completed"}}`))
	assert.Nil(t, state.ActiveTool)
	assert.Len(t, state.ToolHistory, 1)
}

func TestCodexStreamParser_TurnCompleted(t *testing.T) {
	state := &AgentState{}
	parser := NewCodexStreamParser(state, nil)

	parser.parseLine([]byte(`{"type":"turn.completed","usage":{"input_tokens":1000,"output_tokens":100}}`))

	assert.Equal(t, StatusComplete, state.Status)
	assert.Equal(t, 1000, state.Metrics.InputTokens)
	assert.Equal(t, 100, state.Metrics.OutputTokens)
}
```

### Integration Test

```bash
# Manual test with real codex
echo '{"type":"thread.started","thread_id":"test"}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"Hello!"}}
{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}' | go run ./cmd/test-codex-parser
```

## Signal Detection

Codex agent output needs to be scanned for Ticker's signal protocol:

| Signal | Pattern |
|--------|---------|
| COMPLETE | `<promise>COMPLETE</promise>` |
| EJECT | `<promise>EJECT: reason</promise>` |
| BLOCKED | `<promise>BLOCKED: reason</promise>` |

The engine's existing signal detection in `engine.go` will work unchanged since it operates on the final `Result.Output` string.

## Cost Estimation

Codex doesn't provide cost in the JSON stream. We need to calculate it:

```go
// Approximate costs (update as pricing changes)
var codexCosts = map[string]struct{ input, output float64 }{
	"o4-mini":  {0.00015, 0.0006},   // per 1K tokens
	"gpt-4":    {0.03, 0.06},
	"gpt-4o":   {0.005, 0.015},
}

func estimateCost(model string, inputTokens, outputTokens int) float64 {
	costs, ok := codexCosts[model]
	if !ok {
		costs = codexCosts["o4-mini"] // default
	}
	return (float64(inputTokens) * costs.input / 1000) +
	       (float64(outputTokens) * costs.output / 1000)
}
```

## Implementation Checklist

- [ ] Create `internal/agent/codex.go` with `CodexAgent` struct
- [ ] Create `internal/agent/codex_stream.go` with `CodexStreamParser`
- [ ] Add unit tests in `internal/agent/codex_test.go`
- [ ] Add agent selection to config (`agent.type`)
- [ ] Add `--agent` CLI flag to `cmd/ticker/main.go`
- [ ] Update engine to support agent selection
- [ ] Add cost estimation for Codex models
- [ ] Test with real Codex CLI
- [ ] Update documentation

## Estimated Effort

| Task | Estimate |
|------|----------|
| CodexAgent implementation | ~50 lines |
| CodexStreamParser | ~150 lines |
| Tests | ~100 lines |
| Config/CLI integration | ~50 lines |
| **Total** | **~350 lines** |

**Time**: 2-4 hours for full implementation with tests.

## References

- [Codex CLI Reference](https://developers.openai.com/codex/cli/reference/)
- [Non-interactive Mode](https://developers.openai.com/codex/noninteractive/)
- [GitHub: openai/codex](https://github.com/openai/codex)
- [Codex GitHub Action](https://developers.openai.com/codex/github-action/)
