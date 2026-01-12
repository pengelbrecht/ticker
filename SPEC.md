# Ticker: A Go-based Ralph Runner

> Autonomous AI agent loop runner with a beautiful TUI, multi-agent support, and deep Ticks integration.

## Overview

Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents in continuous loops until tasks are complete. It wraps the [Ticks](https://github.com/pengelbrecht/ticks) issue tracker and orchestrates coding agents (Claude Code, Codex, Gemini CLI, Amp, OpenCode) to autonomously complete epics.

### Core Philosophy

- **TUI-first**: Beautiful terminal interface is a must-have, not an afterthought
- **Multi-agent**: Pluggable backends for different AI providers
- **Ticks-native**: Deep integration with `tk` CLI, not a generic task abstraction
- **Parallel execution**: Multiple epics can run simultaneously in git worktrees

## Roadmap

### Phase 1: Core Engine (Claude Code Only) ✅

Minimal viable Ralph runner with feature parity to ralph-ticker bash script. No TUI, no worktrees - just a solid Go foundation.

**Scope:**
- Single epic execution loop
- Claude Code agent only (`claude` CLI)
- Signal detection (COMPLETE, EJECT, BLOCKED)
- Ticks integration (`tk next`, `tk close`, `tk note`)
- Basic budget limits (iterations, cost)
- Epic notes for iteration context (via `tk note`)
- Checkpointing and resume
- Headless CLI operation

**CLI:**
```bash
ticker run <epic-id> --max-iterations 50
ticker run --auto  # auto-select next ready epic
ticker resume <checkpoint-id>
```

**Exit criteria:** Can complete an epic autonomously, equivalent to ralph-ticker.sh

---

### Phase 2: TUI ✅

Add Bubbletea-based terminal UI while maintaining headless mode.

**Scope:**
- Main dashboard view (tasks, output, budget)
- Streaming agent output
- Progress visualization
- Log history panel
- Pause/resume controls
- Interactive epic selection (`ticker` with no args)
- Keyboard navigation

**CLI:**
```bash
ticker                    # Interactive TUI mode
ticker run <epic-id>      # TUI mode by default
ticker run <epic-id> --headless  # Disable TUI
```

**Exit criteria:** Full TUI with all views from spec mockups working.

---

### Phase 3: Verification System ✅

Add automated verification to ensure tasks are actually complete before moving on.

**Design Decision: GitVerifier Only**

After analysis, we implemented GitVerifier only:
- **Agent already tests** - The prompt instructs agents to run tests before closing tasks
- **Avoid double work** - Running test suites twice wastes time, especially for slow suites
- **GitVerifier catches what agent can't** - Uncommitted changes are the one thing the agent can't self-verify

**Scope (implemented):**
- GitVerifier checks for uncommitted changes after agent closes task
- Verification runner integrated into iteration loop
- Config-based enable/disable (`.ticker/config.json`)
- On failure: reopen task, add failure details to epic notes
- TUI shows verification status

**Config example:**
```json
{
  "verification": {
    "enabled": true
  }
}
```

**CLI:**
```bash
ticker run <epic-id> --skip-verify     # Disable verification
ticker run <epic-id> --verify-only     # Run verify without agent (debug)
```

**Exit criteria:** GitVerifier runs automatically, uncommitted changes cause task reopen with notes.

---

### Phase 4: Worktrees & Parallel Execution

Enable running multiple epics simultaneously in isolated git worktrees.

**Scope:**
- Git worktree lifecycle management
- Parallel epic execution
- Per-worktree checkpoints
- Worktree TUI view
- Merge workflow (auto-merge on complete, pause on conflict)
- .gitignore auto-configuration
- Resource management (max parallel, shared budget)
- Cross-epic cost tracking

**CLI:**
```bash
ticker run <epic-id> --worktree
ticker run h8d fbv 5b8 --parallel 3
```

**Exit criteria:** Can run 3 epics in parallel, merge results cleanly.

---

### Phase 5: Multi-Agent Support

Add support for non-Claude coding agents with unified permission management.

**Scope:**
- Agent interface abstraction
- Agent implementations:
  - Codex (OpenAI)
  - Gemini CLI (Google)
  - Amp (Sourcegraph)
  - OpenCode (open source)
- Agent auto-detection and registry
- Unified permission modes (interactive, auto-edit, auto-all, yolo)
- Permission policy engine (allow/deny rules)
- Agent-specific sandbox mapping
- Permission audit logging
- TUI permission view

**CLI:**
```bash
ticker run <epic-id> --agent codex
ticker run <epic-id> --agent gemini --permission auto-edit
ticker run <epic-id> --allow-network --deny-commands "rm -rf"
```

**Exit criteria:** Can complete an epic using any of the 5 supported agents with appropriate permission controls.

---

### Future Considerations

Not in initial roadmap, but worth tracking:

- **Remote execution**: Run agents in cloud containers
- **Team features**: Shared checkpoints, handoff between developers
- **MCP integration**: Leverage agent MCP capabilities
- **Metrics dashboard**: Historical cost/success tracking
- **Plugin system**: Custom verifiers, agents, views

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

### Epic Switcher (Parallel Mode)

When running multiple epics in parallel, the TUI needs to manage focus between them. The main dashboard always shows one "active" epic, while others run in the background.

**Model State for Multi-Epic:**

```go
type Model struct {
    // Multi-epic state
    engines      map[string]*engine.Engine  // epic ID -> engine instance
    epicOrder    []string                   // ordered list of epic IDs (for 1-9 keys)
    activeEpic   string                     // currently displayed epic

    // Per-epic state (keyed by epic ID)
    outputs      map[string]*OutputBuffer   // streaming output per epic
    tasks        map[string][]ticks.Task    // task list per epic
    iterations   map[string]int             // current iteration per epic

    // Shared state
    budget       *budget.Tracker            // aggregate budget across all epics

    // ... existing pane state (renders activeEpic data)
}
```

**Message Routing:**

All engine messages are tagged with their source epic:

```go
// Wrapper for all engine-originated messages
type EngineMsg struct {
    EpicID string
    Inner  tea.Msg  // IterationStartMsg, OutputMsg, SignalMsg, etc.
}

// In Update(), route messages to appropriate state
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case EngineMsg:
        // Always update state for the source epic
        m.updateEpicState(msg.EpicID, msg.Inner)

        // Only trigger view refresh if it's the active epic
        if msg.EpicID == m.activeEpic {
            return m.handleActiveEpicMsg(msg.Inner)
        }
        return m, nil
    }
}
```

**Key Bindings:**

| Key | Action | Context |
|-----|--------|---------|
| `1-9` | Switch to epic by index | Any view |
| `[` | Previous epic | Any view |
| `]` | Next epic | Any view |
| `w` | Open worktrees view | Dashboard |
| `Tab` | Cycle panes (existing) | Dashboard |

**Switching Behavior:**

When switching epics:
1. **Instant switch** - No animation or delay
2. **State preserved** - Output buffer, scroll position, pane focus retained per-epic
3. **Background continues** - Non-active epics keep running, accumulating output
4. **Status bar updates** - Shows active epic ID and quick status of others

**Status Indicators (Header Bar):**

```
┌─────────────────────────────────────────────────────────────────┐
│ ticker v0.1.0    [1:h8d ●]  [2:fbv ↻]  [3:5b8 ✓]    $7.35 total │
│ Agent: claude    Epic: h8d (Parallel test execution)            │
```

Legend: `●` active/focused, `↻` running in background, `✓` complete, `⏸` paused, `✗` failed

**Output Buffering:**

Each epic maintains its own circular output buffer:

```go
type OutputBuffer struct {
    lines      []string
    maxLines   int        // e.g., 10000 lines
    scrollPos  int        // preserved when switching away
    following  bool       // auto-scroll to bottom
}
```

When switching to an epic:
- If `following` was true, scroll to latest output
- If user had scrolled up, restore exact scroll position
- New output since last view is highlighted briefly (optional)

**Notification System:**

Background epics can surface important events:

```go
type Notification struct {
    EpicID    string
    Level     NotificationLevel  // Info, Warning, Error, Complete
    Message   string
    Timestamp time.Time
}

// Shown in status bar or toast overlay
// "fbv: COMPLETE - all tasks done"
// "5b8: BLOCKED - missing API key"
```

**Worktree View Integration:**

The `w` key worktrees view serves as a "bird's eye" overview, while number keys provide quick switching without leaving dashboard context. Users can:
1. Press `w` to see all epics at a glance
2. Press `1-9` to jump directly to an epic from any view
3. Use `[`/`]` to cycle through epics sequentially

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
│  2. Gather context ─────────────► Epic info + epic notes     │
│  3. Build prompt ───────────────► Inject all context         │
│  4. agent.Run(prompt) ──────────► Stream output to TUI       │
│  5. Parse signals ──────────────► COMPLETE/EJECT/BLOCKED     │
│  6. Run verifiers ──────────────► Tests, build, git, tick    │
│  7. Checkpoint if interval ─────► Git commit + JSON state    │
│  8. Loop or exit ───────────────► Based on signal/budget     │
└──────────────────────────────────────────────────────────────┘
```

**Note:** Agent is instructed to add notes via `tk note <epic-id>` for context
that should persist across iterations. Epic notes replace the scratchpad concept.

### Signal Protocol

| Signal | XML Tag | Meaning | Exit Code |
|--------|---------|---------|-----------|
| COMPLETE | `<promise>COMPLETE</promise>` | All tasks done | 0 |
| EJECT | `<promise>EJECT: reason</promise>` | Large install needed | 2 |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Missing credentials/unclear | 3 |
| PAUSED | (user action) | User paused execution | - |
| MAX_ITERATIONS | (internal) | Hit iteration limit | 1 |
| BUDGET_EXCEEDED | (internal) | Hit token/cost/time limit | 1 |

### Verification System (Phase 3)

```go
type Verifier interface {
    Name() string
    Verify(ctx context.Context, taskID string, agentOutput string) *Result
}

type Result struct {
    Verifier   string
    Passed     bool
    Output     string
    Duration   time.Duration
    Error      error
}
```

Built-in verifier:
- **GitVerifier**: Checks for uncommitted changes via `git status --porcelain`

Design rationale: Agent is instructed to run tests before closing tasks (see prompt.go). GitVerifier catches the one thing the agent can't self-verify: uncommitted changes.

On verification failure:
1. Task is reopened (`tk reopen`)
2. Failure details added as epic note
3. Next iteration sees the failure and can fix it

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
│   └── checkpoints/
└── (main repo)
```

### Worktree Lifecycle

1. **Create**: `git worktree add .ticker-worktrees/<epic> -b ticker/<epic>`
2. **Run**: Engine runs in worktree directory, isolated from main
3. **Checkpoint**: Each worktree maintains independent checkpoints
4. **Complete**: On COMPLETE signal, merge back to main
5. **Cleanup**: Remove worktree and branch after successful merge

### Gitignore Configuration

Worktrees inside the repo require careful .gitignore setup to avoid tracking worktree contents while preserving ticker state.

**Required .gitignore entries** (auto-added by `ticker init`):

```gitignore
# Ticker worktrees - never commit these
.ticker-worktrees/

# Ticker runtime state (keep config, ignore runtime)
.ticker/checkpoints/
.ticker/audit.jsonl

# Keep these tracked:
# .ticker/config.json (project settings)
# .ticker/issues/ (if using embedded ticks)
```

**Why this matters:**

| Path | Tracked? | Rationale |
|------|----------|-----------|
| `.ticker-worktrees/` | No | Contains full repo clones, would duplicate everything |
| `.ticker/config.json` | Yes | Shared project configuration |
| `.ticker/checkpoints/` | No | Runtime state, can be large, resumable locally |
| `.ticker/audit.jsonl` | No | Local audit trail, grows unbounded |

### Worktree Complications

**Shared git objects**: All worktrees share the same `.git` directory (in main worktree). This means:
- Commits made in any worktree are immediately visible to all
- Branch operations affect all worktrees
- `git gc` and maintenance affects all worktrees

**File conflicts**: If two epics modify the same file:
- Each worktree has its own working copy (isolated)
- Conflict only surfaces at merge time
- Ticker detects this and pauses for manual resolution

**Nested .ticker directories**: Each worktree gets its own `.ticker/` for isolation:

```
project/                          # Main worktree
├── .git/                         # Shared git database
├── .gitignore                    # Contains .ticker-worktrees/
├── .ticker/
│   ├── config.json               # Tracked - shared config
│   └── checkpoints/              # Ignored - main worktree checkpoints
├── .ticker-worktrees/
│   ├── h8d/                      # Worktree for epic h8d
│   │   ├── .git                  # File pointing to main .git
│   │   ├── .ticker/
│   │   │   ├── config.json       # Same as main (tracked)
│   │   │   └── checkpoints/      # h8d-specific checkpoints
│   │   └── src/...               # Working copy
│   └── fbv/                      # Worktree for epic fbv
│       └── ...
└── src/...                       # Main working copy
```

**Initialization sequence**:

```go
func (m *WorktreeManager) Create(epicID string) error {
    wtPath := filepath.Join(".ticker-worktrees", epicID)
    branch := fmt.Sprintf("ticker/%s", epicID)

    // 1. Ensure .ticker-worktrees/ is in .gitignore
    if err := m.ensureGitignore(); err != nil {
        return err
    }

    // 2. Create worktree with new branch from current HEAD
    cmd := exec.Command("git", "worktree", "add", "-b", branch, wtPath)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("git worktree add: %w", err)
    }

    // 3. Initialize .ticker/ in worktree (inherits config, fresh runtime state)
    wtTickerDir := filepath.Join(wtPath, ".ticker")
    if err := os.MkdirAll(filepath.Join(wtTickerDir, "checkpoints"), 0755); err != nil {
        return err
    }

    // 4. Copy config.json to worktree (will be same via git, but ensure exists)

    return nil
}

func (m *WorktreeManager) ensureGitignore() error {
    const ignoreEntry = ".ticker-worktrees/"

    content, err := os.ReadFile(".gitignore")
    if err != nil && !os.IsNotExist(err) {
        return err
    }

    if strings.Contains(string(content), ignoreEntry) {
        return nil // Already present
    }

    f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return err
    }
    defer f.Close()

    _, err = f.WriteString("\n# Ticker worktrees\n" + ignoreEntry + "\n")
    return err
}
```

**Cleanup sequence**:

```go
func (m *WorktreeManager) Remove(epicID string) error {
    wtPath := filepath.Join(".ticker-worktrees", epicID)
    branch := fmt.Sprintf("ticker/%s", epicID)

    // 1. Remove worktree
    cmd := exec.Command("git", "worktree", "remove", wtPath)
    if err := cmd.Run(); err != nil {
        // Force remove if dirty
        cmd = exec.Command("git", "worktree", "remove", "--force", wtPath)
        cmd.Run()
    }

    // 2. Delete branch (optional, only if merged)
    exec.Command("git", "branch", "-d", branch).Run()

    // 3. Prune worktree metadata
    exec.Command("git", "worktree", "prune").Run()

    return nil
}
```

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
└── config.json
```

### Resume Flow

1. Load checkpoint JSON
2. Restore git state: `git checkout <commit>`
3. Resume engine from iteration N
4. Epic notes are already persisted in ticks, available on resume

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
6. **Verification failure threshold**: How many failures before giving up on a task?
7. **Task skip behavior**: Should we support skipping stuck tasks automatically?

### Parallel Execution

8. **Resource contention**: How to handle if two epics try to modify same file?
9. **Merge conflicts**: Auto-resolve? Pause and alert? Configurable strategy?
10. **Cost allocation**: Track cost per-epic or only aggregate?

### Agents

11. **Agent fallback**: If primary agent fails, try secondary? Chain of agents?
12. **Model selection per task**: Different models for different task types?
13. **Context window management**: How to handle prompts approaching context limit?
14. **Streaming granularity**: Chunk size for TUI streaming? Buffer strategy?

### Integration

15. **Ticks version compatibility**: Minimum `tk` version required?
16. **Git hooks**: Should ticker install/manage git hooks?
17. **CI integration**: GitHub Actions workflow template? Exit codes?
18. **Telemetry**: Anonymous usage stats? Opt-in/out?

### Permissions

19. **Permission mode switching**: Allow changing mode mid-run via TUI?
20. **Cross-agent consistency**: How to handle when agent doesn't support a permission feature?
21. **Rule inheritance**: Should project rules inherit from user-level rules?
22. **Approval timeout**: How long to wait for user approval before skipping/aborting?
23. **Batch approvals**: Allow approving patterns ("always allow go test")?

### Security

24. **Secrets in epic notes**: Warn if API keys detected in notes?
25. **Sandbox enforcement**: Can Ticker enforce sandbox when agent doesn't support it?
26. **Audit log retention**: How long to keep audit logs? Auto-rotate?
27. **Container mode**: Built-in Docker/container isolation option?

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
