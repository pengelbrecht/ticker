package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNew(t *testing.T) {
	cfg := Config{
		EpicID:       "test-123",
		EpicTitle:    "Test Epic",
		MaxCost:      20.0,
		MaxIteration: 50,
	}

	m := New(cfg)

	if m.epicID != "test-123" {
		t.Errorf("expected epicID 'test-123', got '%s'", m.epicID)
	}
	if m.epicTitle != "Test Epic" {
		t.Errorf("expected epicTitle 'Test Epic', got '%s'", m.epicTitle)
	}
	if m.maxCost != 20.0 {
		t.Errorf("expected maxCost 20.0, got %f", m.maxCost)
	}
	if m.maxIter != 50 {
		t.Errorf("expected maxIter 50, got %d", m.maxIter)
	}
	if !m.running {
		t.Error("expected running to be true")
	}
}

func TestInit(t *testing.T) {
	m := New(Config{EpicID: "test"})
	cmd := m.Init()
	if cmd != nil {
		t.Error("expected Init() to return nil")
	}
}

func TestUpdateQuit(t *testing.T) {
	m := New(Config{EpicID: "test"})

	// Test q key
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model := newModel.(Model)
	if !model.quitting {
		t.Error("expected quitting to be true after 'q' key")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestUpdateCtrlC(t *testing.T) {
	m := New(Config{EpicID: "test"})

	// Test ctrl+c
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := newModel.(Model)
	if !model.quitting {
		t.Error("expected quitting to be true after ctrl+c")
	}
	if cmd == nil {
		t.Error("expected quit command")
	}
}

func TestUpdateWindowSize(t *testing.T) {
	m := New(Config{EpicID: "test"})

	newModel, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := newModel.(Model)

	if model.width != 120 {
		t.Errorf("expected width 120, got %d", model.width)
	}
	if model.height != 40 {
		t.Errorf("expected height 40, got %d", model.height)
	}
}

func TestUpdateIterationStart(t *testing.T) {
	m := New(Config{EpicID: "test"})

	newModel, _ := m.Update(IterationStartMsg{
		Iteration: 5,
		TaskID:    "task-1",
		TaskTitle: "Do something",
	})
	model := newModel.(Model)

	if model.iteration != 5 {
		t.Errorf("expected iteration 5, got %d", model.iteration)
	}
	if model.taskID != "task-1" {
		t.Errorf("expected taskID 'task-1', got '%s'", model.taskID)
	}
	if model.taskTitle != "Do something" {
		t.Errorf("expected taskTitle 'Do something', got '%s'", model.taskTitle)
	}
}

func TestUpdateIterationEnd(t *testing.T) {
	m := New(Config{EpicID: "test"})

	newModel, _ := m.Update(IterationEndMsg{
		Iteration: 3,
		Cost:      0.05,
		Tokens:    1000,
	})
	model := newModel.(Model)

	if model.iterations != 3 {
		t.Errorf("expected iterations 3, got %d", model.iterations)
	}
	if model.cost != 0.05 {
		t.Errorf("expected cost 0.05, got %f", model.cost)
	}
	if model.tokens != 1000 {
		t.Errorf("expected tokens 1000, got %d", model.tokens)
	}
}

func TestUpdateOutput(t *testing.T) {
	m := New(Config{EpicID: "test"})

	newModel, _ := m.Update(OutputMsg("Hello "))
	model := newModel.(Model)

	if model.output != "Hello " {
		t.Errorf("expected output 'Hello ', got '%s'", model.output)
	}

	newModel2, _ := model.Update(OutputMsg("World"))
	model2 := newModel2.(Model)

	if model2.output != "Hello World" {
		t.Errorf("expected output 'Hello World', got '%s'", model2.output)
	}
}

func TestUpdateRunComplete(t *testing.T) {
	m := New(Config{EpicID: "test"})

	newModel, _ := m.Update(RunCompleteMsg{Reason: "done"})
	model := newModel.(Model)

	if model.running {
		t.Error("expected running to be false after RunCompleteMsg")
	}
}

func TestView(t *testing.T) {
	m := New(Config{
		EpicID:       "test-123",
		EpicTitle:    "Test Epic",
		MaxCost:      20.0,
		MaxIteration: 50,
	})

	view := m.View()

	// Just verify it returns something non-empty
	if len(view) == 0 {
		t.Error("expected non-empty view")
	}
}

func TestViewQuitting(t *testing.T) {
	m := New(Config{EpicID: "test"})
	m.quitting = true

	view := m.View()

	if view != "Goodbye!\n" {
		t.Errorf("expected 'Goodbye!\\n', got '%s'", view)
	}
}

func TestTaskItem(t *testing.T) {
	item := taskItem{
		id:     "abc",
		title:  "Test task",
		status: "open",
	}

	if item.Title() != "[abc] Test task" {
		t.Errorf("expected '[abc] Test task', got '%s'", item.Title())
	}
	if item.Description() != "open" {
		t.Errorf("expected 'open', got '%s'", item.Description())
	}
	if item.FilterValue() != "Test task" {
		t.Errorf("expected 'Test task', got '%s'", item.FilterValue())
	}
}
