package scaffold

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed templates/interact.js
var interactJSTemplate string

//go:embed templates/demonstra.yml
var workflowTemplate string

type Framework string

const (
	FrameworkPlaywright Framework = "playwright"
	FrameworkCypress    Framework = "cypress"
	FrameworkGeneric    Framework = "generic"
)

type InitOptions struct {
	TargetDir string
	Framework Framework
	Force     bool
}

type InitResult struct {
	Framework    Framework
	CreatedFiles []string
	SkippedFiles []string
}

func Detect(rootDir string) Framework {
	pwFiles := []string{"playwright.config.ts", "playwright.config.js", "playwright.config.mjs"}
	for _, f := range pwFiles {
		if _, err := os.Stat(filepath.Join(rootDir, f)); err == nil {
			return FrameworkPlaywright
		}
	}

	cyFiles := []string{"cypress.config.ts", "cypress.config.js", "cypress.config.mjs"}
	for _, f := range cyFiles {
		if _, err := os.Stat(filepath.Join(rootDir, f)); err == nil {
			return FrameworkCypress
		}
	}

	return FrameworkGeneric
}

func Init(opts InitOptions) (InitResult, error) {
	targetDir := opts.TargetDir
	if targetDir == "" {
		targetDir = "."
	}

	fw := opts.Framework
	if fw == "" {
		fw = Detect(targetDir)
	}

	if fw != FrameworkPlaywright && fw != FrameworkCypress && fw != FrameworkGeneric {
		return InitResult{}, fmt.Errorf("unsupported framework %q (expected 'playwright', 'cypress', or 'generic')", fw)
	}

	res := InitResult{
		Framework: fw,
	}

	filesToCreate := map[string]string{
		filepath.Join(".github", "workflows", "demonstra.yml"): renderWorkflow(fw),
	}

	if fw == FrameworkPlaywright {
		filesToCreate[filepath.Join("e2e", "support", "interact.js")] = interactJSTemplate
	}

	keys := make([]string, 0, len(filesToCreate))
	for k := range filesToCreate {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, relPath := range keys {
		content := filesToCreate[relPath]
		fullPath := filepath.Join(targetDir, relPath)
		if _, err := os.Stat(fullPath); err == nil && !opts.Force {
			res.SkippedFiles = append(res.SkippedFiles, relPath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return res, fmt.Errorf("create directory for %s: %w", relPath, err)
		}

		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			return res, fmt.Errorf("write %s: %w", relPath, err)
		}
		res.CreatedFiles = append(res.CreatedFiles, relPath)
	}

	return res, nil
}

func renderWorkflow(fw Framework) string {
	testCmd := "npx playwright test"
	setupSteps := ""

	switch fw {
	case FrameworkPlaywright:
		testCmd = "npx playwright test"
		setupSteps = "\n      - name: Install test browsers\n        run: npx playwright install --with-deps chromium"
	case FrameworkCypress:
		testCmd = "npx cypress run"
	case FrameworkGeneric:
		testCmd = "npm test # TODO: replace with your E2E test command"
	}

	content := strings.ReplaceAll(workflowTemplate, "{{TEST_COMMAND}}", testCmd)
	content = strings.ReplaceAll(content, "{{SETUP_STEPS}}", setupSteps)
	return content
}
