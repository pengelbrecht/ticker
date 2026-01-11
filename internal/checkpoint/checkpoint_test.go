package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.Dir() != ".ticker/checkpoints" {
		t.Errorf("Dir() = %q, want %q", m.Dir(), ".ticker/checkpoints")
	}
}

func TestNewManagerWithDir(t *testing.T) {
	m := NewManagerWithDir("/custom/path")
	if m.Dir() != "/custom/path" {
		t.Errorf("Dir() = %q, want %q", m.Dir(), "/custom/path")
	}
}

func TestManager_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	cp := &Checkpoint{
		ID:             "abc-5",
		Timestamp:      time.Now().Truncate(time.Second),
		EpicID:         "abc",
		Iteration:      5,
		TotalTokens:    10000,
		TotalCost:      1.50,
		CompletedTasks: []string{"task1", "task2"},
		GitCommit:      "abc123def456",
	}

	// Save
	if err := m.Save(cp); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	filename := filepath.Join(dir, "abc-5.json")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Fatal("checkpoint file was not created")
	}

	// Load
	loaded, err := m.Load("abc-5")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify fields
	if loaded.ID != cp.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, cp.ID)
	}
	if loaded.EpicID != cp.EpicID {
		t.Errorf("EpicID = %q, want %q", loaded.EpicID, cp.EpicID)
	}
	if loaded.Iteration != cp.Iteration {
		t.Errorf("Iteration = %d, want %d", loaded.Iteration, cp.Iteration)
	}
	if loaded.TotalTokens != cp.TotalTokens {
		t.Errorf("TotalTokens = %d, want %d", loaded.TotalTokens, cp.TotalTokens)
	}
	if loaded.TotalCost != cp.TotalCost {
		t.Errorf("TotalCost = %f, want %f", loaded.TotalCost, cp.TotalCost)
	}
	if len(loaded.CompletedTasks) != len(cp.CompletedTasks) {
		t.Errorf("CompletedTasks length = %d, want %d", len(loaded.CompletedTasks), len(cp.CompletedTasks))
	}
	if loaded.GitCommit != cp.GitCommit {
		t.Errorf("GitCommit = %q, want %q", loaded.GitCommit, cp.GitCommit)
	}
}

func TestManager_Save_EmptyID(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	cp := &Checkpoint{
		ID: "",
	}

	err := m.Save(cp)
	if err == nil {
		t.Fatal("Save() should error on empty ID")
	}
}

func TestManager_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	_, err := m.Load("nonexistent")
	if err == nil {
		t.Fatal("Load() should error on nonexistent checkpoint")
	}
}

func TestManager_List(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	// Create multiple checkpoints
	now := time.Now()
	checkpoints := []*Checkpoint{
		{ID: "abc-1", Timestamp: now.Add(-2 * time.Hour), EpicID: "abc", Iteration: 1},
		{ID: "abc-5", Timestamp: now.Add(-1 * time.Hour), EpicID: "abc", Iteration: 5},
		{ID: "xyz-3", Timestamp: now, EpicID: "xyz", Iteration: 3},
	}

	for _, cp := range checkpoints {
		if err := m.Save(cp); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	// List all
	list, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("List() returned %d checkpoints, want 3", len(list))
	}

	// Should be sorted by timestamp, newest first
	if list[0].ID != "xyz-3" {
		t.Errorf("first checkpoint = %q, want %q (newest)", list[0].ID, "xyz-3")
	}
}

func TestManager_List_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	list, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(list) != 0 {
		t.Errorf("List() returned %d checkpoints, want 0", len(list))
	}
}

func TestManager_List_NonexistentDir(t *testing.T) {
	m := NewManagerWithDir("/nonexistent/path/checkpoints")

	list, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if list != nil && len(list) != 0 {
		t.Errorf("List() should return nil or empty for nonexistent dir")
	}
}

func TestManager_ListForEpic(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	// Create checkpoints for different epics
	checkpoints := []*Checkpoint{
		{ID: "abc-1", Timestamp: time.Now(), EpicID: "abc", Iteration: 1},
		{ID: "abc-5", Timestamp: time.Now(), EpicID: "abc", Iteration: 5},
		{ID: "abc-10", Timestamp: time.Now(), EpicID: "abc", Iteration: 10},
		{ID: "xyz-3", Timestamp: time.Now(), EpicID: "xyz", Iteration: 3},
	}

	for _, cp := range checkpoints {
		if err := m.Save(cp); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	// List for epic "abc"
	list, err := m.ListForEpic("abc")
	if err != nil {
		t.Fatalf("ListForEpic() error = %v", err)
	}

	if len(list) != 3 {
		t.Errorf("ListForEpic(abc) returned %d checkpoints, want 3", len(list))
	}

	// Should be sorted by iteration, newest first
	if list[0].Iteration != 10 {
		t.Errorf("first checkpoint iteration = %d, want 10 (highest)", list[0].Iteration)
	}
}

func TestManager_Latest(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	// Create checkpoints
	checkpoints := []*Checkpoint{
		{ID: "abc-1", Timestamp: time.Now(), EpicID: "abc", Iteration: 1},
		{ID: "abc-5", Timestamp: time.Now(), EpicID: "abc", Iteration: 5},
		{ID: "abc-10", Timestamp: time.Now(), EpicID: "abc", Iteration: 10},
	}

	for _, cp := range checkpoints {
		if err := m.Save(cp); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	// Get latest
	latest, err := m.Latest("abc")
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}

	if latest == nil {
		t.Fatal("Latest() returned nil")
	}

	if latest.Iteration != 10 {
		t.Errorf("Latest() iteration = %d, want 10", latest.Iteration)
	}
}

func TestManager_Latest_NoCheckpoints(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	latest, err := m.Latest("abc")
	if err != nil {
		t.Fatalf("Latest() error = %v", err)
	}

	if latest != nil {
		t.Error("Latest() should return nil when no checkpoints exist")
	}
}

func TestManager_Delete(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	cp := &Checkpoint{ID: "abc-5", EpicID: "abc", Iteration: 5}
	if err := m.Save(cp); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Delete
	if err := m.Delete("abc-5"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err := m.Load("abc-5")
	if err == nil {
		t.Error("checkpoint should be deleted")
	}
}

func TestManager_Delete_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	m := NewManagerWithDir(dir)

	// Should not error on nonexistent
	if err := m.Delete("nonexistent"); err != nil {
		t.Errorf("Delete() error = %v, want nil", err)
	}
}

func TestGenerateID(t *testing.T) {
	tests := []struct {
		epicID    string
		iteration int
		want      string
	}{
		{"abc", 1, "abc-1"},
		{"abc", 10, "abc-10"},
		{"h8d", 42, "h8d-42"},
	}

	for _, tt := range tests {
		got := GenerateID(tt.epicID, tt.iteration)
		if got != tt.want {
			t.Errorf("GenerateID(%q, %d) = %q, want %q", tt.epicID, tt.iteration, got, tt.want)
		}
	}
}

func TestNewCheckpoint(t *testing.T) {
	cp := NewCheckpoint("abc", 5, 10000, 1.50, []string{"task1", "task2"})

	if cp.ID != "abc-5" {
		t.Errorf("ID = %q, want %q", cp.ID, "abc-5")
	}
	if cp.EpicID != "abc" {
		t.Errorf("EpicID = %q, want %q", cp.EpicID, "abc")
	}
	if cp.Iteration != 5 {
		t.Errorf("Iteration = %d, want %d", cp.Iteration, 5)
	}
	if cp.TotalTokens != 10000 {
		t.Errorf("TotalTokens = %d, want %d", cp.TotalTokens, 10000)
	}
	if cp.TotalCost != 1.50 {
		t.Errorf("TotalCost = %f, want %f", cp.TotalCost, 1.50)
	}
	if len(cp.CompletedTasks) != 2 {
		t.Errorf("CompletedTasks length = %d, want %d", len(cp.CompletedTasks), 2)
	}
	if cp.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}
	// GitCommit may or may not be set depending on whether we're in a git repo
}

func TestGetGitCommit(t *testing.T) {
	// This test just ensures GetGitCommit doesn't panic
	// The actual value depends on whether we're in a git repo
	commit := GetGitCommit()
	t.Logf("GetGitCommit() = %q", commit)
	// In the ticker repo, we should get a commit
	if commit == "" {
		t.Log("Warning: GetGitCommit() returned empty string (may not be in git repo)")
	}
}
