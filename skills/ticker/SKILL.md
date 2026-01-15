---
name: ticker
description: Work with Ticks issue tracker and Ticker AI agent runner. Use when managing tasks or issues with tk commands, running AI agents on epics, creating ticks from a SPEC.md, or working in a repo with a .tick directory. Triggers on phrases like create ticks, tk, run ticker, epic, close the task, plan this, break this down.
---

# Ticker & Ticks Workflow

Ticker runs AI agents (Claude Code, Codex, Gemini CLI) in continuous loops to complete coding tasks from the Ticks issue tracker.

## Skill Workflow

When invoked, follow this workflow:

### Step 0: Check Prerequisites

**1. Git repository:**
```bash
git status 2>/dev/null || git init
```

**2. GitHub remote (optional but recommended):**
```bash
gh repo view 2>/dev/null || gh repo create <name> --private --source=. --push
```
Ask user for repo name if creating new. Skip if they prefer local-only.

**3. Tools installed:**
```bash
which tk && which ticker
```

If not installed:
```bash
# Install ticks (tk CLI)
curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticks/main/scripts/install.sh | sh

# Install ticker
curl -fsSL https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh | sh
```

**4. Ticks initialized:**
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

## Handoff Signal Protocol

When working on a tick, signal completion or handoff with XML tags. These signals tell the system whether work is complete or needs human involvement.

### Signal Types

| Signal | Format | When to Use |
|--------|--------|-------------|
| COMPLETE | `<promise>COMPLETE</promise>` | Task fully done, all tests pass |
| APPROVAL_NEEDED | `<promise>APPROVAL_NEEDED: reason</promise>` | Work complete, needs human sign-off before closing |
| INPUT_NEEDED | `<promise>INPUT_NEEDED: question</promise>` | Need human to answer a question or make a decision |
| REVIEW_REQUESTED | `<promise>REVIEW_REQUESTED: pr_url</promise>` | PR created, needs code review before merge |
| CONTENT_REVIEW | `<promise>CONTENT_REVIEW: description</promise>` | UI/copy/design needs human judgment |
| ESCALATE | `<promise>ESCALATE: issue</promise>` | Found unexpected issue requiring human direction |
| CHECKPOINT | `<promise>CHECKPOINT: summary</promise>` | Completed a phase, need verification before continuing |
| EJECT | `<promise>EJECT: reason</promise>` | Cannot complete - requires human to do the work |
| BLOCKED | `<promise>BLOCKED: reason</promise>` | Legacy signal (maps to INPUT_NEEDED) |

### When to Use Each Signal

**APPROVAL_NEEDED** — Use for:
- Security-sensitive changes (auth, encryption, access control)
- Database migrations or schema changes
- API contract changes that affect consumers
- Dependency major version upgrades
- Anything with significant production risk

**INPUT_NEEDED** — Use for:
- Business logic decisions you can't make alone
- Configuration choices (which library, which approach)
- Clarification on ambiguous requirements
- "Which X should I use?" questions

**REVIEW_REQUESTED** — Use for:
- After creating any pull request
- Include the full PR URL in the context

**CONTENT_REVIEW** — Use for:
- User-facing text changes (error messages, labels)
- UI component styling or layout decisions
- Marketing copy or documentation tone
- Anything subjective requiring human taste

**ESCALATE** — Use for:
- Security vulnerabilities discovered during work
- Scope significantly larger than expected
- Architectural decisions with major tradeoffs
- Conflicting requirements that need resolution

**CHECKPOINT** — Use for:
- Multi-phase migrations (verify phase 1 before phase 2)
- Large refactors (validate approach on one module first)
- Any work where early validation saves potential rework

**EJECT** — Use for:
- Needs credentials or secrets the agent doesn't have
- Physical or manual setup required
- External system access you genuinely cannot obtain
- Tasks that truly require human hands

### Format Examples

```xml
<!-- Task completed successfully -->
<promise>COMPLETE</promise>

<!-- Work done but needs human approval -->
<promise>APPROVAL_NEEDED: Database migration adds new user_preferences table with 3 columns</promise>

<!-- Need human input to proceed -->
<promise>INPUT_NEEDED: Should the rate limiter use sliding window or fixed window algorithm?</promise>

<!-- PR ready for review -->
<promise>REVIEW_REQUESTED: https://github.com/org/repo/pull/123</promise>

<!-- Content needs human judgment -->
<promise>CONTENT_REVIEW: New error messages in PaymentForm - tone may need adjustment</promise>

<!-- Found issue needing direction -->
<promise>ESCALATE: Found SQL injection vulnerability in legacy code - fix now or create separate task?</promise>

<!-- Phase complete, verify before continuing -->
<promise>CHECKPOINT: Phase 1 complete - migrated 3 of 8 tables. Verify data integrity before continuing</promise>

<!-- Cannot complete, human must do it -->
<promise>EJECT: Task requires AWS Console access to configure IAM roles</promise>
```

### Important Rules

1. **One signal per task** — Emit exactly one signal when done
2. **Be specific** — Include helpful context after the colon
3. **Don't guess** — If you need human input, use INPUT_NEEDED
4. **Don't skip gates** — If something feels risky, use APPROVAL_NEEDED

## Reading Human Feedback

When you pick up a task, it may have been previously handed off to a human. Look for:

1. **Human Feedback section** — In the task prompt, a "Human Feedback" section shows responses from humans
2. **Task notes** — Check notes for answers to questions you asked or feedback on rejected work

**When human feedback is present:**
- Read and understand all feedback points
- Address each issue raised
- Then proceed with the task
- Don't re-ask questions that were already answered

Human feedback appears after:
- Approval was rejected (fix the issues noted)
- Input was provided (use the answer given)
- Review requested changes (implement the changes)
- Escalation got direction (follow the decision made)
- Checkpoint was rejected (redo the phase as directed)

## Pre-Declared Gates (requires field)

Some tasks have a `requires` field set by humans at creation time. This is a **pre-declared approval gate** that you cannot bypass.

| requires value | Meaning |
|----------------|---------|
| `approval` | Must have human sign-off before closing |
| `review` | Must have PR review before closing |
| `content` | Must have content/design review before closing |

**How it works:**
- You don't need to know or check the `requires` field
- Just signal `COMPLETE` when you finish the work
- The system automatically routes to human approval based on `requires`
- If rejected, you'll get feedback and the task returns to you

**Example flow:**
1. Task created with `requires: approval`
2. Agent works on task, signals `COMPLETE`
3. System sees `requires: approval`, sets task to await human approval
4. Human approves → task closes; Human rejects → task returns to agent with feedback

This ensures humans can enforce approval gates on sensitive work without relying on agent judgment

## Assisting Humans with Awaiting Ticks

When working interactively with a user, proactively help them process ticks awaiting human action. This keeps the agent-human workflow moving efficiently.

### Check for Awaiting Ticks

Periodically check if there are ticks waiting for human input:

```bash
tk list --awaiting              # All ticks awaiting human
tk next --awaiting              # Next tick needing attention
```

If you find awaiting ticks, offer to help the user work through them.

### Help Users Respond Efficiently

For each awaiting tick, use **AskUserQuestion** to help the user decide quickly:

**For INPUT_NEEDED (agent asked a question):**
```
The task "Add rate limiting" is waiting for your input:
"Should the rate limiter use sliding window or fixed window algorithm?"

[Sliding window] - More accurate, slightly more complex
[Fixed window] - Simpler, may allow bursts at boundaries
```

Then execute:
```bash
tk note <id> "Use sliding window algorithm" --from human
tk approve <id>
```

**For APPROVAL_NEEDED (agent wants sign-off):**
```
The task "Update auth flow" is awaiting approval:
Agent completed: "Added JWT refresh token rotation"

[Approve] - Work looks good, close the task
[Reject] - Needs changes (I'll ask for feedback)
```

**For CONTENT_REVIEW (subjective judgment):**
```
The task "Error messages" needs content review:
New messages in PaymentForm - see diff at <file>

[Approve] - Tone and content are good
[Reject] - Needs adjustment (I'll ask what to change)
```

### Execute Commands on User's Behalf

When the user makes a decision, run the appropriate commands:

```bash
# User approves
tk approve <id>

# User rejects with feedback
tk note <id> "Make error messages friendlier, less technical" --from human
tk reject <id>

# User provides input/answer
tk note <id> "Use Stripe for payment processing" --from human
tk approve <id>
```

**Important:** Always use `--from human` when adding notes on behalf of the user. This helps the automated agent distinguish human feedback from its own notes.

### Proactive Workflow Assistance

When appropriate, proactively offer to help:

- After running ticker, check if any tasks are now awaiting human input
- When user mentions "waiting on me" or "blocked tasks", check `tk list --awaiting`
- Before starting new work, see if pending human tasks could unblock agent work

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
