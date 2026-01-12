package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGitVerifier(t *testing.T) {
	t.Run("returns nil for non-git directory", func(t *testing.T) {
		// Create temp dir without .git
		dir := t.TempDir()

		v := NewGitVerifier(dir)
		if v != nil {
			t.Error("NewGitVerifier() should return nil for non-git directory")
		}
	})

	t.Run("returns verifier for git directory", func(t *testing.T) {
		dir := createTempGitRepo(t)

		v := NewGitVerifier(dir)
		if v == nil {
			t.Error("NewGitVerifier() should return verifier for git directory")
		}
		if v != nil && v.dir != dir {
			t.Errorf("NewGitVerifier().dir = %q, want %q", v.dir, dir)
		}
	})

	t.Run("returns nil if .git is a file not directory", func(t *testing.T) {
		dir := t.TempDir()
		// Create .git as a file, not directory (edge case)
		if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: elsewhere"), 0644); err != nil {
			t.Fatalf("failed to create .git file: %v", err)
		}

		v := NewGitVerifier(dir)
		if v != nil {
			t.Error("NewGitVerifier() should return nil when .git is a file")
		}
	})
}

func TestGitVerifier_Name(t *testing.T) {
	v := &GitVerifier{dir: "/tmp"}
	if name := v.Name(); name != "git" {
		t.Errorf("GitVerifier.Name() = %q, want %q", name, "git")
	}
}

func TestGitVerifier_Verify(t *testing.T) {
	t.Run("passes with clean working tree", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		result := v.Verify(context.Background(), "test-task", "")

		if !result.Passed {
			t.Errorf("Verify() passed = %v, want true", result.Passed)
		}
		if result.Verifier != "git" {
			t.Errorf("Verify() verifier = %q, want %q", result.Verifier, "git")
		}
		if result.Duration == 0 {
			t.Error("Verify() duration should be > 0")
		}
		if !strings.Contains(result.Output, "clean") {
			t.Errorf("Verify() output = %q, want to contain 'clean'", result.Output)
		}
	})

	t.Run("fails with uncommitted changes", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		// Create a modified file
		existingFile := filepath.Join(dir, "initial.txt")
		if err := os.WriteFile(existingFile, []byte("modified content"), 0644); err != nil {
			t.Fatalf("failed to modify file: %v", err)
		}

		result := v.Verify(context.Background(), "test-task", "")

		if result.Passed {
			t.Error("Verify() passed = true, want false for uncommitted changes")
		}
		if !strings.Contains(result.Output, "initial.txt") {
			t.Errorf("Verify() output = %q, should list modified file", result.Output)
		}
	})

	t.Run("fails with untracked files", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		// Create an untracked file
		untrackedFile := filepath.Join(dir, "untracked.txt")
		if err := os.WriteFile(untrackedFile, []byte("new file"), 0644); err != nil {
			t.Fatalf("failed to create untracked file: %v", err)
		}

		result := v.Verify(context.Background(), "test-task", "")

		if result.Passed {
			t.Error("Verify() passed = true, want false for untracked files")
		}
		if !strings.Contains(result.Output, "untracked.txt") {
			t.Errorf("Verify() output = %q, should list untracked file", result.Output)
		}
	})

	t.Run("fails with staged but uncommitted changes", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		// Create and stage a new file
		stagedFile := filepath.Join(dir, "staged.txt")
		if err := os.WriteFile(stagedFile, []byte("staged content"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
		cmd := exec.Command("git", "add", "staged.txt")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to stage file: %v", err)
		}

		result := v.Verify(context.Background(), "test-task", "")

		if result.Passed {
			t.Error("Verify() passed = true, want false for staged changes")
		}
		if !strings.Contains(result.Output, "staged.txt") {
			t.Errorf("Verify() output = %q, should list staged file", result.Output)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		result := v.Verify(ctx, "test-task", "")

		// Should fail due to context cancellation
		if result.Passed {
			t.Error("Verify() should fail when context is cancelled")
		}
	})
}

// createTempGitRepo creates a temporary directory with an initialized git repo.
// Returns the directory path. The repo has one initial commit.
func createTempGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	// Configure git user (needed for commits)
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to configure git email: %v", err)
	}
	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to configure git name: %v", err)
	}

	// Create initial file and commit
	initialFile := filepath.Join(dir, "initial.txt")
	if err := os.WriteFile(initialFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("failed to create initial file: %v", err)
	}
	cmd = exec.Command("git", "add", "initial.txt")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to stage initial file: %v", err)
	}
	cmd = exec.Command("git", "commit", "-m", "Initial commit")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create initial commit: %v", err)
	}

	return dir
}

func TestFilterExcludedPaths(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "no excluded paths",
			input:    " M src/main.go\n?? readme.txt",
			expected: " M src/main.go\n?? readme.txt",
		},
		{
			name:     "only tick files",
			input:    " M .tick/issues/abc.json\n?? .ticker/checkpoints/xyz.json",
			expected: "",
		},
		{
			name:     "mixed paths",
			input:    " M src/main.go\n M .tick/issues/abc.json\n?? .ticker/checkpoints/xyz.json\n?? readme.txt",
			expected: " M src/main.go\n?? readme.txt",
		},
		{
			name:     "tick in subdirectory is not excluded",
			input:    " M src/.tick/foo.txt",
			expected: " M src/.tick/foo.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterExcludedPaths(tt.input)
			if result != tt.expected {
				t.Errorf("filterExcludedPaths(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGitVerifier_IgnoresTickerMetadata(t *testing.T) {
	t.Run("ignores untracked .tick and .ticker files", func(t *testing.T) {
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		// Create .tick and .ticker directories with files
		tickDir := filepath.Join(dir, ".tick", "issues")
		if err := os.MkdirAll(tickDir, 0755); err != nil {
			t.Fatalf("failed to create .tick dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tickDir, "abc.json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("failed to create tick file: %v", err)
		}

		tickerDir := filepath.Join(dir, ".ticker", "checkpoints")
		if err := os.MkdirAll(tickerDir, 0755); err != nil {
			t.Fatalf("failed to create .ticker dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(tickerDir, "cp.json"), []byte("{}"), 0644); err != nil {
			t.Fatalf("failed to create checkpoint file: %v", err)
		}

		// Verify should pass because .tick/ and .ticker/ are excluded
		result := v.Verify(context.Background(), "test-task", "")

		if !result.Passed {
			t.Errorf("Verify() should pass when only .tick/ and .ticker/ have changes, got output: %s", result.Output)
		}
	})

	t.Run("ignores modified .tick files", func(t *testing.T) {
		// This test specifically checks that MODIFIED .tick/ files are excluded.
		// Git status format for modified files is " M path" (space, M, space, path).
		// The leading space is significant for parsing.
		dir := createTempGitRepo(t)
		v := NewGitVerifier(dir)
		if v == nil {
			t.Fatal("NewGitVerifier returned nil")
		}

		// Create and commit a .tick file first
		tickDir := filepath.Join(dir, ".tick", "issues")
		if err := os.MkdirAll(tickDir, 0755); err != nil {
			t.Fatalf("failed to create .tick dir: %v", err)
		}
		tickFile := filepath.Join(tickDir, "task.json")
		if err := os.WriteFile(tickFile, []byte(`{"status":"open"}`), 0644); err != nil {
			t.Fatalf("failed to create tick file: %v", err)
		}

		// Stage and commit the .tick file
		cmd := exec.Command("git", "add", ".tick/issues/task.json")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to stage .tick file: %v", err)
		}
		cmd = exec.Command("git", "commit", "-m", "Add tick file")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to commit .tick file: %v", err)
		}

		// Now modify the file (this will show as " M .tick/issues/task.json")
		if err := os.WriteFile(tickFile, []byte(`{"status":"closed"}`), 0644); err != nil {
			t.Fatalf("failed to modify tick file: %v", err)
		}

		// Verify should pass because modified .tick/ files are excluded
		result := v.Verify(context.Background(), "test-task", "")

		if !result.Passed {
			t.Errorf("Verify() should pass when only modified .tick/ files exist, got output: %s", result.Output)
		}
	})
}
