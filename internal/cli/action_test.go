package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActionYML_SchemaAndDefaults(t *testing.T) {
	rootPath := filepath.Join("..", "..", "action.yml")
	data, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("failed to read action.yml: %v", err)
	}

	content := string(data)
	requiredKeys := []string{
		"name: 'Demonstra Visual PR Review'",
		"using: 'composite'",
		"command:",
		"results-dir:",
		"github-token:",
		"branch:",
		"strict:",
		"version:",
		"Validate GitHub Token",
		"Ensure FFmpeg is installed",
		"Install Demonstra CLI",
		"Run Test Command and Publish Visual PR Review",
	}

	for _, k := range requiredKeys {
		if !strings.Contains(content, k) {
			t.Errorf("action.yml missing expected section %q", k)
		}
	}
}
