Now I have all the information needed to generate the context document.

# Epic Context: TUI Agent-Human Workflow Status Indicators [zyv]

## Relevant Code

### Primary Files to Modify

| File | Purpose | Key Lines |
|------|---------|-----------|
| `internal/tui/model.go` | TUI model with TaskInfo, StatusIcon, RenderTask | 49-115 |
| `internal/tui/tabs.go` | Tab rendering with getTabStatusIcon | 92-111 |
| `cmd/ticker/main.go` | Task loading functions | 755-792 (loadTasksForEpic), 1335-1386 (refreshTasks + watcher) |

### Key Types

**TaskInfo** (model.go:49-56):
```go
type TaskInfo struct {
    ID        string
    Title     string
    Status    TaskStatus
    BlockedBy []string
    IsCurrent bool
    Awaiting  string  // already exists
}
```

**ticks.Task** (internal/ticks/types.go:10-40):
- Has `Awaiting *string` field (pointer)
- Has `GetAwaitingType()` method for backwards compat with Manual field

**TaskStatus** constants (model.go):
- `TaskStatusOpen`, `TaskStatusInProgress`, `TaskStatusClosed`

### Implemented Functions

**StatusIcon** (model.go:70-91) - Already implements emoji style:
```go
func (t TaskInfo) StatusIcon() string {
    if t.Awaiting != "" { return "👤" }           // Human icon
    if t.Status == TaskStatusOpen && t.IsBlocked() { return "🔴" }
    switch t.Status {
    case TaskStatusInProgress: return "🌕"       // NOTE: Task says 🔵
    case TaskStatusClosed: return "✅"
    case TaskStatusOpen: return "⚪"
    }
}
```

**RenderTask** (model.go:95-115) - Already appends awaiting type:
```go
if t.Awaiting != "" {
    awaitingTag := lipgloss.NewStyle().Foreground(colorPeach).Render("[" + t.Awaiting + "]")
    result += " " + awaitingTag
}
```

**getTabStatusIcon** (tabs.go:98-111) - Already uses emoji:
- Running: 🔵, Complete: ✅, Failed: 🔴, Conflict: ⚠

### Task Loading (main.go)

**loadTasksForEpic** (line 755) - Multi-epic parallel mode:
```go
taskInfos[i] = tui.TaskInfo{
    ID:        t.ID,
    Title:     t.Title,
    Status:    tui.TaskStatus(t.Status),
    BlockedBy: openBlockers,
    Awaiting:  t.GetAwaitingType(),  // Already wired
}
```

**refreshTasks** (line 1335) - Single-epic mode - same pattern

**File watcher** (lines 1377-1386) - Already implemented:
```go
ticksWatcher := engine.NewTicksWatcher("")
defer ticksWatcher.Close()
if changes := ticksWatcher.Changes(); changes != nil {
    go func() {
        for range changes { refreshTasks() }
    }()
}
```

## Architecture Notes

### Data Flow
1. `ticks.Client.ListTasks()` returns `[]ticks.Task`
2. Converted to `[]tui.TaskInfo` in main.go
3. Sent via `tui.TasksUpdateMsg` or `tui.EpicTasksUpdateMsg`
4. TUI model stores in `m.tasks` and renders via `renderTaskPane()`

### TicksWatcher (engine/watcher.go)
- Uses `fsnotify` to watch `.tick/issues/` directory
- Debounces at 100ms
- `Changes()` returns `<-chan struct{}` (nil if fsnotify unavailable)
- `Close()` is safe to call multiple times

## Color Constants (model.go:785-795)

```go
colorBlue     = "#89DCEB"  // Selected items (Sky)
colorBlueAlt  = "#89B4FA"  // In-progress status
colorGreen    = "#A6E3A1"  // Success, closed
colorRed      = "#F38BA8"  // Errors, blocked, bugs
colorPeach    = "#FAB387"  // Warnings, awaiting
colorGray     = "#6C7086"  // Borders, muted
colorLavender = "#A6ADC8"  // Dim text
```

## Testing Patterns

### Test File: `internal/tui/model_test.go`

**Existing StatusIcon tests** (lines 1116-1138, 3973-4094):
- `TestTaskStatusIcon` - basic cases
- `TestTaskInfo_StatusIcon_AllEmojiIcons` - all emoji render
- `TestTaskInfo_StatusIcon_AwaitingPriority` - awaiting > blocked
- `TestTaskInfo_StatusIcon_BlockedPriority` - blocked > open

**RenderTask tests** (lines 1190-1284):
- `TestTaskInfo_RenderTask` - basic rendering
- `TestTaskInfo_RenderTask_WithAwaitingType` - awaiting brackets
- `TestTaskInfo_RenderTask_NoAwaitingType` - no extra brackets

**TasksUpdateMsg test** (line 4096):
- `TestUpdate_TasksUpdateMsg_WithAwaiting` - awaiting field propagates

### Test Pattern
```go
func TestTaskInfo_StatusIcon_Example(t *testing.T) {
    task := TaskInfo{ID: "abc", Status: TaskStatusOpen, Awaiting: "approval"}
    icon := task.StatusIcon()
    if !strings.Contains(icon, "👤") {
        t.Errorf("expected human icon, got %s", icon)
    }
}
```

## Current Status

Based on grep results, most tasks appear **already implemented**:
- ✅ `Awaiting` field exists in TaskInfo (task 2d6)
- ✅ `StatusIcon()` handles awaiting with 👤 (task nu3 - but uses 🌕 not 🔵 for in-progress)
- ✅ `Awaiting` wired in loadTasksForEpic/refreshTasks (task ok0)
- ✅ File watcher exists in refreshTasks (task b2n)
- ✅ Tab icons use emoji style (task 4ds)
- ✅ Tests exist (task 2b8)
- ✅ RenderTask appends `[awaiting-type]` (task jx9)

**Potential discrepancy**: Task nu3 specifies 🔵 for in-progress, but code uses 🌕. May need verification if this is intentional.