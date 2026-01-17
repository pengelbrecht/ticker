Now I have all the information I need to create the context document. Let me write it:

# Epic Context: Phase 1 - Core Engine

Pre-generated context for AI agents working on tasks in epic `cv5`.

---

## Relevant Code

### Project Structure (Existing)
The ticker project is already fully implemented beyond Phase 1. The structure:

```
ticker/
├── cmd/ticker/main.go          # CLI entry (84KB) - Cobra commands
├── internal/
│   ├── agent/                  # Agent interface + Claude impl
│   │   ├── agent.go            # Interface definition
│   │   ├── claude.go           # Claude Code backend
│   │   └── stream.go           # Stream JSON parser
│   ├── budget/                 # Budget tracking
│   │   ├── tracker.go          # Limits + usage tracking
│   │   └── pricing.go          # Model cost tables
│   ├── checkpoint/             # State persistence
│   │   └── checkpoint.go       # Save/load/list checkpoints
│   ├── engine/                 # Core Ralph loop
│   │   ├── engine.go           # Main orchestration (43KB)
│   │   ├── signals.go          # Signal detection
│   │   └── prompt.go           # Prompt builder
│   └── ticks/                  # Ticks CLI wrapper
│       ├── client.go           # tk command wrapper (25KB)
│       └── types.go            # Task/Epic structs
├── go.mod                      # github.com/pengelbrecht/ticker
└── SPEC.md                     # Full specification
```

### Key Interfaces

**Agent Interface** (`internal/agent/agent.go`):
```go
type Agent interface {
    Name() string
    Available() bool
    Run(ctx context.Context, prompt string, opts RunOpts) (*Result, error)
}

type RunOpts struct {
    Stream        chan<- string
    StateCallback func(state *AgentState)
    MaxTokens     int
    Timeout       time.Duration
    WorkDir       string
}

type Result struct {
    Output    string
    TokensIn  int
    TokensOut int
    Cost      float64
    Duration  time.Duration
}
```

**Ticks Client** (`internal/ticks/client.go`):
```go
type Client struct {
    Command string  // "tk" by default
}

func (c *Client) NextTask(epicID string) (*Task, error)
func (c *Client) GetEpic(epicID string) (*Epic, error)
func (c *Client) CloseTask(taskID, reason string) error
func (c *Client) AddNote(issueID, message string) error
func (c *Client) GetNotes(epicID string) ([]string, error)
```

**Signal Detection** (`internal/engine/signals.go`):
```go
type Signal int
const (
    SignalNone Signal = iota
    SignalComplete  // <promise>COMPLETE</promise>
    SignalEject     // <promise>EJECT: reason</promise>
    SignalBlocked   // <promise>BLOCKED: reason</promise>
)

func ParseSignals(output string) (Signal, string)
```

**Budget Tracker** (`internal/budget/tracker.go`):
```go
type Limits struct {
    MaxIterations int
    MaxTokens     int
    MaxCost       float64
    MaxDuration   time.Duration
}

type Tracker struct { /* ... */ }
func (t *Tracker) Add(epicID string, tokens int, cost float64) error
func (t *Tracker) Exceeded() (string, bool)
```

**Checkpoint Manager** (`internal/checkpoint/checkpoint.go`):
```go
type Checkpoint struct {
    ID             string
    EpicID         string
    Iteration      int
    TotalTokens    int
    TotalCost      float64
    CompletedTasks []string
    GitCommit      string
}

type Manager struct { dir string }
func (m *Manager) Save(cp *Checkpoint) error
func (m *Manager) Load(id string) (*Checkpoint, error)
func (m *Manager) List(epicID string) ([]*Checkpoint, error)
```

---

## Architecture Notes

### Engine Run Flow
1. `Engine.Run()` enters main loop
2. Calls `tk next <epic>` to get unblocked task
3. Builds prompt via `PromptBuilder.Build(IterationContext)`
4. Invokes `agent.Run()` with streaming to TUI callbacks
5. Parses output for signals via `ParseSignals()`
6. Checkpoints at configurable intervals
7. Exits on COMPLETE/EJECT/BLOCKED or budget exceeded

### Claude Agent Invocation
```bash
claude --dangerously-skip-permissions --print "<prompt>" --output-format stream-json
```
- Uses `--output-format stream-json` for structured streaming
- Parses JSON events from stdout in real-time
- Captures token counts from API response metadata

### Ticks Integration
All `tk` commands use `--json` flag for machine-readable output:
- `tk next <epic> --json` → returns next unblocked task
- `tk show <id> --json` → returns task/epic details
- `tk close <id> "reason"` → closes task
- `tk note <id> "message"` → adds iteration note

---

## External References

- **Cobra CLI**: `github.com/spf13/cobra` - CLI framework
- **tk CLI**: Ticks issue tracker - see CLAUDE.md for command reference
- **Claude Code**: `claude` binary with `--dangerously-skip-permissions`

---

## Testing Patterns

### Test File Locations
- `internal/engine/engine_test.go` - Core loop tests
- `internal/engine/signals_test.go` - Signal parsing tests
- `internal/ticks/client_test.go` - Ticks wrapper tests
- `internal/agent/claude_test.go` - Agent tests

### Mock Patterns
Tests use interface-based mocking:
```go
type MockAgent struct {
    RunFunc func(ctx context.Context, prompt string, opts RunOpts) (*Result, error)
}
```

### Running Tests
```bash
go test ./...                    # All tests
go test -v ./internal/engine/... # Verbose engine tests
```

---

## Conventions

### Error Handling
- Return `fmt.Errorf("context: %w", err)` for wrapping
- Check errors immediately after calls
- Use sentinel errors for expected conditions (e.g., `ErrNoTasks`)

### Logging
- Use callbacks (`OnOutput`, `OnSignal`) for TUI integration
- No direct stdout in library code

### Naming
- Files: `snake_case.go`
- Types: `PascalCase`
- Private: `camelCase`
- Test files: `*_test.go`

### JSON Parsing
- Use `encoding/json` with struct tags
- Define types in `types.go` files
- Handle `--json` flag output from `tk` commands