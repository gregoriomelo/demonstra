package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectFramework(t *testing.T) {
	t.Run("detects Playwright when playwright.config.ts exists", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "playwright.config.ts"), []byte("// config"), 0o644); err != nil {
			t.Fatal(err)
		}
		fw := Detect(tmp)
		if fw != FrameworkPlaywright {
			t.Errorf("expected %s, got %s", FrameworkPlaywright, fw)
		}
	})

	t.Run("detects Playwright when playwright.config.js exists", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "playwright.config.js"), []byte("// config"), 0o644); err != nil {
			t.Fatal(err)
		}
		fw := Detect(tmp)
		if fw != FrameworkPlaywright {
			t.Errorf("expected %s, got %s", FrameworkPlaywright, fw)
		}
	})

	t.Run("detects Cypress when cypress.config.js exists", func(t *testing.T) {
		tmp := t.TempDir()
		if err := os.WriteFile(filepath.Join(tmp, "cypress.config.js"), []byte("// config"), 0o644); err != nil {
			t.Fatal(err)
		}
		fw := Detect(tmp)
		if fw != FrameworkCypress {
			t.Errorf("expected %s, got %s", FrameworkCypress, fw)
		}
	})

	t.Run("falls back to generic when unknown", func(t *testing.T) {
		tmp := t.TempDir()
		fw := Detect(tmp)
		if fw != FrameworkGeneric {
			t.Errorf("expected %s, got %s", FrameworkGeneric, fw)
		}
	})
}

func TestInit_ScaffoldsPlaywrightFiles(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "playwright.config.js"), []byte("// pw"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Init(InitOptions{
		TargetDir: tmp,
		Force:     false,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	if res.Framework != FrameworkPlaywright {
		t.Errorf("expected framework %s, got %s", FrameworkPlaywright, res.Framework)
	}

	workflowPath := filepath.Join(tmp, ".github", "workflows", "demonstra.yml")
	workflowData, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("expected workflow file to be created: %v", err)
	}
	if !strings.Contains(string(workflowData), "uses: demonstra-me/demonstra@v1") {
		t.Errorf("workflow should dogfood official composite action: %s", string(workflowData))
	}

	interactPath := filepath.Join(tmp, "e2e", "support", "interact.js")
	interactData, err := os.ReadFile(interactPath)
	if err != nil {
		t.Fatalf("expected interact.js to be created: %v", err)
	}
	if !strings.Contains(string(interactData), "__demonstra_radar") || !strings.Contains(string(interactData), "__demonstra_hud") {
		t.Errorf("interact.js missing radar or hud components: %s", string(interactData))
	}
	if !strings.Contains(string(interactData), "escapeHTML") {
		t.Errorf("interact.js missing HTML escaping protection: %s", string(interactData))
	}
}

func TestInit_CypressDoesNotInstallPlaywright(t *testing.T) {
	tmp := t.TempDir()
	res, err := Init(InitOptions{
		TargetDir: tmp,
		Framework: FrameworkCypress,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if res.Framework != FrameworkCypress {
		t.Errorf("expected %s, got %s", FrameworkCypress, res.Framework)
	}

	workflowData, err := os.ReadFile(filepath.Join(tmp, ".github", "workflows", "demonstra.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(workflowData), "playwright install") {
		t.Errorf("Cypress workflow should not contain playwright install: %s", string(workflowData))
	}
	if !strings.Contains(string(workflowData), "npx cypress run") {
		t.Errorf("Cypress workflow missing cypress run command: %s", string(workflowData))
	}
}

func TestInit_InvalidFrameworkError(t *testing.T) {
	tmp := t.TempDir()
	_, err := Init(InitOptions{
		TargetDir: tmp,
		Framework: "unsupported-fw",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported framework") {
		t.Fatalf("expected unsupported framework error, got %v", err)
	}
}

func TestInit_ForceProtection(t *testing.T) {
	tmp := t.TempDir()
	workflowDir := filepath.Join(tmp, ".github", "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workflowFile := filepath.Join(workflowDir, "demonstra.yml")
	if err := os.WriteFile(workflowFile, []byte("existing-workflow"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Without Force: should skip existing file
	res, err := Init(InitOptions{
		TargetDir: tmp,
		Framework: FrameworkPlaywright,
		Force:     false,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if len(res.SkippedFiles) == 0 {
		t.Errorf("expected demonstra.yml to be skipped without force")
	}
	content, _ := os.ReadFile(workflowFile)
	if string(content) != "existing-workflow" {
		t.Errorf("existing workflow overwritten without force")
	}

	// With Force: should overwrite
	resForce, err := Init(InitOptions{
		TargetDir: tmp,
		Framework: FrameworkPlaywright,
		Force:     true,
	})
	if err != nil {
		t.Fatalf("Init with force failed: %v", err)
	}
	if len(resForce.CreatedFiles) == 0 {
		t.Errorf("expected files to be created/overwritten with force")
	}
	newContent, _ := os.ReadFile(workflowFile)
	if string(newContent) == "existing-workflow" {
		t.Errorf("expected workflow to be overwritten with force")
	}
}
