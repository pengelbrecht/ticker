# Ticker: A Go-based Ralph Runner

> Autonomous AI agent loop runner with a beautiful TUI, multi-agent support, and deep Ticks integration.

## Overview

Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents in continuous loops until tasks are complete. It wraps the [Ticks](https://github.com/pengelbrecht/ticks) issue tracker and orchestrates AI agents (Claude, Gemini, Ollama) to autonomously complete epics.

### Core Philosophy

- **TUI-first**: Beautiful terminal interface is a must-have, not an afterthought
- **Multi-agent**: Pluggable backends for different AI providers
- **Ticks-native**: Deep integration with `tk` CLI, not a generic task abstraction
- **Parallel execution**: Multiple epics can run simultaneously in git worktrees

## Architecture

```
ticker/
├── cmd/ticker/main.go
├── internal/
│   ├── engine/
│   │   ├── engine.go        # Core Ralph loop orchestration
│   │   ├── iteration.go     # Single iteration lifecycle
│   │   ├── signals.go       # COMPLETE/EJECT/BLOCKED detection
│   │   ├── verification.go  # Post-iteration verification hooks
│   │   ├── prompt.go        # Prompt builder
│   │   └── recovery.go      # Error handling & retry logic
│   │
│   ├── agent/
│   │   ├── agent.go         # Interface: Run(prompt) -> (output, tokens, error)
│   │   ├── claude.go        # Claude CLI backend
│   │   ├── gemini.go        # Gemini CLI backend
│   │   ├── ollama.go        # Local models via Ollama
│   │   └── registry.go      # Auto-detect available agents
│   │
│   ├── ticks/
│   │   ├── client.go        # Wrap `tk` CLI calls
│   │   ├── epic.go          # Epic operations
│   │   ├── task.go          # Task operations
│   │   └── deps.go          # Dependency graph queries
│   │
│   ├── budget/
│   │   ├── limits.go        # Iteration/token/cost/time limits
│   │   ├── tracker.go       # Running totals
│   │   └── pricing.go       # Model cost tables
│   │
│   ├── checkpoint/
│   │   ├── checkpoint.go    # Checkpoint data structures
│   │   ├── git.go           # Git-based snapshots
│   │   ├── scratchpad.go    # .ticker/scratchpad.md persistence
│   │   └── resume.go        # Resume from checkpoint
│   │
│   ├── worktree/
│   │   ├── manager.go       # Git worktree lifecycle
│   │   ├── parallel.go      # Parallel epic execution
│   │   └── merge.go         # Merge completed worktrees
│   │
│   └── tui/
│       ├── app.go           # Main Bubbletea app
│       ├── views/
│       │   ├── dashboard.go # Main split-pane view
│       │   ├── progress.go  # Epic/task progress bars
│       │   ├── output.go    # Streaming agent output
│       │   ├── budget.go    # Cost/token meters
│       │   ├── log.go       # Scrollable history
│       │   ├── graph.go     # Dependency visualization
│       │   └── parallel.go  # Multi-worktree status view
│       ├── keys.go          # Keybindings
│       └── styles.go        # Lipgloss theming
│
└── pkg/
    └── prompt/
        ├── builder.go       # Construct iteration prompts
        └── templates/       # Prompt templates
```

## TUI Design

### Main Dashboard Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ ticker v0.1.0         Epic: h8d (Parallel test execution)       │
│ Agent: claude         Iteration: 7/50      Cost: $2.34          │
├─────────────────────────────┬───────────────────────────────────┤
│ TASKS                       │ AGENT OUTPUT                      │
│ ─────                       │ ──────────────                    │
│ ✓ iei Browser pool impl     │ Reading src/worker/pool.go...     │
│ ✓ klm CLI --parallel flag   │                                   │
│ ● mqp Worker pool [blocked] │ I'll implement the job queue      │
│ → e4m Tests for parallel    │ using a buffered channel...       │
│   zn9 Benchmark suite       │                                   │
│                             │ ```go                             │
│ BUDGET                      │ type WorkerPool struct {          │
│ ──────                      │     jobs chan Job                 │
│ Iterations: ████████░░ 7/50 │     results chan Result           │
│ Tokens:     ███░░░░░░░ 34k  │ }                                 │
│ Cost:       ██░░░░░░░░ $2   │ ```                               │
│ Time:       █████░░░░░ 12m  │                                   │
├─────────────────────────────┴───────────────────────────────────┤
│ [7] Completed e4m: Added 12 tests for parallel execution        │
│ [6] Completed klm: CLI now accepts --parallel N flag            │
│ [5] Completed iei: Browser pool with configurable size          │
└─────────────────────────────────────────────────────────────────┘
 q quit  p pause  r resume  v verbose  g graph  w worktrees  ? help
```

### Parallel Worktrees View (`w` key)

```
┌─────────────────────────────────────────────────────────────────┐
│ PARALLEL EPICS (3 active)                                       │
├─────────────────────────────────────────────────────────────────┤
│ ▶ h8d  Parallel test execution    iter 7/50   $2.34  ████████░░ │
│   ├─ worktree: .ticker-worktrees/h8d                            │
│   ├─ branch: ticker/h8d                                         │
│   └─ current: e4m (Tests for parallel)                          │
│                                                                 │
│ ▶ fbv  Bug Fixes & Documentation  iter 3/20   $0.89  ██░░░░░░░░ │
│   ├─ worktree: .ticker-worktrees/fbv                            │
│   ├─ branch: ticker/fbv                                         │
│   └─ current: gjq (Fix null pointer)                            │
│                                                                 │
│ ▶ 5b8  Benchmark Suite            iter 12/30  $4.12  ██████░░░░ │
│   ├─ worktree: .ticker-worktrees/5b8                            │
│   ├─ branch: ticker/5b8                                         │
│   └─ current: zn9 (Playwright comparison)                       │
├─────────────────────────────────────────────────────────────────┤
│ TOTAL: 22 iterations | $7.35 | 3 epics in progress              │
└─────────────────────────────────────────────────────────────────┘
 1-9 focus epic  m merge completed  k kill epic  + add epic  esc back
```

### Dependency Graph View (`g` key)

```
┌─────────────────────────────────────────────────────────────────┐
│ DEPENDENCY GRAPH: h8d (Parallel test execution)                 │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ┌─────────┐                                                   │
│   │ h8d     │ ← EPIC                                            │
│   │ (epic)  │                                                   │
│   └────┬────┘                                                   │
│        │                                                        │
│   ┌────┴────┬──────────┬──────────┐                             │
│   ▼         ▼          ▼          ▼                             │
│ ┌───┐    ┌───┐     ┌───┐      ┌───┐                             │
│ │iei│✓   │klm│✓    │e4m│→     │zn9│                             │
│ └─┬─┘    └───┘     └───┘      └───┘                             │
│   │                                                             │
│   ▼                                                             │
│ ┌───┐                                                           │
│ │mqp│● blocked                                                  │
│ └───┘                                                           │
│                                                                 │
│ Legend: ✓ done  → in progress  ● blocked  ○ pending             │
└─────────────────────────────────────────────────────────────────┘
```

## Engine Design

### Core Loop

```go
type Engine struct {
    agent      agent.Agent
    ticks      *ticks.Client
    budget     *budget.Tracker
    checkpoint *checkpoint.Manager
    verify     []Verifier
    scratchpad *Scratchpad

    // Callbacks for TUI
    onIterationStart  func(IterationContext)
    onIterationEnd    func(IterationResult)
    onOutputStream    func(chunk string)
    onSignal          func(Signal, string)
}

type RunConfig struct {
    EpicID          string
    MaxIterations   int
    MaxTokens       int
    MaxCost         float64
    MaxDuration     time.Duration
    CheckpointEvery int
    Worktree        bool
    ResumeFrom      string
}
```

### Iteration Lifecycle

```
┌──────────────────────────────────────────────────────────────┐
│                      ITERATION LIFECYCLE                     │
│                                                              │
│  1. tk next <epic> ─────────────► Get next unblocked task    │
│  2. Gather context ─────────────► Epic, scratchpad, notes    │
│  3. Build prompt ───────────────► Inject all context         │
│  4. agent.Run(prompt) ──────────► Stream output to TUI       │
│  5. Parse signals ──────────────► COMPLETE/EJECT/BLOCKED     │
│  6. Run verifiers ──────────────► Tests, build, git, tick    │
│  7. Update scratchpad ──────────► Persist learnings          │
│  8. Checkpoint if interval ─────► Git commit + JSON state    │
│  9. Loop or exit ───────────────► Based on signal/budget     │
└──────────────────────────────────────────────────────────────┘
```

### Signal Protocol

| Signal | XML Tag | Meaning | Exit Code |
|--------|---------|---------|-----------|
| COMPLETE | `<promise>COMPLETE</promise>` | All tasks done | 0 |
| EJECT | `<promise>EJECT: reason</promise>` | Large install needed | 2 |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Missing credentials/unclear | 3 |
| PAUSED | (user action) | User paused execution | - |
| MAX_ITERATIONS | (internal) | Hit iteration limit | 1 |
| BUDGET_EXCEEDED | (internal) | Hit token/cost/time limit | 1 |

### Verification System

```go
type Verifier interface {
    Name() string
    Verify(ctx context.Context, task *ticks.Task, output string) error
}
```

Built-in verifiers:
- **TestVerifier**: Runs test suite (configurable command)
- **BuildVerifier**: Checks project builds
- **GitVerifier**: Ensures changes were committed
- **TickVerifier**: Confirms task was closed in Ticks
- **ScriptVerifier**: Custom verification scripts

Verification failures inject feedback into scratchpad for next iteration.

## Agent Interface

```go
type Agent interface {
    Name() string
    Available() bool
    Run(ctx context.Context, prompt string, opts RunOpts) (*Result, error)
}

type RunOpts struct {
    Stream    chan<- string  // For TUI streaming
    MaxTokens int
    Timeout   time.Duration
}

type Result struct {
    Output      string
    TokensIn    int
    TokensOut   int
    Cost        float64
    Duration    time.Duration
    Signal      Signal
    SignalReason string
}
```

### Supported Agents

| Agent | Backend | Detection |
|-------|---------|-----------|
| Claude | `claude` CLI | `which claude` |
| Gemini | `gemini` CLI | `which gemini` |
| Ollama | HTTP API | `curl localhost:11434` |

Agent registry auto-detects available agents and allows selection via flag or TUI.

## Parallel Worktree Execution

### Architecture

```
project/
├── .ticker-worktrees/
│   ├── h8d/              # Worktree for epic h8d
│   │   └── (full repo)
│   ├── fbv/              # Worktree for epic fbv
│   │   └── (full repo)
│   └── 5b8/              # Worktree for epic 5b8
│       └── (full repo)
├── .ticker/
│   ├── config.json
│   ├── scratchpad.md
│   └── checkpoints/
└── (main repo)
```

### Worktree Lifecycle

1. **Create**: `git worktree add .ticker-worktrees/<epic> -b ticker/<epic>`
2. **Run**: Engine runs in worktree directory, isolated from main
3. **Checkpoint**: Each worktree maintains independent checkpoints
4. **Complete**: On COMPLETE signal, merge back to main
5. **Cleanup**: Remove worktree and branch after successful merge

### Parallel Manager

```go
type ParallelManager struct {
    worktrees map[string]*Worktree
    engines   map[string]*Engine
    maxParallel int
}

func (m *ParallelManager) Start(epicID string) error
func (m *ParallelManager) Stop(epicID string) error
func (m *ParallelManager) Merge(epicID string) error
func (m *ParallelManager) Status() []WorktreeStatus
```

### Resource Management

- Configurable max parallel epics (default: 3)
- Shared budget tracker across all epics
- Per-epic and global iteration limits
- TUI shows aggregate cost/tokens

## Checkpointing & Recovery

### Checkpoint Data

```go
type Checkpoint struct {
    ID             string    `json:"id"`
    Timestamp      time.Time `json:"timestamp"`
    EpicID         string    `json:"epic_id"`
    Iteration      int       `json:"iteration"`
    TotalTokens    int       `json:"total_tokens"`
    TotalCost      float64   `json:"total_cost"`
    CompletedTasks []string  `json:"completed_tasks"`
    Scratchpad     string    `json:"scratchpad"`
    GitCommit      string    `json:"git_commit"`
}
```

### Storage

```
.ticker/
├── checkpoints/
│   ├── h8d-7.json
│   ├── h8d-14.json
│   └── fbv-3.json
└── scratchpad.md
```

### Resume Flow

1. Load checkpoint JSON
2. Restore git state: `git checkout <commit>`
3. Restore scratchpad content
4. Resume engine from iteration N

## Budget Management

### Limits

```go
type Limits struct {
    MaxIterations int
    MaxTokens     int
    MaxCost       float64
    MaxDuration   time.Duration
}
```

### Pricing Table

| Model | Input (per 1M) | Output (per 1M) |
|-------|----------------|-----------------|
| claude-sonnet-4 | $3.00 | $15.00 |
| claude-opus-4 | $15.00 | $75.00 |
| gemini-2.0-flash | $0.10 | $0.40 |
| gemini-2.5-pro | $1.25 | $10.00 |
| ollama/* | $0.00 | $0.00 |

### Budget Callbacks

```go
type BudgetCallbacks struct {
    OnWarning  func(metric string, percent float64)  // 80% threshold
    OnCritical func(metric string, percent float64)  // 95% threshold
    OnExceeded func(metric string)                   // 100% - stop
}
```

## CLI Interface

```bash
# Run single epic
ticker run h8d --max-iterations 50 --max-cost 10.00

# Run single epic in worktree
ticker run h8d --worktree

# Run multiple epics in parallel
ticker run h8d fbv 5b8 --parallel 3

# Auto-select mode (pick highest priority ready epic)
ticker run --auto --max-iterations 100

# Resume from checkpoint
ticker resume h8d-14

# List checkpoints
ticker checkpoints

# Show status of running epics
ticker status

# Headless mode (no TUI, for CI)
ticker run h8d --headless --json-output

# Agent selection
ticker run h8d --agent gemini
ticker run h8d --agent ollama:llama3.2
```

## Configuration

### `.ticker/config.json`

```json
{
  "default_agent": "claude",
  "max_parallel": 3,
  "checkpoint_interval": 5,
  "budget": {
    "max_iterations": 50,
    "max_cost": 20.00,
    "max_tokens": 500000
  },
  "verification": {
    "test_command": "go test ./...",
    "build_command": "go build ./...",
    "custom_scripts": []
  },
  "agents": {
    "claude": {
      "flags": ["--dangerously-skip-permissions"]
    },
    "ollama": {
      "model": "llama3.2",
      "endpoint": "http://localhost:11434"
    }
  }
}
```

## Error Handling

### Retry Strategy

| Error Type | Strategy | Max Retries |
|------------|----------|-------------|
| Network timeout | Exponential backoff | 3 |
| Rate limit | Backoff with jitter | 5 |
| Agent refusal | Skip task, log warning | 0 |
| Build failure | Inject feedback, retry | 2 |
| Test failure | Inject feedback, retry | 2 |

### Error Recovery

```go
type ErrorStrategy int

const (
    ErrorRetry ErrorStrategy = iota
    ErrorSkip
    ErrorAbort
)
```

## Open Questions

### TUI

1. **Split pane ratios**: Fixed or resizable? User preference?
2. **Color scheme**: Dark only? Light mode? Theme system?
3. **Log persistence**: Save TUI logs to file? Where?
4. **Notification sounds**: Beep on completion/error? Configurable?

### Engine

5. **Iteration timeout**: Per-iteration timeout (current: 5min)? Configurable?
6. **Scratchpad size limit**: Auto-summarize when too large? What threshold?
7. **Verification failure threshold**: How many failures before giving up on a task?
8. **Task skip behavior**: Should we support skipping stuck tasks automatically?

### Parallel Execution

9. **Resource contention**: How to handle if two epics try to modify same file?
10. **Merge conflicts**: Auto-resolve? Pause and alert? Configurable strategy?
11. **Shared vs isolated scratchpad**: Per-worktree or shared across epics?
12. **Cost allocation**: Track cost per-epic or only aggregate?

### Agents

13. **Agent fallback**: If primary agent fails, try secondary? Chain of agents?
14. **Model selection per task**: Different models for different task types?
15. **Context window management**: How to handle prompts approaching context limit?
16. **Streaming granularity**: Chunk size for TUI streaming? Buffer strategy?

### Integration

17. **Ticks version compatibility**: Minimum `tk` version required?
18. **Git hooks**: Should ticker install/manage git hooks?
19. **CI integration**: GitHub Actions workflow template? Exit codes?
20. **Telemetry**: Anonymous usage stats? Opt-in/out?

### Security

21. **Secrets in scratchpad**: Warn if API keys detected in scratchpad?
22. **Sandbox mode**: Run agent with restricted permissions option?
23. **Audit log**: Track all agent actions for review?

## References

- [Ticks](https://github.com/pengelbrecht/ticks) - Git-backed issue tracker
- [ralph-ticker](https://github.com/user/ralph-patterns) - Original bash implementation
- [Ralph Wiggum Technique](https://ghuntley.com/ralph/) - Origin of the pattern
- [ralph-orchestrator](https://github.com/mikeyobrien/ralph-orchestrator) - Python implementation
- [vercel ralph-loop-agent](https://github.com/vercel-labs/ralph-loop-agent) - AI SDK implementation
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Go TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - TUI styling

## License

MIT
