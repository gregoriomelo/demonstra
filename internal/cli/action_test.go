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

func TestActionYML_UsesEnvVarsForInputs(t *testing.T) {
	rootPath := filepath.Join("..", "..", "action.yml")
	data, err := os.ReadFile(rootPath)
	if err != nil {
		t.Fatalf("failed to read action.yml: %v", err)
	}

	content := string(data)
	requiredEnvVars := []string{
		"TEST_CMD: ${{ inputs.command }}",
		"RESULTS_DIR: ${{ inputs.results-dir }}",
		"REPORT_FILE: ${{ inputs.report-file }}",
		"BRANCH: ${{ inputs.branch }}",
		"STRICT: ${{ inputs.strict }}",
		"GITHUB_TOKEN: ${{ inputs.github-token }}",
	}

	for _, ev := range requiredEnvVars {
		if !strings.Contains(content, ev) {
			t.Errorf("action.yml missing env mapping %q", ev)
		}
	}

	runIdx := strings.Index(content, "Run Test Command and Publish Visual PR Review")
	if runIdx == -1 {
		t.Fatalf("missing run step")
	}
	runSection := content[runIdx:]
	runBlockIndex := strings.Index(runSection, "run: |")
	if runBlockIndex != -1 {
		runBlock := runSection[runBlockIndex:]
		if strings.Contains(runBlock, "${{ inputs.") {
			t.Errorf("action.yml run block should not directly interpolate ${{ inputs.* }}, use env vars instead")
		}
	}

	// Composite actions do not have access to 'secrets' context
	if strings.Contains(content, "secrets.") {
		t.Errorf("action.yml should not reference 'secrets.' context directly")
	}
}

