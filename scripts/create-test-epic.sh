#!/bin/bash
# Create a test epic with simple tasks for TUI testing
# Usage: ./scripts/create-test-epic.sh

set -e

# Delete existing test epic if it exists
echo "Cleaning up old test epic..."
existing=$(tk list -t epic --json 2>/dev/null | jq -r '.ticks[]? | select(.title == "TUI Test Epic") | .id' || true)
if [ -n "$existing" ]; then
    echo "Deleting existing test epic: $existing"
    # Delete child tasks first
    tk list -parent "$existing" --json 2>/dev/null | jq -r '.ticks[]?.id' | while read -r task_id; do
        [ -n "$task_id" ] && tk delete "$task_id" -y 2>/dev/null || true
    done
    tk delete "$existing" -y 2>/dev/null || true
fi

echo "Creating test epic..."
EPIC_ID=$(tk create "TUI Test Epic" -d "Simple tasks for testing the ticker TUI" -t epic -p 2)
echo "Created epic: $EPIC_ID"

echo "Creating test tasks..."

# Simple tasks Claude can complete quickly
T1=$(tk create "What is 2+2?" -d "Calculate 2+2 and report the answer." -t task -parent "$EPIC_ID" -p 2)
echo "  Task $T1: Math question"

T2=$(tk create "Report repo name" -d "Read go.mod and report the module name." -t task -parent "$EPIC_ID" -p 2)
echo "  Task $T2: Repo name"

T3=$(tk create "Count Go files" -d "Count how many .go files are in the internal/ directory." -t task -parent "$EPIC_ID" -p 2)
echo "  Task $T3: Count files"

T4=$(tk create "List CLI commands" -d "Read cmd/ticker/main.go and list all subcommands defined." -t task -parent "$EPIC_ID" -p 3)
echo "  Task $T4: List commands"

T5=$(tk create "What day is it?" -d "Report today's date." -t task -parent "$EPIC_ID" -p 3)
echo "  Task $T5: Date question"

# Rich markdown output test
T6=$(tk create "Generate project summary with rich markdown" -d "Create a summary of this project using rich markdown formatting. Include:
- A table comparing the main packages (name, purpose, key files)
- Code snippets showing key type definitions
- A bullet list of features
- Headers and subheaders
Output should exercise markdown rendering: tables, code blocks, lists, emphasis, etc." -t task -parent "$EPIC_ID" -p 2)
echo "  Task $T6: Rich markdown output"

# A blocked task to test blocked display
T7=$(tk create "Summarize after counting" -d "Summarize findings after the count task is done." -t task -parent "$EPIC_ID" -p 3 -blocked-by "$T3")
echo "  Task $T7: Blocked task (blocked by $T3)"

echo ""
echo "Test epic ready: $EPIC_ID"
echo "Run with: ./ticker run $EPIC_ID"
