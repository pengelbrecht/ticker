package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/verify"
	"github.com/pengelbrecht/ticker/internal/worktree"
)

// TicksClient defines the interface for ticks operations used by the Engine.
// This allows for mocking in tests while the production code uses *ticks.Client.
type TicksClient interface {
	GetEpic(epicID string) (*ticks.Epic, error)
	GetTask(taskID string) (*ticks.Task, error)
	NextTask(epicID string) (*ticks.Task, error)
	HasOpenTasks(epicID string) (bool, error)
	CloseTask(taskID, reason string) error
	CloseEpic(epicID, reason string) error
	ReopenTask(taskID string) error
	AddNote(issueID, message string, extraArgs ...string) error
	GetNotes(epicID string) ([]string, error)
	GetHumanNotes(issueID string) ([]ticks.Note, error)
	SetStatus(issueID, status string) error
	SetAwaiting(taskID, awaiting, note string) error
	SetRunRecord(taskID string, record *agent.RunRecord) error
}

// Engine orchestrates the Ralph iteration loop.
type Engine struct {
	agent      agent.Agent
	ticks      TicksClient
	budget     *budget.Tracker
	checkpoint *checkpoint.Manager
	prompt     *PromptBuilder

	// Verification enabled flag (set via EnableVerification)
	verifyEnabled bool

	// Callbacks for TUI integration (optional)
	OnIterationStart func(ctx IterationContext)
	OnIterationEnd   func(result *IterationResult)
	OnOutput         func(chunk string)
	OnSignal         func(signal Signal, reason string)

	// Verification callbacks for TUI status display (optional)
	OnVerificationStart func(taskID string)
	OnVerificationEnd   func(taskID string, results *verify.Results)

	// Watch mode callback - called when no tasks available and entering idle state.
	OnIdle func()

	// Rich streaming callback for real-time agent state updates.
	// Called whenever agent state changes (text, thinking, tools, metrics).
	// If set, this provides structured updates; OnOutput is still called for backward compat.
	OnAgentState func(snap agent.AgentStateSnapshot)
}

// RunConfig configures an engine run.
type RunConfig struct {
	// EpicID is the epic to work on.
	EpicID string

	// MaxIterations is the maximum number of iterations (0 = 50 default).
	MaxIterations int

	// MaxCost is the maximum cost in USD (0 = disabled/unlimited).
	MaxCost float64

	// MaxDuration is the maximum wall-clock time (0 = unlimited).
	MaxDuration time.Duration

	// CheckpointEvery saves a checkpoint every N iterations (0 = 5 default).
	CheckpointEvery int

	// ResumeFrom is the checkpoint ID to resume from (empty = start fresh).
	ResumeFrom string

	// AgentTimeout is the per-iteration timeout for the agent (0 = 30 minutes default).
	AgentTimeout time.Duration

	// PauseChan is a channel that signals pause/resume. When true, engine pauses.
	// Nil means no pause support.
	PauseChan <-chan bool

	// MaxTaskRetries is the maximum iterations on the same task before assuming stuck (0 = 3 default).
	MaxTaskRetries int

	// SkipVerify disables verification even if configured (--skip-verify flag).
	SkipVerify bool

	// UseWorktree enables running in an isolated git worktree.
	// When true, a worktree is created before running and cleaned up after.
	UseWorktree bool

	// RepoRoot is the root of the git repository for worktree operations.
	// Required when UseWorktree is true. If empty, current working directory is used.
	RepoRoot string

	// WorkDir overrides the working directory for the agent.
	// If set, the agent runs in this directory instead of the current directory.
	// Used by parallel runner to pass pre-created worktree paths.
	WorkDir string

	// Watch enables watch mode - engine idles when no tasks available instead of exiting.
	Watch bool

	// WatchTimeout is the maximum duration to watch for tasks (0 = unlimited).
	// Only used when Watch is true.
	WatchTimeout time.Duration

	// WatchPollInterval is how often to poll for new tasks when idle (0 = 10s default).
	// Only used when Watch is true.
	WatchPollInterval time.Duration

	// DebounceInterval is how long to wait after a task becomes available before picking it up.
	// This prevents race conditions when a human is still editing (e.g., adding notes after reject).
	// 0 means no debounce (default, backwards compatible).
	DebounceInterval time.Duration
}

// Defaults for RunConfig.
const (
	DefaultMaxIterations     = 50
	DefaultMaxCost           = 0 // Disabled by default (most users have subscriptions)
	DefaultCheckpointEvery   = 5
	DefaultAgentTimeout      = 30 * time.Minute
	DefaultMaxTaskRetries    = 3
	DefaultWatchPollInterval = 10 * time.Second
)

// Exit reason constants for worktree cleanup decisions.
const (
	// ExitReasonAllTasksCompleted indicates epic is fully done - cleanup worktree.
	ExitReasonAllTasksCompleted = "all tasks completed"

	// ExitReasonNoTasksFound indicates no tasks to work on - cleanup worktree.
	ExitReasonNoTasksFound = "no tasks found"

	// ExitReasonTasksAwaitingHuman indicates tasks are blocked/awaiting - preserve worktree.
	ExitReasonTasksAwaitingHuman = "no ready tasks (remaining tasks are blocked or awaiting human)"

	// ExitReasonWatchTimeout indicates watch mode timed out - preserve worktree.
	ExitReasonWatchTimeout = "watch timeout"
)

// ShouldCleanupWorktree determines if a worktree should be removed based on exit reason.
// Returns true only when the epic is fully complete (all tasks done or no tasks found).
// Returns false for handoffs, budget limits, interruptions, and other cases where
// the worktree should be preserved for resumption.
func ShouldCleanupWorktree(exitReason string) bool {
	// Only cleanup when epic is truly complete
	switch exitReason {
	case ExitReasonAllTasksCompleted, ExitReasonNoTasksFound:
		return true
	default:
		// Preserve worktree for:
		// - Tasks awaiting human intervention
		// - Context cancellation (user interrupt)
		// - Budget limits (may resume)
		// - Stuck on task (needs debugging)
		// - Any other unexpected exit
		return false
	}
}

// RunResult contains the outcome of an engine run.
type RunResult struct {
	// EpicID is the epic that was worked on.
	EpicID string

	// Iterations is the total number of iterations completed.
	Iterations int

	// TotalTokens is the cumulative token usage.
	TotalTokens int

	// TotalCost is the cumulative cost in USD.
	TotalCost float64

	// Duration is the total wall-clock time.
	Duration time.Duration

	// CompletedTasks lists task IDs that were closed.
	CompletedTasks []string

	// Signal is the exit signal (if any).
	Signal Signal

	// SignalReason is the reason for EJECT or BLOCKED signals.
	SignalReason string

	// ExitReason describes why the run ended.
	ExitReason string
}

// IterationResult contains the outcome of a single iteration.
type IterationResult struct {
	// Iteration is the iteration number (1-indexed).
	Iteration int

	// TaskID is the task that was worked on.
	TaskID string

	// TaskTitle is the title of the task.
	TaskTitle string

	// Output is the agent's full output.
	Output string

	// TokensIn is the input token count.
	TokensIn int

	// TokensOut is the output token count.
	TokensOut int

	// Cost is the iteration cost in USD.
	Cost float64

	// Duration is how long the iteration took.
	Duration time.Duration

	// Signal is any signal detected in the output.
	Signal Signal

	// SignalReason is the reason for EJECT or BLOCKED signals.
	SignalReason string

	// Error is any error that occurred.
	Error error

	// IsTimeout indicates the iteration was terminated due to timeout.
	// When true, Output may contain partial output captured before timeout.
	IsTimeout bool
}

// NewEngine creates a new engine with the given dependencies.
func NewEngine(a agent.Agent, t TicksClient, b *budget.Tracker, c *checkpoint.Manager) *Engine {
	return &Engine{
		agent:      a,
		ticks:      t,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
	}
}

// EnableVerification enables verification after task completion.
// When enabled, GitVerifier runs in the appropriate working directory.
func (e *Engine) EnableVerification() {
	e.verifyEnabled = true
}

// Run executes the engine loop until completion, signal, or budget exceeded.
func (e *Engine) Run(ctx context.Context, config RunConfig) (result *RunResult, err error) {
	// Apply defaults
	if config.MaxIterations == 0 {
		config.MaxIterations = DefaultMaxIterations
	}
	// Note: MaxCost == 0 means disabled (unlimited), not a default value
	if config.CheckpointEvery == 0 {
		config.CheckpointEvery = DefaultCheckpointEvery
	}
	if config.MaxTaskRetries == 0 {
		config.MaxTaskRetries = DefaultMaxTaskRetries
	}
	if config.AgentTimeout == 0 {
		config.AgentTimeout = DefaultAgentTimeout
	}
	if config.Watch && config.WatchPollInterval == 0 {
		config.WatchPollInterval = DefaultWatchPollInterval
	}

	// Calculate watch deadline (0 = unlimited)
	var watchDeadline time.Time
	if config.Watch && config.WatchTimeout > 0 {
		watchDeadline = time.Now().Add(config.WatchTimeout)
	}

	// Initialize state
	state := &runState{
		epicID:         config.EpicID,
		iteration:      0,
		completedTasks: []string{},
		startTime:      time.Now(),
	}

	// Handle worktree mode
	var wtManager *worktree.Manager
	var wt *worktree.Worktree
	if config.UseWorktree {
		// Determine repo root
		repoRoot := config.RepoRoot
		if repoRoot == "" {
			repoRoot, err = os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("getting working directory: %w", err)
			}
		}

		// Create worktree manager
		wtManager, err = worktree.NewManager(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("creating worktree manager: %w", err)
		}

		// Create worktree for this epic
		wt, err = wtManager.Create(config.EpicID)
		if err != nil {
			// If worktree already exists, try to get it
			if errors.Is(err, worktree.ErrWorktreeExists) {
				wt, err = wtManager.Get(config.EpicID)
				if err != nil {
					return nil, fmt.Errorf("getting existing worktree: %w", err)
				}
			} else {
				return nil, fmt.Errorf("creating worktree: %w", err)
			}
		}

		// Set the work directory in state
		state.workDir = wt.Path

		// Cleanup worktree based on exit reason when function returns.
		// Only cleanup when epic is truly complete (all tasks done or no tasks found).
		// Preserve worktree for handoffs, interruptions, and budget limits.
		defer func() {
			if wtManager != nil && wt != nil && result != nil {
				if ShouldCleanupWorktree(result.ExitReason) {
					_ = wtManager.Remove(config.EpicID)
				}
			}
		}()
	}

	// Allow WorkDir override (used by parallel runner with pre-created worktrees)
	if config.WorkDir != "" {
		state.workDir = config.WorkDir
	}

	// Resume from checkpoint if specified
	if config.ResumeFrom != "" {
		cp, err := e.checkpoint.Load(config.ResumeFrom)
		if err != nil {
			return nil, fmt.Errorf("loading checkpoint: %w", err)
		}
		state.iteration = cp.Iteration
		state.completedTasks = cp.CompletedTasks
		// Note: budget tracker starts fresh, but we could restore from checkpoint
	}

	// Get epic info
	epic, err := e.ticks.GetEpic(config.EpicID)
	if err != nil {
		return nil, fmt.Errorf("getting epic: %w", err)
	}

	// Validate that the ID refers to an epic, not a task
	if epic.Type != "epic" {
		errMsg := fmt.Sprintf("'%s' is a %s, not an epic", config.EpicID, epic.Type)
		// Try to get parent epic to suggest it
		task, taskErr := e.ticks.GetTask(config.EpicID)
		if taskErr == nil && task.Parent != "" {
			parentEpic, parentErr := e.ticks.GetEpic(task.Parent)
			if parentErr == nil {
				errMsg = fmt.Sprintf("%s\nParent epic: %s (%s)\nRun: ticker run %s",
					errMsg, task.Parent, parentEpic.Title, task.Parent)
			}
		}
		return nil, errors.New(errMsg)
	}

	state.epic = epic

	// Main loop
	for {
		// Check context cancellation
		if ctx.Err() != nil {
			e.writeInterruptionNotes(state, config.EpicID)
			return state.toResult("context cancelled", e.budget.Usage()), ctx.Err()
		}

		// Check budget limits before starting iteration
		if shouldStop, reason := e.budget.ShouldStop(); shouldStop {
			return state.toResult(reason, e.budget.Usage()), nil
		}

		// Check for pause signal
		if config.PauseChan != nil {
			select {
			case paused := <-config.PauseChan:
				if paused {
					// Wait for unpause
					for paused {
						select {
						case <-ctx.Done():
							e.writeInterruptionNotes(state, config.EpicID)
							return state.toResult("context cancelled while paused", e.budget.Usage()), ctx.Err()
						case paused = <-config.PauseChan:
						}
					}
				}
			default:
				// Not paused, continue
			}
		}

		// Get next task with optional debounce
		task, err := e.getNextTaskWithDebounce(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("getting next task: %w", err)
		}

		// No ready tasks - check if epic is truly complete or just blocked
		if task == nil {
			// Check if there are still open/in_progress tasks (blocked)
			hasOpen, err := e.ticks.HasOpenTasks(config.EpicID)
			if err != nil {
				return nil, fmt.Errorf("checking open tasks: %w", err)
			}

			if hasOpen {
				// There are tasks but they're all blocked or awaiting human
				if config.Watch {
					// Watch mode: enter idle state and poll for changes
					idleResult := e.handleWatchIdle(ctx, config, state, watchDeadline)
					if idleResult != nil {
						// Idle period ended (timeout or context cancelled)
						return idleResult, nil
					}
					// Tasks became available, continue loop
					continue
				}
				// Non-watch mode: exit
				return state.toResult(ExitReasonTasksAwaitingHuman, e.budget.Usage()), nil
			}

			// All tasks are closed - epic complete
			state.signal = SignalComplete
			reason := ExitReasonAllTasksCompleted
			if state.iteration == 0 {
				reason = ExitReasonNoTasksFound
			}
			if err := e.ticks.CloseEpic(config.EpicID, reason); err != nil {
				// Log but don't fail - epic may already be closed or race condition
				fmt.Fprintf(os.Stderr, "warning: failed to close epic %s: %v\n", config.EpicID, err)
			}
			return state.toResult(reason, e.budget.Usage()), nil
		}

		// Stuck loop detection - catch agent forgetting to close tasks
		if task.ID == state.lastTaskID {
			state.sameTaskCount++
			if state.sameTaskCount > config.MaxTaskRetries {
				return state.toResult(fmt.Sprintf("stuck on task %s after %d iterations - may need manual review", task.ID, state.sameTaskCount), e.budget.Usage()), nil
			}
		} else {
			state.lastTaskID = task.ID
			state.sameTaskCount = 1
		}

		// Track current task for interruption notes
		state.currentTaskID = task.ID
		state.currentTaskTitle = task.Title

		// Run iteration
		state.iteration++
		iterResult := e.runIteration(ctx, state, task, config.AgentTimeout)

		// Update budget
		e.budget.Add(iterResult.TokensIn, iterResult.TokensOut, iterResult.Cost)

		// Call callback
		if e.OnIterationEnd != nil {
			e.OnIterationEnd(iterResult)
		}

		// Handle timeout specially - add detailed note for recovery
		if iterResult.IsTimeout {
			note := buildTimeoutNote(state.iteration, iterResult.TaskID, config.AgentTimeout, iterResult.Output)
			_ = e.ticks.AddNote(config.EpicID, note)
			continue // Try next iteration
		}

		// Handle iteration error
		if iterResult.Error != nil {
			// Add note about the error for next iteration
			_ = e.ticks.AddNote(config.EpicID, fmt.Sprintf("Iteration %d error: %v", state.iteration, iterResult.Error))
			continue // Try next iteration
		}

		// Check if task was closed by the agent - run verification if so
		if !config.SkipVerify && e.verifyEnabled {
			taskClosed, err := e.wasTaskClosed(task.ID)
			if err != nil {
				// Log but don't fail on status check error
				_ = e.ticks.AddNote(config.EpicID, fmt.Sprintf("Warning: could not check task status: %v", err))
			} else if taskClosed {
				// Run verification in the correct working directory
				verifyResult := e.runVerification(ctx, task.ID, iterResult.Output, config.EpicID, state.workDir)
				if verifyResult != nil && !verifyResult.AllPassed {
					// Verification failed - reopen task and add note
					if err := e.ticks.ReopenTask(task.ID); err != nil {
						_ = e.ticks.AddNote(config.EpicID, fmt.Sprintf("Warning: could not reopen task %s: %v", task.ID, err))
					}
					// Add epic note with failure details
					note := buildVerificationFailureNote(state.iteration, task.ID, verifyResult)
					_ = e.ticks.AddNote(config.EpicID, note)
					// Continue to next iteration - agent will see the failure in notes
					continue
				}
				// Verification passed - track as completed
				state.completedTasks = append(state.completedTasks, task.ID)
			}
		}

		// Handle signals - process with handleSignal and continue to next task
		if iterResult.Signal != SignalNone {
			state.signal = iterResult.Signal
			state.signalReason = iterResult.SignalReason

			if e.OnSignal != nil {
				e.OnSignal(iterResult.Signal, iterResult.SignalReason)
			}

			// Special case: COMPLETE signal is ignored (ticker handles completion via tk next)
			if iterResult.Signal == SignalComplete {
				if e.OnOutput != nil {
					e.OnOutput("\n[Warning: Agent emitted COMPLETE signal - ignoring. Ticker handles completion automatically.]\n")
				}
				// Continue to next iteration - don't close epic
			} else {
				// All other signals (handoff signals) set the task to awaiting state
				// and continue to the next available task
				if err := e.handleSignal(task, iterResult.Signal, iterResult.SignalReason); err != nil {
					// Log error but don't fail - task state update is not critical
					_ = e.ticks.AddNote(config.EpicID, fmt.Sprintf("Warning: could not update task %s awaiting state: %v", task.ID, err))
				}
				// Continue to next task - never block waiting for human response
				// The task is now awaiting human, so tk next won't return it
				continue
			}
		}

		// Checkpoint if at interval
		if config.CheckpointEvery > 0 && state.iteration%config.CheckpointEvery == 0 {
			usage := e.budget.Usage()
			cp := checkpoint.NewCheckpoint(
				config.EpicID,
				state.iteration,
				usage.TotalTokens(),
				usage.Cost,
				state.completedTasks,
			)
			if err := e.checkpoint.Save(cp); err != nil {
				// Log but don't fail on checkpoint error
				_ = e.ticks.AddNote(config.EpicID, fmt.Sprintf("Checkpoint error at iteration %d: %v", state.iteration, err))
			}
		}
	}
}

// runState holds the mutable state during a run.
type runState struct {
	epicID         string
	epic           *ticks.Epic
	iteration      int
	completedTasks []string
	startTime      time.Time
	signal         Signal
	signalReason   string

	// Stuck loop detection
	lastTaskID    string
	sameTaskCount int

	// Current task being worked on (for interruption notes)
	currentTaskID    string
	currentTaskTitle string

	// Worktree support
	workDir string // Working directory for agent (worktree path or empty for current dir)
}

// toResult converts run state to a RunResult.
func (s *runState) toResult(exitReason string, budgetUsage budget.Usage) *RunResult {
	return &RunResult{
		EpicID:         s.epicID,
		Iterations:     s.iteration,
		CompletedTasks: s.completedTasks,
		Duration:       time.Since(s.startTime),
		Signal:         s.signal,
		SignalReason:   s.signalReason,
		ExitReason:     exitReason,
		TotalCost:      budgetUsage.Cost,
		TotalTokens:    budgetUsage.TotalTokens(),
	}
}

// runIteration executes a single iteration.
func (e *Engine) runIteration(ctx context.Context, state *runState, task *ticks.Task, timeout time.Duration) *IterationResult {
	result := &IterationResult{
		Iteration: state.iteration,
		TaskID:    task.ID,
		TaskTitle: task.Title,
	}

	// Mark task as in_progress before starting (enables crash recovery)
	if err := e.ticks.SetStatus(task.ID, "in_progress"); err != nil {
		// Log but continue - status update is not critical
		_ = e.ticks.AddNote(state.epicID, fmt.Sprintf("Warning: could not mark %s as in_progress: %v", task.ID, err))
	}

	// Refresh epic to get latest notes
	epic, err := e.ticks.GetEpic(state.epicID)
	if err != nil {
		result.Error = fmt.Errorf("refreshing epic: %w", err)
		return result
	}
	state.epic = epic

	// Get epic notes
	notes, err := e.ticks.GetNotes(state.epicID)
	if err != nil {
		// Continue without notes
		notes = nil
	}

	// Get human feedback notes for this task
	humanNotes, err := e.ticks.GetHumanNotes(task.ID)
	if err != nil {
		// Continue without human notes
		humanNotes = nil
	}

	// Build prompt
	iterCtx := IterationContext{
		Iteration:     state.iteration,
		Epic:          epic,
		Task:          task,
		EpicNotes:     notes,
		HumanFeedback: humanNotes,
	}

	if e.OnIterationStart != nil {
		e.OnIterationStart(iterCtx)
	}

	prompt := e.prompt.Build(iterCtx)

	// Create context with timeout
	iterCtx2, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Run agent
	startTime := time.Now()

	opts := agent.RunOpts{
		Timeout: timeout,
		WorkDir: state.workDir,
	}

	// Set up rich streaming callback if configured (preferred)
	if e.OnAgentState != nil {
		opts.StateCallback = e.OnAgentState
	}

	// Set up legacy streaming if callback is configured (backward compat)
	var streamChan chan string
	if e.OnOutput != nil {
		streamChan = make(chan string, 100)
		opts.Stream = streamChan

		// Forward stream to callback
		go func() {
			for chunk := range streamChan {
				e.OnOutput(chunk)
			}
		}()
	}

	agentResult, err := e.agent.Run(iterCtx2, prompt, opts)

	// Close stream channel
	if streamChan != nil {
		close(streamChan)
	}

	result.Duration = time.Since(startTime)

	// Handle timeout specially - capture partial output
	if errors.Is(err, agent.ErrTimeout) {
		result.IsTimeout = true
		// agentResult contains partial output on timeout
		if agentResult != nil {
			result.Output = agentResult.Output
			result.TokensIn = agentResult.TokensIn
			result.TokensOut = agentResult.TokensOut
			result.Cost = agentResult.Cost
			// Still persist the partial RunRecord
			if agentResult.Record != nil {
				_ = e.ticks.SetRunRecord(task.ID, agentResult.Record)
			}
		}
		return result
	}

	if err != nil {
		result.Error = fmt.Errorf("agent run: %w", err)
		return result
	}

	result.Output = agentResult.Output
	result.TokensIn = agentResult.TokensIn
	result.TokensOut = agentResult.TokensOut
	result.Cost = agentResult.Cost

	// Persist RunRecord to task (enables viewing historical run data)
	if agentResult.Record != nil {
		if err := e.ticks.SetRunRecord(task.ID, agentResult.Record); err != nil {
			// Log but don't fail on record persistence error
			_ = e.ticks.AddNote(state.epicID, fmt.Sprintf("Warning: could not persist RunRecord for %s: %v", task.ID, err))
		}
	}

	// Parse signals
	result.Signal, result.SignalReason = ParseSignals(agentResult.Output)

	return result
}

// buildTimeoutNote creates a detailed note about a timeout for recovery.
// Includes iteration number, task ID, timeout duration, and partial output summary.
func buildTimeoutNote(iteration int, taskID string, timeout time.Duration, partialOutput string) string {
	note := fmt.Sprintf("Iteration %d timed out after %v on task %s.", iteration, timeout, taskID)

	if partialOutput != "" {
		// Truncate partial output for the note (keep last portion as most relevant)
		const maxOutputLen = 500
		outputSummary := partialOutput
		if len(outputSummary) > maxOutputLen {
			outputSummary = "..." + outputSummary[len(outputSummary)-maxOutputLen:]
		}
		// Clean up for note format (replace newlines with spaces for readability)
		outputSummary = strings.ReplaceAll(outputSummary, "\n", " ")
		outputSummary = strings.Join(strings.Fields(outputSummary), " ") // normalize whitespace
		note += fmt.Sprintf(" Partial output: %s", outputSummary)
	} else {
		note += " No output captured before timeout."
	}

	return note
}

// writeInterruptionNotes writes notes to both the epic and current task when interrupted.
func (e *Engine) writeInterruptionNotes(state *runState, epicID string) {
	if state.currentTaskID == "" {
		// No task in progress, just write epic note
		_ = e.ticks.AddNote(epicID, fmt.Sprintf("Run interrupted by user at iteration %d. No task was in progress.", state.iteration))
		return
	}

	// Build interruption message
	msg := fmt.Sprintf("Run interrupted by user at iteration %d while working on task %s (%s).",
		state.iteration, state.currentTaskID, state.currentTaskTitle)

	// Write note to epic
	_ = e.ticks.AddNote(epicID, msg+" Task may be partially complete - review before continuing.")

	// Write note to the interrupted task
	_ = e.ticks.AddNote(state.currentTaskID, "Work on this task was interrupted by user. May be partially complete.")
}

// wasTaskClosed checks if a task was closed by the agent.
func (e *Engine) wasTaskClosed(taskID string) (bool, error) {
	task, err := e.ticks.GetTask(taskID)
	if err != nil {
		return false, err
	}
	return task.Status == "closed", nil
}

// runVerification executes verification for a completed task.
// workDir specifies the directory to verify (worktree path or empty for cwd).
// Returns nil if verification is not enabled.
func (e *Engine) runVerification(ctx context.Context, taskID string, agentOutput string, epicID string, workDir string) *verify.Results {
	if !e.verifyEnabled {
		return nil
	}

	// Determine verification directory
	dir := workDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil
		}
	}

	// Create GitVerifier for the correct directory
	gitVerifier := verify.NewGitVerifier(dir)
	if gitVerifier == nil {
		return nil // Not a git repo
	}

	// Call start callback
	if e.OnVerificationStart != nil {
		e.OnVerificationStart(taskID)
	}

	// Run verification
	runner := verify.NewRunner(dir, gitVerifier)
	results := runner.Run(ctx, taskID, agentOutput)

	// Call end callback
	if e.OnVerificationEnd != nil {
		e.OnVerificationEnd(taskID, results)
	}

	return results
}

// signalToAwaiting maps signals to their corresponding awaiting states.
// Signals not in this map don't trigger awaiting (e.g., SignalComplete, SignalNone).
var signalToAwaiting = map[Signal]string{
	SignalEject:           "work",
	SignalBlocked:         "input", // Legacy - maps to InputNeeded for backwards compatibility
	SignalApprovalNeeded:  "approval",
	SignalInputNeeded:     "input",
	SignalReviewRequested: "review",
	SignalContentReview:   "content",
	SignalEscalate:        "escalation",
	SignalCheckpoint:      "checkpoint",
}

// handleSignal processes an agent signal and updates the task state accordingly.
// For COMPLETE signals, it checks the task's requires field before closing.
// For handoff signals (EJECT, BLOCKED, etc.), it sets the task to awaiting state.
// Returns nil for unknown signals or SignalNone (no-op).
func (e *Engine) handleSignal(task *ticks.Task, signal Signal, context string) error {
	switch signal {
	case SignalNone:
		// No signal detected - nothing to do
		return nil

	case SignalComplete:
		// Check for pre-declared approval gate
		if task.Requires != nil && *task.Requires != "" {
			note := "Work complete, requires " + *task.Requires
			return e.ticks.SetAwaiting(task.ID, *task.Requires, note)
		}
		return e.ticks.CloseTask(task.ID, "Completed by agent")

	default:
		// Check if this signal maps to an awaiting state
		if awaiting, ok := signalToAwaiting[signal]; ok {
			return e.ticks.SetAwaiting(task.ID, awaiting, context)
		}
		// Unknown signal - gracefully ignore
		return nil
	}
}

// buildVerificationFailureNote creates a note about verification failure.
// Includes iteration, task ID, and truncated verification output.
func buildVerificationFailureNote(iteration int, taskID string, results *verify.Results) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Iteration %d: Verification failed for task %s.", iteration, taskID))

	// Add details from failed verifiers
	for _, r := range results.FailedResults() {
		sb.WriteString(fmt.Sprintf(" [%s] ", r.Verifier))
		if r.Output != "" {
			// Truncate output if too long
			output := r.Output
			const maxLen = 300
			if len(output) > maxLen {
				output = output[:maxLen] + "..."
			}
			// Clean up for single-line note
			output = strings.ReplaceAll(output, "\n", " | ")
			output = strings.Join(strings.Fields(output), " ")
			sb.WriteString(output)
		}
	}

	sb.WriteString(" Please fix and close the task again.")
	return sb.String()
}

// getNextTaskWithDebounce gets the next available task with optional debounce.
// If DebounceInterval is set, it waits after a task becomes available to allow
// humans to finish editing (e.g., adding notes after reject).
// After the debounce wait, it re-fetches the task to get any updates.
func (e *Engine) getNextTaskWithDebounce(ctx context.Context, config RunConfig) (*ticks.Task, error) {
	task, err := e.ticks.NextTask(config.EpicID)
	if err != nil || task == nil {
		return task, err
	}

	// No debounce configured - return immediately
	if config.DebounceInterval <= 0 {
		return task, nil
	}

	// Task just became available - wait for potential follow-up edits
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(config.DebounceInterval):
		// Continue after debounce
	}

	// Re-fetch the task to get any updates made during debounce period
	// (e.g., human might have added notes, changed description, etc.)
	return e.ticks.GetTask(task.ID)
}

// handleWatchIdle enters idle state and polls for new tasks.
// Returns nil if tasks become available (continue processing).
// Returns a RunResult if watch should end (timeout or cancellation).
func (e *Engine) handleWatchIdle(ctx context.Context, config RunConfig, state *runState, watchDeadline time.Time) *RunResult {
	// Notify caller that we're entering idle state
	if e.OnIdle != nil {
		e.OnIdle()
	}

	// Poll for new tasks until available, timeout, or cancellation
	for {
		// Check watch timeout
		if !watchDeadline.IsZero() && time.Now().After(watchDeadline) {
			return state.toResult(ExitReasonWatchTimeout, e.budget.Usage())
		}

		// Wait for poll interval or cancellation
		select {
		case <-ctx.Done():
			e.writeInterruptionNotes(state, config.EpicID)
			return state.toResult("context cancelled while idle", e.budget.Usage())

		case <-time.After(config.WatchPollInterval):
			// Check for new tasks
			task, err := e.ticks.NextTask(config.EpicID)
			if err != nil {
				// Log but continue polling - transient errors shouldn't stop watch
				continue
			}

			if task != nil {
				// Tasks available - return nil to continue processing
				return nil
			}

			// Still no tasks - check if epic is now complete
			hasOpen, err := e.ticks.HasOpenTasks(config.EpicID)
			if err != nil {
				continue
			}

			if !hasOpen {
				// All tasks closed while idle - epic complete
				state.signal = SignalComplete
				reason := ExitReasonAllTasksCompleted
				if err := e.ticks.CloseEpic(config.EpicID, reason); err != nil {
					fmt.Fprintf(os.Stderr, "warning: failed to close epic %s: %v\n", config.EpicID, err)
				}
				return state.toResult(reason, e.budget.Usage())
			}

			// Still blocked/awaiting - re-trigger OnIdle callback
			if e.OnIdle != nil {
				e.OnIdle()
			}
		}
	}
}
