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

**If no spec exists:** → Go to Step 2a (Create Spec)
**If spec exists but incomplete:** → Go to Step 2b (Complete Spec)
**If spec is complete:** → Skip to Step 3 (Create Ticks)

### Step 2a: Create Spec Through Conversation

Have a natural conversation with the user to understand their idea:

1. **Let them describe it** — Don't interrupt, let them explain the full vision
2. **Ask clarifying questions** — Dig into unclear areas through back-and-forth dialogue
3. **Optionally use AskUserQuestion** — For quick multiple-choice decisions
4. **Write SPEC.md** — Once you have enough detail, generate the spec

**Conversation topics to explore:**
- What problem does this solve? Who's it for?
- Core features vs nice-to-haves
- Technical constraints or preferences
- How will users interact with it?
- What does "done" look like?

**Example flow:**
```
User: "I want to build a CLI tool for managing dotfiles"

Claude: "Interesting! A few questions to understand the scope:
- Should it handle syncing between machines, or just organization?
- Are you thinking symlinks, copies, or templating?
- Any existing dotfile managers you like or want to improve on?"

User: "Syncing via git, symlinks, and I want it simpler than chezmoi"

Claude: "Got it - simple git-based syncing with symlinks. What about:
- Multi-machine configs (work vs personal)?
- Secret handling (API keys, tokens)?
- Bootstrap for new machines?"
...
```

Continue until you can write a complete SPEC.md with clear features and acceptance criteria.

### Step 2b: Complete Existing Spec

If SPEC.md exists but has gaps:

1. **Read the spec** — Identify what's missing or unclear
2. **Ask targeted questions** — Focus on the gaps, don't re-ask obvious things
3. **Update SPEC.md** — Fill in the missing details

Use AskUserQuestion for quick decisions, conversation for complex topics.

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
```bash
tk create "Add email validation to registration" \
  -d "Implement email validation with test cases:
- valid@example.com → valid
- invalid@ → invalid
- @nodomain.com → invalid
- empty string → invalid

Run: go test ./internal/validation/..." \
  -acceptance "All validation tests pass, no regressions" \
  -parent <epic-id>
```

**Bad tick (no tests):**
```bash
tk create "Add email validation" -d "Make sure emails are valid"
# No acceptance criteria, no test cases - agent will guess
```

See `references/tick-patterns.md` for more TDD patterns.

### Step 3: Create Ticks from Spec

Transform the spec into ticks organized by epic.

**For phased specs:** Focus on creating ticks for the current/next phase only. Don't create ticks for future phases—they may change based on learnings from earlier phases.

**Use AskUserQuestion** if questions arise while creating ticks:
- Unclear requirements or edge cases
- Missing acceptance criteria
- Ambiguous priorities or dependencies
- Implementation approach decisions

**Epic organization:**
1. Group related tasks into logical epics (auth, API, UI, etc.)
2. Create a **"Manual Tasks"** epic for anything requiring human intervention
3. Set up dependencies between tasks using `-blocked-by`

```bash
# Create epics
tk create "Authentication" -t epic
tk create "API Endpoints" -t epic

# Create tasks with acceptance criteria
tk create "Add JWT token generation" \
  -d "Implement JWT signing and verification" \
  -acceptance "JWT tests pass, tokens validate correctly" \
  -parent <auth-epic>

tk create "Add login endpoint" \
  -d "POST /api/login with email/password" \
  -acceptance "Login endpoint tests pass, returns valid JWT" \
  -parent <api-epic> \
  -blocked-by <jwt-task>

# Manual tasks - use -manual flag (skipped by tk next)
tk create "Set up production database" -manual \
  -d "Create RDS instance and configure access" \
  -acceptance "Database accessible, migrations run"

tk create "Create Stripe API keys" -manual \
  -d "Set up Stripe account and get API credentials"
```

**Manual tasks** (use `-manual` flag):
- Setting up external services (databases, auth providers)
- Creating accounts or API keys
- Design decisions needing human judgment
- Anything requiring credentials or secrets

Manual tasks are skipped by `tk next` and ticker automation. They appear in `tk list -manual`.

### Step 3b: Guide User Through Blocking Manual Tasks

**Critical:** If manual tasks block automated tasks, guide the user through them before running ticker.

```bash
# Check for blocking manual tasks
tk list -manual
tk blocked  # See what's waiting on manual tasks
```

**When manual tasks block automation:**

1. **Identify blocking manual tasks** — Find manual tasks that other tasks depend on
2. **Guide user step-by-step** — Walk them through each manual task
3. **Verify completion** — Confirm the task is done before closing
4. **Close and unblock** — `tk close <id> "reason"` to unblock dependent tasks

**Example guidance flow:**

```
I see 2 manual tasks that block automated work:

1. **Set up PostgreSQL database** (blocks: API endpoints epic)
   - Create database instance (RDS, Supabase, or local)
   - Note the connection string
   - Run: `tk close abc "Created RDS instance, connection string in .env"`

2. **Create Stripe API keys** (blocks: payment tasks)
   - Go to dashboard.stripe.com
   - Create test API keys
   - Add to .env: STRIPE_SECRET_KEY=sk_test_...
   - Run: `tk close def "Stripe keys configured in .env"`

Once these are done, I can run the automated epics.
```

Always resolve blocking manual tasks before starting ticker, otherwise automation will stall.

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
tk create "Title" -d "Description" -acceptance "Tests pass"  # Task with acceptance criteria
tk create "Title" -t epic                                    # Create epic
tk create "Title" -parent <epic-id>                          # Task under epic
tk create "Title" -blocked-by <task-id>                      # Blocked task
tk create "Title" -manual                                    # Manual task (skipped by automation)

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
