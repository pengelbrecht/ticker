package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/pengelbrecht/ticker/internal/agent"
	"github.com/pengelbrecht/ticker/internal/budget"
	"github.com/pengelbrecht/ticker/internal/checkpoint"
	epiccontext "github.com/pengelbrecht/ticker/internal/context"
	"github.com/pengelbrecht/ticker/internal/engine"
	"github.com/pengelbrecht/ticker/internal/parallel"
	"github.com/pengelbrecht/ticker/internal/runlog"
	"github.com/pengelbrecht/ticker/internal/ticks"
	"github.com/pengelbrecht/ticker/internal/tui"
	"github.com/pengelbrecht/ticker/internal/update"
	"github.com/pengelbrecht/ticker/internal/verify"
	"github.com/pengelbrecht/ticker/internal/worktree"
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
	Use:   "run <epic-id> [epic-id...]",
	Short: "Run one or more epics with the AI agent",
	Long: `Run starts the Ralph loop for the specified epic(s). The agent will iterate
through tasks until completion, ejection, or budget limits are reached.

When multiple epics are provided, they run in parallel in isolated git worktrees.
Use --parallel to control concurrency (default: number of epics).

AUTO MODE:
  Use --auto to automatically select work. By default, --auto picks ready epics.
  Add --include-standalone to also process tasks without a parent epic.
  Add --include-orphans to also process tasks whose parent epic is closed.
  Add --all for both standalone and orphan tasks.

  In auto mode, ticker continuously processes work:
    1. Runs available epics until all complete
    2. Then runs standalone tasks (if --include-standalone)
    3. Then runs orphan tasks (if --include-orphans)
    4. Exits when no more work is available

  This works in both TUI and headless modes.

AGENT SIGNALS:
  The agent communicates task state via XML signals in its output:

  COMPLETE          Task fully done, close the tick
  APPROVAL_NEEDED   Work done, needs human sign-off before closing
  INPUT_NEEDED      Agent needs information or decision from human
  REVIEW_REQUESTED  PR created, needs code review before merging
  CONTENT_REVIEW    UI/copy/design needs human judgment
  ESCALATE          Found unexpected issue, needs human direction
  CHECKPOINT        Phase complete, verify before continuing
  EJECT             Agent cannot complete, human must do the work
  BLOCKED           (Legacy) Maps to INPUT_NEEDED

  Format: <promise>SIGNAL_TYPE</promise> or <promise>SIGNAL_TYPE: context</promise>

TASK FILTERING:
  Ticker automatically skips tasks where:
  - awaiting is set (task waiting for human response)
  - blocked by another open task
  - status is closed

  This means ticker never blocks on human input. After any handoff signal,
  it immediately continues to the next available task.

HUMAN WORKFLOW:
  While ticker runs, humans can review and respond to handed-off tasks:

  List tasks needing attention:
    tk list --awaiting              # All tasks awaiting human
    tk list --awaiting approval     # Only approval requests
    tk next --awaiting              # Get next task for human

  Respond to tasks:
    tk approve <id>                 # Approve work (closes or returns to agent)
    tk reject <id> "feedback"       # Reject with feedback (returns to agent)

  After approve/reject, the task returns to ticker's queue if not closed.

Exit codes:
  0 - Success (all epics completed)
  1 - Max iterations reached
  2 - Agent ejected
  3 - Agent blocked
  4 - Error

Examples:
  ticker run abc123                      # Single epic with TUI
  ticker run abc123 --worktree           # Single epic in worktree
  ticker run abc def ghi                 # Three epics in parallel
  ticker run abc def ghi --parallel 2    # Three epics, max 2 at a time
  ticker run --auto                      # Auto-select and run epics
  ticker run --auto --include-standalone # Run epics, then standalone tasks
  ticker run --auto --all --headless     # Headless: run all available work`,
	Run: runRun,
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

var mergeCmd = &cobra.Command{
	Use:   "merge <epic-id>",
	Short: "Retry merging a conflicted epic's worktree branch",
	Long: `Retry merging an epic that previously failed due to merge conflicts.

After manually resolving conflicts in the main repository:
  1. git checkout main
  2. git merge ticker/<epic-id>
  3. Resolve conflicts and commit

Then run 'ticker merge <epic-id>' to verify the merge and clean up the worktree.

If the branch hasn't been merged yet, this command will attempt to merge it
and show any remaining conflicts.`,
	Args: cobra.ExactArgs(1),
	Run:  runMerge,
}

var contextCmd = &cobra.Command{
	Use:   "context <epic-id>",
	Short: "Manage epic context documents",
	Long: `Generate and manage pre-computed context documents for epics.

Context documents are generated by analyzing the codebase and epic tasks,
then stored in .ticker/context/<epic-id>.md. These documents are automatically
injected into task prompts to reduce redundant context-gathering.

Default behavior (no flags):
  - If context exists: display it
  - If no context exists: generate it

Flags:
  --show     Display existing context (error if none exists)
  --refresh  Force regeneration even if context exists
  --delete   Remove the context file

Examples:
  ticker context abc123           # Generate if missing, show if exists
  ticker context abc123 --show    # Display existing context
  ticker context abc123 --refresh # Force regenerate
  ticker context abc123 --delete  # Remove context file`,
	Args: cobra.ExactArgs(1),
	Run:  runContext,
}

func init() {
	// Run command flags
	runCmd.Flags().IntP("max-iterations", "n", 50, "Maximum number of iterations")
	runCmd.Flags().Float64("max-cost", 0, "Maximum cost in USD (0 = disabled)")
	runCmd.Flags().Int("checkpoint-interval", 5, "Save checkpoint every N iterations (0 to disable)")
	runCmd.Flags().Int("max-task-retries", 3, "Maximum iterations on same task before assuming stuck")
	runCmd.Flags().Bool("auto", false, "Auto-select next ready epic")
	runCmd.Flags().Bool("headless", false, "Run without TUI (stdout/stderr only)")
	runCmd.Flags().Bool("jsonl", false, "Output JSON Lines format (requires --headless)")
	runCmd.Flags().Bool("skip-verify", true, "Skip git verification after task completion (default: true)")
	runCmd.Flags().Bool("verify-only", false, "Run verification without the agent (for debugging)")
	runCmd.Flags().Bool("worktree", false, "Run epic(s) in isolated git worktree")
	runCmd.Flags().Int("parallel", 0, "Max parallel epics (default: number of epics)")
	runCmd.Flags().Bool("watch", false, "Watch mode: idle when no tasks available instead of exiting")
	runCmd.Flags().Duration("timeout", 0, "Watch timeout: stop watching after this duration (default: unlimited)")
	runCmd.Flags().Duration("poll", 10*time.Second, "Poll interval for watch mode (default: 10s)")
	runCmd.Flags().Duration("debounce", 0, "Wait before picking up newly available tasks (prevents race with human edits)")
	runCmd.Flags().Bool("include-standalone", false, "Include standalone tasks (no parent epic) in auto mode")
	runCmd.Flags().Bool("include-orphans", false, "Include orphaned tasks (parent epic closed) in auto mode")
	runCmd.Flags().Bool("all", false, "Include all task types (standalone + orphans) in auto mode")

	// Context command flags
	contextCmd.Flags().Bool("show", false, "Display existing context (error if none exists)")
	contextCmd.Flags().Bool("refresh", false, "Force regeneration even if context exists")
	contextCmd.Flags().Bool("delete", false, "Remove the context file")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(checkpointsCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(mergeCmd)
	rootCmd.AddCommand(contextCmd)
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
	jsonl, _ := cmd.Flags().GetBool("jsonl")
	maxIterations, _ := cmd.Flags().GetInt("max-iterations")
	maxCost, _ := cmd.Flags().GetFloat64("max-cost")
	checkpointInterval, _ := cmd.Flags().GetInt("checkpoint-interval")
	maxTaskRetries, _ := cmd.Flags().GetInt("max-task-retries")
	skipVerify, _ := cmd.Flags().GetBool("skip-verify")
	verifyOnly, _ := cmd.Flags().GetBool("verify-only")
	useWorktree, _ := cmd.Flags().GetBool("worktree")
	maxParallel, _ := cmd.Flags().GetInt("parallel")
	watch, _ := cmd.Flags().GetBool("watch")
	watchTimeout, _ := cmd.Flags().GetDuration("timeout")
	watchPollInterval, _ := cmd.Flags().GetDuration("poll")
	debounceInterval, _ := cmd.Flags().GetDuration("debounce")
	includeStandalone, _ := cmd.Flags().GetBool("include-standalone")
	includeOrphans, _ := cmd.Flags().GetBool("include-orphans")
	includeAll, _ := cmd.Flags().GetBool("all")

	// --all is shorthand for standalone + orphans
	if includeAll {
		includeStandalone = true
		includeOrphans = true
	}

	// Check mutual exclusivity - only error if user explicitly set --skip-verify with --verify-only
	if verifyOnly {
		if cmd.Flags().Changed("skip-verify") && skipVerify {
			fmt.Fprintln(os.Stderr, "Error: --skip-verify and --verify-only are mutually exclusive")
			os.Exit(ExitError)
		}
		// --verify-only implies verification should run
		skipVerify = false
	}

	// --jsonl requires --headless
	if jsonl && !headless {
		fmt.Fprintln(os.Stderr, "Error: --jsonl requires --headless")
		os.Exit(ExitError)
	}

	// --auto implies --watch (continuous operation)
	if auto {
		watch = true
	}

	// Watch mode validation (only warn if not using --auto, since --auto implies --watch)
	if watchTimeout > 0 && !watch && !auto {
		fmt.Fprintln(os.Stderr, "Warning: --timeout has no effect without --watch or --auto")
	}
	if watchPollInterval != 10*time.Second && !watch && !auto {
		fmt.Fprintln(os.Stderr, "Warning: --poll has no effect without --watch or --auto")
	}

	// Handle --verify-only mode (no epic required, runs in current directory)
	if verifyOnly {
		runVerifyOnly()
		return
	}

	// Validate --parallel flag
	if maxParallel < 0 {
		fmt.Fprintln(os.Stderr, "Error: --parallel must be >= 0")
		os.Exit(ExitError)
	}

	var epicIDs []string
	var epicTitles []string
	// standaloneTask is set when running in standalone/orphan task mode (no epic)
	var standaloneTask *ticks.Task

	if len(args) > 0 {
		epicIDs = args
	} else if auto {
		ticksClient := ticks.NewClient()

		// Auto-select: use tk to find ready epics
		// If --parallel is specified, select up to that many epics
		selectCount := 1
		if maxParallel > 0 {
			selectCount = maxParallel
		}
		selected, err := autoSelectEpics(selectCount)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error auto-selecting epics: %v\n", err)
			os.Exit(ExitError)
		}

		// If we found epics, use them
		if len(selected) > 0 {
			epicIDs = selected
			if len(epicIDs) == 1 {
				fmt.Printf("Auto-selected epic: %s\n", epicIDs[0])
			} else {
				fmt.Printf("Auto-selected %d epics: %v\n", len(epicIDs), epicIDs)
			}
		} else {
			// No ready epics - try standalone/orphan tasks if enabled
			// Priority: standalone tasks (housekeeping) > orphaned tasks (cleanup)
			if includeStandalone {
				task, err := ticksClient.NextTaskWithOptions(ticks.StandaloneOnly())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error finding standalone tasks: %v\n", err)
					os.Exit(ExitError)
				}
				if task != nil {
					standaloneTask = task
					fmt.Printf("Auto-selected standalone task: [%s] %s\n", task.ID, task.Title)
				}
			}

			if standaloneTask == nil && includeOrphans {
				task, err := ticksClient.NextTaskWithOptions(ticks.OrphanedOnly())
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error finding orphaned tasks: %v\n", err)
					os.Exit(ExitError)
				}
				if task != nil {
					standaloneTask = task
					fmt.Printf("Auto-selected orphaned task: [%s] %s (parent: %s)\n", task.ID, task.Title, task.Parent)
				}
			}

			// If still nothing found
			if standaloneTask == nil {
				if !watch {
					// No watch mode - exit immediately
					if includeStandalone || includeOrphans {
						fmt.Fprintln(os.Stderr, "No ready epics, standalone tasks, or orphaned tasks found")
					} else {
						fmt.Fprintln(os.Stderr, "No ready epics found")
						fmt.Fprintln(os.Stderr, "Tip: Use --include-standalone or --include-orphans to also process tasks without active epics")
					}
					os.Exit(ExitError)
				}

				// Watch mode - use file watcher with polling fallback
				pollInterval := watchPollInterval
				if pollInterval == 0 {
					pollInterval = 10 * time.Second
				}
				var watchDeadline time.Time
				if watchTimeout > 0 {
					watchDeadline = time.Now().Add(watchTimeout)
				}

				// Create file watcher for .tick/issues directory
				watcher := engine.NewTicksWatcher("")
				defer watcher.Close()
				fileChanges := watcher.Changes() // nil if fsnotify unavailable

				if jsonl {
					if watcher.UsingFsnotify() {
						fmt.Printf(`{"type":"watch_idle","message":"No tasks found, watching for new tasks (fsnotify)"}` + "\n")
					} else {
						fmt.Printf(`{"type":"watch_idle","message":"No tasks found, watching for new tasks (polling)"}` + "\n")
					}
				} else {
					if watcher.UsingFsnotify() {
						fmt.Printf("[WATCH] No tasks found, watching for new tasks (fsnotify + poll every %s)\n", pollInterval)
					} else {
						fmt.Printf("[WATCH] No tasks found, watching for new tasks (poll every %s)\n", pollInterval)
					}
				}

				// checkForTasks is a helper to find available work
				checkForTasks := func() bool {
					// Try to find epics first
					selected, _ = autoSelectEpics(selectCount)
					if len(selected) > 0 {
						epicIDs = selected
						if jsonl {
							fmt.Printf(`{"type":"watch_found","epic_ids":%q}`+"\n", epicIDs)
						} else {
							fmt.Printf("[WATCH] Found epic(s): %v\n", epicIDs)
						}
						return true
					}

					// Try standalone tasks
					if includeStandalone {
						task, _ := ticksClient.NextTaskWithOptions(ticks.StandaloneOnly())
						if task != nil {
							standaloneTask = task
							if jsonl {
								fmt.Printf(`{"type":"watch_found","task_id":"%s","task_type":"standalone"}`+"\n", task.ID)
							} else {
								fmt.Printf("[WATCH] Found standalone task: [%s] %s\n", task.ID, task.Title)
							}
							return true
						}
					}

					// Try orphaned tasks
					if includeOrphans {
						task, _ := ticksClient.NextTaskWithOptions(ticks.OrphanedOnly())
						if task != nil {
							standaloneTask = task
							if jsonl {
								fmt.Printf(`{"type":"watch_found","task_id":"%s","task_type":"orphan"}`+"\n", task.ID)
							} else {
								fmt.Printf("[WATCH] Found orphaned task: [%s] %s\n", task.ID, task.Title)
							}
							return true
						}
					}
					return false
				}

				// Watch until we find something or timeout
				for {
					// Check timeout
					if !watchDeadline.IsZero() && time.Now().After(watchDeadline) {
						if jsonl {
							fmt.Printf(`{"type":"watch_timeout","message":"Watch timeout reached"}` + "\n")
						} else {
							fmt.Fprintln(os.Stderr, "[WATCH] Timeout reached, exiting")
						}
						os.Exit(ExitError)
					}

					// Wait for file change or poll interval
					select {
					case <-fileChanges:
						// File changed - check for tasks immediately
						if checkForTasks() {
							break
						}
					case <-time.After(pollInterval):
						// Periodic poll
						if checkForTasks() {
							break
						}
					}

					// Check if we found something (break from outer loop)
					if len(epicIDs) > 0 || standaloneTask != nil {
						break
					}
				}
			}
		}
	} else if !headless {
		// Interactive mode: show epic picker
		selected := runPicker()
		if selected == nil {
			os.Exit(0) // User quit without selecting
		}
		epicIDs = []string{selected.ID}
		epicTitles = []string{selected.Title}
	} else {
		fmt.Fprintln(os.Stderr, "Error: either provide an epic-id or use --auto")
		os.Exit(ExitError)
	}

	// Handle standalone/orphan task mode (no epic)
	if standaloneTask != nil {
		if !headless {
			fmt.Fprintln(os.Stderr, "Note: TUI mode not supported for standalone tasks. Use --headless.")
		}
		runStandaloneTask(standaloneTask, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, jsonl, includeStandalone, includeOrphans)
		return
	}

	// Validate epic IDs: exist, open, unique
	ticksClient := ticks.NewClient()
	if err := validateEpicIDs(ticksClient, epicIDs); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Get epic titles (if not already from picker)
	if len(epicTitles) == 0 {
		epicTitles = make([]string, len(epicIDs))
		for i, id := range epicIDs {
			epic, err := ticksClient.GetEpic(id)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error getting epic %s: %v\n", id, err)
				os.Exit(ExitError)
			}
			epicTitles[i] = epic.Title
		}
	}

	// Multiple epics implies --worktree
	if len(epicIDs) > 1 {
		useWorktree = true
	}

	// Warn if --parallel specified with single epic
	if maxParallel > 0 && len(epicIDs) == 1 {
		fmt.Fprintln(os.Stderr, "Warning: --parallel ignored with single epic")
		maxParallel = 0
	}

	// Handle multiple epics
	if len(epicIDs) > 1 {
		// Multiple epics - use ParallelRunner
		if maxParallel == 0 {
			maxParallel = len(epicIDs)
		}
		if !headless {
			runParallelWithTUI(epicIDs, epicTitles, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, maxParallel)
		} else {
			runParallelHeadless(epicIDs, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, maxParallel, jsonl)
		}
		return
	}

	// Single epic - use existing Engine
	epicID := epicIDs[0]
	epicTitle := epicTitles[0]

	// TUI mode (default)
	if !headless {
		runWithTUI(epicID, epicTitle, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, useWorktree, watch, watchTimeout, watchPollInterval, debounceInterval, auto, includeStandalone, includeOrphans)
		return
	}

	// Headless mode - run in a loop if auto mode with standalone/orphan support
	ticksClientLoop := ticks.NewClient()

	for {
		exitCode := runHeadless(epicID, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, useWorktree, jsonl, watch, watchTimeout, watchPollInterval, debounceInterval)

		// If not in auto mode with continuation support, exit immediately
		if !auto || (!includeStandalone && !includeOrphans) {
			os.Exit(exitCode)
		}

		// Epic completed - check for more work using shared function
		nextWork := findNextWork(ticksClientLoop, includeStandalone, includeOrphans)
		if nextWork == nil {
			if jsonl {
				fmt.Printf(`{"type":"auto_complete","message":"No more epics or tasks found"}` + "\n")
			} else {
				fmt.Println("[AUTO] No more epics or tasks found")
			}
			os.Exit(exitCode)
		}

		if nextWork.IsStandalone {
			// Switch to standalone task mode
			if jsonl {
				fmt.Printf(`{"type":"auto_next","task_id":"%s","title":"%s","mode":"standalone"}`+"\n", nextWork.Task.ID, nextWork.Task.Title)
			} else {
				fmt.Printf("[AUTO] Switching to standalone task: [%s] %s\n", nextWork.Task.ID, nextWork.Task.Title)
			}
			runStandaloneTask(nextWork.Task, maxIterations, maxCost, checkpointInterval, maxTaskRetries, skipVerify, jsonl, includeStandalone, includeOrphans)
			return // runStandaloneTask exits on its own
		}

		// Continue with next epic
		epicID = nextWork.EpicID
		epicTitle = nextWork.EpicTitle
		if jsonl {
			fmt.Printf(`{"type":"auto_next","epic_id":"%s","title":"%s"}`+"\n", epicID, epicTitle)
		} else {
			fmt.Printf("[AUTO] Found next epic: [%s] %s\n", epicID, epicTitle)
		}
	}
}

// validateEpicIDs checks that all epic IDs exist, are open, and are unique.
func validateEpicIDs(client *ticks.Client, epicIDs []string) error {
	seen := make(map[string]bool)
	for _, id := range epicIDs {
		// Check for duplicates
		if seen[id] {
			return fmt.Errorf("duplicate epic ID: %s", id)
		}
		seen[id] = true

		// Check existence and status
		epic, err := client.GetEpic(id)
		if err != nil {
			return fmt.Errorf("epic %s not found: %w", id, err)
		}
		if !epic.IsOpen() {
			return fmt.Errorf("epic %s is not open (status: %s)", id, epic.Status)
		}
	}
	return nil
}

func runParallelWithTUI(epicIDs, epicTitles []string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify bool, maxParallel int) {
	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Get current working directory for worktree manager
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(ExitError)
	}

	// Initialize worktree manager
	wtManager, err := worktree.NewManager(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing worktree manager: %v\n", err)
		os.Exit(ExitError)
	}

	// Check for uncommitted changes on main before starting
	isDirty, dirtyFiles, err := wtManager.IsDirty()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking git status: %v\n", err)
		os.Exit(ExitError)
	}
	if isDirty {
		// Check if only tick files are dirty - auto-commit if so
		if onlyTick, tickFiles := wtManager.IsOnlyTickFilesDirty(dirtyFiles); onlyTick {
			fmt.Fprintf(os.Stderr, "Auto-committing tick status updates:\n")
			for _, f := range tickFiles {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			if err := wtManager.AutoCommitTickFiles(); err != nil {
				fmt.Fprintf(os.Stderr, "Error auto-committing tick files: %v\n", err)
				os.Exit(ExitError)
			}
			fmt.Fprintf(os.Stderr, "Done.\n\n")
		} else {
			fmt.Fprintf(os.Stderr, "Error: Cannot start parallel run - main branch has uncommitted changes\n\n")
			fmt.Fprintf(os.Stderr, "Dirty files:\n")
			for _, f := range dirtyFiles {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			fmt.Fprintf(os.Stderr, "\nPlease commit, stash, or discard these changes before running.\n")
			os.Exit(ExitError)
		}
	}

	// Initialize merge manager
	mergeManager, err := worktree.NewMergeManager(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing merge manager: %v\n", err)
		os.Exit(ExitError)
	}

	// Create shared budget tracker
	sharedBudget := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations * len(epicIDs), // Total across all epics
		MaxCost:       maxCost,                      // Shared cost limit
	})

	// Create TUI model with first epic as initial
	pauseChan := make(chan bool, 1)
	m := tui.New(tui.Config{
		EpicID:       epicIDs[0],
		EpicTitle:    epicTitles[0],
		MaxCost:      maxCost,
		MaxIteration: maxIterations,
		PauseChan:    pauseChan,
	})

	// Create program
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Check claude availability
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintln(os.Stderr, "Error: claude CLI not found. Please install Claude Code.")
		os.Exit(ExitError)
	}

	// Engine factory creates a new engine for each epic
	ticksClient := ticks.NewClient()
	checkpointMgr := checkpoint.NewManager()

	// Helper to load tasks for an epic (defined before factory so it can be used in callbacks)
	loadTasksForEpic := func(epicID string) {
		tasks, err := ticksClient.ListTasks(epicID)
		if err != nil {
			return
		}
		// Build map of task statuses to filter closed blockers
		taskStatus := make(map[string]string, len(tasks))
		for _, t := range tasks {
			taskStatus[t.ID] = t.Status
		}
		taskInfos := make([]tui.TaskInfo, len(tasks))
		for i, t := range tasks {
			// Filter BlockedBy to only include open blockers
			var openBlockers []string
			for _, blockerID := range t.BlockedBy {
				if taskStatus[blockerID] == "open" {
					openBlockers = append(openBlockers, blockerID)
				}
			}
			taskInfos[i] = tui.TaskInfo{
				ID:        t.ID,
				Title:     t.Title,
				Status:    tui.TaskStatus(t.Status),
				BlockedBy: openBlockers,
				Awaiting:  t.GetAwaitingType(),
			}
		}
		p.Send(tui.EpicTasksUpdateMsg{EpicID: epicID, Tasks: taskInfos})

		// Fetch and send RunRecords for closed tasks
		for _, t := range tasks {
			if t.Status == "closed" {
				if record, err := ticksClient.GetRunRecord(t.ID); err == nil && record != nil {
					p.Send(tui.EpicTaskRunRecordMsg{EpicID: epicID, TaskID: t.ID, RunRecord: record})
				}
			}
		}
	}

	engineFactory := func(epicID string) *engine.Engine {
		claudeAgent := agent.NewClaudeAgent()
		eng := engine.NewEngine(
			claudeAgent,
			ticksClient,
			sharedBudget,
			checkpointMgr,
		)

		// Set up context generation
		contextStore := epiccontext.NewStore()
		contextGenerator, err := epiccontext.NewGenerator(claudeAgent)
		if err == nil {
			eng.SetContextComponents(contextStore, contextGenerator)
		}

		if !skipVerify {
			if isVerificationEnabled() {
				eng.EnableVerification()
			}
		}

		// Track previous snapshot state for delta-based TUI updates (per-engine)
		var prevOutput string

		// Rich streaming callback - converts AgentStateSnapshot to TUI messages
		eng.OnAgentState = func(snap agent.AgentStateSnapshot) {
			// Send text deltas (only new content since last update)
			if snap.Output != prevOutput {
				delta := snap.Output[len(prevOutput):]
				if delta != "" {
					p.Send(tui.EpicOutputMsg{EpicID: epicID, Text: delta})
				}
				prevOutput = snap.Output
			}
		}

		// Note: OnOutput callback not set in TUI mode - OnAgentState handles streaming

		eng.OnIterationStart = func(ctx engine.IterationContext) {
			prevOutput = "" // Reset for new iteration
			p.Send(tui.EpicIterationStartMsg{
				EpicID:    epicID,
				Iteration: ctx.Iteration,
				TaskID:    ctx.Task.ID,
				TaskTitle: ctx.Task.Title,
			})
		}

		eng.OnIterationEnd = func(result *engine.IterationResult) {
			p.Send(tui.EpicIterationEndMsg{
				EpicID:    epicID,
				Iteration: result.Iteration,
				Cost:      result.Cost,
				Tokens:    result.TokensIn + result.TokensOut,
			})
			loadTasksForEpic(epicID)
		}

		eng.OnVerificationStart = func(taskID string) {
			p.Send(tui.VerifyStartMsg{TaskID: taskID})
		}

		eng.OnVerificationEnd = func(taskID string, results *verify.Results) {
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
			loadTasksForEpic(epicID)
		}

		// Context generation callbacks for multi-epic TUI
		eng.OnContextGenerating = func(eid string, taskCount int) {
			p.Send(tui.EpicContextGeneratingMsg{EpicID: eid, TaskCount: taskCount})
		}
		eng.OnContextGenerated = func(eid string, tokenCount int) {
			p.Send(tui.EpicContextGeneratedMsg{EpicID: eid, Tokens: tokenCount})
		}
		eng.OnContextLoaded = func(eid string, _ string) {
			p.Send(tui.EpicContextLoadedMsg{EpicID: eid})
		}
		eng.OnContextSkipped = func(eid string, reason string) {
			p.Send(tui.EpicContextSkippedMsg{EpicID: eid, Reason: reason})
		}
		eng.OnContextFailed = func(eid string, errMsg string) {
			p.Send(tui.EpicContextFailedMsg{EpicID: eid, Error: errMsg})
		}

		return eng
	}

	// Create parallel runner config
	runnerConfig := parallel.RunnerConfig{
		EpicIDs:         epicIDs,
		MaxParallel:     maxParallel,
		SharedBudget:    sharedBudget,
		WorktreeManager: wtManager,
		MergeManager:    mergeManager,
		EngineFactory:   engineFactory,
		EngineConfig: engine.RunConfig{
			MaxIterations:   maxIterations,
			MaxCost:         maxCost,
			CheckpointEvery: checkpointInterval,
			MaxTaskRetries:  maxTaskRetries,
			UseWorktree:     true,
		},
	}

	runner := parallel.NewRunner(runnerConfig)

	// Set up callbacks to send messages to TUI
	runner.SetCallbacks(parallel.RunnerCallbacks{
		OnEpicStart: func(epicID string) {
			p.Send(tui.EpicStatusMsg{EpicID: epicID, Status: tui.EpicTabStatusRunning})
			loadTasksForEpic(epicID)
		},
		OnEpicComplete: func(epicID string, result *engine.RunResult) {
			p.Send(tui.EpicStatusMsg{EpicID: epicID, Status: tui.EpicTabStatusComplete})
			loadTasksForEpic(epicID)
		},
		OnEpicFailed: func(epicID string, err error) {
			p.Send(tui.EpicStatusMsg{EpicID: epicID, Status: tui.EpicTabStatusFailed})
		},
		OnEpicConflict: func(epicID string, conflict *parallel.ConflictState) {
			p.Send(tui.EpicConflictMsg{
				EpicID:       epicID,
				Files:        conflict.Files,
				Branch:       conflict.Branch,
				WorktreePath: conflict.Worktree,
			})
		},
		OnStatusChange: func(epicID string, status string) {
			// Refresh tasks when status changes (task completed, etc.)
			loadTasksForEpic(epicID)
		},
		OnMessage: func(message string) {
			// Display global status message in status bar
			p.Send(tui.GlobalStatusMsg{Message: message})
		},
	})

	// Run parallel runner in background
	go func() {
		// Brief delay to let TUI initialize, then add epic tabs
		time.Sleep(100 * time.Millisecond)
		for i, id := range epicIDs {
			p.Send(tui.EpicAddedMsg{EpicID: id, Title: epicTitles[i]})
		}

		// Load tasks for all epics
		time.Sleep(50 * time.Millisecond)
		for _, id := range epicIDs {
			loadTasksForEpic(id)
		}

		result, err := runner.Run(ctx)
		if err != nil {
			p.Send(tui.ErrorMsg{Err: err})
			return
		}

		// Send completion message
		p.Send(tui.RunCompleteMsg{
			Reason:     "all epics completed",
			Iterations: 0, // Aggregate not available in simple form
			Cost:       result.TotalCost,
		})
	}()

	// Run TUI (blocks until quit)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(ExitError)
	}

	cancel()
}

func runParallelHeadless(epicIDs []string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify bool, maxParallel int, jsonl bool) {
	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a headless output formatter per epic (for prefixed output)
	outputs := make(map[string]*engine.HeadlessOutput)
	for _, id := range epicIDs {
		outputs[id] = engine.NewHeadlessOutput(jsonl, id)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		// Signal interruption for all epics
		for _, out := range outputs {
			out.Interrupted()
		}
		cancel()
	}()

	// Get current working directory for worktree manager
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Error getting current directory: %v\n", err)
		os.Exit(ExitError)
	}

	// Initialize worktree manager
	wtManager, err := worktree.NewManager(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Error initializing worktree manager: %v\n", err)
		os.Exit(ExitError)
	}

	// Check for uncommitted changes on main before starting
	isDirty, dirtyFiles, err := wtManager.IsDirty()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Error checking git status: %v\n", err)
		os.Exit(ExitError)
	}
	if isDirty {
		// Check if only tick files are dirty - auto-commit if so
		if onlyTick, tickFiles := wtManager.IsOnlyTickFilesDirty(dirtyFiles); onlyTick {
			fmt.Fprintf(os.Stderr, "[INFO] Auto-committing tick status updates:\n")
			for _, f := range tickFiles {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			if err := wtManager.AutoCommitTickFiles(); err != nil {
				fmt.Fprintf(os.Stderr, "[ERROR] Error auto-committing tick files: %v\n", err)
				os.Exit(ExitError)
			}
			fmt.Fprintf(os.Stderr, "[INFO] Done.\n\n")
		} else {
			fmt.Fprintf(os.Stderr, "[ERROR] Cannot start parallel run: main branch has uncommitted changes\n")
			fmt.Fprintf(os.Stderr, "\nDirty files:\n")
			for _, f := range dirtyFiles {
				fmt.Fprintf(os.Stderr, "  %s\n", f)
			}
			fmt.Fprintf(os.Stderr, "\nPlease commit, stash, or discard these changes before running.\n")
			os.Exit(ExitError)
		}
	}

	// Initialize merge manager
	mergeManager, err := worktree.NewMergeManager(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Error initializing merge manager: %v\n", err)
		os.Exit(ExitError)
	}

	// Create shared budget tracker
	sharedBudget := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations * len(epicIDs),
		MaxCost:       maxCost,
	})

	// Check claude availability
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintf(os.Stderr, "[ERROR] claude CLI not found - please install Claude Code\n")
		os.Exit(ExitError)
	}

	// Engine factory creates a new engine for each epic
	ticksClient := ticks.NewClient()
	checkpointMgr := checkpoint.NewManager()

	engineFactory := func(epicID string) *engine.Engine {
		claudeAgent := agent.NewClaudeAgent()
		eng := engine.NewEngine(
			claudeAgent,
			ticksClient,
			sharedBudget,
			checkpointMgr,
		)

		// Set up context generation (use discard logger in jsonl mode)
		contextStore := epiccontext.NewStore()
		var contextGenOpts []epiccontext.GeneratorOption
		if jsonl {
			contextGenOpts = append(contextGenOpts, epiccontext.WithLogger(slog.New(slog.DiscardHandler)))
		}
		contextGenerator, err := epiccontext.NewGenerator(claudeAgent, contextGenOpts...)
		if err == nil {
			eng.SetContextComponents(contextStore, contextGenerator)
		}

		if !skipVerify {
			if isVerificationEnabled() {
				eng.EnableVerification()
			}
		}

		// Get the output formatter for this epic
		out := outputs[epicID]

		// Track verification pass status for task_complete output
		var verifyPassed bool = true

		// Set up task-level callbacks for headless output
		eng.OnOutput = func(chunk string) {
			if out != nil {
				out.Output(chunk)
			}
		}

		eng.OnIterationStart = func(iterCtx engine.IterationContext) {
			verifyPassed = true // Reset for new task
			if out != nil {
				out.Task(iterCtx.Task, iterCtx.Iteration)
				if iterCtx.EpicContext != "" {
					out.ContextInjected(iterCtx.Task.ID, iterCtx.EpicContext)
				}
			}
		}

		eng.OnSignal = func(sig engine.Signal, reason string) {
			if out != nil {
				out.Signal(sig, reason)
			}
		}

		eng.OnVerificationStart = func(taskID string) {
			if out != nil {
				out.VerifyStart(taskID)
			}
		}

		eng.OnVerificationEnd = func(taskID string, results *verify.Results) {
			if results != nil {
				verifyPassed = results.AllPassed
			}
			if out != nil {
				out.VerifyEnd(taskID, results)
				out.TaskComplete(taskID, verifyPassed)
			}
		}

		// Context generation callbacks for multi-epic headless mode
		eng.OnContextGenerating = func(eid string, taskCount int) {
			if out != nil {
				out.ContextGenerating(eid, taskCount)
			}
		}
		eng.OnContextGenerated = func(eid string, tokenCount int) {
			if out != nil {
				out.ContextGenerated(eid, tokenCount)
			}
		}
		eng.OnContextLoaded = func(eid string, content string) {
			if out != nil {
				out.ContextLoaded(eid, content)
			}
		}
		eng.OnContextSkipped = func(eid string, reason string) {
			if out != nil {
				out.ContextSkipped(eid, reason)
			}
		}
		eng.OnContextFailed = func(eid string, errMsg string) {
			if out != nil {
				out.ContextFailed(eid, errMsg)
			}
		}
		eng.OnContextActive = func(eid string) {
			if out != nil {
				out.ContextActive(eid)
			}
		}

		return eng
	}

	// Create parallel runner config
	runnerConfig := parallel.RunnerConfig{
		EpicIDs:         epicIDs,
		MaxParallel:     maxParallel,
		SharedBudget:    sharedBudget,
		WorktreeManager: wtManager,
		MergeManager:    mergeManager,
		EngineFactory:   engineFactory,
		EngineConfig: engine.RunConfig{
			MaxIterations:   maxIterations,
			MaxCost:         maxCost,
			CheckpointEvery: checkpointInterval,
			MaxTaskRetries:  maxTaskRetries,
			UseWorktree:     true,
		},
	}

	runner := parallel.NewRunner(runnerConfig)

	// Set up callbacks for headless output with [PREFIX] format
	runner.SetCallbacks(parallel.RunnerCallbacks{
		OnEpicStart: func(epicID string) {
			if jsonl {
				out := outputs[epicID]
				if out != nil {
					epic, _ := ticksClient.GetEpic(epicID)
					if epic != nil {
						out.Start(epic, maxIterations, maxCost)
					}
				}
			} else {
				fmt.Printf("[%s] [START] Epic starting\n", epicID)
			}
		},
		OnEpicComplete: func(epicID string, result *engine.RunResult) {
			if jsonl {
				out := outputs[epicID]
				if out != nil && result != nil {
					out.Complete(result)
				}
			} else {
				fmt.Printf("[%s] [COMPLETE] Epic finished\n", epicID)
			}
		},
		OnEpicFailed: func(epicID string, err error) {
			out := outputs[epicID]
			if out != nil {
				out.Error(err)
			} else {
				fmt.Printf("[%s] [ERROR] %v\n", epicID, err)
			}
		},
		OnEpicConflict: func(epicID string, conflict *parallel.ConflictState) {
			if jsonl {
				// Conflict as JSON
				fmt.Printf(`{"type":"conflict","epic_id":"%s","branch":"%s","worktree":"%s","files":%q}`+"\n",
					epicID, conflict.Branch, conflict.Worktree, conflict.Files)
			} else {
				// Print prominent conflict banner
				fmt.Println()
				fmt.Println("══════════════════════════════════════════════════════════════")
				fmt.Printf("  MERGE CONFLICT - Epic %s\n", epicID)
				fmt.Println("══════════════════════════════════════════════════════════════")
				fmt.Println()
				fmt.Printf("  Branch:   %s\n", conflict.Branch)
				fmt.Printf("  Worktree: %s\n", conflict.Worktree)
				fmt.Println()
				fmt.Println("  Conflicting files:")
				for _, f := range conflict.Files {
					fmt.Printf("    • %s\n", f)
				}
				fmt.Println()
				fmt.Println("  To resolve:")
				fmt.Println("    1. git checkout main")
				fmt.Printf("    2. git merge %s\n", conflict.Branch)
				fmt.Println("    3. Resolve conflicts and commit")
				fmt.Println()
				fmt.Printf("  Then run: ticker merge %s\n", epicID)
				fmt.Println()
				fmt.Println("══════════════════════════════════════════════════════════════")
				fmt.Println()
			}
		},
	})

	// Output start message
	if jsonl {
		fmt.Printf(`{"type":"parallel_start","epics":%d,"max_parallel":%d,"max_iterations":%d,"max_cost":%.2f}`+"\n",
			len(epicIDs), maxParallel, maxIterations*len(epicIDs), maxCost)
	} else {
		fmt.Printf("[START] Parallel run: %d epics (max %d concurrent)\n", len(epicIDs), maxParallel)
		fmt.Printf("[START] Budget: max %d iterations total, $%.2f\n", maxIterations*len(epicIDs), maxCost)
	}

	result, err := runner.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(ExitError)
	}

	// Print summary
	if jsonl {
		fmt.Printf(`{"type":"parallel_complete","duration_ms":%d,"total_cost":%.4f,"total_tokens":%d}`+"\n",
			result.Duration.Milliseconds(), result.TotalCost, result.TotalTokens)
	} else {
		fmt.Println()
		fmt.Printf("[COMPLETE] Parallel run finished\n")
		fmt.Printf("[COMPLETE] Duration: %v, Cost: $%.4f, Tokens: %d\n",
			result.Duration.Round(1000000000), result.TotalCost, result.TotalTokens)
	}

	// Print per-epic results
	allSuccess := true
	var conflicts []*parallel.EpicStatus
	for _, status := range result.Statuses {
		if status.Status != "completed" {
			allSuccess = false
		}
		if status.Status == "conflict" {
			conflicts = append(conflicts, status)
		}
		if jsonl {
			errStr := ""
			if status.Error != nil {
				errStr = status.Error.Error()
			}
			fmt.Printf(`{"type":"epic_status","epic_id":"%s","status":"%s","error":"%s"}`+"\n",
				status.EpicID, status.Status, errStr)
		} else {
			icon := "+"
			if status.Status != "completed" {
				icon = "-"
			}
			fmt.Printf("[COMPLETE] %s %s: %s\n", icon, status.EpicID, status.Status)
			if status.Error != nil {
				fmt.Printf("[COMPLETE]   Error: %v\n", status.Error)
			}
		}
	}

	// Print final conflict summary if any
	if len(conflicts) > 0 && !jsonl {
		fmt.Println()
		fmt.Println("════════════════════════════════════════════════════════════════")
		fmt.Printf("  %d EPIC(S) NEED MANUAL MERGE RESOLUTION\n", len(conflicts))
		fmt.Println("════════════════════════════════════════════════════════════════")
		for _, status := range conflicts {
			fmt.Printf("\n  Epic: %s\n", status.EpicID)
			if status.Conflict != nil {
				fmt.Printf("  Branch: %s\n", status.Conflict.Branch)
				fmt.Printf("  Run: ticker merge %s\n", status.EpicID)
			}
		}
		fmt.Println()
		fmt.Println("════════════════════════════════════════════════════════════════")
	}

	if allSuccess {
		os.Exit(ExitSuccess)
	}
	os.Exit(ExitError)
}

func runWithTUI(epicID, epicTitle string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify, useWorktree, watch bool, watchTimeout, watchPollInterval, debounceInterval time.Duration, auto, includeStandalone, includeOrphans bool) {
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

	// Set up context generation
	contextStore := epiccontext.NewStore()
	contextGenerator, err := epiccontext.NewGenerator(claudeAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not create context generator: %v\n", err)
	} else {
		eng.SetContextComponents(contextStore, contextGenerator)
	}

	// Set up verification runner (unless --skip-verify)
	if !skipVerify {
		if isVerificationEnabled() {
			eng.EnableVerification()
		}
	}

	// Create run logger
	runLogger, err := runlog.New(epicID)
	if err != nil {
		// Log warning but continue without run logging
		fmt.Fprintf(os.Stderr, "Warning: could not create run log: %v\n", err)
	} else {
		eng.SetRunLog(runLogger)
		runLogger.LogRunStart("tui", false)
	}

	// Helper to refresh task list in TUI
	refreshTasks := func() {
		tasks, err := ticksClient.ListTasks(epicID)
		if err != nil {
			return // Silently ignore errors
		}
		// Build map of task statuses to filter closed blockers
		taskStatus := make(map[string]string, len(tasks))
		for _, t := range tasks {
			taskStatus[t.ID] = t.Status
		}
		taskInfos := make([]tui.TaskInfo, len(tasks))
		for i, t := range tasks {
			// Filter BlockedBy to only include open blockers
			var openBlockers []string
			for _, blockerID := range t.BlockedBy {
				if taskStatus[blockerID] == "open" {
					openBlockers = append(openBlockers, blockerID)
				}
			}
			taskInfos[i] = tui.TaskInfo{
				ID:        t.ID,
				Title:     t.Title,
				Status:    tui.TaskStatus(t.Status),
				BlockedBy: openBlockers,
				Awaiting:  t.GetAwaitingType(),
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

	// Start file watcher for external tick changes
	ticksWatcher := engine.NewTicksWatcher("")
	defer ticksWatcher.Close()
	if changes := ticksWatcher.Changes(); changes != nil {
		go func() {
			for range changes {
				refreshTasks()
			}
		}()
	}

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

	// Note: OnOutput callback not set in TUI mode - OnAgentState handles streaming

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

	// Context generation callbacks for TUI status display
	eng.OnContextGenerating = func(epicID string, taskCount int) {
		p.Send(tui.ContextGeneratingMsg{EpicID: epicID, TaskCount: taskCount})
	}
	eng.OnContextGenerated = func(epicID string, tokenCount int) {
		p.Send(tui.ContextGeneratedMsg{EpicID: epicID, Tokens: tokenCount})
	}
	eng.OnContextLoaded = func(epicID string, _ string) {
		p.Send(tui.ContextLoadedMsg{EpicID: epicID})
	}
	eng.OnContextSkipped = func(epicID string, reason string) {
		p.Send(tui.ContextSkippedMsg{EpicID: epicID, Reason: reason})
	}
	eng.OnContextFailed = func(epicID string, errMsg string) {
		p.Send(tui.ContextFailedMsg{EpicID: epicID, Error: errMsg})
	}

	// Set up OnIdle callback for watch mode TUI updates
	eng.OnIdle = func() {
		p.Send(tui.IdleMsg{})
	}

	// Run engine in background with auto-continuation support
	go func() {
		currentEpicID := epicID
		totalIterations := 0
		totalCost := 0.0

		for {
			config := engine.RunConfig{
				EpicID:            currentEpicID,
				MaxIterations:     maxIterations,
				MaxCost:           maxCost,
				CheckpointEvery:   checkpointInterval,
				MaxTaskRetries:    maxTaskRetries,
				PauseChan:         pauseChan,
				UseWorktree:       useWorktree,
				Watch:             watch,
				WatchTimeout:      watchTimeout,
				WatchPollInterval: watchPollInterval,
				DebounceInterval:  debounceInterval,
			}

			result, err := eng.Run(ctx, config)

			// Log run end
			if runLogger != nil {
				if result != nil {
					signalStr := ""
					if result.Signal != engine.SignalNone {
						signalStr = result.Signal.String()
					}
					runLogger.LogRunEnd(runlog.RunEndData{
						ExitReason:     result.ExitReason,
						Iterations:     result.Iterations,
						CompletedTasks: result.CompletedTasks,
						TotalTokens:    result.TotalTokens,
						TotalCost:      result.TotalCost,
						Duration:       result.Duration,
						Signal:         signalStr,
						SignalReason:   result.SignalReason,
					})
				}
				runLogger.Close()
				runLogger = nil // Don't close again
			}

			if err != nil {
				p.Send(tui.ErrorMsg{Err: err})
				return
			}

			totalIterations += result.Iterations
			totalCost += result.TotalCost

			// If not in auto mode with continuation support, we're done
			if !auto || (!includeStandalone && !includeOrphans) {
				p.Send(tui.RunCompleteMsg{
					Reason:     result.ExitReason,
					Signal:     result.Signal.String(),
					Iterations: totalIterations,
					Cost:       totalCost,
				})
				return
			}

			// Check for more work
			nextWork := findNextWork(ticksClient, includeStandalone, includeOrphans)
			if nextWork == nil {
				p.Send(tui.RunCompleteMsg{
					Reason:     "all work completed",
					Signal:     result.Signal.String(),
					Iterations: totalIterations,
					Cost:       totalCost,
				})
				return
			}

			// Handle standalone task - run it directly here
			if nextWork.IsStandalone {
				p.Send(tui.GlobalStatusMsg{Message: fmt.Sprintf("[AUTO] Switching to standalone task: [%s] %s", nextWork.Task.ID, nextWork.Task.Title)})

				// Run standalone task using the same pattern as runStandaloneTask but with TUI output
				runStandaloneInTUI(ctx, p, nextWork.Task, ticksClient, claudeAgent, budgetTracker, checkpointMgr, skipVerify, includeStandalone, includeOrphans)

				// After standalone tasks complete, check for more epics
				nextWork = findNextWork(ticksClient, includeStandalone, includeOrphans)
				if nextWork == nil || nextWork.IsStandalone {
					// No more epics, and standalone was just handled
					p.Send(tui.RunCompleteMsg{
						Reason:     "all work completed",
						Iterations: totalIterations,
						Cost:       totalCost,
					})
					return
				}
			}

			// Continue with next epic
			currentEpicID = nextWork.EpicID
			p.Send(tui.GlobalStatusMsg{Message: fmt.Sprintf("[AUTO] Continuing with epic: [%s] %s", nextWork.EpicID, nextWork.EpicTitle)})

			// Refresh task list for the new epic
			refreshTasks()
		}
	}()

	// Run TUI (blocks until quit)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
		os.Exit(ExitError)
	}

	// Cancel engine context when TUI exits
	cancel()
}

// runHeadless runs an epic in headless mode and returns the exit code.
// Returns ExitSuccess, ExitMaxIterations, ExitEject, ExitBlocked, or ExitError.
func runHeadless(epicID string, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify, useWorktree, jsonl, watch bool, watchTimeout, watchPollInterval, debounceInterval time.Duration) int {
	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create headless output formatter (empty epicID = single epic mode)
	out := engine.NewHeadlessOutput(jsonl, "")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		out.Interrupted()
		cancel()
	}()

	// Initialize components
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		out.Error(fmt.Errorf("claude CLI not found - please install Claude Code"))
		return ExitError
	}

	ticksClient := ticks.NewClient()
	budgetTracker := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations,
		MaxCost:       maxCost,
	})
	checkpointMgr := checkpoint.NewManager()

	// Get epic info for start message
	epic, err := ticksClient.GetEpic(epicID)
	if err != nil {
		out.Error(fmt.Errorf("failed to get epic %s: %w", epicID, err))
		return ExitError
	}

	// Create and configure engine
	eng := engine.NewEngine(claudeAgent, ticksClient, budgetTracker, checkpointMgr)

	// Set up context generation (use discard logger in jsonl mode)
	contextStore := epiccontext.NewStore()
	var contextGenOpts []epiccontext.GeneratorOption
	if jsonl {
		contextGenOpts = append(contextGenOpts, epiccontext.WithLogger(slog.New(slog.DiscardHandler)))
	}
	contextGenerator, err := epiccontext.NewGenerator(claudeAgent, contextGenOpts...)
	if err != nil {
		if !jsonl {
			fmt.Fprintf(os.Stderr, "Warning: could not create context generator: %v\n", err)
		}
	} else {
		eng.SetContextComponents(contextStore, contextGenerator)
	}

	// Set up verification runner (unless --skip-verify)
	if !skipVerify {
		if isVerificationEnabled() {
			eng.EnableVerification()
		}
	}

	// Create run logger
	runLogger, err := runlog.New(epicID)
	if err != nil {
		// Log warning but continue without run logging
		if !jsonl {
			fmt.Fprintf(os.Stderr, "Warning: could not create run log: %v\n", err)
		}
	} else {
		eng.SetRunLog(runLogger)
		runLogger.LogRunStart("headless", true)
	}

	// Track verification pass status for task_complete output
	var verifyPassed bool = true

	// Set up output callback for headless mode
	eng.OnOutput = func(chunk string) {
		out.Output(chunk)
	}

	eng.OnIterationStart = func(iterCtx engine.IterationContext) {
		verifyPassed = true // Reset for new task
		out.Task(iterCtx.Task, iterCtx.Iteration)
		if iterCtx.EpicContext != "" {
			out.ContextInjected(iterCtx.Task.ID, iterCtx.EpicContext)
		}
	}

	eng.OnIterationEnd = func(result *engine.IterationResult) {
		// Token counts are only in final summary, not per iteration
	}

	eng.OnSignal = func(sig engine.Signal, reason string) {
		out.Signal(sig, reason)
	}

	// Verification callbacks for headless mode
	eng.OnVerificationStart = func(taskID string) {
		out.VerifyStart(taskID)
	}

	eng.OnVerificationEnd = func(taskID string, results *verify.Results) {
		if results != nil {
			verifyPassed = results.AllPassed
			out.VerifyEnd(taskID, results)
		}
		// Output task complete after verification
		out.TaskComplete(taskID, verifyPassed)
	}

	// Set up OnIdle callback for watch mode headless output
	eng.OnIdle = func() {
		if jsonl {
			fmt.Println(`{"type":"idle","message":"waiting for tasks"}`)
		} else {
			fmt.Println("[IDLE] No tasks available, waiting...")
		}
	}

	// Context generation callbacks for headless mode
	eng.OnContextGenerating = func(epicID string, taskCount int) {
		out.ContextGenerating(epicID, taskCount)
	}
	eng.OnContextGenerated = func(epicID string, tokenCount int) {
		out.ContextGenerated(epicID, tokenCount)
	}
	eng.OnContextLoaded = func(epicID string, content string) {
		out.ContextLoaded(epicID, content)
	}
	eng.OnContextSkipped = func(epicID string, reason string) {
		out.ContextSkipped(epicID, reason)
	}
	eng.OnContextFailed = func(epicID string, errMsg string) {
		out.ContextFailed(epicID, errMsg)
	}
	eng.OnContextActive = func(epicID string) {
		out.ContextActive(epicID)
	}

	// Output start
	out.Start(epic, maxIterations, maxCost)
	if useWorktree && !jsonl {
		fmt.Println("[START] Running in isolated worktree")
	}
	if watch && !jsonl {
		if watchTimeout > 0 {
			fmt.Printf("[START] Watch mode enabled (timeout: %v, poll: %v)\n", watchTimeout, watchPollInterval)
		} else {
			fmt.Printf("[START] Watch mode enabled (poll: %v)\n", watchPollInterval)
		}
	}

	// Run
	config := engine.RunConfig{
		EpicID:            epicID,
		MaxIterations:     maxIterations,
		MaxCost:           maxCost,
		CheckpointEvery:   checkpointInterval,
		MaxTaskRetries:    maxTaskRetries,
		UseWorktree:       useWorktree,
		Watch:             watch,
		WatchTimeout:      watchTimeout,
		WatchPollInterval: watchPollInterval,
		DebounceInterval:  debounceInterval,
	}

	result, err := eng.Run(ctx, config)

	// Log run end
	if runLogger != nil {
		if result != nil {
			signalStr := ""
			if result.Signal != engine.SignalNone {
				signalStr = result.Signal.String()
			}
			runLogger.LogRunEnd(runlog.RunEndData{
				ExitReason:     result.ExitReason,
				Iterations:     result.Iterations,
				CompletedTasks: result.CompletedTasks,
				TotalTokens:    result.TotalTokens,
				TotalCost:      result.TotalCost,
				Duration:       result.Duration,
				Signal:         signalStr,
				SignalReason:   result.SignalReason,
			})
		}
		runLogger.Close()
	}

	if err != nil {
		out.Error(err)
		return ExitError
	}

	// Output final summary
	out.Complete(result)

	// Return appropriate exit code
	switch result.Signal {
	case engine.SignalComplete:
		return ExitSuccess
	case engine.SignalEject:
		return ExitEject
	case engine.SignalBlocked:
		return ExitBlocked
	default:
		// Check if it was budget exceeded
		if result.Iterations >= maxIterations {
			return ExitMaxIterations
		}
		return ExitSuccess
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

	// Set up context generation
	contextStore := epiccontext.NewStore()
	contextGenerator, err := epiccontext.NewGenerator(claudeAgent)
	if err == nil {
		eng.SetContextComponents(contextStore, contextGenerator)
	}

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

// autoSelectEpics uses tk to find up to max ready epics.
// Returns epic IDs sorted by priority.
func autoSelectEpics(max int) ([]string, error) {
	ticksClient := ticks.NewClient()
	epics, err := ticksClient.ListReadyEpics()
	if err != nil {
		return nil, err
	}
	if len(epics) == 0 {
		return nil, nil
	}

	// Take up to max epics
	count := len(epics)
	if count > max {
		count = max
	}

	ids := make([]string, count)
	for i := 0; i < count; i++ {
		ids[i] = epics[i].ID
	}
	return ids, nil
}

// WorkItem represents the next piece of work to run - either an epic or a standalone task.
type WorkItem struct {
	// Epic fields (mutually exclusive with Task)
	EpicID    string
	EpicTitle string

	// Task fields (for standalone/orphan tasks)
	Task *ticks.Task

	// IsStandalone is true when this is a standalone/orphan task (not an epic)
	IsStandalone bool
}

// findNextWork finds the next available work item (epic or standalone task).
// Priority: epics first, then standalone tasks, then orphan tasks.
func findNextWork(client *ticks.Client, includeStandalone, includeOrphans bool) *WorkItem {
	// Try to find an epic first
	epicIDs, _ := autoSelectEpics(1)
	if len(epicIDs) > 0 {
		epic, err := client.GetEpic(epicIDs[0])
		if err == nil {
			return &WorkItem{
				EpicID:       epic.ID,
				EpicTitle:    epic.Title,
				IsStandalone: false,
			}
		}
	}

	// Try standalone tasks
	if includeStandalone {
		task, _ := client.NextTaskWithOptions(ticks.StandaloneOnly())
		if task != nil {
			return &WorkItem{
				Task:         task,
				IsStandalone: true,
			}
		}
	}

	// Try orphaned tasks
	if includeOrphans {
		task, _ := client.NextTaskWithOptions(ticks.OrphanedOnly())
		if task != nil {
			return &WorkItem{
				Task:         task,
				IsStandalone: true,
			}
		}
	}

	return nil
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

// isVerificationEnabled checks if verification should be enabled.
// Returns false if verification is disabled via config.
func isVerificationEnabled() bool {
	dir, err := os.Getwd()
	if err != nil {
		return false
	}

	// Check config
	config, err := verify.LoadConfig(dir)
	if err != nil {
		// Config error - log but continue without verification
		fmt.Fprintf(os.Stderr, "Warning: error loading verification config: %v\n", err)
		return false
	}
	return config.IsEnabled()
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

// runMerge attempts to merge a previously conflicted epic's worktree branch.
func runMerge(cmd *cobra.Command, args []string) {
	epicID := args[0]

	// Get current working directory
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Create worktree manager
	wtManager, err := worktree.NewManager(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Check if worktree exists for this epic
	wt, err := wtManager.Get(epicID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}
	if wt == nil {
		fmt.Fprintf(os.Stderr, "No worktree found for epic %s\n", epicID)
		fmt.Fprintln(os.Stderr, "The worktree may have already been cleaned up, or the epic ID is incorrect.")
		os.Exit(ExitError)
	}

	// Create merge manager
	mergeManager, err := worktree.NewMergeManager(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	// Check if branch is already merged (user resolved manually)
	branch := worktree.BranchPrefix + epicID
	if isBranchMerged(dir, branch, mergeManager.MainBranch()) {
		fmt.Printf("Branch %s is already merged into %s\n", branch, mergeManager.MainBranch())
		fmt.Println("Cleaning up worktree...")

		if err := wtManager.Remove(epicID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree: %v\n", err)
		} else {
			fmt.Println("Worktree cleaned up successfully")
		}
		os.Exit(ExitSuccess)
	}

	// Attempt merge
	fmt.Printf("Attempting to merge %s into %s...\n", branch, mergeManager.MainBranch())
	result, err := mergeManager.Merge(wt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(ExitError)
	}

	if !result.Success {
		// Still has conflicts
		fmt.Println()
		fmt.Println("══════════════════════════════════════════════════════════════")
		fmt.Println("  MERGE CONFLICT - Manual resolution required")
		fmt.Println("══════════════════════════════════════════════════════════════")
		fmt.Println()
		if len(result.Conflicts) > 0 {
			fmt.Println("  Conflicting files:")
			for _, f := range result.Conflicts {
				fmt.Printf("    • %s\n", f)
			}
			fmt.Println()
		}
		if result.ErrorMessage != "" {
			fmt.Printf("  Error: %s\n", result.ErrorMessage)
			fmt.Println()
		}
		fmt.Println("  To resolve:")
		fmt.Println("    1. git checkout", mergeManager.MainBranch())
		fmt.Printf("    2. git merge %s\n", branch)
		fmt.Println("    3. Resolve conflicts and commit")
		fmt.Println()
		fmt.Printf("  Then run: ticker merge %s\n", epicID)
		fmt.Println()
		fmt.Println("══════════════════════════════════════════════════════════════")

		// Abort the failed merge to clean up state
		_ = mergeManager.AbortMerge()
		os.Exit(ExitError)
	}

	// Success!
	fmt.Printf("Successfully merged %s (commit: %s)\n", branch, result.MergeCommit[:8])
	fmt.Println("Cleaning up worktree...")

	if err := wtManager.Remove(epicID); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to remove worktree: %v\n", err)
	} else {
		fmt.Println("Worktree cleaned up successfully")
	}

	os.Exit(ExitSuccess)
}

// runStandaloneInTUI runs standalone tasks with output sent to the TUI.
// This is used when auto mode switches from epic to standalone task processing.
func runStandaloneInTUI(ctx context.Context, p *tea.Program, initialTask *ticks.Task, ticksClient *ticks.Client, claudeAgent *agent.ClaudeAgent, budgetTracker *budget.Tracker, checkpointMgr *checkpoint.Manager, skipVerify, includeStandalone, includeOrphans bool) {
	currentTask := initialTask

	for currentTask != nil {
		// Check context cancellation
		if ctx.Err() != nil {
			return
		}

		// Check budget limits
		if shouldStop, _ := budgetTracker.ShouldStop(); shouldStop {
			return
		}

		p.Send(tui.GlobalStatusMsg{Message: fmt.Sprintf("Running standalone task: [%s] %s", currentTask.ID, currentTask.Title)})

		// Build prompt using the prompt builder
		promptBuilder := engine.NewPromptBuilder()

		// Get task notes for human feedback context
		humanNotes, _ := ticksClient.GetHumanNotes(currentTask.ID)

		// Try to get parent epic info for context (even if closed/orphaned)
		var parentEpic *ticks.Epic
		var epicNotes []string
		if currentTask.Parent != "" {
			if epic, err := ticksClient.GetEpic(currentTask.Parent); err == nil {
				parentEpic = epic
				epicNotes, _ = ticksClient.GetNotes(currentTask.Parent)
			}
		}

		iterCtx := engine.IterationContext{
			Iteration:     1,
			Epic:          parentEpic,
			Task:          currentTask,
			EpicNotes:     epicNotes,
			HumanFeedback: humanNotes,
		}

		prompt := promptBuilder.Build(iterCtx)

		// Mark task as in_progress before starting
		_ = ticksClient.SetStatus(currentTask.ID, "in_progress")

		// Run the agent
		agentResult, err := claudeAgent.Run(ctx, prompt, agent.RunOpts{
			Timeout: 30 * time.Minute,
		})

		if err != nil {
			p.Send(tui.ErrorMsg{Err: err})
			return
		}

		// Send output to TUI
		p.Send(tui.AgentTextMsg{Text: agentResult.Output})

		// Update budget tracking
		budgetTracker.Add(agentResult.TokensIn, agentResult.TokensOut, agentResult.Cost)

		// Store run record on task
		if agentResult.Record != nil {
			_ = ticksClient.SetRunRecord(currentTask.ID, agentResult.Record)
		}

		// Parse signals from output
		signal, signalReason := engine.ParseSignals(agentResult.Output)

		if signal != engine.SignalNone && signal != engine.SignalComplete {
			handleStandaloneSignal(ticksClient, currentTask, signal, signalReason)
		}

		// Check if task was closed
		updatedTask, err := ticksClient.GetTask(currentTask.ID)
		if err == nil && updatedTask.Status == "closed" {
			// Run verification if enabled
			if !skipVerify && isVerificationEnabled() {
				passed := runStandaloneVerification(ctx, currentTask.ID, agentResult.Output)
				if !passed {
					_ = ticksClient.ReopenTask(currentTask.ID)
					continue // Retry the same task
				}
			}
		}

		// Get next standalone/orphan task
		var nextTask *ticks.Task
		if includeStandalone {
			nextTask, _ = ticksClient.NextTaskWithOptions(ticks.StandaloneOnly())
		}
		if nextTask == nil && includeOrphans {
			nextTask, _ = ticksClient.NextTaskWithOptions(ticks.OrphanedOnly())
		}

		currentTask = nextTask
	}
}

// runStandaloneTask runs a single standalone or orphan task (task without active parent epic).
// Unlike epic-based runs, this directly processes one task at a time and then looks for the next.
// includeStandalone and includeOrphans control which task types to continue picking up after each completion.
func runStandaloneTask(initialTask *ticks.Task, maxIterations int, maxCost float64, checkpointInterval, maxTaskRetries int, skipVerify, jsonl, includeStandalone, includeOrphans bool) {
	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create headless output formatter
	out := engine.NewHeadlessOutput(jsonl, "")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		out.Interrupted()
		cancel()
	}()

	// Initialize components
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		out.Error(fmt.Errorf("claude CLI not found - please install Claude Code"))
		os.Exit(ExitError)
	}

	ticksClient := ticks.NewClient()
	budgetTracker := budget.NewTracker(budget.Limits{
		MaxIterations: maxIterations,
		MaxCost:       maxCost,
	})
	checkpointMgr := checkpoint.NewManager()

	// Create engine for running iterations
	eng := engine.NewEngine(claudeAgent, ticksClient, budgetTracker, checkpointMgr)

	// Set up context generation (use discard logger in jsonl mode)
	contextStore := epiccontext.NewStore()
	var contextGenOpts []epiccontext.GeneratorOption
	if jsonl {
		contextGenOpts = append(contextGenOpts, epiccontext.WithLogger(slog.New(slog.DiscardHandler)))
	}
	contextGenerator, err := epiccontext.NewGenerator(claudeAgent, contextGenOpts...)
	if err == nil {
		eng.SetContextComponents(contextStore, contextGenerator)
	}

	// Set up verification runner (unless --skip-verify)
	if !skipVerify {
		if isVerificationEnabled() {
			eng.EnableVerification()
		}
	}

	// Track verification pass status for task_complete output
	var verifyPassed bool = true

	// Set up output callbacks
	eng.OnOutput = func(chunk string) {
		out.Output(chunk)
	}

	eng.OnSignal = func(sig engine.Signal, reason string) {
		out.Signal(sig, reason)
	}

	// Output start message
	if jsonl {
		fmt.Printf(`{"type":"standalone_start","task_id":"%s","title":"%s","max_iterations":%d,"max_cost":%.2f}`+"\n",
			initialTask.ID, initialTask.Title, maxIterations, maxCost)
	} else {
		fmt.Printf("[START] Standalone task mode\n")
		fmt.Printf("[START] Budget: max %d iterations, $%.2f\n", maxIterations, maxCost)
	}

	// Run tasks in a loop
	currentTask := initialTask
	iteration := 0
	totalCost := 0.0
	totalTokens := 0

	for currentTask != nil {
		// Check context cancellation
		if ctx.Err() != nil {
			break
		}

		// Check budget limits
		if shouldStop, _ := budgetTracker.ShouldStop(); shouldStop {
			break
		}

		iteration++

		// Output task start
		if jsonl {
			fmt.Printf(`{"type":"task","task_id":"%s","title":"%s","iteration":%d}`+"\n",
				currentTask.ID, currentTask.Title, iteration)
		} else {
			fmt.Printf("[TASK] %s - %s (iteration %d)\n", currentTask.ID, currentTask.Title, iteration)
		}

		// Build prompt using the prompt builder
		promptBuilder := engine.NewPromptBuilder()

		// Get task notes for human feedback context
		humanNotes, _ := ticksClient.GetHumanNotes(currentTask.ID)

		// Try to get parent epic info for context (even if closed/orphaned)
		var parentEpic *ticks.Epic
		var epicNotes []string
		if currentTask.Parent != "" {
			if epic, err := ticksClient.GetEpic(currentTask.Parent); err == nil {
				parentEpic = epic
				epicNotes, _ = ticksClient.GetNotes(currentTask.Parent)
			}
		}

		iterCtx := engine.IterationContext{
			Iteration:     iteration,
			Epic:          parentEpic,
			Task:          currentTask,
			EpicNotes:     epicNotes,
			HumanFeedback: humanNotes,
		}

		prompt := promptBuilder.Build(iterCtx)

		// Mark task as in_progress before starting (enables crash recovery)
		if err := ticksClient.SetStatus(currentTask.ID, "in_progress"); err != nil {
			// Log but continue - status update is not critical
			if jsonl {
				fmt.Printf(`{"type":"warning","message":"could not mark %s as in_progress: %v"}`+"\n", currentTask.ID, err)
			} else {
				fmt.Printf("[WARN] Could not mark %s as in_progress: %v\n", currentTask.ID, err)
			}
		}

		// Run the agent
		agentResult, err := claudeAgent.Run(ctx, prompt, agent.RunOpts{
			Timeout: 30 * time.Minute,
			Stream:  nil, // Use callback instead
		})

		if err != nil {
			if jsonl {
				fmt.Printf(`{"type":"error","error":"%s"}`+"\n", err.Error())
			} else {
				fmt.Printf("[ERROR] %s\n", err.Error())
			}
			break
		}

		// Stream output
		out.Output(agentResult.Output)

		// Update budget tracking
		budgetTracker.Add(agentResult.TokensIn, agentResult.TokensOut, agentResult.Cost)
		totalCost += agentResult.Cost
		totalTokens += agentResult.TokensIn + agentResult.TokensOut

		// Store run record on task (enables viewing conversation history)
		if agentResult.Record != nil {
			_ = ticksClient.SetRunRecord(currentTask.ID, agentResult.Record)
		}

		// Parse signals from output
		signal, signalReason := engine.ParseSignals(agentResult.Output)

		if signal != engine.SignalNone {
			out.Signal(signal, signalReason)

			// Handle handoff signals by setting awaiting state
			if signal != engine.SignalComplete {
				handleStandaloneSignal(ticksClient, currentTask, signal, signalReason)
			}
		}

		// Check if task was closed
		updatedTask, err := ticksClient.GetTask(currentTask.ID)
		if err == nil && updatedTask.Status == "closed" {
			// Run verification if enabled
			if !skipVerify && isVerificationEnabled() {
				verifyPassed = runStandaloneVerification(ctx, currentTask.ID, agentResult.Output)
				if !verifyPassed {
					// Reopen the task if verification failed
					_ = ticksClient.ReopenTask(currentTask.ID)
					if jsonl {
						fmt.Printf(`{"type":"verify_failed","task_id":"%s"}`+"\n", currentTask.ID)
					} else {
						fmt.Printf("[VERIFY] %s - failed, task reopened\n", currentTask.ID)
					}
					// Continue with same task
					continue
				}
			}

			out.TaskComplete(currentTask.ID, verifyPassed)
		}

		// Get next task based on priority:
		// 1. Standalone tasks (if enabled)
		// 2. Orphaned tasks (if enabled)
		var nextTask *ticks.Task
		if includeStandalone {
			nextTask, _ = ticksClient.NextTaskWithOptions(ticks.StandaloneOnly())
		}
		if nextTask == nil && includeOrphans {
			nextTask, _ = ticksClient.NextTaskWithOptions(ticks.OrphanedOnly())
		}

		currentTask = nextTask
	}

	// Output completion summary
	if jsonl {
		fmt.Printf(`{"type":"standalone_complete","iterations":%d,"total_cost":%.4f,"total_tokens":%d}`+"\n",
			iteration, totalCost, totalTokens)
	} else {
		fmt.Printf("\n[COMPLETE] Standalone task run finished\n")
		fmt.Printf("[COMPLETE] %d iterations, $%.4f, %d tokens\n", iteration, totalCost, totalTokens)
	}

	os.Exit(ExitSuccess)
}

// handleStandaloneSignal processes a handoff signal for a standalone task.
func handleStandaloneSignal(client *ticks.Client, task *ticks.Task, sig engine.Signal, context string) {
	// Map signals to awaiting states
	awaitingMap := map[engine.Signal]string{
		engine.SignalEject:           "work",
		engine.SignalBlocked:         "input",
		engine.SignalApprovalNeeded:  "approval",
		engine.SignalInputNeeded:     "input",
		engine.SignalReviewRequested: "review",
		engine.SignalContentReview:   "content",
		engine.SignalEscalate:        "escalation",
		engine.SignalCheckpoint:      "checkpoint",
	}

	if awaiting, ok := awaitingMap[sig]; ok {
		_ = client.SetAwaiting(task.ID, awaiting, context)
	}
}

// runStandaloneVerification runs verification for a standalone task.
func runStandaloneVerification(ctx context.Context, taskID string, agentOutput string) bool {
	dir, err := os.Getwd()
	if err != nil {
		return true // Skip verification on error
	}

	gitVerifier := verify.NewGitVerifier(dir)
	if gitVerifier == nil {
		return true // Not a git repo, skip verification
	}

	runner := verify.NewRunner(dir, gitVerifier)
	results := runner.Run(ctx, taskID, agentOutput)

	return results == nil || results.AllPassed
}

// isBranchMerged checks if a branch has been merged into main.
func isBranchMerged(repoRoot, branch, mainBranch string) bool {
	// Check if branch still exists
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	if cmd.Run() != nil {
		// Branch doesn't exist - consider it merged (or deleted)
		return true
	}

	// Check if branch is ancestor of main (i.e., merged)
	cmd = exec.Command("git", "merge-base", "--is-ancestor", branch, mainBranch)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}

// runContext implements the 'ticker context' command for managing epic context documents.
func runContext(cmd *cobra.Command, args []string) {
	epicID := args[0]
	showOnly, _ := cmd.Flags().GetBool("show")
	refresh, _ := cmd.Flags().GetBool("refresh")
	deleteContext, _ := cmd.Flags().GetBool("delete")

	// Validate mutual exclusivity of flags
	flagCount := 0
	if showOnly {
		flagCount++
	}
	if refresh {
		flagCount++
	}
	if deleteContext {
		flagCount++
	}
	if flagCount > 1 {
		fmt.Fprintln(os.Stderr, "Error: --show, --refresh, and --delete are mutually exclusive")
		os.Exit(ExitError)
	}

	// Validate epic exists
	ticksClient := ticks.NewClient()
	epic, err := ticksClient.GetEpic(epicID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: epic %s not found: %v\n", epicID, err)
		os.Exit(ExitError)
	}

	// Initialize context store
	store := epiccontext.NewStore()
	contextExists := store.Exists(epicID)

	// Handle --delete flag
	if deleteContext {
		if !contextExists {
			fmt.Printf("No context exists for epic %s\n", epicID)
			os.Exit(ExitSuccess)
		}
		if err := store.Delete(epicID); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting context: %v\n", err)
			os.Exit(ExitError)
		}
		fmt.Printf("Context deleted for epic %s\n", epicID)
		os.Exit(ExitSuccess)
	}

	// Handle --show flag
	if showOnly {
		if !contextExists {
			fmt.Fprintf(os.Stderr, "Error: no context exists for epic %s\n", epicID)
			fmt.Fprintln(os.Stderr, "Run 'ticker context <epic-id>' to generate context.")
			os.Exit(ExitError)
		}
		content, err := store.Load(epicID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading context: %v\n", err)
			os.Exit(ExitError)
		}
		fmt.Print(content)
		os.Exit(ExitSuccess)
	}

	// Handle --refresh flag or default behavior
	if contextExists && !refresh {
		// Default: show existing context
		content, err := store.Load(epicID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading context: %v\n", err)
			os.Exit(ExitError)
		}
		fmt.Print(content)
		os.Exit(ExitSuccess)
	}

	// Generate context (either refresh requested or context doesn't exist)
	generateContext(epicID, epic, ticksClient, store, refresh)
}

// generateContext handles the context generation process with progress output.
func generateContext(epicID string, epic *ticks.Epic, ticksClient *ticks.Client, store *epiccontext.Store, isRefresh bool) {
	// Check claude availability
	claudeAgent := agent.NewClaudeAgent()
	if !claudeAgent.Available() {
		fmt.Fprintln(os.Stderr, "Error: claude CLI not found. Please install Claude Code.")
		os.Exit(ExitError)
	}

	// Get tasks for the epic
	tasks, err := ticksClient.ListTasks(epicID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing tasks: %v\n", err)
		os.Exit(ExitError)
	}

	// Check if epic has enough tasks to benefit from context
	if len(tasks) <= 1 {
		fmt.Printf("Epic %s has only %d task(s). Context generation skipped (no amortization benefit).\n", epicID, len(tasks))
		os.Exit(ExitSuccess)
	}

	// Create generator
	generator, err := epiccontext.NewGenerator(claudeAgent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating generator: %v\n", err)
		os.Exit(ExitError)
	}

	// Progress output
	if isRefresh {
		fmt.Printf("Regenerating context for epic [%s] %s...\n", epicID, epic.Title)
	} else {
		fmt.Printf("Generating context for epic [%s] %s...\n", epicID, epic.Title)
	}
	fmt.Printf("  Tasks: %d\n", len(tasks))
	fmt.Println("  Analyzing codebase...")

	// Set up context with signal handling for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nInterrupted, cancelling...")
		cancel()
	}()

	// Generate the context
	startTime := time.Now()
	content, err := generator.Generate(ctx, epic, tasks)
	duration := time.Since(startTime)

	if err != nil {
		if ctx.Err() == context.Canceled {
			fmt.Fprintln(os.Stderr, "Context generation cancelled")
			os.Exit(ExitError)
		}
		fmt.Fprintf(os.Stderr, "Error generating context: %v\n", err)
		os.Exit(ExitError)
	}

	// Save the context
	if err := store.Save(epicID, content); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving context: %v\n", err)
		os.Exit(ExitError)
	}

	// Estimate token count (rough: ~4 chars per token for English text)
	estimatedTokens := len(content) / 4

	// Success output
	fmt.Println()
	if isRefresh {
		fmt.Printf("Context regenerated for epic %s\n", epicID)
	} else {
		fmt.Printf("Context generated for epic %s\n", epicID)
	}
	fmt.Printf("  Duration: %v\n", duration.Round(time.Second))
	fmt.Printf("  Size: ~%d tokens\n", estimatedTokens)
	fmt.Printf("  Saved to: %s/%s.md\n", store.Dir(), epicID)

	os.Exit(ExitSuccess)
}
