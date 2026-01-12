# Ticker - AI Agent Loop Runner

Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents (Claude Code, Codex, Gemini CLI) in continuous loops until tasks are complete. It wraps the [Ticks](https://github.com/pengelbrecht/ticks) issue tracker and orchestrates coding agents to autonomously complete epics.

## Quick Start

```bash
# Build locally
go build -o ticker ./cmd/ticker

# Run with TUI (interactive epic picker)
./ticker

# Run specific epic
./ticker run <epic-id>

# Run headless (no TUI)
./ticker run <epic-id> --headless

# Auto-select next ready epic
./ticker run --auto
```

## Project Structure

```
ticker/
├── cmd/ticker/main.go      # CLI entry point (cobra)
├── internal/
│   ├── agent/              # AI agent interface + Claude implementation
│   ├── budget/             # Cost/token/iteration tracking
│   ├── checkpoint/         # Save/resume state
│   ├── engine/             # Core Ralph loop orchestration
│   ├── ticks/              # Ticks CLI wrapper (tk commands)
│   └── tui/                # Bubble Tea TUI (model.go is main file)
├── SPEC.md                 # Full specification document
└── .tick/issues/           # Ticks issue storage
```

## Working with Ticks (tk CLI)

Ticker uses the `tk` CLI for issue tracking. Key commands:

### Creating Ticks

```bash
# Create a task
tk create "Task title" -d "Description" -p 2 -t task

# Create an epic (parent for tasks)
tk create "Epic title" -d "Description" -t epic

# Create task under an epic
tk create "Task title" -d "Description" -t task -parent <epic-id>

# Create with labels
tk create "Task title" -l "bug,urgent"

# Create blocked by another task
tk create "Task title" -blocked-by <task-id>
```

### Listing and Querying

```bash
tk list                     # List all open ticks
tk list -t epic             # List epics only
tk list -t task             # List tasks only
tk list -parent <epic-id>   # List tasks in epic
tk list -s open             # List open ticks
tk list -s closed           # List closed ticks
tk list --json              # JSON output
tk ready                    # List ready (unblocked) tasks
tk next <epic-id>           # Get next task to work on
tk blocked                  # List blocked tasks
```

### Managing Ticks

```bash
tk show <id>                # Show tick details
tk close <id> "reason"      # Close a tick
tk reopen <id>              # Reopen a tick
tk update <id> -d "new desc" # Update description
tk block <id> -b <blocker>  # Add blocker
tk unblock <id> -b <blocker> # Remove blocker
tk note <id> "note text"    # Add note to tick
tk notes <id>               # Show notes
```

### Tick Types

- **epic**: Container for related tasks (has children)
- **task**: Individual work item (can have parent epic)

### Tick Statuses

- **open**: Active, can be worked on
- **closed**: Completed

### Priority Levels

- 0: Critical
- 1: High
- 2: Medium (default)
- 3: Low
- 4: Backlog

## Engine Signal Protocol

The agent communicates completion via XML tags in output:

| Signal | Tag | Meaning |
|--------|-----|---------|
| COMPLETE | `<promise>COMPLETE</promise>` | All tasks done |
| EJECT | `<promise>EJECT: reason</promise>` | Large install needed |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Missing credentials |

## TUI Key Bindings

| Key | Action |
|-----|--------|
| Tab | Cycle panes (tasks/output/log) |
| j/k | Scroll down/up |
| g/G | Jump to top/bottom |
| Enter | View selected task's output history |
| Esc | Return to live output |
| p | Pause/resume |
| q | Quit |

## Testing

```bash
go test ./...                    # Run all tests
go test ./internal/tui/...       # Test TUI only
go test -v ./internal/engine/... # Verbose engine tests
```

## Building

```bash
# Development build
go build -o ticker ./cmd/ticker

# With version info
go build -ldflags "-X main.Version=v1.0.0" -o ticker ./cmd/ticker
```

## Test Data

```bash
# Create/recreate test epic with simple tasks for TUI testing
./scripts/create-test-epic.sh

# Then run ticker on the test epic
./ticker run <epic-id>
```

## Architecture Notes

- **TUI**: Uses Bubble Tea framework with Catppuccin Mocha color palette
- **Engine**: Runs iterations, detects signals, manages checkpoints
- **Agent**: Wraps `claude` CLI with streaming output
- **Budget**: Tracks iterations, tokens, cost against limits
- **Checkpoints**: Saved to `.ticker/checkpoints/` for resume
