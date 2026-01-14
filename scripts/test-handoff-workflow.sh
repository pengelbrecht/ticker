#!/bin/bash
# Test script for agent-human handoff workflow
# Usage: ./scripts/test-handoff-workflow.sh
#
# This script creates a test epic with various task types to verify
# the agent-human handoff workflow. It covers:
# - Normal tasks (agent completes, auto-closes)
# - Pre-declared approval gates (agent completes, awaits human approval)
# - Content review gates (agent completes, awaits human judgment)
# - Human-assigned work (skipped by agent, awaits human)
# - Blocked tasks (not ready until blocker closed)

set -e

echo "=== Agent-Human Handoff Workflow Test ==="
echo ""

# Delete existing test epic if it exists
echo "Cleaning up old test epic..."
existing=$(tk list -t epic --json 2>/dev/null | jq -r '.ticks[]? | select(.title == "Handoff Test Epic") | .id' || true)
if [ -n "$existing" ]; then
    echo "Deleting existing epic: $existing"
    # Delete child tasks first
    tk list -parent "$existing" --json 2>/dev/null | jq -r '.ticks[]?.id' | while read -r task_id; do
        [ -n "$task_id" ] && tk delete "$task_id" -y 2>/dev/null || true
    done
    tk delete "$existing" -y 2>/dev/null || true
fi

echo ""
echo "Creating test epic with mixed tasks..."
EPIC_ID=$(tk create "Handoff Test Epic" -d "Test epic for agent-human handoff workflow" -t epic -p 2)
echo "Created epic: $EPIC_ID"
echo ""

# Task 1: Normal task (agent completes, auto-closes)
T1=$(tk create "Simple calculation task" \
    -d "Calculate 2+2 and report the answer. This is a simple task that should complete normally." \
    -t task -parent "$EPIC_ID" -p 2)
echo "Task $T1: Normal task (should auto-close on completion)"

# Task 2: Pre-declared approval gate
T2=$(tk create "Security-sensitive change" \
    -d "Make a security-related code change. Even when the agent completes this, it requires human approval before closing." \
    -t task -parent "$EPIC_ID" -p 1 --requires approval)
echo "Task $T2: Requires approval (agent completes -> awaiting approval)"

# Task 3: Pre-declared review gate
T3=$(tk create "API endpoint implementation" \
    -d "Implement a new REST API endpoint. Requires code review before closing." \
    -t task -parent "$EPIC_ID" -p 2 --requires review)
echo "Task $T3: Requires review (agent completes -> awaiting review)"

# Task 4: Pre-declared content gate
T4=$(tk create "Update error messages" \
    -d "Improve the error messages in the UI. Requires content/UX review." \
    -t task -parent "$EPIC_ID" -p 2 --requires content)
echo "Task $T4: Requires content review (agent completes -> awaiting content)"

# Task 5: Human-assigned work (agent cannot do this)
T5=$(tk create "Configure production credentials" \
    -d "Set up AWS credentials in production. This requires human access to the AWS console." \
    -t task -parent "$EPIC_ID" -p 1 --awaiting work)
echo "Task $T5: Awaiting work (skipped by agent, assigned to human)"

# Task 6: Blocked by T5 (tests blocked + awaiting interaction)
T6=$(tk create "Deploy to production" \
    -d "Deploy the application to production. Blocked until credentials are configured." \
    -t task -parent "$EPIC_ID" -p 1 --blocked-by "$T5")
echo "Task $T6: Blocked by $T5 (becomes ready after T5 closes)"

# Task 7: Another normal task to verify agent continues after handoffs
T7=$(tk create "Write documentation" \
    -d "Document the new features. This is a normal task that should complete after other tasks." \
    -t task -parent "$EPIC_ID" -p 3)
echo "Task $T7: Normal task (should complete normally)"

echo ""
echo "=== Test Epic Created ==="
echo ""
echo "Epic ID: $EPIC_ID"
echo ""
echo "Task Summary:"
echo "  $T1 - Normal task (auto-closes)"
echo "  $T2 - Requires approval"
echo "  $T3 - Requires review"
echo "  $T4 - Requires content review"
echo "  $T5 - Awaiting human work (skipped by agent)"
echo "  $T6 - Blocked by $T5"
echo "  $T7 - Normal task"
echo ""
echo "=== Running Ticker ==="
echo ""
echo "Run ticker on this epic:"
echo "  ./ticker run $EPIC_ID"
echo ""
echo "Expected agent behavior:"
echo "  1. Completes '$T1' (Simple calculation) -> auto-closes"
echo "  2. Completes '$T2' (Security change) -> awaiting approval"
echo "  3. Completes '$T3' (API endpoint) -> awaiting review"
echo "  4. Completes '$T4' (Error messages) -> awaiting content"
echo "  5. Skips '$T5' (Credentials) -> already awaiting work"
echo "  6. Skips '$T6' (Deploy) -> blocked by $T5"
echo "  7. Completes '$T7' (Documentation) -> auto-closes"
echo ""
echo "After agent stops, verify the state:"
echo "  tk list --parent $EPIC_ID               # See all tasks"
echo "  tk list --awaiting=                     # See tasks awaiting human"
echo ""
echo "=== Human Workflow Commands ==="
echo ""
echo "1. View what needs attention:"
echo "   tk list --awaiting="
echo "   tk next $EPIC_ID --awaiting="
echo ""
echo "2. Review a specific task:"
echo "   tk show <task-id>"
echo "   tk notes <task-id>"
echo ""
echo "3. Approve a task (closes work/approval/review/content tasks):"
echo "   tk approve $T2"
echo ""
echo "4. Reject a task with feedback (returns to agent queue):"
echo "   tk reject $T2 'Add more error handling before approval'"
echo ""
echo "5. Complete human work (closes task):"
echo "   tk approve $T5   # After you've done the credentials work"
echo ""
echo "6. Unblock dependent task:"
echo "   After $T5 is approved, $T6 becomes ready for the agent"
echo ""
echo "=== Test Scenarios ==="
echo ""
echo "Scenario A: Approval Flow"
echo "  1. Run ticker -> agent completes $T2, task awaits approval"
echo "  2. tk approve $T2 -> task closes"
echo ""
echo "Scenario B: Rejection/Retry Flow"
echo "  1. Run ticker -> agent completes $T2, task awaits approval"
echo "  2. tk reject $T2 'Needs more tests'"
echo "  3. Run ticker -> agent picks up $T2 again with your feedback"
echo "  4. Agent addresses feedback, completes again -> awaits approval"
echo "  5. tk approve $T2 -> task closes"
echo ""
echo "Scenario C: Human Work Flow"
echo "  1. $T5 was created with --awaiting work (skipped by agent)"
echo "  2. Human configures credentials manually"
echo "  3. tk approve $T5 -> task closes, $T6 becomes unblocked"
echo "  4. Run ticker -> agent completes $T6"
echo ""
echo "Scenario D: Eject Flow (simulated)"
echo "  If an agent encounters work it can't do, it emits:"
echo "    <promise>EJECT: Cannot access AWS console</promise>"
echo "  This sets the task to awaiting=work for human intervention."
echo ""
