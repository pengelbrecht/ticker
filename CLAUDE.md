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
│   ├── context/            # Epic context generation and storage
│   ├── engine/             # Core Ralph loop orchestration
│   ├── ticks/              # Ticks CLI wrapper (tk commands)
│   └── tui/                # Bubble Tea TUI (model.go is main file)
├── SPEC.md                 # Full specification document
├── .tick/issues/           # Ticks issue storage
└── .ticker/
    ├── config.json         # Ticker configuration
    ├── checkpoints/        # Checkpoint files for resume
    ├── context/            # Pre-generated epic context docs
    └── runs/               # Run logs
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

# Create a manual task (requires human intervention, skipped by ticker)
tk create "Configure AWS credentials" --manual -parent <epic-id>
```

### Manual Tasks (Human Intervention)

Tasks marked with `--manual` require human intervention and are automatically skipped by ticker. Use this for tasks that cannot be completed by AI agents (e.g., configuring credentials, physical setup, external approvals).

```bash
# Create a manual task
tk create "Set up AWS IAM role" --manual -parent <epic-id>

# View all manual tasks
tk list --manual

# Mark existing task as manual
tk update <id> --manual=true

# Remove manual flag
tk update <id> --manual=false

# Include manual tasks in ready/next queries (not recommended for automation)
tk ready --include-manual
tk next <epic-id> --include-manual
```

**Important**: `tk next` and `tk ready` exclude manual tasks by default. Ticker uses `tk next` to get work, so manual tasks are automatically skipped.

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

## Linting

**Important:** Always lint before committing changes.

```bash
golangci-lint run               # Run all linters
golangci-lint run --fix         # Auto-fix issues where possible
```

Install golangci-lint:
```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

Linting runs automatically on PRs via GitHub Actions.

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
- **Context**: Pre-generated epic context stored in `.ticker/context/`

## Epic Context

Ticker can pre-generate context documents for epics to reduce redundant codebase exploration. Instead of each task spending tokens gathering similar context, a shared context document is generated once and injected into every task's prompt.

### How It Works

1. When an epic with 2+ tasks starts, ticker generates a context document
2. The document is stored at `.ticker/context/<epic-id>.md`
3. Context is injected into every task's prompt template
4. Tasks start with relevant code locations, patterns, and architecture already known

**Skipped for single-task epics** - Context generation is skipped for epics with ≤1 tasks since there's no amortization benefit.

### CLI Commands

```bash
# Generate context (or show if exists)
ticker context <epic-id>

# Force regeneration
ticker context <epic-id> --refresh

# View existing context only (error if none exists)
ticker context <epic-id> --show

# Delete context file
ticker context <epic-id> --delete
```

### Configuration

Add to `.ticker/config.json`:

```json
{
  "context": {
    "enabled": true,
    "max_tokens": 4000,
    "auto_refresh_days": 0,
    "generation_timeout": "5m",
    "generation_model": ""
  }
}
```

| Option | Default | Description |
|--------|---------|-------------|
| `enabled` | `true` | Enable/disable context generation |
| `max_tokens` | `4000` | Target size for context documents (100-100000) |
| `auto_refresh_days` | `0` | Days until auto-refresh (0 = never, max 365) |
| `generation_timeout` | `"5m"` | Max generation time (1s-1h) |
| `generation_model` | `""` | Model override for generation (empty = default) |

### Context Document Contents

Generated context includes:
- **Relevant Code** - Files, types, and functions related to the epic
- **Architecture Notes** - How components interact
- **External References** - Documentation links for frameworks/libraries
- **Testing Patterns** - Test structure and mocking approaches
- **Conventions** - Error handling, logging, naming patterns

### Example Workflow

```bash
# Create an epic with multiple tasks
tk create "Add user authentication" -t epic
tk create "Create User model" -t task -parent abc
tk create "Add login endpoint" -t task -parent abc
tk create "Add session middleware" -t task -parent abc

# Start ticker - context generates automatically
ticker run abc

# Or manually generate/view context
ticker context abc
```

### Troubleshooting

**Context not generating:**
- Check epic has more than 1 task (`tk list -parent <epic-id>`)
- Verify context is enabled in config (`"enabled": true`)
- Context may already exist - use `--refresh` to regenerate

**Context seems stale:**
- Use `ticker context <epic-id> --refresh` to regenerate
- Set `auto_refresh_days` in config for automatic refresh

**Context too large/small:**
- Adjust `max_tokens` in config (default 4000)
- Regenerate with `--refresh`

**Generation timing out:**
- Increase `generation_timeout` in config
- Default is 5 minutes, max is 1 hour

**Generation fails:**
- Ticker proceeds without context (non-fatal)
- Check agent availability and API keys
- Review logs for specific errors
