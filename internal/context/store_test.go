package context

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewStore(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore() returned nil")
	}
	if s.Dir() != ".ticker/context" {
		t.Errorf("Dir() = %q, want %q", s.Dir(), ".ticker/context")
	}
}

func TestNewStoreWithDir(t *testing.T) {
	s := NewStoreWithDir("/custom/path")
	if s.Dir() != "/custom/path" {
		t.Errorf("Dir() = %q, want %q", s.Dir(), "/custom/path")
	}
}

func TestStore_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	content := "# Epic Context\n\nThis is test content."

	// Save
	if err := s.Save("abc", content); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists with correct permissions
	filename := filepath.Join(dir, "abc.md")
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		t.Fatal("context file was not created")
	}
	if info.Mode().Perm() != 0644 {
		t.Errorf("file permissions = %o, want %o", info.Mode().Perm(), 0644)
	}

	// Load
	loaded, err := s.Load("abc")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded != content {
		t.Errorf("Load() = %q, want %q", loaded, content)
	}
}

func TestStore_Save_EmptyID(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	err := s.Save("", "content")
	if err == nil {
		t.Fatal("Save() should error on empty ID")
	}
}

func TestStore_Save_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "nested", "context")
	s := NewStoreWithDir(nestedDir)

	content := "test content"

	if err := s.Save("abc", content); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify nested directory was created
	if _, err := os.Stat(nestedDir); os.IsNotExist(err) {
		t.Error("nested directory was not created")
	}

	// Verify file exists
	filename := filepath.Join(nestedDir, "abc.md")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Error("context file was not created")
	}
}

func TestStore_Save_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Save initial content
	if err := s.Save("abc", "initial content"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Save updated content (should be atomic)
	if err := s.Save("abc", "updated content"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify no temp file left behind
	tempFile := filepath.Join(dir, "abc.md.tmp")
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful save")
	}

	// Verify content was updated
	loaded, err := s.Load("abc")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded != "updated content" {
		t.Errorf("Load() = %q, want %q", loaded, "updated content")
	}
}

func TestStore_Load_NotExists(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Load non-existent file should return empty string, not error
	loaded, err := s.Load("nonexistent")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if loaded != "" {
		t.Errorf("Load() = %q, want empty string", loaded)
	}
}

func TestStore_Load_EmptyID(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	_, err := s.Load("")
	if err == nil {
		t.Fatal("Load() should error on empty ID")
	}
}

func TestStore_Load_DirectoryNotExists(t *testing.T) {
	s := NewStoreWithDir("/nonexistent/path/context")

	// Should return empty string, not error
	loaded, err := s.Load("abc")
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if loaded != "" {
		t.Errorf("Load() = %q, want empty string", loaded)
	}
}

func TestStore_Exists(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Should not exist initially
	if s.Exists("abc") {
		t.Error("Exists() = true, want false for non-existent")
	}

	// Save content
	if err := s.Save("abc", "content"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Should exist now
	if !s.Exists("abc") {
		t.Error("Exists() = false, want true after save")
	}
}

func TestStore_Exists_EmptyID(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	if s.Exists("") {
		t.Error("Exists() = true, want false for empty ID")
	}
}

func TestStore_Delete(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Save content
	if err := s.Save("abc", "content"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify exists
	if !s.Exists("abc") {
		t.Fatal("context should exist after save")
	}

	// Delete
	if err := s.Delete("abc"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	if s.Exists("abc") {
		t.Error("context should not exist after delete")
	}
}

func TestStore_Delete_Nonexistent(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Should not error on nonexistent (idempotent)
	if err := s.Delete("nonexistent"); err != nil {
		t.Errorf("Delete() error = %v, want nil", err)
	}
}

func TestStore_Delete_EmptyID(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	err := s.Delete("")
	if err == nil {
		t.Fatal("Delete() should error on empty ID")
	}
}

func TestStore_MultipleEpics(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Save multiple epics
	epics := map[string]string{
		"abc": "# Context for abc\n\nContent A",
		"def": "# Context for def\n\nContent B",
		"xyz": "# Context for xyz\n\nContent C",
	}

	for id, content := range epics {
		if err := s.Save(id, content); err != nil {
			t.Fatalf("Save(%s) error = %v", id, err)
		}
	}

	// Verify all exist and have correct content
	for id, want := range epics {
		if !s.Exists(id) {
			t.Errorf("Exists(%s) = false, want true", id)
		}

		got, err := s.Load(id)
		if err != nil {
			t.Errorf("Load(%s) error = %v", id, err)
		}
		if got != want {
			t.Errorf("Load(%s) = %q, want %q", id, got, want)
		}
	}

	// Delete one and verify others remain
	if err := s.Delete("def"); err != nil {
		t.Fatalf("Delete(def) error = %v", err)
	}

	if s.Exists("def") {
		t.Error("def should not exist after delete")
	}
	if !s.Exists("abc") {
		t.Error("abc should still exist")
	}
	if !s.Exists("xyz") {
		t.Error("xyz should still exist")
	}
}

func TestStore_LargeContent(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	// Create content larger than typical (simulating ~4000 token context doc)
	content := "# Epic Context\n\n"
	for i := 0; i < 1000; i++ {
		content += "This is line number " + string(rune('0'+i%10)) + " of the context document.\n"
	}

	if err := s.Save("large", content); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := s.Load("large")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded != content {
		t.Error("Large content was not preserved correctly")
	}
}

func TestStore_SpecialCharactersInContent(t *testing.T) {
	dir := t.TempDir()
	s := NewStoreWithDir(dir)

	content := "# Epic Context\n\n" +
		"Special chars: <>&\"'\n" +
		"Unicode: 你好世界 🚀\n" +
		"Code block:\n```go\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n```\n"

	if err := s.Save("special", content); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loaded, err := s.Load("special")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded != content {
		t.Errorf("Content with special characters was not preserved.\nGot: %q\nWant: %q", loaded, content)
	}
}
