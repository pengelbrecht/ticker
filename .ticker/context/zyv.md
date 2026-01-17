# Epic Context: [zyv] TUI: Agent-Human Workflow Status Indicators

## Relevant Code

### TUI Core Files
- **internal/tui/model.go** - Main TUI model containing:
  - `TaskInfo` struct (lines 49-56): Already has `Awaiting string` field ✓
  - `StatusIcon()` method (lines 70-91): Already implements emoji icons with awaiting priority ✓
  - `RenderTask()` method (lines 95-115): Already appends `[awaiting-type]` to task title ✓
  - Color constants (lines 783-796): Catppuccin Mocha palette with colorGray, colorBlue, colorGreen, colorRed, colorPeach
  - `TaskStatus` enum (lines 42-46): open, in_progress, closed

- **internal/tui/tabs.go** - Tab rendering:
  - `getTabStatusIcon()` (lines 98-111): Already uses emoji style (🔵, ✅, 🔴, ⚠) ✓
  - `renderTabBar()`, `renderSingleTab()` - Tab bar rendering

- **internal/tui/model_test.go** - Test file with:
  - `TestTaskInfo_StatusIcon_AllEmojiIcons` (line 3973)
  - `TestTaskInfo_StatusIcon_AwaitingPriority` (line 4017)
  - `TestTaskInfo_StatusIcon_BlockedPriority` (line 4045)
  - `TestUpdate_TasksUpdateMsg_WithAwaiting` (line 4096)
  - `TestTaskInfo_RenderTask_WithAwaitingType` (line 1216)

### Main Entry Point
- **cmd/ticker/main.go**:
  - `refreshTasks()` (lines 1335-1372): Maps ticks.Task to tui.TaskInfo with `Awaiting: t.GetAwaitingType()` ✓
  - `loadTasksForEpic()` (lines 755-792): Same mapping for parallel mode ✓
  - TicksWatcher setup (lines 1377-1386): Already connects watcher to refreshTasks() ✓

### Ticks Integration
- **internal/ticks/types.go**:
  - `Task` struct (lines 10-35): Has `Manual bool`, `Awaiting *string`, `Verdict *string` fields
  - `GetAwaitingType()` method: Returns awaiting type string or "work" for backwards-compat Manual=true

### File Watcher
- **internal/engine/watcher.go**:
  - `TicksWatcher` struct (lines 14-22): Watches `.tick/issues` for changes
  - `Changes()` (lines 86-91): Returns channel for change notifications
  - `NewTicksWatcher()` (lines 42-81): Creates watcher with debouncing (100ms default)

## Architecture Notes

### Data Flow for Task Status
1. Ticks stored in `.tick/issues/<id>.json` files
2. `ticks.Client.ListTasks()` reads and parses JSON files
3. `refreshTasks()` or `loadTasksForEpic()` converts `ticks.Task` → `tui.TaskInfo`
4. TUI receives `TasksUpdateMsg` and updates display

### Status Icon Priority (implemented)
1. Awaiting → 👤 (highest priority)
2. Blocked → 🔴
3. InProgress → 🌕
4. Closed → ✅
5. Open → ⚪

### File Watcher Integration (implemented)
- Watcher created in main.go after engine setup
- Goroutine listens on `watcher.Changes()` channel
- On notification, calls `refreshTasks()`

## Conventions

### Emoji Style Guide (documented in epic)
- ⚪ Open (ready for agent)
- 🔵 In Progress (agent working) - Note: TUI uses 🌕 for task animation
- ✅ Closed/Done
- 🔴 Blocked
- 👤 Awaiting Human

### Testing Patterns
- Table-driven tests with `testCases` slice
- Test function names: `TestTypeName_MethodName_Scenario`
- Use `strings.Contains()` for checking emoji output
- Verify priority ordering with multiple test cases

### Color Usage
```go
colorPeach    = "#FAB387"  // Awaiting tags, warnings
colorRed      = "#F38BA8"  // Blocked status
colorGreen    = "#A6E3A1"  // Closed status
colorBlueAlt  = "#89B4FA"  // In progress
colorGray     = "#6C7086"  // Open, borders
colorLavender = "#A6ADC8"  // Dim text, IDs
```

## Current State Analysis

Looking at the modified files in git status:
- All 7 tasks appear to be **already implemented** based on the code analysis:
  1. ✅ TaskInfo.Awaiting field exists (model.go:55)
  2. ✅ StatusIcon() uses emojis with awaiting priority (model.go:70-91)
  3. ✅ refreshTasks() passes Awaiting field (main.go:1359)
  4. ✅ TicksWatcher connected to refreshTasks() (main.go:1377-1386)
  5. ✅ Tab icons already emoji style (tabs.go:98-111)
  6. ✅ Tests exist for all icon behaviors (model_test.go:3973-4127)
  7. ✅ RenderTask() shows awaiting type in brackets (model.go:109-112)

Tasks may be ready for verification/closure or may need review of implementation details.