package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

var rootCmd = &cobra.Command{
	Use:   "ticker",
	Short: "Autonomous AI agent loop runner",
	Long: `Ticker is a Go implementation of the Ralph Wiggum technique - running AI agents
in continuous loops until tasks are complete. It wraps the Ticks issue tracker
and orchestrates coding agents to autonomously complete epics.`,
	Version: version,
}

var runCmd = &cobra.Command{
	Use:   "run <epic-id>",
	Short: "Run an epic with the AI agent",
	Long: `Run starts the Ralph loop for the specified epic. The agent will iterate
through tasks until completion, ejection, or budget limits are reached.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		auto, _ := cmd.Flags().GetBool("auto")
		if len(args) == 0 && !auto {
			fmt.Fprintln(os.Stderr, "Error: either provide an epic-id or use --auto")
			os.Exit(1)
		}
		// TODO: Implement run logic
		fmt.Println("ticker run is not yet implemented")
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume <checkpoint-id>",
	Short: "Resume from a checkpoint",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// TODO: Implement resume logic
		fmt.Println("ticker resume is not yet implemented")
	},
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

		// Run install script via curl | sh
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
	runCmd.Flags().Float64("max-cost", 20.0, "Maximum cost in dollars")
	runCmd.Flags().Bool("auto", false, "Auto-select next ready epic")
	runCmd.Flags().Bool("headless", false, "Run without TUI (stdout/stderr only)")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(resumeCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
