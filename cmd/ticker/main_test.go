package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestFlagParsing tests that the CLI flags are correctly defined and parsed.
func TestFlagParsing(t *testing.T) {
	// Test that skip-verify flag is registered
	flag := runCmd.Flags().Lookup("skip-verify")
	if flag == nil {
		t.Fatal("--skip-verify flag not registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--skip-verify default value = %q, want %q", flag.DefValue, "false")
	}

	// Test that verify-only flag is registered
	flag = runCmd.Flags().Lookup("verify-only")
	if flag == nil {
		t.Fatal("--verify-only flag not registered")
	}
	if flag.DefValue != "false" {
		t.Errorf("--verify-only default value = %q, want %q", flag.DefValue, "false")
	}
}

// TestMutualExclusivity tests that --skip-verify and --verify-only are mutually exclusive.
// This requires building the binary and running it as a subprocess.
func TestMutualExclusivity(t *testing.T) {
	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "ticker")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/ticker")
	cmd.Dir = getProjectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	// Run with both flags
	cmd = exec.Command(binary, "run", "--skip-verify", "--verify-only")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Should exit with error code 4 (ExitError)
	if err == nil {
		t.Fatal("expected error when using both --skip-verify and --verify-only")
	}

	// Check error message
	if !bytes.Contains(stderr.Bytes(), []byte("mutually exclusive")) {
		t.Errorf("expected error message to mention 'mutually exclusive', got: %s", stderr.String())
	}
}

// TestVerifyOnlyWithoutEpic tests that --verify-only works without an epic ID.
func TestVerifyOnlyWithoutEpic(t *testing.T) {
	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "ticker")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/ticker")
	cmd.Dir = getProjectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	// Create a test git repo
	testDir := t.TempDir()
	runGit(t, testDir, "init")
	runGit(t, testDir, "config", "user.email", "test@test.com")
	runGit(t, testDir, "config", "user.name", "Test")

	// Create initial commit
	testFile := filepath.Join(testDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, testDir, "add", "test.txt")
	runGit(t, testDir, "commit", "-m", "initial")

	// Run verify-only in clean directory - should pass
	cmd = exec.Command(binary, "run", "--verify-only")
	cmd.Dir = testDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err != nil {
		t.Fatalf("verify-only failed on clean repo: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	// Should have output about verification
	if !bytes.Contains(stdout.Bytes(), []byte("verification")) && !bytes.Contains(stdout.Bytes(), []byte("Verification")) {
		t.Errorf("expected output to mention verification, got: %s", stdout.String())
	}
}

// TestVerifyOnlyWithUncommittedChanges tests that --verify-only fails with uncommitted changes.
func TestVerifyOnlyWithUncommittedChanges(t *testing.T) {
	// Build the binary
	tmpDir := t.TempDir()
	binary := filepath.Join(tmpDir, "ticker")
	cmd := exec.Command("go", "build", "-o", binary, "./cmd/ticker")
	cmd.Dir = getProjectRoot(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	// Create a test git repo
	testDir := t.TempDir()
	runGit(t, testDir, "init")
	runGit(t, testDir, "config", "user.email", "test@test.com")
	runGit(t, testDir, "config", "user.name", "Test")

	// Create initial commit
	testFile := filepath.Join(testDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, testDir, "add", "test.txt")
	runGit(t, testDir, "commit", "-m", "initial")

	// Create uncommitted change
	if err := os.WriteFile(testFile, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	// Run verify-only - should fail
	cmd = exec.Command(binary, "run", "--verify-only")
	cmd.Dir = testDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	if err == nil {
		t.Fatal("expected verify-only to fail with uncommitted changes")
	}

	// Should exit with code 1 (verification failure)
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 1 {
			t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
		}
	}

	// Should have output about failure
	if !bytes.Contains(stdout.Bytes(), []byte("failed")) && !bytes.Contains(stdout.Bytes(), []byte("FAIL")) {
		t.Errorf("expected output to mention failure, got: %s", stdout.String())
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// getProjectRoot returns the project root directory (two levels up from cmd/ticker)
func getProjectRoot(t *testing.T) string {
	t.Helper()
	wd := mustGetwd(t)
	return filepath.Join(wd, "..", "..")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
