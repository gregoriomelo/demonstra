package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/demonstra-me/demonstra/internal/collector"
	"github.com/demonstra-me/demonstra/internal/github"
	"github.com/demonstra-me/demonstra/internal/media"
	"github.com/demonstra-me/demonstra/internal/render"
	"github.com/demonstra-me/demonstra/internal/scaffold"
)

type Env struct {
	LookupEnv func(string) string
	Stdout    io.Writer
	Stderr    io.Writer
}

type options struct {
	resultsDir string
	reportFile string
	repo       string
	prNumber   int
	sha        string
	token      string
	branch     string
	prBranch   string
	apiURL     string
	webURL     string
	strict     bool
}

func Execute(ctx context.Context, args []string, env Env) int {
	if env.LookupEnv == nil {
		env.LookupEnv = os.Getenv
	}
	if env.Stdout == nil {
		env.Stdout = os.Stdout
	}
	if env.Stderr == nil {
		env.Stderr = os.Stderr
	}

	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage: demonstra <init|record|publish> [flags] [-- command...]")
		return 2
	}

	subcmd := args[0]
	if subcmd == "init" {
		return runInitCommand(args[1:], env)
	}

	if subcmd != "record" && subcmd != "publish" {
		fmt.Fprintf(env.Stderr, "unknown subcommand %q (expected 'init', 'record', or 'publish')\n", subcmd)
		return 2
	}

	flagArgs := args[1:]
	var testCmdArgs []string
	for i, a := range flagArgs {
		if a == "--" {
			testCmdArgs = flagArgs[i+1:]
			flagArgs = flagArgs[:i]
			break
		}
	}

	opts, err := parseOptions(subcmd, flagArgs, env.LookupEnv)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error parsing flags: %v\n", err)
		return 2
	}

	testExitCode := 0
	if subcmd == "record" {
		if len(testCmdArgs) == 0 {
			fmt.Fprintln(env.Stderr, "error: 'demonstra record' requires a command after '--'")
			return 2
		}
		testExitCode = runTestCommand(ctx, testCmdArgs, env.Stdout, env.Stderr)
	}

	if err := runPublishPipeline(ctx, opts, env); err != nil {
		if opts.strict {
			fmt.Fprintf(env.Stderr, "error: %v\n", err)
			if testExitCode != 0 {
				return testExitCode
			}
			return 1
		}
		fmt.Fprintf(env.Stderr, "warning: demonstra publish failed: %v\n", err)
	}

	return testExitCode
}

func runInitCommand(flagArgs []string, env Env) int {
	fs := flag.NewFlagSet("demonstra init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var dir string
	var fw string
	var force bool
	fs.StringVar(&dir, "dir", ".", "Project root directory")
	fs.StringVar(&fw, "framework", "", "Override framework (playwright, cypress, generic)")
	fs.BoolVar(&force, "force", false, "Overwrite existing workflow and helper files")

	if err := fs.Parse(flagArgs); err != nil {
		fmt.Fprintf(env.Stderr, "error parsing flags: %v\n", err)
		return 2
	}

	res, err := scaffold.Init(scaffold.InitOptions{
		TargetDir: dir,
		Framework: scaffold.Framework(fw),
		Force:     force,
	})
	if err != nil {
		fmt.Fprintf(env.Stderr, "demonstra init error: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Stdout, "Demonstra initialized for %s project!\n", res.Framework)
	for _, f := range res.CreatedFiles {
		fmt.Fprintf(env.Stdout, "  ✓ Created %s\n", f)
	}
	for _, f := range res.SkippedFiles {
		fmt.Fprintf(env.Stdout, "  - Skipped %s (already exists, use --force to overwrite)\n", f)
	}
	return 0
}

func parseOptions(subcmd string, flagArgs []string, lookupEnv func(string) string) (options, error) {
	fs := flag.NewFlagSet("demonstra "+subcmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var opts options
	var shaFlag string
	fs.StringVar(&opts.resultsDir, "results-dir", "./test-results", "Directory containing test videos and screenshots")
	fs.StringVar(&opts.reportFile, "report-file", "", "Optional Playwright JSON report path")
	fs.StringVar(&opts.repo, "repo", lookupEnv("GITHUB_REPOSITORY"), "GitHub owner/repo")
	fs.IntVar(&opts.prNumber, "pr", 0, "Pull request number")
	fs.StringVar(&shaFlag, "sha", "", "Commit SHA")
	fs.StringVar(&opts.token, "token", lookupEnv("GITHUB_TOKEN"), "GitHub token")
	fs.StringVar(&opts.branch, "branch", "demonstra-assets", "Orphan branch for storing assets")
	fs.StringVar(&opts.prBranch, "pr-branch", lookupEnv("GITHUB_HEAD_REF"), "Pull request head branch name")
	fs.StringVar(&opts.apiURL, "api-url", "https://api.github.com", "GitHub REST API base URL")
	fs.StringVar(&opts.webURL, "web-url", "https://github.com", "GitHub web base URL")
	fs.BoolVar(&opts.strict, "strict", false, "Fail with non-zero exit code if visual publishing fails")

	if err := fs.Parse(flagArgs); err != nil {
		return opts, err
	}

	opts.sha = shaFlag
	if opts.prNumber == 0 {
		if prStr := lookupEnv("DEMONSTRA_PR_NUMBER"); prStr != "" {
			if n, err := strconv.Atoi(prStr); err == nil {
				opts.prNumber = n
			}
		}
	}
	if eventPath := lookupEnv("GITHUB_EVENT_PATH"); eventPath != "" {
		prNum, headRef, headSHA := parseGitHubEvent(eventPath)
		if opts.prNumber == 0 && prNum > 0 {
			opts.prNumber = prNum
		}
		if opts.prBranch == "" && headRef != "" {
			opts.prBranch = headRef
		}
		if opts.sha == "" && headSHA != "" {
			opts.sha = headSHA
		}
	}
	if opts.sha == "" {
		opts.sha = lookupEnv("GITHUB_SHA")
	}

	return opts, nil
}

func parseGitHubEvent(eventPath string) (int, string, string) {
	data, err := os.ReadFile(eventPath)
	if err != nil {
		return 0, "", ""
	}
	var payload struct {
		PullRequest struct {
			Number int `json:"number"`
			Head   struct {
				Ref string `json:"ref"`
				SHA string `json:"sha"`
			} `json:"head"`
		} `json:"pull_request"`
		Issue struct {
			Number int `json:"number"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, "", ""
	}
	if payload.PullRequest.Number > 0 {
		return payload.PullRequest.Number, payload.PullRequest.Head.Ref, payload.PullRequest.Head.SHA
	}
	return payload.Issue.Number, "", ""
}

func runTestCommand(ctx context.Context, cmdArgs []string, stdout, stderr io.Writer) int {
	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func runPublishPipeline(ctx context.Context, opts options, env Env) error {
	parts := strings.SplitN(opts.repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid or missing --repo %q (expected owner/repo)", opts.repo)
	}
	if opts.prNumber <= 0 {
		return fmt.Errorf("missing pull request number (--pr or GITHUB_EVENT_PATH)")
	}
	if strings.TrimSpace(opts.token) == "" {
		return fmt.Errorf("missing GitHub token (--token or GITHUB_TOKEN environment variable)")
	}

	scenarios, err := collector.Discover(opts.resultsDir, opts.reportFile)
	if err != nil {
		return fmt.Errorf("discover test artifacts: %w", err)
	}
	if len(scenarios) == 0 {
		fmt.Fprintf(env.Stderr, "warning: no scenarios with videos or screenshots found in %s\n", opts.resultsDir)
		return nil
	}

	tmpMediaDir, err := os.MkdirTemp("", "demonstra-media-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpMediaDir)

	conv := media.NewConverter(nil)
	assetInputs := make([]github.ScenarioAssetInput, 0, len(scenarios))

	for i, sc := range scenarios {
		var gifPath string
		var webpPath string
		var videoPath string
		outGIF := filepath.Join(tmpMediaDir, fmt.Sprintf("%d-%s.gif", i+1, sc.Slug))
		outWebP := filepath.Join(tmpMediaDir, fmt.Sprintf("%d-%s.webp", i+1, sc.Slug))
		outMP4 := filepath.Join(tmpMediaDir, fmt.Sprintf("%d-%s.mp4", i+1, sc.Slug))

		if sc.VideoPath != "" {
			if err := conv.ConvertToGIF(ctx, sc.VideoPath, outGIF); err != nil {
				fmt.Fprintf(os.Stderr, "demonstra: warning: ffmpeg gif conversion failed for %q: %v\n", sc.Title, err)
				if len(sc.Screenshots) > 0 {
					if fbErr := media.EncodeGIFsFromPNGs(sc.Screenshots, outGIF, 120); fbErr == nil {
						gifPath = outGIF
					} else {
						return fmt.Errorf("convert video for %q: %w", sc.Title, err)
					}
				} else {
					return fmt.Errorf("convert video for %q: %w", sc.Title, err)
				}
			} else {
				gifPath = outGIF
			}

			if err := conv.ConvertToWebP(ctx, sc.VideoPath, outWebP); err != nil {
				fmt.Fprintf(os.Stderr, "demonstra: warning: webp conversion failed for %q: %v\n", sc.Title, err)
			} else {
				webpPath = outWebP
			}

			if err := conv.ConvertToMP4(ctx, sc.VideoPath, outMP4); err != nil {
				fmt.Fprintf(os.Stderr, "demonstra: warning: mp4 conversion failed for %q: %v\n", sc.Title, err)
			} else {
				videoPath = outMP4
			}
		} else if len(sc.Screenshots) > 0 {
			if err := media.EncodeGIFsFromPNGs(sc.Screenshots, outGIF, 120); err == nil {
				gifPath = outGIF
			}
		}
		assetInputs = append(assetInputs, github.ScenarioAssetInput{
			Scenario:       sc,
			GIFPath:        gifPath,
			WebPPath:       webpPath,
			VideoPath:      videoPath,
			ScreenshotPath: sc.Screenshots,
		})
	}

	ghClient := github.NewClient(parts[0], parts[1], opts.token, opts.branch)
	ghClient.BaseURL = opts.apiURL
	ghClient.WebBaseURL = opts.webURL

	uploaded, prevRuns, err := ghClient.UploadPRScenarios(ctx, opts.prNumber, opts.sha, assetInputs)
	if err != nil {
		return fmt.Errorf("upload scenario assets to GitHub: %w", err)
	}

	md := render.FormatComment(render.RenderInput{
		PRNumber:     opts.prNumber,
		BranchName:   opts.prBranch,
		CommitSHA:    opts.sha,
		AssetBranch:  opts.branch,
		Scenarios:    uploaded,
		PreviousRuns: prevRuns,
	})

	if summaryPath := env.LookupEnv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
		_ = render.WriteStepSummary(summaryPath, md)
	}

	if err := ghClient.UpsertPRComment(ctx, opts.prNumber, md); err != nil {
		return fmt.Errorf("upsert PR comment: %w", err)
	}

	fmt.Fprintf(env.Stdout, "demonstra.me: published %d visual scenario(s) to PR #%d\n", len(uploaded), opts.prNumber)
	return nil
}
