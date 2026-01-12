package worktree

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultWorktreeDir is the default directory name for storing worktrees.
const DefaultWorktreeDir = ".worktrees"

// BranchPrefix is the prefix for worktree branch names.
const BranchPrefix = "ticker/"

// ErrNotGitRepo is returned when the directory is not a git repository.
var ErrNotGitRepo = errors.New("not a git repository")

// ErrWorktreeExists is returned when a worktree already exists for the epic.
var ErrWorktreeExists = errors.New("worktree already exists")

// ErrWorktreeNotFound is returned when a worktree doesn't exist for the epic.
var ErrWorktreeNotFound = errors.New("worktree not found")

// Worktree represents an active git worktree.
type Worktree struct {
	Path    string    // Absolute path to worktree directory
	Branch  string    // Branch name (e.g., ticker/abc123)
	EpicID  string    // Associated epic ID
	Created time.Time // When worktree was created
}

// Manager handles git worktree lifecycle.
type Manager struct {
	repoRoot    string // Root of main repository
	worktreeDir string // Base directory for worktrees (default: .worktrees)
}

// NewManager creates a worktree manager for the given repository.
// Returns error if not a git repository.
func NewManager(repoRoot string) (*Manager, error) {
	// Verify it's a git repository by checking for .git
	gitDir := filepath.Join(repoRoot, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return nil, ErrNotGitRepo
	}
	// .git can be a directory (normal repo) or a file (worktree itself)
	if !info.IsDir() && !info.Mode().IsRegular() {
		return nil, ErrNotGitRepo
	}

	return &Manager{
		repoRoot:    repoRoot,
		worktreeDir: filepath.Join(repoRoot, DefaultWorktreeDir),
	}, nil
}

// Create creates a new worktree for an epic.
// Branch name: ticker/<epic-id>
// Path: <repoRoot>/.worktrees/<epic-id>
// Creates branch from current HEAD if it doesn't exist.
func (m *Manager) Create(epicID string) (*Worktree, error) {
	wtPath := m.worktreePath(epicID)
	branch := m.branchName(epicID)

	// Check if worktree path already exists
	if _, err := os.Stat(wtPath); err == nil {
		return nil, ErrWorktreeExists
	}

	// Ensure .worktrees/ is gitignored before creating any worktrees
	if _, err := EnsureGitignore(m.repoRoot); err != nil {
		return nil, fmt.Errorf("ensuring gitignore: %w", err)
	}

	// Ensure worktree directory exists
	if err := os.MkdirAll(m.worktreeDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree directory: %w", err)
	}

	// Check if branch already exists
	branchExists := m.branchExists(branch)

	var cmd *exec.Cmd
	if branchExists {
		// Use existing branch
		cmd = exec.Command("git", "worktree", "add", wtPath, branch)
	} else {
		// Create new branch from HEAD
		cmd = exec.Command("git", "worktree", "add", wtPath, "-b", branch)
	}
	cmd.Dir = m.repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree: %s: %w", strings.TrimSpace(string(output)), err)
	}

	return &Worktree{
		Path:    wtPath,
		Branch:  branch,
		EpicID:  epicID,
		Created: time.Now(),
	}, nil
}

// Remove deletes a worktree and its branch.
// Force removes even if there are uncommitted changes.
func (m *Manager) Remove(epicID string) error {
	wtPath := m.worktreePath(epicID)
	branch := m.branchName(epicID)

	// Check if worktree exists
	if _, err := os.Stat(wtPath); os.IsNotExist(err) {
		return ErrWorktreeNotFound
	}

	// Remove worktree (force to handle uncommitted changes)
	cmd := exec.Command("git", "worktree", "remove", wtPath, "--force")
	cmd.Dir = m.repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove worktree: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Delete the branch
	if m.branchExists(branch) {
		cmd = exec.Command("git", "branch", "-D", branch)
		cmd.Dir = m.repoRoot
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to delete branch: %s: %w", strings.TrimSpace(string(output)), err)
		}
	}

	return nil
}

// Get returns the worktree for an epic, or nil if not exists.
func (m *Manager) Get(epicID string) (*Worktree, error) {
	worktrees, err := m.List()
	if err != nil {
		return nil, err
	}

	for _, wt := range worktrees {
		if wt.EpicID == epicID {
			return wt, nil
		}
	}

	return nil, nil
}

// List returns all active ticker worktrees.
func (m *Manager) List() ([]*Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.repoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}

	return m.parseWorktreeList(output)
}

// Exists checks if a worktree exists for the epic.
func (m *Manager) Exists(epicID string) bool {
	wtPath := m.worktreePath(epicID)
	_, err := os.Stat(wtPath)
	return err == nil
}

// worktreePath returns the path for an epic's worktree.
func (m *Manager) worktreePath(epicID string) string {
	return filepath.Join(m.worktreeDir, epicID)
}

// branchName returns the branch name for an epic.
func (m *Manager) branchName(epicID string) string {
	return BranchPrefix + epicID
}

// branchExists checks if a branch exists.
func (m *Manager) branchExists(branch string) bool {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = m.repoRoot
	return cmd.Run() == nil
}

// parseWorktreeList parses the output of `git worktree list --porcelain`.
// Format:
//
//	worktree /path/to/worktree
//	HEAD <commit>
//	branch refs/heads/<branch>
//	<blank line>
func (m *Manager) parseWorktreeList(output []byte) ([]*Worktree, error) {
	var worktrees []*Worktree
	var current *Worktree

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "worktree ") {
			// Start of new worktree entry
			path := strings.TrimPrefix(line, "worktree ")
			current = &Worktree{Path: path}
		} else if strings.HasPrefix(line, "branch ") && current != nil {
			// Branch line
			branch := strings.TrimPrefix(line, "branch refs/heads/")
			current.Branch = branch

			// Check if this is a ticker worktree
			if strings.HasPrefix(branch, BranchPrefix) {
				current.EpicID = strings.TrimPrefix(branch, BranchPrefix)
				worktrees = append(worktrees, current)
			}
			current = nil
		} else if line == "" {
			// End of entry - reset current
			current = nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse worktree list: %w", err)
	}

	return worktrees, nil
}
