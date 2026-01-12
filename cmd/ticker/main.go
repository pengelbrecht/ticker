package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	"github.com/pengelbrecht/ticker/internal/engine"
	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/tui"
	"github.com/pengelbrecht/ticker/internal/update"
	"github.com/pengelbrecht/ticker/internal/verify"
)

// Version is set at build time via ldflags
var Version = "dev"

// Exit codes per spec
const (
	ExitSuccess       = 0
	ExitMaxIterations = 1
	ExitEject         = 2
	ExitBlocked       = 3
	ExitError         = 4
)

var rootCmd = &cobra.Command{
	Use:   "ticker",
	Short: "Autonomous AI agent loop runner",
	Long: `Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents
in continuous loops until tasks are complete. It wraps the Ticks issue tracker
and orchestrates coding agents to autonomously complete epics.`,
	Version: Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Check for updates periodically (once per day, cached)
		if notice := update.CheckPeriodically(Version); notice != "" {
			fmt.Fprintln(os.Stderr, notice)
		}
	},
}

var runCmd = &cobra.Command{
	Use:   "run [epic-id]",
	Short: "Run an epic with the AI agent",
	Long: `Run starts the Ralph loop for the specified epic. The agent will iterate
through tasks until completion, ejection, or budget limits are reached.

Exit codes:
  0 - Success (epic completed)
  1 - Max iterations reached
  2 - Agent ejected
  3 - Agent blocked
  4 - Error`,
	Args: cobra.MaximumNArgs(1),
	Run:  runRun,
}

var resumeCmd = &cobra.Command{
	Use:   "resume <checkpoint-id>",
	Short: "Resume from a checkpoint",
	Long: `Resume continues a run from a saved checkpoint.

The checkpoint ID can be found using 'ticker checkpoints'.`,
	Args: cobra.ExactArgs(1),
	Run:  runResume,
}

var checkpointsCmd = &cobra.Command{
	Use:   "checkpoints [epic-id]",
	Short: "List available checkpoints",
	Long: `List all saved checkpoints, optionally filtered by epic.

Checkpoints are saved at regular intervals during a run and can be
used with 'ticker resume' to continue from that point.`,
	Args: cobra.MaximumNArgs(1),
	Run:  runCheckpoints,
}

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade ticker to the latest version",
	Long:  `Downloads and installs the latest version of ticker.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Current version: %s\n", Version)
		fmt.Println("Checking for updates...")

		release, hasUpdate, err := update.CheckForUpdate(Version)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking for updates: %v\n", err)
			os.Exit(1)
		}

		if !hasUpdate {
			fmt.Println("Already at latest version")
			return
		}

		fmt.Printf("New version available: %s\n", release.Version)
		fmt.Println("Upgrading...")

		if err := update.Update(Version); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			method := update.DetectInstallMethod()
			fmt.Fprintln(os.Stderr, update.UpdateInstructions(method))
			os.Exit(1)
		}

		fmt.Printf("Successfully upgraded to %s\n", release.Version)
	},
}

func init() {
	// Run command flags
	runCmd.Flags().IntP("max-iterations", "n", 50, "Maximum number of iterations")
	runCmd.Flags().Float64("max-cost", 0, "Maximum cost in USD (0 = disabled)")
	runCmd.Flags().Int("checkpoint-interval", 5, "Save checkpoint every N iterations (0 to disable)")
	runCmd.Flags().Int("max-task-retries", 3, "Maximum iterations on same task before assuming stuck")
	runCmd.Flags().Bool("auto", false, "Auto-select next ready epic")
	runCmd.Flags().Bool("headless", false, "Run without TUI (stdout/stderr only)")
	runCmd.Flags().Bool("skip-verify", false, "Skip verification after task completion")
	runCmd.Flags().Bool("verify-only", false, "Run verification without the agent (for debugging)")
	runCmd.Flags().Bool("worktree", false, "Run epic in isolated git worktree")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(checkpointsCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(ExitError)
	}
}

func runRun(cmd *cobra.Command, args []string) {
	auto, _ := cmd.Flags().GetBool("auto")
	headless, _ := cmd.Flags().GetBool("headless")
	maxIterations, _ := cmd.Flags().GetInt("max-iterations")
	maxCost, _ := cmd.Flags().GetFloat64("max-cost")
	checkpointInterval, _ := cmd.Flags().GetInt("checkpoint-interval")
	maxTaskRetries, _ := cmd.Flags().GetInt("max-task-retries")
	skipVerify, _ := cmd.Flags().GetBool("skip-verify")
	verifyOnly, _ := cmd.Flags().GetBool("verify-only")
	useWorktree, _ := cmd.Flags().GetBool("worktree")

	// Check mutual exclusivity
	if skipVerify && verifyOnly {
		fmt.Fprintln(os.Stderr, "Error: --skip-verify and --verify-only are mutually exclusive")
		os.Exit(ExitError)
	}

	// Handle --verify-only mode (no epic required, runs in current directory)
	if verifyOnly {
		runVerifyOnly()
		return
	}

	var epicID string
	var epicTitle string

	if len(args) > 0 {
		epicID = args[0]
	} else if auto {
		// Auto-select: use tk to find a ready epic
		selected, err := autoSelectEpic()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error auto-selecting epic: %v\n", err)
			os.Exit(ExitError)
		}
		if selected == "" {
			fmt.Fprintln(os.Stderr, "No ready epics found")
			os.Exit(ExitError)
		}
		epicID = selected
		fmt.Printf("Auto-selected epic: %s\n", epicID)
	} else if !headless {
		// Interactive mode: show epic picker
		selected := runPicker()
		if selected == nil {
			os.Exit(0) // User quit without selecting
		}
		epicID = selected.ID
		epicTitle = selected.Title
	} else {
		fmt.Fprintln(os.Stderr, "Error: either provide an epic-id or use --auto")
		os.Exit(ExitError)
	}

	// Get epic info for TUI (if not already from picker)
	if epicTitle == "" {
		ticksClient := ticks.NewClient()
		epic, err := ticksClient.GetEpic(epicID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting epic: %v\n", err)
			os.Exit(ExitError)
		}
		epicTitle = epic.Title
	}

	// TUI mode (default)
	if !headless {
		runWithTUI(epicID, epicTitle, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, useWorktree)
		return
	}

	// Headless mode
	runHeadless(epicID, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, useWorktree)
}

func runWithTUI(epicID, epicTitle string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify, useWorktree bool) {
	// Create pause channel for TUI <-> engine communication
	pauseChan := make(chan bool, 1)

	// Create TUI model
	m := tui.New(tui.Config{
		EpicID:       epicID,
		EpicTitle:    epicTitle,
		MaxCost:      maxCost,
		MaxIteration: maxIterations,
		PauseChan:    pauseChan,
	})

	// Create program
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Create context for engine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize engine components
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintln(os.Stderr, "Error: claude CLI not found. Please install Claude Code.")
		os.Exit(ExitError)
	}

	ticksClient := ticks.NewClient()
	budgetTracker := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations,
		MaxCost:       maxCost,
	})
	checkpointMgr := checkpoint.NewManager()

	// Create engine
	eng := engine.NewEngine(claudeAgent, ticksClient, budgetTracker, checkpointMgr)

	// Set up verification runner (unless --skip-verify)
	if !skipVerify {
		if runner := createVerifyRunner(); runner != nil {
			eng.SetVerifyRunner(runner)
		}
	}

	// Helper to refresh task list in TUI
	refreshTasks := func() {
		tasks, err := ticksClient.ListTasks(epicID)
		if err != nil {
			return // Silently ignore errors
		}
		taskInfos := make([]tui.TaskInfo, len(tasks))
		for i, t := range tasks {
			taskInfos[i] = tui.TaskInfo{
				ID:        t.ID,
				Title:     t.Title,
				Status:    tui.TaskStatus(t.Status),
				BlockedBy: t.BlockedBy,
			}
		}
		p.Send(tui.TasksUpdateMsg{Tasks: taskInfos})

		// Fetch and send RunRecords for closed tasks
		for _, t := range tasks {
			if t.Status == "closed" {
				if record, err := ticksClient.GetRunRecord(t.ID); err == nil && record != nil {
					p.Send(tui.TaskRunRecordMsg{TaskID: t.ID, RunRecord: record})
				}
			}
		}
	}

	// Initial task list load
	go refreshTasks()

	// Wire engine callbacks to send TUI messages

	// Track previous snapshot state for delta-based TUI updates
	var prevOutput, prevThinking string
	var prevToolID string

	// Rich streaming callback - converts AgentStateSnapshot to TUI messages
	eng.OnAgentState = func(snap agent.AgentStateSnapshot) {
		// Send text deltas (only new content since last update)
		if snap.Output != prevOutput {
			delta := snap.Output[len(prevOutput):]
			if delta != "" {
				p.Send(tui.AgentTextMsg{Text: delta})
			}
			prevOutput = snap.Output
		}

		// Send thinking deltas
		if snap.Thinking != prevThinking {
			delta := snap.Thinking[len(prevThinking):]
			if delta != "" {
				p.Send(tui.AgentThinkingMsg{Text: delta})
			}
			prevThinking = snap.Thinking
		}

		// Send tool activity updates
		if snap.ActiveTool != nil && snap.ActiveTool.ID != prevToolID {
			// New tool started
			p.Send(tui.AgentToolStartMsg{
				ID:   snap.ActiveTool.ID,
				Name: snap.ActiveTool.Name,
			})
			prevToolID = snap.ActiveTool.ID
		} else if snap.ActiveTool == nil && prevToolID != "" {
			// Tool ended - find it in history to get duration and error status
			for _, tool := range snap.ToolHistory {
				if tool.ID == prevToolID {
					p.Send(tui.AgentToolEndMsg{
						ID:       tool.ID,
						Name:     tool.Name,
						Duration: tool.Duration,
						IsError:  tool.IsError,
					})
					break
				}
			}
			prevToolID = ""
		}

		// Send metrics update (including model name)
		p.Send(tui.AgentMetricsMsg{
			InputTokens:         snap.Metrics.InputTokens,
			OutputTokens:        snap.Metrics.OutputTokens,
			CacheReadTokens:     snap.Metrics.CacheReadTokens,
			CacheCreationTokens: snap.Metrics.CacheCreationTokens,
			CostUSD:             snap.Metrics.CostUSD,
			Model:               snap.Model,
		})

		// Send status update
		p.Send(tui.AgentStatusMsg{
			Status: snap.Status,
			Error:  snap.ErrorMsg,
		})
	}

	// Legacy output callback kept for backward compatibility
	eng.OnOutput = func(chunk string) {
		p.Send(tui.OutputMsg(chunk))
	}

	eng.OnIterationStart = func(ctx engine.IterationContext) {
		// Reset delta tracking state for new iteration
		prevOutput = ""
		prevThinking = ""
		prevToolID = ""

		p.Send(tui.IterationStartMsg{
			Iteration: ctx.Iteration,
			TaskID:    ctx.Task.ID,
			TaskTitle: ctx.Task.Title,
		})
	}

	eng.OnIterationEnd = func(result *engine.IterationResult) {
		p.Send(tui.IterationEndMsg{
			Iteration: result.Iteration,
			Cost:      result.Cost,
			Tokens:    result.TokensIn + result.TokensOut,
		})
		// Refresh task list after each iteration
		go refreshTasks()
	}

	eng.OnSignal = func(sig engine.Signal, reason string) {
		p.Send(tui.SignalMsg{Signal: sig.String(), Reason: reason})
	}

	// Verification callbacks for TUI status display
	eng.OnVerificationStart = func(taskID string) {
		p.Send(tui.VerifyStartMsg{TaskID: taskID})
	}

	eng.OnVerificationEnd = func(taskID string, results *verify.Results) {
		// Build summary from results
		summary := ""
		passed := true
		if results != nil {
			passed = results.AllPassed
			summary = results.Summary()
		}
		p.Send(tui.VerifyResultMsg{
			TaskID:  taskID,
			Passed:  passed,
			Summary: summary,
		})
		// Refresh task list after verification (task may have been reopened)
		go refreshTasks()
	}

	// Run engine in background
	go func() {
		config := engine.RunConfig{
			EpicID:          epicID,
			MaxIterations:   maxIterations,
			MaxCost:         maxCost,
			CheckpointEvery: checkpointInterval,
			MaxTaskRetries:  maxTaskRetries,
			PauseChan:       pauseChan,
			UseWorktree:     useWorktree,
		}

		result, err := eng.Run(ctx, config)
		if err != nil {
			p.Send(tui.ErrorMsg{Err: err})
			return
		}

		p.Send(tui.RunCompleteMsg{
			Reason:     result.ExitReason,
			Signal:     result.Signal.String(),
			Iterations: result.Iterations,
			Cost:       result.TotalCost,
		})
	}()

	// Run TUI (blocks until quit)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(ExitError)
	}

	// Cancel engine context when TUI exits
	cancel()
}

func runHeadless(epicID string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify, useWorktree bool) {
	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, shutting down...")
		cancel()
	}()

	// Initialize components
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintln(os.Stderr, "Error: claude CLI not found. Please install Claude Code.")
		os.Exit(ExitError)
	}

	ticksClient := ticks.NewClient()
	budgetTracker := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations,
		MaxCost:       maxCost,
	})
	checkpointMgr := checkpoint.NewManager()

	// Create and configure engine
	eng := engine.NewEngine(claudeAgent, ticksClient, budgetTracker, checkpointMgr)

	// Set up verification runner (unless --skip-verify)
	if !skipVerify {
		if runner := createVerifyRunner(); runner != nil {
			eng.SetVerifyRunner(runner)
		}
	}

	// Set up output callback for headless mode
	eng.OnOutput = func(chunk string) {
		fmt.Print(chunk)
	}

	eng.OnIterationStart = func(ctx engine.IterationContext) {
		fmt.Printf("\n=== Iteration %d: [%s] %s ===\n", ctx.Iteration, ctx.Task.ID, ctx.Task.Title)
	}

	eng.OnIterationEnd = func(result *engine.IterationResult) {
		fmt.Printf("\n--- Iteration %d complete (tokens: %d, cost: $%.4f) ---\n",
			result.Iteration, result.TokensIn+result.TokensOut, result.Cost)
	}

	eng.OnSignal = func(sig engine.Signal, reason string) {
		if reason != "" {
			fmt.Printf("\nSignal: %s - %s\n", sig, reason)
		} else {
			fmt.Printf("\nSignal: %s\n", sig)
		}
	}

	// Verification callbacks for headless mode
	eng.OnVerificationStart = func(taskID string) {
		fmt.Printf("\n[Verification] Running verification checks for task %s...\n", taskID)
	}

	eng.OnVerificationEnd = func(taskID string, results *verify.Results) {
		if results == nil {
			return
		}
		if results.AllPassed {
			fmt.Println("[Verification] ✓ All checks passed")
		} else {
			fmt.Println("[Verification] ✗ Verification failed:")
			fmt.Println(results.Summary())
			fmt.Println("[Verification] Task reopened - please address the issues above")
		}
	}

	// Run
	config := engine.RunConfig{
		EpicID:          epicID,
		MaxIterations:   maxIterations,
		MaxCost:         maxCost,
		CheckpointEvery: checkpointInterval,
		MaxTaskRetries:  maxTaskRetries,
		UseWorktree:     useWorktree,
	}

	fmt.Printf("Starting ticker run for epic %s\n", epicID)
	if useWorktree {
		fmt.Println("Running in isolated worktree")
	}
	fmt.Printf("Budget: max %d iterations, $%.2f\n", maxIterations, maxCost)
	fmt.Println()

	result, err := eng.Run(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== Run Complete ===")
	fmt.Printf("Epic: %s\n", result.EpicID)
	fmt.Printf("Iterations: %d\n", result.Iterations)
	fmt.Printf("Duration: %v\n", result.Duration.Round(1000000000))
	fmt.Printf("Exit reason: %s\n", result.ExitReason)

	// Exit with appropriate code
	switch result.Signal {
	case engine.SignalComplete:
		os.Exit(ExitSuccess)
	case engine.SignalEject:
		os.Exit(ExitEject)
	case engine.SignalBlocked:
		os.Exit(ExitBlocked)
	default:
		// Check if it was budget exceeded
		if result.Iterations >= maxIterations {
			os.Exit(ExitMaxIterations)
		}
		os.Exit(ExitSuccess)
	}
}

func runResume(cmd *cobra.Command, args []string) {
	checkpointID := args[0]

	// Load checkpoint
	checkpointMgr := checkpoint.NewManager()
	cp, err := checkpointMgr.Load(checkpointID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading checkpoint: %v\n", err)
		os.Exit(ExitError)
	}

	fmt.Printf("Resuming from checkpoint %s\n", checkpointID)
	fmt.Printf("Epic: %s, Iteration: %d, Cost: $%.4f\n", cp.EpicID, cp.Iteration, cp.TotalCost)

	// Get remaining budget (use defaults minus what was used)
	remainingIterations := engine.DefaultMaxIterations - cp.Iteration
	remainingCost := engine.DefaultMaxCost - cp.TotalCost

	if remainingIterations <= 0 {
		fmt.Fprintln(os.Stderr, "Error: checkpoint already at iteration limit")
		os.Exit(ExitError)
	}
	if remainingCost <= 0 {
		fmt.Fprintln(os.Stderr, "Error: checkpoint already at cost limit")
		os.Exit(ExitError)
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nInterrupted, shutting down...")
		cancel()
	}()

	// Initialize components
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintln(os.Stderr, "Error: claude CLI not found. Please install Claude Code.")
		os.Exit(ExitError)
	}

	ticksClient := ticks.NewClient()
	budgetTracker := budget.NewTracker(budget.Limits{
		MaxIterations: remainingIterations,
		MaxCost:       remainingCost,
	})

	// Create and configure engine
	eng := engine.NewEngine(claudeAgent, ticksClient, budgetTracker, checkpointMgr)

	eng.OnOutput = func(chunk string) {
		fmt.Print(chunk)
	}

	eng.OnIterationStart = func(ctx engine.IterationContext) {
		fmt.Printf("\n=== Iteration %d: [%s] %s ===\n", ctx.Iteration, ctx.Task.ID, ctx.Task.Title)
	}

	eng.OnIterationEnd = func(result *engine.IterationResult) {
		fmt.Printf("\n--- Iteration %d complete (tokens: %d, cost: $%.4f) ---\n",
			result.Iteration, result.TokensIn+result.TokensOut, result.Cost)
	}

	// Run with resume
	config := engine.RunConfig{
		EpicID:     cp.EpicID,
		ResumeFrom: checkpointID,
	}

	result, err := eng.Run(ctx, config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Print summary
	fmt.Println()
	fmt.Println("=== Run Complete ===")
	fmt.Printf("Epic: %s\n", result.EpicID)
	fmt.Printf("Iterations: %d (resumed from %d)\n", result.Iterations, cp.Iteration)
	fmt.Printf("Exit reason: %s\n", result.ExitReason)

	switch result.Signal {
	case engine.SignalComplete:
		os.Exit(ExitSuccess)
	case engine.SignalEject:
		os.Exit(ExitEject)
	case engine.SignalBlocked:
		os.Exit(ExitBlocked)
	default:
		os.Exit(ExitSuccess)
	}
}

func runCheckpoints(cmd *cobra.Command, args []string) {
	checkpointMgr := checkpoint.NewManager()

	var checkpoints []checkpoint.Checkpoint
	var err error

	if len(args) > 0 {
		checkpoints, err = checkpointMgr.ListForEpic(args[0])
	} else {
		checkpoints, err = checkpointMgr.List()
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing checkpoints: %v\n", err)
		os.Exit(ExitError)
	}

	if len(checkpoints) == 0 {
		fmt.Println("No checkpoints found")
		return
	}

	fmt.Printf("%-15s %-10s %-10s %-12s %-20s\n", "ID", "Epic", "Iteration", "Cost", "Timestamp")
	fmt.Println("-------------------------------------------------------------------")
	for _, cp := range checkpoints {
		fmt.Printf("%-15s %-10s %-10d $%-11.4f %s\n",
			cp.ID, cp.EpicID, cp.Iteration, cp.TotalCost, cp.Timestamp.Format("2006-01-02 15:04"))
	}
}

// autoSelectEpic uses tk to find a ready epic
func autoSelectEpic() (string, error) {
	ticksClient := ticks.NewClient()
	epic, err := ticksClient.NextReadyEpic()
	if err != nil {
		return "", err
	}
	if epic == nil {
		return "", nil
	}
	return epic.ID, nil
}

// runPicker shows the interactive epic picker and returns the selected epic
func runPicker() *tui.EpicInfo {
	ticksClient := ticks.NewClient()

	// Get ready epics
	epics, err := ticksClient.ListReadyEpics()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing epics: %v\n", err)
		os.Exit(ExitError)
	}

	if len(epics) == 0 {
		fmt.Fprintln(os.Stderr, "No ready epics found")
		os.Exit(0)
	}

	// Convert to EpicInfo with task counts
	epicInfos := make([]tui.EpicInfo, len(epics))
	for i, e := range epics {
		tasks, _ := ticksClient.ListTasks(e.ID)
		epicInfos[i] = tui.EpicInfo{
			ID:       e.ID,
			Title:    e.Title,
			Priority: e.Priority,
			Tasks:    len(tasks),
		}
	}

	// Run picker
	p := tui.NewPicker(epicInfos)
	program := tea.NewProgram(p, tea.WithAltScreen())

	model, err := program.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running picker: %v\n", err)
		os.Exit(ExitError)
	}

	picker := model.(tui.Picker)
	return picker.Selected()
}

// createVerifyRunner creates a verification runner for the current directory.
// Returns nil if verification is disabled via config or not in a git repo.
func createVerifyRunner() *verify.Runner {
	dir, err := os.Getwd()
	if err != nil {
		return nil
	}

	// Check config
	config, err := verify.LoadConfig(dir)
	if err != nil {
		// Config error - log but continue without verification
		fmt.Fprintf(os.Stderr, "Warning: error loading verification config: %v\n", err)
		return nil
	}
	if !config.IsEnabled() {
		return nil
	}

	// Create GitVerifier (returns nil if not in git repo)
	gitVerifier := verify.NewGitVerifier(dir)
	if gitVerifier == nil {
		return nil
	}

	return verify.NewRunner(dir, gitVerifier)
}

// runVerifyOnly runs verification without the agent (--verify-only mode).
// Useful for debugging verification setup.
func runVerifyOnly() {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(ExitError)
	}

	fmt.Println("Running verification (--verify-only mode)")
	fmt.Println()

	// Check config
	config, err := verify.LoadConfig(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading verification config: %v\n", err)
		os.Exit(ExitError)
	}
	if !config.IsEnabled() {
		fmt.Println("Verification is disabled in .ticker/config.json")
		os.Exit(ExitSuccess)
	}

	// Create GitVerifier
	gitVerifier := verify.NewGitVerifier(dir)
	if gitVerifier == nil {
		fmt.Println("Not a git repository - GitVerifier not available")
		os.Exit(ExitSuccess)
	}

	// Create runner and run verification
	runner := verify.NewRunner(dir, gitVerifier)
	ctx := context.Background()
	results := runner.Run(ctx, "", "") // Empty task ID and output for verify-only

	// Output results
	fmt.Println(results.Summary())

	// Exit with appropriate code
	if results.AllPassed {
		os.Exit(ExitSuccess)
	}
	os.Exit(1) // Exit code 1 for verification failure
}
