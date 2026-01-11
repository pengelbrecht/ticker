# Ticker: A Go-based Ralph Runner

> Autonomous AI agent loop runner with a beautiful TUI, multi-agent support, and deep Ticks integration.

## Overview

Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents in continuous loops until tasks are complete. It wraps the [Ticks](https://github.com/pengelbrecht/ticks) issue tracker and orchestrates coding agents (Claude Code, Codex, Gemini CLI, Amp, OpenCode) to autonomously complete epics.

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
│   │   ├── claude.go        # Claude Code backend
│   │   ├── codex.go         # OpenAI Codex backend
│   │   ├── gemini.go        # Gemini CLI backend
│   │   ├── amp.go           # Sourcegraph Amp backend
│   │   ├── opencode.go      # OpenCode backend
│   │   ├── registry.go      # Auto-detect available agents
│   │   └── permissions.go   # Permission mode abstraction
│   │
│   ├── permissions/
│   │   ├── modes.go         # Permission mode definitions
│   │   ├── policy.go        # Allow/deny rule engine
│   │   ├── sandbox.go       # Sandbox configuration per agent
│   │   └── audit.go         # Permission decision logging
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

| Agent | CLI Command | Detection | Native Permissions |
|-------|-------------|-----------|-------------------|
| Claude Code | `claude` | `which claude` | normal, auto-accept, plan, yolo |
| Codex | `codex` | `which codex` | read-only, auto, full-access, yolo |
| Gemini CLI | `gemini` | `which gemini` | permissive-open, strict, yolo |
| Amp | `amp` | `which amp` | (uses Claude permissions) |
| OpenCode | `opencode` | `which opencode` | (provider-dependent) |

Agent registry auto-detects available agents and allows selection via flag or TUI.

### Agent Details

**Claude Code** (Anthropic)
- Primary agent, most mature Ralph support
- Modes: `--dangerously-skip-permissions` for full autonomy
- Streaming output, MCP support, subagents

**Codex** (OpenAI)
- Powered by GPT-5.2-Codex (o3 variant optimized for coding)
- Sandbox: Seatbelt (macOS), Landlock+seccomp (Linux)
- `--full-auto` for low-friction mode, `--yolo` for no restrictions

**Gemini CLI** (Google)
- ReAct loop with built-in tools
- Sandbox profiles: permissive-open (default), strict, custom
- `--yolo` mode for trusted workspaces

**Amp** (Sourcegraph)
- Built on Claude Opus 4.5, up to 200k context
- Thread persistence across sessions
- Subagent spawning for parallel subtasks

**OpenCode** (Open Source)
- Provider-agnostic (Claude, OpenAI, Google, local models)
- Built-in Plan and Build agents
- 650k+ monthly active developers

## Permission Management

Unlike the original ralph-ticker bash script which uses `--dangerously-skip-permissions` exclusively, Ticker provides granular permission control across all supported agents.

### Permission Modes

Ticker abstracts each agent's native permission system into a unified model:

```go
type PermissionMode int

const (
    // PermissionInteractive - prompt for every potentially dangerous operation
    // Maps to: Claude normal, Codex read-only, Gemini strict
    PermissionInteractive PermissionMode = iota

    // PermissionAutoEdit - auto-approve file edits, prompt for shell/network
    // Maps to: Claude auto-accept, Codex auto, Gemini permissive-open
    PermissionAutoEdit

    // PermissionAutoAll - auto-approve edits and shell within workspace
    // Maps to: Codex full-access (workspace-scoped)
    PermissionAutoAll

    // PermissionYOLO - no restrictions, full autonomy
    // Maps to: Claude --dangerously-skip-permissions, Codex --yolo, Gemini --yolo
    PermissionYOLO
)
```

### Agent Permission Mapping

| Ticker Mode | Claude Code | Codex | Gemini CLI |
|-------------|-------------|-------|------------|
| `interactive` | normal | read-only | strict |
| `auto-edit` | auto-accept edits | auto | permissive-open |
| `auto-all` | (not native) | full-access | permissive-open |
| `yolo` | --dangerously-skip-permissions | --yolo | --yolo |

### Permission Policies

Fine-grained control via allow/deny rules:

```go
type PermissionPolicy struct {
    // Explicit denials (checked first)
    Deny []PermissionRule `json:"deny"`

    // Explicit allows (checked second)
    Allow []PermissionRule `json:"allow"`

    // Default for unmatched operations
    Default PermissionDecision `json:"default"` // "ask", "allow", "deny"
}

type PermissionRule struct {
    // What operation type
    Operation string `json:"operation"` // "file_write", "file_read", "shell", "network"

    // Path patterns (glob)
    Paths []string `json:"paths,omitempty"` // ["src/**", "!src/secrets/**"]

    // Command patterns (for shell)
    Commands []string `json:"commands,omitempty"` // ["go *", "npm *", "!rm -rf *"]

    // Network patterns
    Hosts []string `json:"hosts,omitempty"` // ["github.com", "*.googleapis.com"]
}
```

### Sandbox Configuration

Per-agent sandbox settings:

```go
type SandboxConfig struct {
    // Filesystem restrictions
    WorkspaceOnly   bool     `json:"workspace_only"`   // Restrict writes to workspace
    AllowedPaths    []string `json:"allowed_paths"`    // Additional write paths
    DeniedPaths     []string `json:"denied_paths"`     // Explicit denials

    // Network restrictions
    NetworkEnabled  bool     `json:"network_enabled"`
    AllowedHosts    []string `json:"allowed_hosts"`    // Whitelist
    DeniedHosts     []string `json:"denied_hosts"`     // Blacklist

    // Process restrictions
    AllowedCommands []string `json:"allowed_commands"` // Command whitelist
    DeniedCommands  []string `json:"denied_commands"`  // Command blacklist
    MaxProcessTime  Duration `json:"max_process_time"` // Per-command timeout
}
```

### CLI Permission Flags

```bash
# Permission modes
ticker run h8d --permission interactive  # Prompt for everything
ticker run h8d --permission auto-edit    # Auto-approve edits only
ticker run h8d --permission auto-all     # Auto-approve edits + workspace shell
ticker run h8d --permission yolo         # Full autonomy (use with caution)

# Fine-grained overrides
ticker run h8d --allow-network           # Enable network access
ticker run h8d --allow-paths "/tmp,/var/cache"
ticker run h8d --deny-commands "rm -rf,sudo"

# Sandbox presets
ticker run h8d --sandbox strict          # Maximum restrictions
ticker run h8d --sandbox permissive      # Reasonable defaults
ticker run h8d --sandbox none            # No sandbox (yolo mode)
```

### Configuration File

```json
{
  "permissions": {
    "default_mode": "auto-edit",
    "policy": {
      "deny": [
        {"operation": "file_write", "paths": ["**/.env", "**/secrets/**"]},
        {"operation": "shell", "commands": ["rm -rf /*", "sudo *"]},
        {"operation": "network", "hosts": ["*.malware.com"]}
      ],
      "allow": [
        {"operation": "file_write", "paths": ["src/**", "test/**", "docs/**"]},
        {"operation": "shell", "commands": ["go *", "npm *", "git *", "tk *"]},
        {"operation": "network", "hosts": ["github.com", "pkg.go.dev"]}
      ],
      "default": "ask"
    },
    "sandbox": {
      "workspace_only": true,
      "network_enabled": false,
      "max_process_time": "5m"
    }
  },
  "agents": {
    "claude": {
      "permission_flags": []
    },
    "codex": {
      "sandbox_mode": "workspace-write",
      "approval_mode": "on-request"
    },
    "gemini": {
      "sandbox_profile": "permissive-open"
    }
  }
}
```

### Permission Audit Log

All permission decisions are logged for review:

```go
type PermissionEvent struct {
    Timestamp   time.Time         `json:"timestamp"`
    Iteration   int               `json:"iteration"`
    EpicID      string            `json:"epic_id"`
    Agent       string            `json:"agent"`
    Operation   string            `json:"operation"`
    Target      string            `json:"target"`      // file path, command, URL
    Decision    PermissionDecision `json:"decision"`   // allowed, denied, asked
    Reason      string            `json:"reason"`      // which rule matched
    UserAction  string            `json:"user_action"` // if asked: approved/rejected
}
```

Log stored at `.ticker/audit.jsonl`:

```jsonl
{"timestamp":"2025-01-11T10:23:45Z","iteration":3,"epic_id":"h8d","agent":"claude","operation":"file_write","target":"src/worker/pool.go","decision":"allowed","reason":"matches allow rule: src/**"}
{"timestamp":"2025-01-11T10:24:12Z","iteration":3,"epic_id":"h8d","agent":"claude","operation":"shell","target":"go test ./...","decision":"allowed","reason":"matches allow rule: go *"}
{"timestamp":"2025-01-11T10:25:03Z","iteration":3,"epic_id":"h8d","agent":"claude","operation":"network","target":"api.github.com","decision":"asked","reason":"no matching rule","user_action":"approved"}
```

### TUI Permission View (`a` key)

```
┌─────────────────────────────────────────────────────────────────┐
│ PERMISSIONS                                          Mode: auto-edit │
├─────────────────────────────────────────────────────────────────┤
│ RECENT DECISIONS (last 10)                                      │
│ ───────────────────────────                                     │
│ ✓ file_write  src/worker/pool.go           allowed (rule)       │
│ ✓ shell       go test ./...                allowed (rule)       │
│ ? network     api.github.com               asked → approved     │
│ ✓ file_write  src/worker/queue.go          allowed (rule)       │
│ ✗ file_write  .env.local                   denied (rule)        │
│ ✓ shell       git commit -m "..."          allowed (rule)       │
│                                                                 │
│ PENDING APPROVAL                                                │
│ ────────────────                                                │
│ (none)                                                          │
│                                                                 │
│ STATS: 47 allowed | 2 denied | 3 asked                          │
└─────────────────────────────────────────────────────────────────┘
 m change mode  p edit policy  e export audit  esc back
```

### Safety Recommendations

| Context | Recommended Mode | Rationale |
|---------|------------------|-----------|
| Trusted personal project | `auto-all` | Fast iteration, low risk |
| Team codebase | `auto-edit` | Protect shell access |
| Open source contribution | `interactive` | Maximum oversight |
| CI/CD pipeline | `yolo` + container | Isolated environment |
| Production-adjacent | `interactive` | Safety first |

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

Agents have different pricing models. Ticker tracks token usage and estimates costs where possible:

| Agent | Pricing Model | Cost Tracking |
|-------|---------------|---------------|
| Claude Code | API usage or Max subscription | Per-token estimates |
| Codex | ChatGPT Plus/Pro subscription | Per-token estimates |
| Gemini CLI | Free tier / API usage | Per-token estimates |
| Amp | Pay-as-you-go or $10/day | Per-token estimates |
| OpenCode | Provider-dependent | Provider-dependent |

**Underlying Model Costs (for estimation):**

| Model | Input (per 1M) | Output (per 1M) |
|-------|----------------|-----------------|
| claude-sonnet-4 | $3.00 | $15.00 |
| claude-opus-4.5 | $15.00 | $75.00 |
| gpt-5.2-codex | ~$5.00 | ~$20.00 |
| gemini-2.5-pro | $1.25 | $10.00 |

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
# Interactive TUI mode - browse and select epics to run
ticker

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
ticker run h8d --agent codex
ticker run h8d --agent gemini
ticker run h8d --agent amp
ticker run h8d --agent opencode
```

### Interactive Mode (`ticker` with no args)

Launches full TUI for epic selection and management:

```
┌─────────────────────────────────────────────────────────────────┐
│ TICKER                                         Agent: claude    │
├─────────────────────────────────────────────────────────────────┤
│ SELECT EPICS TO RUN                                             │
│ ───────────────────                                             │
│                                                                 │
│   [ ] h8d  Parallel test execution       P2  5 tasks  ready    │
│   [x] fbv  Bug Fixes & Documentation     P1  3 tasks  ready    │
│   [ ] 5b8  Benchmark Suite               P2  8 tasks  blocked  │
│   [ ] hhm  Homebrew distribution         P2  2 tasks  ready    │
│                                                                 │
│ SETTINGS                                                        │
│ ────────                                                        │
│   Max iterations: 50        Permission: auto-edit               │
│   Max cost: $20.00          Parallel: 3                         │
│   Worktrees: enabled        Agent: claude                       │
│                                                                 │
│ READY: 1 epic selected (fbv)                                    │
└─────────────────────────────────────────────────────────────────┘
 space toggle  a select all  enter start  s settings  q quit
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
    "codex": {
      "sandbox_mode": "workspace-write",
      "approval_mode": "on-request"
    },
    "gemini": {
      "sandbox_profile": "permissive-open"
    },
    "amp": {
      "model": "claude-opus-4-5"
    },
    "opencode": {
      "provider": "anthropic"
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

### Permissions

21. **Permission mode switching**: Allow changing mode mid-run via TUI?
22. **Cross-agent consistency**: How to handle when agent doesn't support a permission feature?
23. **Rule inheritance**: Should project rules inherit from user-level rules?
24. **Approval timeout**: How long to wait for user approval before skipping/aborting?
25. **Batch approvals**: Allow approving patterns ("always allow go test")?

### Security

26. **Secrets in scratchpad**: Warn if API keys detected in scratchpad?
27. **Sandbox enforcement**: Can Ticker enforce sandbox when agent doesn't support it?
28. **Audit log retention**: How long to keep audit logs? Auto-rotate?
29. **Container mode**: Built-in Docker/container isolation option?

## References

### Core
- [Ticks](https://github.com/pengelbrecht/ticks) - Git-backed issue tracker
- [ralph-ticker](https://github.com/user/ralph-patterns) - Original bash implementation
- [Ralph Wiggum Technique](https://ghuntley.com/ralph/) - Origin of the pattern

### Ralph Implementations
- [ralph-orchestrator](https://github.com/mikeyobrien/ralph-orchestrator) - Python implementation
- [vercel ralph-loop-agent](https://github.com/vercel-labs/ralph-loop-agent) - AI SDK implementation

### Coding Agents
- [Claude Code](https://claude.ai/code) - Anthropic's coding agent
- [Codex CLI](https://github.com/openai/codex) - OpenAI's coding agent
- [Gemini CLI](https://developers.google.com/gemini-code-assist/docs/gemini-cli) - Google's coding agent
- [Amp](https://ampcode.com/) - Sourcegraph's coding agent
- [OpenCode](https://github.com/opencode-ai/opencode) - Open source coding agent

### TUI
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Go TUI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - TUI styling

## License

MIT
