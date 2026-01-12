package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitVerifier checks that there are no uncommitted changes.
type GitVerifier struct {
	dir string
}

// excludedPaths are paths that GitVerifier ignores.
// These are ticker's own metadata files that change during execution.
var excludedPaths = []string{
	".tick/",
	".ticker/",
}

// NewGitVerifier creates a git verifier for the given directory.
// Returns nil if directory is not a git repository.
func NewGitVerifier(dir string) *GitVerifier {
	// Check if .git exists in the directory
	gitDir := filepath.Join(dir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	return &GitVerifier{dir: dir}
}

// Name returns "git".
func (v *GitVerifier) Name() string {
	return "git"
}

// Verify checks for uncommitted changes using git status.
// Passes if working tree is clean (nothing to commit).
// Fails if there are uncommitted changes, listing them in output.
func (v *GitVerifier) Verify(ctx context.Context, taskID string, agentOutput string) *Result {
	start := time.Now()

	result := &Result{
		Verifier: v.Name(),
	}

	// Run git status --porcelain
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = v.dir

	output, err := cmd.Output()
	result.Duration = time.Since(start)

	if err != nil {
		// Check if git is not installed
		if execErr, ok := err.(*exec.Error); ok {
			result.Passed = false
			result.Error = execErr
			result.Output = "git command not found"
			return result
		}
		// Other error (e.g., not a git repo, though NewGitVerifier should catch that)
		result.Passed = false
		result.Error = err
		result.Output = err.Error()
		return result
	}

	// Filter out excluded paths (ticker metadata)
	outputStr := filterExcludedPaths(strings.TrimSpace(string(output)))

	// Empty output means clean working tree
	if outputStr == "" {
		result.Passed = true
		result.Output = "working tree clean"
		return result
	}

	// Non-empty output means uncommitted changes
	result.Passed = false
	result.Output = outputStr
	return result
}

// filterExcludedPaths removes lines matching excluded paths from git status output.
// Git status --porcelain format: "XY PATH" where XY is status, PATH is file path.
func filterExcludedPaths(output string) string {
	if output == "" {
		return ""
	}

	lines := strings.Split(output, "\n")
	var filtered []string

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Git status --porcelain format: first 2 chars are status, then space, then path
		// e.g., " M .tick/issues/lgt.json" or "?? .ticker/checkpoints/foo.json"
		path := ""
		if len(line) > 3 {
			path = line[3:] // Skip "XY " prefix
		}

		excluded := false
		for _, excludedPath := range excludedPaths {
			if strings.HasPrefix(path, excludedPath) {
				excluded = true
				break
			}
		}

		if !excluded {
			filtered = append(filtered, line)
		}
	}

	return strings.Join(filtered, "\n")
}
