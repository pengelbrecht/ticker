---
name: ticker
description: Work with Ticks issue tracker and Ticker AI agent runner. Use when managing tasks/issues with `tk` commands, running AI agents on epics, creating ticks from a SPEC.md, or working in a repo with a `.tick/` directory. Triggers on phrases like "create ticks", "tk", "run ticker", "epic", "close the task", "plan this", "break this down".
---

# Ticker & Ticks Workflow

Ticker runs AI agents (Claude Code, Codex, Gemini CLI) in continuous loops to complete coding tasks from the Ticks issue tracker.

## Skill Workflow

When invoked, follow this workflow:

### Step 0: Check Prerequisites

Verify `tk` and `ticker` are installed:

```bash
which tk && which ticker
```

**If not installed**, install them:

```bash
# Install ticks (tk CLI)
curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticks/main/scripts/install.sh | sh

# Install ticker
curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh | sh
```

Also verify the repo has ticks initialized:
```bash
ls .tick/ 2>/dev/null || tk init
```

### Step 1: Check for SPEC.md

Look for a SPEC.md (or similar spec file) in the repo root.

**If no spec exists:**
- Ask the user what they want to build
- Create SPEC.md through the interview process below

**If spec exists but is incomplete:**
- Read it and identify gaps
- Interview the user to fill in missing details
- Update SPEC.md with the answers

**If spec is complete:**
- Skip to Step 3 (Create Ticks)

### Step 2: Interview to Complete Spec

Use AskUserQuestion to interview the user in depth. Ask about:

- **Technical implementation** — Architecture, patterns, libraries
- **UI/UX** — User flows, edge cases, error states
- **Testing strategy** — What needs tests, coverage expectations
- **Concerns** — Performance, security, scalability
- **Tradeoffs** — What's negotiable vs non-negotiable
- **Scope** — What's in v1 vs future

**Important:** Ask non-obvious questions. Don't ask "What language?" if it's clear from context. Dig into the nuances.

Continue interviewing until you have enough detail to create atomic, implementable tasks. Then update SPEC.md with the gathered information.

## Test-Driven Development (Critical)

**AI agents work best with test-driven tasks.** Tests provide:
- Clear acceptance criteria the agent can verify
- Immediate feedback on correctness
- Guard rails against regressions

When creating ticks, structure them for TDD:

1. **Write test first** — Each feature tick should specify expected test cases
2. **Include test commands** — Tell the agent how to run tests (`go test`, `npm test`, etc.)
3. **Define success criteria** — "Tests pass" is unambiguous; "looks good" is not

**Good tick (test-driven):**
```
Title: Add email validation to registration

Description:
Implement email validation with these test cases:
- valid@example.com → valid
- invalid@ → invalid
- @nodomain.com → invalid
- empty string → invalid

Run tests: go test ./internal/validation/...
Acceptance: All tests pass, no regressions
```

**Bad tick (no tests):**
```
Title: Add email validation
Description: Make sure emails are valid
```

See `references/tick-patterns.md` for more TDD patterns.

### Step 3: Create Ticks from Spec

Transform the spec into ticks organized by epic.

**Epic organization:**
1. Group related tasks into logical epics (auth, API, UI, etc.)
2. Create a **"Manual Tasks"** epic for anything requiring human intervention
3. Set up dependencies between tasks using `-blocked-by`

```bash
# Create epics
tk create "Authentication" -t epic
tk create "API Endpoints" -t epic
tk create "Manual Tasks" -t epic -d "Tasks requiring human intervention"

# Create tasks under epics
tk create "Add JWT token generation" -parent <auth-epic>
tk create "Add login endpoint" -parent <api-epic> -blocked-by <jwt-task>
tk create "Set up production database" -parent <manual-epic>
```

**Manual tasks** (require `--manual` flag in ticker):
- Setting up external services (databases, auth providers)
- Creating accounts or API keys
- Design decisions needing human judgment
- Anything requiring credentials or secrets

### Step 4: Optimize for Parallelization

Review each epic and consider splitting if:
- Epic has many independent tasks (no dependencies between them)
- Tasks could run in parallel but are grouped together

**Split large epics:**
```
Before: "Build Dashboard" (8 independent tasks)
After:  "Build Dashboard (1/2)" (4 tasks)
        "Build Dashboard (2/2)" (4 tasks)
```

This allows ticker to run both epic halves in parallel.

**Guidelines:**
- Aim for 3-5 tasks per epic for optimal parallelization
- Keep dependent task chains in the same epic
- Independent tasks can be split across epics

### Step 5: Run Ticker

Ask the user how they want to run:

```
How would you like to run these epics?

1. Headless (I'll run ticker for you)
   - Runs in background, I'll report results

2. Interactive TUI (you run it)
   - You get real-time visibility and control
   - Command: ticker run <epic-ids...>
```

**If headless:**
```bash
ticker run <epic1> <epic2> --headless --parallel <n>
```

**If TUI:**
Provide the command for the user to run:
```bash
ticker run <epic1> <epic2>
```

## Quick Reference

### Ticks CLI (`tk`)

```bash
# Create ticks
tk create "Title" -d "Description"           # Create task
tk create "Title" -t epic                    # Create epic
tk create "Title" -parent <epic-id>          # Task under epic
tk create "Title" -blocked-by <task-id>      # Blocked task

# List and query
tk list                                      # All open ticks
tk list -t epic                              # Epics only
tk list -parent <epic-id>                    # Tasks in epic
tk ready                                     # Unblocked tasks
tk next <epic-id>                            # Next task to work on

# Manage
tk show <id>                                 # Show details
tk close <id> "reason"                       # Close tick
tk note <id> "note text"                     # Add note
```

See `references/tk-commands.md` for full reference.

### Running Ticker

```bash
# Interactive TUI
ticker run <epic-id>                         # Single epic
ticker run <epic1> <epic2>                   # Multiple epics

# Headless
ticker run <epic-id> --headless              # Single epic
ticker run <epic1> <epic2> --headless        # Parallel epics
ticker run --auto --parallel 3               # Auto-select epics
```

## Signal Protocol

When working on a tick, signal completion with XML tags:

| Signal | Tag | When to Use |
|--------|-----|-------------|
| COMPLETE | `<promise>COMPLETE</promise>` | All work done, tests pass |
| EJECT | `<promise>EJECT: reason</promise>` | Need human help |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Missing credentials |

## Creating Good Ticks

See `references/tick-patterns.md` for detailed patterns.

**Key principles:**
1. **Atomic** — One clear deliverable per tick
2. **Testable** — Clear acceptance criteria
3. **Independent** — Minimize dependencies
4. **AI-friendly** — Include enough context for autonomous completion

**Bad tick:**
```
Title: Build the feature
```

**Good tick:**
```
Title: Add email validation to registration form
Description:
- Validate email format on blur
- Show error message below input
- Prevent form submission if invalid
- Add unit tests for validation
```
