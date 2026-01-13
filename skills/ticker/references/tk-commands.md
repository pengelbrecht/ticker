# tk Command Reference

Complete reference for the Ticks CLI.

## Creating Ticks

```bash
tk create "Title" [flags]
```

| Flag | Description |
|------|-------------|
| `-d, --description` | Tick description |
| `-acceptance` | Acceptance criteria (how to verify done) |
| `-t, --type` | Type: `task` (default) or `epic` |
| `-p, --priority` | Priority: 0=Critical, 1=High, 2=Medium, 3=Low, 4=Backlog |
| `-l, --labels` | Comma-separated labels |
| `-parent` | Parent epic ID |
| `-blocked-by` | Blocking tick ID(s) |
| `-manual` | Mark as requiring human intervention (skipped by automation) |
| `-defer` | Defer until date (YYYY-MM-DD) |
| `-external-ref` | External reference (e.g., gh-42) |

**Examples:**
```bash
# Basic task
tk create "Fix login bug" -d "Users can't login with special chars" -p 1

# Task with acceptance criteria (recommended for AI agents)
tk create "Add email validation" \
  -d "Validate email format on registration form" \
  -acceptance "All validation tests pass"

# Epic
tk create "Auth System" -t epic -d "Complete authentication implementation"

# Task with dependencies
tk create "Add OAuth" -parent abc -blocked-by def,ghi

# Manual task (requires human intervention)
tk create "Set up Stripe account" -manual -d "Create account and get API keys"
```

## Listing Ticks

```bash
tk list [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --type` | Filter by type: `task` or `epic` |
| `-s, --status` | Filter by status: `open` or `closed` |
| `-p, --priority` | Filter by priority (0-4) |
| `-l, --labels` | Filter by labels |
| `-parent` | Filter by parent epic |
| `--json` | Output as JSON |

**Special commands:**
```bash
tk ready                    # List ready (unblocked) tasks
tk blocked                  # List blocked tasks
tk next <epic-id>           # Get next task in epic
```

## Viewing Ticks

```bash
tk show <id> [flags]
```

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |

## Updating Ticks

```bash
tk update <id> [flags]
```

| Flag | Description |
|------|-------------|
| `-t, --title` | New title |
| `-d, --description` | New description |
| `-p, --priority` | New priority |
| `-l, --labels` | New labels (replaces existing) |

## Status Changes

```bash
tk close <id> "reason"      # Close with reason
tk reopen <id>              # Reopen closed tick
```

## Dependencies

```bash
tk block <id> -b <blocker-id>     # Add blocker
tk unblock <id> -b <blocker-id>   # Remove blocker
```

## Notes

```bash
tk note <id> "note text"    # Add note
tk notes <id>               # List notes
```

## Output Formats

Most commands support `--json` for machine-readable output:

```bash
tk list --json | jq '.ticks[] | select(.priority == 1)'
tk show abc --json | jq '.description'
```
