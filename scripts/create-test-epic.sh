#!/bin/bash
# Create test epics with simple tasks for TUI and parallel testing
# Usage: ./scripts/create-test-epic.sh

set -e

# Delete existing test epics if they exist
echo "Cleaning up old test epics..."
for title in "TUI Test Epic" "Parallel Test Epic"; do
    existing=$(tk list -t epic --json 2>/dev/null | jq -r ".ticks[]? | select(.title == \"$title\") | .id" || true)
    if [ -n "$existing" ]; then
        echo "Deleting existing epic: $existing ($title)"
        # Delete child tasks first
        tk list -parent "$existing" --json 2>/dev/null | jq -r '.ticks[]?.id' | while read -r task_id; do
            [ -n "$task_id" ] && tk delete "$task_id" -y 2>/dev/null || true
        done
        tk delete "$existing" -y 2>/dev/null || true
    fi
done

# Create first test epic
echo ""
echo "Creating test epic 1..."
EPIC1=$(tk create "TUI Test Epic" -d "Simple tasks for testing the ticker TUI" -t epic -p 2)
echo "Created epic: $EPIC1"

echo "Creating test tasks for epic 1..."
T1=$(tk create "What is 2+2?" -d "Calculate 2+2 and report the answer." -t task -parent "$EPIC1" -p 2)
echo "  Task $T1: Math question"

T2=$(tk create "Report repo name" -d "Read go.mod and report the module name." -t task -parent "$EPIC1" -p 2)
echo "  Task $T2: Repo name"

T3=$(tk create "Count Go files" -d "Count how many .go files are in the internal/ directory." -t task -parent "$EPIC1" -p 2)
echo "  Task $T3: Count files"

T4=$(tk create "List CLI commands" -d "Read cmd/ticker/main.go and list all subcommands defined." -t task -parent "$EPIC1" -p 3)
echo "  Task $T4: List commands"

# Create second test epic
echo ""
echo "Creating test epic 2..."
EPIC2=$(tk create "Parallel Test Epic" -d "Simple tasks for testing parallel execution" -t epic -p 2)
echo "Created epic: $EPIC2"

echo "Creating test tasks for epic 2..."
T5=$(tk create "What is 3+3?" -d "Calculate 3+3 and report the answer." -t task -parent "$EPIC2" -p 2)
echo "  Task $T5: Math question"

T6=$(tk create "Report Go version" -d "Check go.mod and report the Go version required." -t task -parent "$EPIC2" -p 2)
echo "  Task $T6: Go version"

T7=$(tk create "Count test files" -d "Count how many _test.go files are in the internal/ directory." -t task -parent "$EPIC2" -p 2)
echo "  Task $T7: Count test files"

echo ""
echo "Test epics ready:"
echo "  Epic 1: $EPIC1 (TUI Test Epic)"
echo "  Epic 2: $EPIC2 (Parallel Test Epic)"
echo ""
echo "Run single:   ./ticker run $EPIC1"
echo "Run parallel: ./ticker run $EPIC1 $EPIC2"
echo "Run auto:     ./ticker run --auto --parallel 2"
