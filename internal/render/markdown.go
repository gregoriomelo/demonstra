package render

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/demonstra-me/demonstra/internal/collector"
	"github.com/demonstra-me/demonstra/internal/github"
)

const MaxCommentLength = 65000

type RenderInput struct {
	PRNumber     int
	BranchName   string
	CommitSHA    string
	AssetBranch  string
	Scenarios    []github.UploadedScenario
	PreviousRuns []github.PreviousRun
}

func FormatComment(in RenderInput) string {
	out := renderWithOptions(in, true, len(in.Scenarios))
	if len(out) <= MaxCommentLength {
		return out
	}

	// Step 1 degradation: omit inline step screenshot blocks
	out = renderWithOptions(in, false, len(in.Scenarios))
	if len(out) <= MaxCommentLength {
		return out
	}

	// Step 2 degradation: reduce number of rendered scenario cards to fit MaxCommentLength
	for limit := len(in.Scenarios) - 1; limit >= 0; limit-- {
		candidate := renderWithOptions(in, false, limit)
		if len(candidate) <= MaxCommentLength {
			return candidate
		}
	}
	return out[:MaxCommentLength]
}

func renderWithOptions(in RenderInput, includeScreenshots bool, maxCards int) string {
	shortSHA := github.ShortCommitSHA(in.CommitSHA)
	assetBranch := in.AssetBranch
	if assetBranch == "" {
		assetBranch = "demonstra-assets"
	}

	var b strings.Builder
	b.WriteString(github.CommentMarker + "\n")
	if in.BranchName != "" {
		fmt.Fprintf(&b, "## 🎬 demonstra.me Visual Review — `%s`\n\n", escapeCell(in.BranchName))
	} else {
		b.WriteString("## 🎬 demonstra.me Visual Review\n\n")
	}

	fmt.Fprintf(&b, "Commit `%s` • Stored in `%s/pr-%d`\n\n", shortSHA, assetBranch, in.PRNumber)

	// Summary Table
	b.WriteString("| # | Scenario | Spec File | Status | Duration |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for i, s := range in.Scenarios {
		specCell := "—"
		if s.Scenario.SpecFile != "" {
			if s.Scenario.Line > 0 {
				specCell = fmt.Sprintf("`%s:%d`", escapeCell(s.Scenario.SpecFile), s.Scenario.Line)
			} else {
				specCell = fmt.Sprintf("`%s`", escapeCell(s.Scenario.SpecFile))
			}
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			i+1,
			escapeCell(s.Scenario.Title),
			specCell,
			formatStatus(s.Scenario.Status),
			formatDuration(s.Scenario.Duration),
		)
	}
	b.WriteString("\n")

	// Scenario Cards
	for i, s := range in.Scenarios {
		if i >= maxCards {
			fmt.Fprintf(&b, "_… %d additional scenarios stored in `%s/pr-%d/%s`_\n\n", len(in.Scenarios)-maxCards, assetBranch, in.PRNumber, shortSHA)
			break
		}
		openAttr := ""
		if s.Scenario.Status == collector.StatusFailed || i < 3 {
			openAttr = " open"
		}
		durSuffix := ""
		if s.Scenario.Duration > 0 {
			durSuffix = fmt.Sprintf(" (%s)", formatDuration(s.Scenario.Duration))
		}
		fmt.Fprintf(&b, "<details%s>\n<summary><strong>Scenario %d: %s%s</strong></summary>\n\n",
			openAttr,
			i+1,
			escapeHTML(s.Scenario.Title),
			durSuffix,
		)
		animURL := s.WebPURL
		if animURL == "" {
			animURL = s.GIFURL
		}

		if animURL != "" {
			if s.VideoURL != "" {
				fmt.Fprintf(&b, "[![%s](%s)](%s)\n\n", escapeCell(s.Scenario.Title), animURL, s.VideoURL)
				fmt.Fprintf(&b, "🎬 [▶️ **Watch with Controls & Timeline Scrubbing (MP4)**](%s)\n\n", s.VideoURL)
			} else {
				fmt.Fprintf(&b, "![%s](%s)\n\n", escapeCell(s.Scenario.Title), animURL)
			}
		} else if s.VideoURL != "" {
			fmt.Fprintf(&b, "🎬 [▶️ **Watch Scenario Video (MP4)**](%s)\n\n", s.VideoURL)
		}
		if includeScreenshots && len(s.ScreenshotURLs) > 0 {
			fmt.Fprintf(&b, "<details><summary>View %d Step Screenshots</summary>\n\n", len(s.ScreenshotURLs))
			for idx, u := range s.ScreenshotURLs {
				fmt.Fprintf(&b, "![Step %d](%s)\n\n", idx+1, u)
			}
			b.WriteString("</details>\n\n")
		}
		b.WriteString("</details>\n\n")
	}

	if len(in.PreviousRuns) > 0 {
		fmt.Fprintf(&b, "<details><summary>Previous runs on this PR (%d)</summary>\n\n", len(in.PreviousRuns))
		for _, pr := range in.PreviousRuns {
			fmt.Fprintf(&b, "- [`%s`](%s)\n", pr.ShortSHA, pr.TreeURL)
		}
		b.WriteString("\n</details>\n")
	}

	return b.String()
}

func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.TrimSpace(s)
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return strings.TrimSpace(s)
}

func formatStatus(st collector.Status) string {
	switch st {
	case collector.StatusFailed:
		return "✗ Failed"
	case collector.StatusSkipped:
		return "○ Skipped"
	default:
		return "✓ Passed"
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func WriteStepSummary(summaryPath string, markdown string) error {
	if summaryPath == "" {
		return nil
	}
	f, err := os.OpenFile(summaryPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(markdown)
	return err
}
