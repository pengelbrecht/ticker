package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2ETestFileExists(t *testing.T) {
	// Find the testdata directory relative to the project root
	testdataPath := filepath.Join("..", "..", "testdata", "e2e_test.txt")

	if _, err := os.Stat(testdataPath); os.IsNotExist(err) {
		t.Fatalf("testdata/e2e_test.txt does not exist at %s", testdataPath)
	}
}

func TestE2ETestFileContent(t *testing.T) {
	testdataPath := filepath.Join("..", "..", "testdata", "e2e_test.txt")

	content, err := os.ReadFile(testdataPath)
	if err != nil {
		t.Fatalf("failed to read testdata/e2e_test.txt: %v", err)
	}

	contentStr := string(content)

	// Verify it contains 'E2E Test File'
	if !strings.Contains(contentStr, "E2E Test File") {
		t.Errorf("testdata/e2e_test.txt should contain 'E2E Test File', got: %q", contentStr)
	}

	// Verify it contains 'Hello from E2E test'
	if !strings.Contains(contentStr, "Hello from E2E test") {
		t.Errorf("testdata/e2e_test.txt should contain 'Hello from E2E test', got: %q", contentStr)
	}
}
