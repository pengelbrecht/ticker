# tk Command Reference

Complete reference for the Ticks CLI.

## Creating Ticks

```bash
tk create "Title" [flags]
```

| Flag | Description |
|------|-------------|
| `-d, --description` | Tick description |
| `-t, --type` | Type: `task` (default) or `epic` |
| `-p, --priority` | Priority: 0=Critical, 1=High, 2=Medium, 3=Low, 4=Backlog |
| `-l, --labels` | Comma-separated labels |
| `-parent` | Parent epic ID |
| `-blocked-by` | Blocking tick ID(s) |

**Examples:**
```bash
tk create "Fix login bug" -d "Users can't login with special chars" -p 1
tk create "Auth System" -t epic -d "Complete authentication implementation"
tk create "Add OAuth" -parent abc -blocked-by def,ghi
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
