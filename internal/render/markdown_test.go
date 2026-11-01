package render

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/demonstra-me/demonstra/internal/collector"
	"github.com/demonstra-me/demonstra/internal/github"
)

func TestFormatComment_LayoutA1SmartExpansion(t *testing.T) {
	in := RenderInput{
		PRNumber:    42,
		BranchName:  "feat/checkout-coupon",
		CommitSHA:   "a8f921c99999",
		AssetBranch: "demonstra-assets",
		Scenarios: []github.UploadedScenario{
			{
				Scenario: collector.Scenario{
					Title:    "Scenario One",
					Slug:     "scenario-one",
					SpecFile: "e2e/checkout.spec.ts",
					Line:     14,
					Status:   collector.StatusPassed,
					Duration: 6400 * time.Millisecond,
				},
				GIFURL:         "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/scenario-one/flow.gif?raw=true",
				ScreenshotURLs: []string{"https://example.com/s1.png", "https://example.com/s2.png"},
			},
			{
				Scenario: collector.Scenario{
					Title:    "Scenario Two",
					Slug:     "scenario-two",
					SpecFile: "e2e/checkout.spec.ts",
					Line:     28,
					Status:   collector.StatusPassed,
					Duration: 4100 * time.Millisecond,
				},
				GIFURL: "https://example.com/gif2.gif",
			},
			{
				Scenario: collector.Scenario{
					Title:    "Scenario Three",
					Slug:     "scenario-three",
					SpecFile: "e2e/checkout.spec.ts",
					Line:     45,
					Status:   collector.StatusPassed,
					Duration: 3200 * time.Millisecond,
				},
				GIFURL: "https://example.com/gif3.gif",
			},
			{
				Scenario: collector.Scenario{
					Title:    "Scenario Four Collapsed",
					Slug:     "scenario-four",
					SpecFile: "e2e/checkout.spec.ts",
					Line:     60,
					Status:   collector.StatusPassed,
					Duration: 5000 * time.Millisecond,
				},
				GIFURL: "https://example.com/gif4.gif",
			},
			{
				Scenario: collector.Scenario{
					Title:    "Scenario Five Failed",
					Slug:     "scenario-five",
					SpecFile: "e2e/checkout.spec.ts",
					Line:     80,
					Status:   collector.StatusFailed,
					Duration: 2500 * time.Millisecond,
				},
				GIFURL: "https://example.com/gif5.gif",
			},
		},
		PreviousRuns: []github.PreviousRun{
			{
				ShortSHA: "1111111",
				TreeURL:  "https://github.com/acme/shop/tree/demonstra-assets/pr-42/1111111",
			},
		},
	}

	got := FormatComment(in)

	if !strings.HasPrefix(got, github.CommentMarker+"\n") {
		t.Errorf("expected comment to start with %q, got:\n%s", github.CommentMarker, got)
	}
	for i := 1; i <= 5; i++ {
		rowPrefix := fmt.Sprintf("| %d |", i)
		if !strings.Contains(got, rowPrefix) {
			t.Errorf("expected summary table row %q in output", rowPrefix)
		}
	}
	if !strings.Contains(got, "<details open>\n<summary><strong>Scenario 1: Scenario One") {
		t.Errorf("expected Scenario 1 to use <details open>, got:\n%s", got)
	}
	if !strings.Contains(got, "<details open>\n<summary><strong>Scenario 2: Scenario Two") {
		t.Errorf("expected Scenario 2 to use <details open>")
	}
	if !strings.Contains(got, "<details open>\n<summary><strong>Scenario 3: Scenario Three") {
		t.Errorf("expected Scenario 3 to use <details open>")
	}
	if !strings.Contains(got, "<details>\n<summary><strong>Scenario 4: Scenario Four Collapsed") {
		t.Errorf("expected Scenario 4 (index 3, passed) to use collapsed <details>, got:\n%s", got)
	}
	if !strings.Contains(got, "<details open>\n<summary><strong>Scenario 5: Scenario Five Failed") {
		t.Errorf("expected Scenario 5 (failed) to use <details open>, got:\n%s", got)
	}
	if !strings.Contains(got, "<details><summary>View 2 Step Screenshots</summary>") {
		t.Errorf("expected nested step screenshots block in Scenario 1")
	}
	if !strings.Contains(got, "<details><summary>Previous runs on this PR (1)</summary>") || !strings.Contains(got, "https://github.com/acme/shop/tree/demonstra-assets/pr-42/1111111") {
		t.Errorf("expected previous runs footer in output")
	}
}

func TestFormatComment_TableEscapingAndLengthGuardrail(t *testing.T) {
	var scenarios []github.UploadedScenario
	longPad := strings.Repeat("x", 4000)
	for i := 0; i < 25; i++ {
		scenarios = append(scenarios, github.UploadedScenario{
			Scenario: collector.Scenario{
				Title:    fmt.Sprintf("Checkout | Pipe\nNewline %d", i),
				Slug:     fmt.Sprintf("scen-%d", i),
				SpecFile: "e2e/spec.ts",
				Status:   collector.StatusPassed,
				Duration: time.Second,
			},
			GIFURL:         "https://example.com/" + longPad + ".gif",
			ScreenshotURLs: []string{"https://example.com/shot-" + longPad + ".png"},
		})
	}

	got := FormatComment(RenderInput{
		PRNumber:    1,
		CommitSHA:   "abcdef1",
		AssetBranch: "demonstra-assets",
		Scenarios:   scenarios,
	})

	if !strings.Contains(got, `Checkout \| Pipe Newline 0`) {
		t.Errorf("expected escaped pipe and newline in table cell, got:\n%s", got[:500])
	}
	if len(got) > MaxCommentLength {
		t.Errorf("expected rendered comment length <= %d, got %d", MaxCommentLength, len(got))
	}
}

func TestFormatComment_VideoControlsLink(t *testing.T) {
	in := RenderInput{
		PRNumber:    42,
		CommitSHA:   "a8f921c",
		AssetBranch: "demonstra-assets",
		Scenarios: []github.UploadedScenario{
			{
				Scenario: collector.Scenario{
					Title:    "Checkout flow",
					Slug:     "checkout-flow",
					Status:   collector.StatusPassed,
					Duration: 5 * time.Second,
				},
				GIFURL:   "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/checkout-flow/flow.gif?raw=true",
				VideoURL: "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/checkout-flow/video.mp4",
			},
		},
	}
	got := FormatComment(in)

	wantClickableGIF := "[![Checkout flow](https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/checkout-flow/flow.gif?raw=true)](https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/checkout-flow/video.mp4)"
	if !strings.Contains(got, wantClickableGIF) {
		t.Errorf("expected clickable GIF link in comment, got:\n%s", got)
	}

	wantVideoLink := "🎬 [▶️ **Watch with Controls & Timeline Scrubbing (MP4)**](https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/checkout-flow/video.mp4)"
	if !strings.Contains(got, wantVideoLink) {
		t.Errorf("expected video controls link in comment, got:\n%s", got)
	}
}

func TestWriteStepSummary(t *testing.T) {
	tmp := t.TempDir()
	summaryFile := filepath.Join(tmp, "step_summary.md")
	content := "## Summary\nHello world\n"
	if err := WriteStepSummary(summaryFile, content); err != nil {
		t.Fatalf("WriteStepSummary failed: %v", err)
	}
	data, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Errorf("expected %q, got %q", content, string(data))
	}
}
