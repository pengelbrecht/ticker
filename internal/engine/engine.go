package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	"github.com/pengelbrecht/ticker/internal/ticks"
)

// Engine orchestrates the Ralph iteration loop.
type Engine struct {
	agent      agent.Agent
	ticks      *ticks.Client
	budget     *budget.Tracker
	checkpoint *checkpoint.Manager
	prompt     *PromptBuilder

	// Callbacks for TUI integration (optional)
	OnIterationStart func(ctx IterationContext)
	OnIterationEnd   func(result *IterationResult)
	OnOutput         func(chunk string)
	OnSignal         func(signal Signal, reason string)

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

	// MaxCost is the maximum cost in USD (0 = 20.00 default).
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
}

// Defaults for RunConfig.
const (
	DefaultMaxIterations   = 50
	DefaultMaxCost         = 20.00
	DefaultCheckpointEvery = 5
	DefaultAgentTimeout    = 30 * time.Minute
	DefaultMaxTaskRetries  = 3
)

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
func NewEngine(a agent.Agent, t *ticks.Client, b *budget.Tracker, c *checkpoint.Manager) *Engine {
	return &Engine{
		agent:      a,
		ticks:      t,
		budget:     b,
		checkpoint: c,
		prompt:     NewPromptBuilder(),
	}
}

// Run executes the engine loop until completion, signal, or budget exceeded.
func (e *Engine) Run(ctx context.Context, config RunConfig) (*RunResult, error) {
	// Apply defaults
	if config.MaxIterations == 0 {
		config.MaxIterations = DefaultMaxIterations
	}
	if config.MaxCost == 0 {
		config.MaxCost = DefaultMaxCost
	}
	if config.CheckpointEvery == 0 {
		config.CheckpointEvery = DefaultCheckpointEvery
	}
	if config.MaxTaskRetries == 0 {
		config.MaxTaskRetries = DefaultMaxTaskRetries
	}
	if config.AgentTimeout == 0 {
		config.AgentTimeout = DefaultAgentTimeout
	}

	// Initialize state
	state := &runState{
		epicID:         config.EpicID,
		iteration:      0,
		completedTasks: []string{},
		startTime:      time.Now(),
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
	state.epic = epic

	// Main loop
	for {
		// Check context cancellation
		if ctx.Err() != nil {
			return state.toResult("context cancelled"), ctx.Err()
		}

		// Check budget limits before starting iteration
		if shouldStop, reason := e.budget.ShouldStop(); shouldStop {
			return state.toResult(reason), nil
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
							return state.toResult("context cancelled while paused"), ctx.Err()
						case paused = <-config.PauseChan:
						}
					}
				}
			default:
				// Not paused, continue
			}
		}

		// Get next task
		task, err := e.ticks.NextTask(config.EpicID)
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
				// There are tasks but they're all blocked - don't close epic
				return state.toResult("no ready tasks (remaining tasks are blocked)"), nil
			}

			// All tasks are closed - epic complete
			state.signal = SignalComplete
			reason := "all tasks completed"
			if state.iteration == 0 {
				reason = "no tasks found"
			}
			_ = e.ticks.CloseEpic(config.EpicID, reason)
			return state.toResult(reason), nil
		}

		// Stuck loop detection - catch agent forgetting to close tasks
		if task.ID == state.lastTaskID {
			state.sameTaskCount++
			if state.sameTaskCount > config.MaxTaskRetries {
				return state.toResult(fmt.Sprintf("stuck on task %s after %d iterations - may need manual review", task.ID, state.sameTaskCount)), nil
			}
		} else {
			state.lastTaskID = task.ID
			state.sameTaskCount = 1
		}

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

		// Handle signals
		if iterResult.Signal != SignalNone {
			state.signal = iterResult.Signal
			state.signalReason = iterResult.SignalReason

			if e.OnSignal != nil {
				e.OnSignal(iterResult.Signal, iterResult.SignalReason)
			}

			switch iterResult.Signal {
			case SignalComplete:
				// Agent emitted COMPLETE - ignore it and continue loop
				// Ticker detects completion naturally via tk next returning nil
				// Log this as a warning since agent shouldn't emit COMPLETE
				if e.OnOutput != nil {
					e.OnOutput("\n[Warning: Agent emitted COMPLETE signal - ignoring. Ticker handles completion automatically.]\n")
				}
				// Continue to next iteration - don't close epic
			case SignalEject:
				return state.toResult(fmt.Sprintf("agent ejected: %s", iterResult.SignalReason)), nil
			case SignalBlocked:
				return state.toResult(fmt.Sprintf("agent blocked: %s", iterResult.SignalReason)), nil
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
	lastTaskID     string
	sameTaskCount  int
}

// toResult converts run state to a RunResult.
func (s *runState) toResult(exitReason string) *RunResult {
	return &RunResult{
		EpicID:         s.epicID,
		Iterations:     s.iteration,
		CompletedTasks: s.completedTasks,
		Duration:       time.Since(s.startTime),
		Signal:         s.signal,
		SignalReason:   s.signalReason,
		ExitReason:     exitReason,
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

	// Build prompt
	iterCtx := IterationContext{
		Iteration: state.iteration,
		Epic:      epic,
		Task:      task,
		EpicNotes: notes,
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
