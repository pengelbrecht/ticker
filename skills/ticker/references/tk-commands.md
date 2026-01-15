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
| `-manual` | [DEPRECATED] Use `--awaiting work` instead |
| `--requires` | Pre-declared approval gate: `approval`, `review`, `content` |
| `--awaiting` | Immediate human assignment: `work`, `approval`, `input`, `review`, `content`, `escalation`, `checkpoint` |
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

# Task requiring approval before closing
tk create "Update auth flow" --requires approval -d "Security-sensitive change"

# Task requiring content review
tk create "Redesign error messages" --requires content

# Task assigned directly to human (skipped by agent)
tk create "Configure AWS credentials" --awaiting work
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
| `--awaiting` | Filter by awaiting status (see below) |
| `--json` | Output as JSON |

**Special commands:**
```bash
tk ready                    # List ready (unblocked) tasks
tk blocked                  # List blocked tasks
tk next <epic-id>           # Get next task for agent in epic
tk next <epic-id> --awaiting # Get next task for human in epic
```

**Awaiting filters:**
```bash
tk list --awaiting              # All ticks awaiting human action
tk list --awaiting approval     # Only ticks awaiting approval
tk list --awaiting input,review # Multiple awaiting types
tk next --awaiting              # Next task for human (any epic)
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
| `--awaiting` | Set awaiting status (or `--awaiting=null` to clear) |
| `--verdict` | Set human verdict: `approved` or `rejected` |

## Status Changes

```bash
tk close <id> "reason"      # Close with reason
tk reopen <id>              # Reopen closed tick
```

## Human Verdicts

Commands for humans responding to agent handoffs:

```bash
tk approve <id>                     # Approve tick awaiting human verdict
tk reject <id>                      # Reject tick (returns to agent)
tk reject <id> "feedback"           # Reject with feedback note
```

**What happens on verdict:**

| awaiting | approved | rejected |
|----------|----------|----------|
| `work` | Closes tick | (invalid) |
| `approval` | Closes tick | Back to agent |
| `input` | Back to agent (with answer) | Closes tick |
| `review` | Closes tick (merge PR) | Back to agent |
| `content` | Closes tick | Back to agent |
| `escalation` | Back to agent (with direction) | Closes tick |
| `checkpoint` | Back to agent (next phase) | Back to agent (redo) |

## Dependencies

```bash
tk block <id> -b <blocker-id>     # Add blocker
tk unblock <id> -b <blocker-id>   # Remove blocker
```

## Notes

```bash
tk note <id> "note text"              # Add note (default: from agent)
tk note <id> "note text" --from agent # Explicit agent note
tk note <id> "note text" --from human # Human note (feedback, answers)
tk notes <id>                         # List notes
```

**When to use `--from human`:**
- Human providing feedback after rejecting work
- Human answering a question (INPUT_NEEDED)
- Human giving direction on escalation
- Any note that should be attributed to human, not agent

## Output Formats

Most commands support `--json` for machine-readable output:

```bash
tk list --json | jq '.ticks[] | select(.priority == 1)'
tk show abc --json | jq '.description'
```

## Awaiting States Reference

| awaiting | Meaning | Human Action |
|----------|---------|--------------|
| `work` | Human must do the task | Complete work, then approve |
| `approval` | Agent done, needs sign-off | Review and approve/reject |
| `input` | Agent needs information | Provide answer in note, approve |
| `review` | PR needs code review | Review PR, approve/reject |
| `content` | UI/copy needs judgment | Judge quality, approve/reject |
| `escalation` | Agent found issue | Decide direction, approve/reject |
| `checkpoint` | Phase complete | Verify, approve to continue |
