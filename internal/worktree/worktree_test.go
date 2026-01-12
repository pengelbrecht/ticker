package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManager(t *testing.T) {
	t.Run("returns error for non-git directory", func(t *testing.T) {
		dir := t.TempDir()

		_, err := NewManager(dir)
		if err != ErrNotGitRepo {
			t.Errorf("NewManager() error = %v, want %v", err, ErrNotGitRepo)
		}
	})

	t.Run("returns manager for git directory", func(t *testing.T) {
		dir := createTempGitRepo(t)

		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}
		if m == nil {
			t.Fatal("NewManager() returned nil")
		}
		if m.repoRoot != dir {
			t.Errorf("Manager.repoRoot = %q, want %q", m.repoRoot, dir)
		}
		if m.worktreeDir != filepath.Join(dir, DefaultWorktreeDir) {
			t.Errorf("Manager.worktreeDir = %q, want %q", m.worktreeDir, filepath.Join(dir, DefaultWorktreeDir))
		}
	})

	t.Run("returns error for nonexistent directory", func(t *testing.T) {
		_, err := NewManager("/nonexistent/path")
		if err != ErrNotGitRepo {
			t.Errorf("NewManager() error = %v, want %v", err, ErrNotGitRepo)
		}
	})
}

func TestManager_Create(t *testing.T) {
	t.Run("creates worktree with new branch", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		wt, err := m.Create("abc123")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify worktree properties
		if wt.EpicID != "abc123" {
			t.Errorf("Worktree.EpicID = %q, want %q", wt.EpicID, "abc123")
		}
		if wt.Branch != "ticker/abc123" {
			t.Errorf("Worktree.Branch = %q, want %q", wt.Branch, "ticker/abc123")
		}
		expectedPath := filepath.Join(dir, DefaultWorktreeDir, "abc123")
		if wt.Path != expectedPath {
			t.Errorf("Worktree.Path = %q, want %q", wt.Path, expectedPath)
		}
		if wt.Created.IsZero() {
			t.Error("Worktree.Created should not be zero")
		}

		// Verify worktree directory exists
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			t.Error("Worktree directory should exist")
		}

		// Verify branch exists
		if !m.branchExists("ticker/abc123") {
			t.Error("Branch ticker/abc123 should exist")
		}
	})

	t.Run("creates worktree with existing branch", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create branch first
		cmd := exec.Command("git", "branch", "ticker/existing")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to create branch: %v", err)
		}

		wt, err := m.Create("existing")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if wt.Branch != "ticker/existing" {
			t.Errorf("Worktree.Branch = %q, want %q", wt.Branch, "ticker/existing")
		}
	})

	t.Run("returns error if worktree already exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create first worktree
		_, err = m.Create("dup123")
		if err != nil {
			t.Fatalf("Create() first call error = %v", err)
		}

		// Try to create duplicate
		_, err = m.Create("dup123")
		if err != ErrWorktreeExists {
			t.Errorf("Create() second call error = %v, want %v", err, ErrWorktreeExists)
		}
	})

	t.Run("creates .worktrees directory if not exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		worktreesDir := filepath.Join(dir, DefaultWorktreeDir)
		if _, err := os.Stat(worktreesDir); !os.IsNotExist(err) {
			t.Fatal(".worktrees directory should not exist yet")
		}

		_, err = m.Create("new123")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if _, err := os.Stat(worktreesDir); os.IsNotExist(err) {
			t.Error(".worktrees directory should exist after Create()")
		}
	})
}

func TestManager_Remove(t *testing.T) {
	t.Run("removes worktree and branch", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create worktree
		wt, err := m.Create("rem123")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify it exists
		if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
			t.Fatal("Worktree should exist before remove")
		}

		// Remove it
		if err := m.Remove("rem123"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		// Verify worktree directory is gone
		if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
			t.Error("Worktree directory should not exist after Remove()")
		}

		// Verify branch is deleted
		if m.branchExists("ticker/rem123") {
			t.Error("Branch ticker/rem123 should not exist after Remove()")
		}
	})

	t.Run("removes worktree with uncommitted changes", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create worktree
		wt, err := m.Create("dirty")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Create uncommitted file in worktree
		uncommittedFile := filepath.Join(wt.Path, "uncommitted.txt")
		if err := os.WriteFile(uncommittedFile, []byte("dirty"), 0644); err != nil {
			t.Fatalf("failed to create uncommitted file: %v", err)
		}

		// Remove should still succeed (force)
		if err := m.Remove("dirty"); err != nil {
			t.Fatalf("Remove() error = %v", err)
		}

		// Verify worktree is gone
		if _, err := os.Stat(wt.Path); !os.IsNotExist(err) {
			t.Error("Worktree directory should not exist after force Remove()")
		}
	})

	t.Run("returns error for nonexistent worktree", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		err = m.Remove("nonexistent")
		if err != ErrWorktreeNotFound {
			t.Errorf("Remove() error = %v, want %v", err, ErrWorktreeNotFound)
		}
	})
}

func TestManager_Get(t *testing.T) {
	t.Run("returns worktree if exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create worktree
		created, err := m.Create("get123")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Get it back
		got, err := m.Get("get123")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got == nil {
			t.Fatal("Get() returned nil, want worktree")
		}
		if got.EpicID != created.EpicID {
			t.Errorf("Get().EpicID = %q, want %q", got.EpicID, created.EpicID)
		}
		// Compare paths by evaluating symlinks (macOS /var -> /private/var)
		gotPath, _ := filepath.EvalSymlinks(got.Path)
		createdPath, _ := filepath.EvalSymlinks(created.Path)
		if gotPath != createdPath {
			t.Errorf("Get().Path = %q, want %q", got.Path, created.Path)
		}
		if got.Branch != created.Branch {
			t.Errorf("Get().Branch = %q, want %q", got.Branch, created.Branch)
		}
	})

	t.Run("returns nil if not exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		got, err := m.Get("nonexistent")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if got != nil {
			t.Errorf("Get() = %v, want nil", got)
		}
	})
}

func TestManager_List(t *testing.T) {
	t.Run("returns empty list when no ticker worktrees", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		worktrees, err := m.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(worktrees) != 0 {
			t.Errorf("List() returned %d worktrees, want 0", len(worktrees))
		}
	})

	t.Run("returns all ticker worktrees", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create multiple worktrees
		_, err = m.Create("list1")
		if err != nil {
			t.Fatalf("Create(list1) error = %v", err)
		}
		_, err = m.Create("list2")
		if err != nil {
			t.Fatalf("Create(list2) error = %v", err)
		}
		_, err = m.Create("list3")
		if err != nil {
			t.Fatalf("Create(list3) error = %v", err)
		}

		worktrees, err := m.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(worktrees) != 3 {
			t.Errorf("List() returned %d worktrees, want 3", len(worktrees))
		}

		// Verify epic IDs
		epicIDs := make(map[string]bool)
		for _, wt := range worktrees {
			epicIDs[wt.EpicID] = true
		}
		for _, expected := range []string{"list1", "list2", "list3"} {
			if !epicIDs[expected] {
				t.Errorf("List() missing epic %q", expected)
			}
		}
	})

	t.Run("ignores non-ticker worktrees", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		// Create a ticker worktree
		_, err = m.Create("ticker1")
		if err != nil {
			t.Fatalf("Create(ticker1) error = %v", err)
		}

		// Create a non-ticker worktree directly
		otherPath := filepath.Join(dir, ".worktrees", "other")
		cmd := exec.Command("git", "worktree", "add", otherPath, "-b", "feature/other")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("failed to create non-ticker worktree: %v", err)
		}

		worktrees, err := m.List()
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if len(worktrees) != 1 {
			t.Errorf("List() returned %d worktrees, want 1 (should ignore non-ticker)", len(worktrees))
		}
		if worktrees[0].EpicID != "ticker1" {
			t.Errorf("List()[0].EpicID = %q, want %q", worktrees[0].EpicID, "ticker1")
		}
	})
}

func TestManager_Exists(t *testing.T) {
	t.Run("returns true if worktree exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		_, err = m.Create("exists1")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if !m.Exists("exists1") {
			t.Error("Exists() = false, want true")
		}
	})

	t.Run("returns false if worktree not exists", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		if m.Exists("nonexistent") {
			t.Error("Exists() = true, want false")
		}
	})
}

func TestWorktree_WorkingDirectory(t *testing.T) {
	t.Run("worktree has correct working directory", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		wt, err := m.Create("workdir")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Verify we can run git commands in the worktree
		cmd := exec.Command("git", "status")
		cmd.Dir = wt.Path
		if err := cmd.Run(); err != nil {
			t.Errorf("git status in worktree failed: %v", err)
		}

		// Verify the worktree has the initial.txt file from main
		initialFile := filepath.Join(wt.Path, "initial.txt")
		if _, err := os.Stat(initialFile); os.IsNotExist(err) {
			t.Error("Worktree should contain initial.txt from main branch")
		}
	})

	t.Run("changes in worktree are isolated", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		wt, err := m.Create("isolated")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		// Create file in worktree
		worktreeFile := filepath.Join(wt.Path, "worktree-only.txt")
		if err := os.WriteFile(worktreeFile, []byte("worktree content"), 0644); err != nil {
			t.Fatalf("failed to create file in worktree: %v", err)
		}

		// Verify file doesn't exist in main repo
		mainFile := filepath.Join(dir, "worktree-only.txt")
		if _, err := os.Stat(mainFile); !os.IsNotExist(err) {
			t.Error("File created in worktree should not exist in main repo")
		}
	})
}

func TestBranchNaming(t *testing.T) {
	t.Run("branch name includes epic ID", func(t *testing.T) {
		dir := createTempGitRepo(t)
		m, err := NewManager(dir)
		if err != nil {
			t.Fatalf("NewManager() error = %v", err)
		}

		wt, err := m.Create("abc123")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		if !strings.HasPrefix(wt.Branch, BranchPrefix) {
			t.Errorf("Branch %q should have prefix %q", wt.Branch, BranchPrefix)
		}
		if wt.Branch != "ticker/abc123" {
			t.Errorf("Branch = %q, want %q", wt.Branch, "ticker/abc123")
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
