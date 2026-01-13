---
name: ticker
description: Work with Ticks issue tracker and Ticker AI agent runner. Use when managing tasks/issues with `tk` commands, running AI agents on epics, or working in a repo with a `.tick/` directory. Triggers on phrases like "create a tick", "tk", "run ticker", "epic", "close the task".
---

# Ticker & Ticks Workflow

Ticker runs AI agents (Claude Code, Codex, Gemini CLI) in continuous loops to complete coding tasks from the Ticks issue tracker.

## Quick Reference

### Ticks CLI (`tk`)

```bash
# Create ticks
tk create "Title" -d "Description"           # Create task
tk create "Title" -t epic                    # Create epic
tk create "Title" -parent <epic-id>          # Create task under epic
tk create "Title" -blocked-by <task-id>      # Create blocked task

# List and query
tk list                                      # All open ticks
tk list -t epic                              # Epics only
tk list -parent <epic-id>                    # Tasks in epic
tk ready                                     # Unblocked tasks
tk next <epic-id>                            # Next task to work on
tk blocked                                   # Blocked tasks

# Manage ticks
tk show <id>                                 # Show details
tk close <id> "reason"                       # Close tick
tk reopen <id>                               # Reopen tick
tk note <id> "note text"                     # Add note
tk block <id> -b <blocker>                   # Add blocker
tk unblock <id> -b <blocker>                 # Remove blocker
```

See `references/tk-commands.md` for full command reference.

### Running Ticker

```bash
# Interactive TUI
ticker                                       # Epic picker
ticker run <epic-id>                         # Run specific epic

# Headless mode
ticker run <epic-id> --headless              # Single epic
ticker run <epic1> <epic2> --headless        # Parallel epics

# Auto-select
ticker run --auto                            # Next ready epic
ticker run --auto --parallel 3               # Multiple epics
```

## Signal Protocol

When working on a tick, signal completion with XML tags:

| Signal | Tag | When to Use |
|--------|-----|-------------|
| COMPLETE | `<promise>COMPLETE</promise>` | All work done, tests pass |
| EJECT | `<promise>EJECT: reason</promise>` | Need human help (large install, unclear requirements) |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Missing credentials, external dependency |

**Important:** Only signal COMPLETE when the task is truly done. If tests fail or work remains, fix it first.

## Creating Good Ticks

See `references/tick-patterns.md` for detailed patterns.

**Key principles:**
1. **Atomic** — One clear deliverable per tick
2. **Testable** — Clear acceptance criteria
3. **Independent** — Minimize dependencies
4. **AI-friendly** — Include enough context for autonomous completion

**Bad tick:**
```
Title: Improve the codebase
```

**Good tick:**
```
Title: Add input validation to user registration form
Description:
- Validate email format (RFC 5322)
- Require password 8+ chars with number
- Show inline error messages
- Add unit tests for validation functions
```

## Working on a Tick

When assigned a tick:

1. **Read the tick** — `tk show <id>` for full context
2. **Check blockers** — Are dependencies resolved?
3. **Implement** — Make changes, write tests
4. **Verify** — Run tests, check lint
5. **Add notes** — `tk note <id> "what you did"`
6. **Close** — `tk close <id> "summary of solution"`

## Epic Workflow

Epics group related tasks:

```bash
# Create epic with tasks
tk create "User Authentication" -t epic
tk create "Add login form" -parent <epic-id>
tk create "Add password reset" -parent <epic-id> -blocked-by <login-task-id>
tk create "Add OAuth support" -parent <epic-id> -blocked-by <login-task-id>
```

Ticker runs tasks in dependency order, closing the epic when all tasks complete.

## Parallel Execution

When running multiple epics:
- Each epic runs in an isolated git worktree
- All worktrees share the same `.tick/` database
- Merges happen automatically on completion
- Conflicts are flagged for manual resolution
