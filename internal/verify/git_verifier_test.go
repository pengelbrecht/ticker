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
