package collector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscover_FromPlaywrightJSONReport(t *testing.T) {
	resultsDir := t.TempDir()

	scen1Dir := filepath.Join(resultsDir, "checkout-coupon")
	if err := os.MkdirAll(scen1Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	video1 := filepath.Join(scen1Dir, "video.webm")
	shot1 := filepath.Join(scen1Dir, "step-1.png")
	shot2 := filepath.Join(scen1Dir, "step-2.png")
	for _, p := range []string{video1, shot1, shot2} {
		if err := os.WriteFile(p, []byte("fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	scen2Dir := filepath.Join(resultsDir, "checkout-expired")
	if err := os.MkdirAll(scen2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	video2 := filepath.Join(scen2Dir, "video.webm")
	if err := os.WriteFile(video2, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}

	reportPath := filepath.Join(resultsDir, "results.json")
	reportJSON := fmt.Sprintf(`{
  "suites": [
    {
      "title": "e2e/checkout.spec.ts",
      "file": "e2e/checkout.spec.ts",
      "suites": [
        {
          "title": "Checkout Flow",
          "file": "e2e/checkout.spec.ts",
          "specs": [
            {
              "title": "Apply 20%% coupon | checkout/cart",
              "file": "e2e/checkout.spec.ts",
              "line": 14,
              "tests": [
                {
                  "results": [
                    {
                      "status": "passed",
                      "duration": 6400,
                      "attachments": [
                        {"name": "video", "contentType": "video/webm", "path": %q},
                        {"name": "step-1", "contentType": "image/png", "path": %q},
                        {"name": "step-2", "contentType": "image/png", "path": %q}
                      ]
                    }
                  ]
                }
              ]
            },
            {
              "title": "Expired coupon error",
              "file": "e2e/checkout.spec.ts",
              "line": 42,
              "tests": [
                {
                  "results": [
                    {
                      "status": "unexpected",
                      "duration": 4100,
                      "attachments": [
                        {"name": "video", "contentType": "video/webm", "path": %q}
                      ]
                    }
                  ]
                }
              ]
            }
          ]
        }
      ]
    }
  ]
}`, video1, shot1, shot2, video2)

	if err := os.WriteFile(reportPath, []byte(reportJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	scenarios, err := Discover(resultsDir, reportPath)
	if err != nil {
		t.Fatalf("Discover returned unexpected error: %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}

	s0 := scenarios[0]
	if s0.Title != "Apply 20% coupon | checkout/cart" {
		t.Errorf("expected title %q, got %q", "Apply 20% coupon | checkout/cart", s0.Title)
	}
	if s0.Slug != "apply-20-coupon-checkout-cart" {
		t.Errorf("expected slug %q, got %q", "apply-20-coupon-checkout-cart", s0.Slug)
	}
	if s0.SpecFile != "e2e/checkout.spec.ts" || s0.Line != 14 {
		t.Errorf("expected spec e2e/checkout.spec.ts:14, got %s:%d", s0.SpecFile, s0.Line)
	}
	if s0.Status != StatusPassed {
		t.Errorf("expected StatusPassed, got %s", s0.Status)
	}
	if s0.Duration != 6400*time.Millisecond {
		t.Errorf("expected duration 6400ms, got %v", s0.Duration)
	}
	if !strings.HasSuffix(s0.VideoPath, "video.webm") {
		t.Errorf("expected VideoPath ending with video.webm, got %s", s0.VideoPath)
	}
	if len(s0.Screenshots) != 2 {
		t.Errorf("expected 2 screenshots, got %d", len(s0.Screenshots))
	}

	s1 := scenarios[1]
	if s1.Status != StatusFailed {
		t.Errorf("expected StatusFailed for unexpected result, got %s", s1.Status)
	}
}

func TestDiscover_FallbackDirectoryScan(t *testing.T) {
	resultsDir := t.TempDir()

	dir1 := filepath.Join(resultsDir, "a-checkout-Apply-valid-coupon-chromium")
	dir2 := filepath.Join(resultsDir, "b-checkout-Expired-coupon-chromium")
	for _, d := range []string{dir1, dir2} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if err := os.WriteFile(filepath.Join(dir1, "video.webm"), []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir1, "test-failed-1.png"), []byte("png1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "recording.mp4"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}

	scenarios, err := Discover(resultsDir, "")
	if err != nil {
		t.Fatalf("Discover fallback failed: %v", err)
	}
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 scenarios, got %d", len(scenarios))
	}
	if scenarios[0].Status != StatusFailed {
		t.Errorf("expected first scenario to be StatusFailed due to test-failed-1.png, got %s", scenarios[0].Status)
	}
	if len(scenarios[0].Screenshots) != 1 {
		t.Errorf("expected 1 screenshot in first scenario, got %d", len(scenarios[0].Screenshots))
	}
	if scenarios[1].Status != StatusPassed {
		t.Errorf("expected second scenario to be StatusPassed, got %s", scenarios[1].Status)
	}
	if !strings.HasSuffix(scenarios[1].VideoPath, "recording.mp4") {
		t.Errorf("expected recording.mp4, got %s", scenarios[1].VideoPath)
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("  Apply $20 Coupon / Cart! "); got != "apply-20-coupon-cart" {
		t.Errorf("expected apply-20-coupon-cart, got %q", got)
	}
	if got := Slugify("!!!"); got != "scenario" {
		t.Errorf("expected scenario, got %q", got)
	}
}
