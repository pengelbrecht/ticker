# Epic Context: [8my] Phase 4: Worktrees & Parallel Execution

## Relevant Code

### Engine (`internal/engine/`)
- **engine.go:40-82** - `Engine` struct with callbacks for TUI integration (OnIterationStart, OnIterationEnd, OnOutput, OnSignal, etc.)
- **engine.go:85-145** - `RunConfig` with existing `UseWorktree`, `RepoRoot`, `WorkDir` fields
- **engine.go:192-220** - `RunResult` output structure
- **engine.go:460-517** - Worktree creation/cleanup logic already integrated
- **engine.go:569-879** - Main loop, calls `runIteration()` passing `state.workDir`
- **engine.go:923-1050** - `runIteration()` passes `WorkDir` to agent via `RunOpts`

### Agent (`internal/agent/`)
- **agent.go:14-24** - `Agent` interface: `Run(ctx, prompt, opts) (*Result, error)`
- **agent.go:26-48** - `RunOpts` with `WorkDir` and `Timeout` fields
- **claude.go:59** - Sets `cmd.Dir = opts.WorkDir` for execution directory

### Budget (`internal/budget/`)
- **tracker.go:60-67** - `Tracker` struct with `sync.RWMutex` for thread-safety
- **tracker.go:33-40** - `EpicUsage` for per-epic tracking
- **tracker.go:102-122** - `AddForEpic()` thread-safe per-epic recording
- All public methods mutex-protected

### Checkpoint (`internal/checkpoint/`)
- **checkpoint.go:15-47** - `Checkpoint` struct with `WorktreePath`, `WorktreeBranch` fields
- **checkpoint.go:226-231** - `NewCheckpointWithWorktree()` constructor
- **checkpoint.go:241-268** - `PrepareResume()` handles worktree recovery

### Worktree (`internal/worktree/`)
- **worktree.go:31-42** - `Worktree` and `Manager` structs
- **worktree.go:81-159** - `Create()` with branch creation, .tick symlink
- **worktree.go:163-191** - `Remove()` with force delete
- **merge.go:18-31** - `MergeResult` and `MergeManager`
- **merge.go:55-104** - `Merge()` with conflict detection
- **conflict.go:11-27** - `ConflictState` and `ConflictHandler`
- **gitignore.go** - `EnsureGitignore()` for .worktrees/

### Parallel (`internal/parallel/`)
- **runner.go:14-37** - `RunnerConfig` with EpicIDs, MaxParallel, SharedBudget
- **runner.go:44-70** - `EpicStatus`, `ConflictState`, `ParallelResult`
- **runner.go:123-160** - `Run()` with semaphore for concurrency limit
- **runner.go:163-255** - `runEpic()` goroutine with worktree/merge handling

### TUI (`internal/tui/`)
- **model.go:674-777** - `Model` with `multiEpic`, `epicTabs[]`, `activeTab`
- **model.go:279-335** - `EpicTab` struct mirrors Model fields per-epic
- **tabs.go** - Tab rendering, switching, sync methods
- **model.go:362-367** - `EpicConflictMsg` for conflicts
- **model.go:784-796** - Catppuccin Mocha color palette

### CLI (`cmd/ticker/`)
- **main.go:56-132** - `run` command accepts multiple epic IDs
- **main.go:65-77** - `--worktree`, `--parallel N` flags

### Headless (`internal/engine/`)
- **headless_output.go:19-25** - `HeadlessOutput` with epicID prefix support

## Architecture Notes

### Data Flow for Parallel Execution
```
CLI (epic IDs, --parallel N)
  → ParallelRunner (semaphore + goroutines)
    → WorktreeManager.Create(epicID)
    → Engine.Run(WorkDir=wt.Path)
      → agent.Run(WorkDir)
    → MergeManager.Merge(wt)
    → ConflictHandler if conflicts
  → ParallelResult
```

### Key Integration Points
1. **Engine ↔ Parallel**: `RunConfig.WorkDir`, shared budget, callbacks
2. **Worktree ↔ Merge**: Create → run → merge → cleanup
3. **Engine ↔ Checkpoint**: Save/restore worktree path+branch
4. **TUI ↔ Parallel**: EpicAddedMsg, EpicStatusMsg, tab switching

### Worktree Lifecycle
- Directory: `.worktrees/<epic-id>/`
- Branch: `ticker/<epic-id>`
- `.tick/` symlinked from main repo
- Cleanup on success/error, preserve on conflict

## Testing Patterns

### Mock Implementations
- **engine_test.go:18-55** - `mockAgent` with response queue
- **engine_test.go:58-150** - `mockTicksClient` with call tracking
- **runner_test.go** - Status transition tests, concurrency tests

### Pattern
```go
mock := &mockAgent{responses: []string{"output1", "output2"}}
client := &mockTicksClient{tasks: tasks}
engine := NewEngine(mock, client, budget, checkpoint)
result, _ := engine.Run(ctx, config)
assert.Equal(t, expected, mock.callCount)
```

## Conventions

### Error Handling
- Critical errors returned, non-critical logged to notes
- Partial output captured on timeout (`IsTimeout=true`)
- Verification/context failures don't abort run

### Thread Safety
- Budget tracker: mutex + return copies
- Conflict handler: mutex-protected map
- Parallel runner: WaitGroup + buffered channel semaphore

### Directory Layout
- `.worktrees/` - gitignored
- `.ticker/checkpoints/`, `.ticker/context/`, `.ticker/runs/`
- `.tick/` - symlinked into worktrees