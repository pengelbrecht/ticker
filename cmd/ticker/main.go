package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	"github.com/pengelbrecht/ticker/internal/engine"
	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/tui"
)

var version = "0.1.0"

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
	Version: version,
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

const installScriptURL = "https://raw.githubusercontent.com/pengelbrecht/ticker/main/scripts/install.sh"

var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade ticker to the latest version",
	Long:  `Downloads and runs the installation script to upgrade ticker in-place.`,
	Run: func(cmd *cobra.Command, args []string) {
		if runtime.GOOS == "windows" {
			fmt.Fprintln(os.Stderr, "Error: upgrade command is not supported on Windows")
			fmt.Fprintln(os.Stderr, "Please download the latest release manually from GitHub")
			os.Exit(1)
		}

		fmt.Printf("Current version: %s\n", version)
		fmt.Println("Checking for updates...")

		shellCmd := exec.Command("sh", "-c", fmt.Sprintf("curl -fsSL %s | sh", installScriptURL))
		shellCmd.Stdout = os.Stdout
		shellCmd.Stderr = os.Stderr
		shellCmd.Stdin = os.Stdin

		if err := shellCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running upgrade: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// Run command flags
	runCmd.Flags().IntP("max-iterations", "n", 50, "Maximum number of iterations")
	runCmd.Flags().Float64("max-cost", 20.0, "Maximum cost in USD")
	runCmd.Flags().Int("checkpoint-interval", 5, "Save checkpoint every N iterations (0 to disable)")
	runCmd.Flags().Bool("auto", false, "Auto-select next ready epic")
	runCmd.Flags().Bool("headless", false, "Run without TUI (stdout/stderr only)")

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

	var epicID string
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
	} else {
		fmt.Fprintln(os.Stderr, "Error: either provide an epic-id or use --auto")
		os.Exit(ExitError)
	}

	// Get epic info for TUI
	ticksClient := ticks.NewClient()
	epic, err := ticksClient.GetEpic(epicID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting epic: %v\n", err)
		os.Exit(ExitError)
	}

	// TUI mode (default)
	if !headless {
		runWithTUI(epicID, epic.Title, maxIterations, maxCost, checkpointInterval)
		return
	}

	// Headless mode
	runHeadless(epicID, maxIterations, maxCost, checkpointInterval)
}

func runWithTUI(epicID, epicTitle string, maxIterations int, maxCost float64, checkpointInterval int) {
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
	}

	// Initial task list load
	go refreshTasks()

	// Wire engine callbacks to send TUI messages
	eng.OnOutput = func(chunk string) {
		p.Send(tui.OutputMsg(chunk))
	}

	eng.OnIterationStart = func(ctx engine.IterationContext) {
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

	// Run engine in background
	go func() {
		config := engine.RunConfig{
			EpicID:          epicID,
			MaxIterations:   maxIterations,
			MaxCost:         maxCost,
			CheckpointEvery: checkpointInterval,
			PauseChan:       pauseChan,
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

func runHeadless(epicID string, maxIterations int, maxCost float64, checkpointInterval int) {
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

	// Run
	config := engine.RunConfig{
		EpicID:          epicID,
		MaxIterations:   maxIterations,
		MaxCost:         maxCost,
		CheckpointEvery: checkpointInterval,
	}

	fmt.Printf("Starting ticker run for epic %s\n", epicID)
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
	// Use tk ready to find epics with no blockers
	cmd := exec.Command("tk", "ready", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tk ready failed: %w", err)
	}

	// For now, just get the first line which should be the epic ID
	// In a full implementation, we'd parse JSON and pick intelligently
	if len(output) == 0 {
		return "", nil
	}

	// Simple approach: tk ready might return issue IDs, pick first epic type
	ticksClient := ticks.NewClient()

	// Try to find epics via tk
	// This is a simplified version - we'd need proper tk integration
	_ = ticksClient

	return "", fmt.Errorf("auto-select not fully implemented - please specify an epic ID")
}
