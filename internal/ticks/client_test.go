package ticks

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient()
	if c.Command != "tk" {
		t.Errorf("expected Command to be 'tk', got %q", c.Command)
	}
}

func TestTaskIsOpen(t *testing.T) {
	task := &Task{Status: "open"}
	if !task.IsOpen() {
		t.Error("expected IsOpen() to return true for open status")
	}
	if task.IsClosed() {
		t.Error("expected IsClosed() to return false for open status")
	}
}

func TestTaskIsClosed(t *testing.T) {
	task := &Task{Status: "closed"}
	if task.IsOpen() {
		t.Error("expected IsOpen() to return false for closed status")
	}
	if !task.IsClosed() {
		t.Error("expected IsClosed() to return true for closed status")
	}
}

func TestEpicIsOpen(t *testing.T) {
	epic := &Epic{Status: "open"}
	if !epic.IsOpen() {
		t.Error("expected IsOpen() to return true for open status")
	}
	if epic.IsClosed() {
		t.Error("expected IsClosed() to return false for open status")
	}
}

func TestEpicIsClosed(t *testing.T) {
	epic := &Epic{Status: "closed"}
	if epic.IsOpen() {
		t.Error("expected IsOpen() to return false for closed status")
	}
	if !epic.IsClosed() {
		t.Error("expected IsClosed() to return true for closed status")
	}
}
