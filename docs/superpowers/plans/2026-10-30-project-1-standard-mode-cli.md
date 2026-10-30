# Project 1: Standard Mode CLI (`demonstra`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the standalone `demonstra` Go CLI that executes or inspects Playwright E2E test runs in GitHub Actions, converts `.webm`/`.mp4` recordings into optimized GIFs via FFmpeg, persists GIFs and screenshots to the `demonstra-assets` orphan branch via the GitHub API, and posts/updates a Layout A1 visual review comment on the Pull Request.

**Architecture:** A modular, zero-CGO Go CLI split into four focused internal packages (`collector`, `media`, `github`, `render`) orchestrated by `internal/cli` and `cmd/demonstra`. External system dependencies (`ffmpeg` CLI and GitHub REST API) sit behind small interfaces/HTTP endpoints so every package is unit-tested deterministically without network access or local media binaries.

**Tech Stack:** Go (`go 1.23+`, standard library `net/http`, `os/exec`, `encoding/json`, `flag`), FFmpeg CLI (`palettegen`/`paletteuse`), GitHub REST & Git Data v3 API.

**Spec:** `docs/superpowers/specs/2026-10-30-project-1-standard-mode-cli-design.md`

## Global Constraints

- **Module Path:** `github.com/demonstra-me/demonstra` (standard library only; zero third-party Go dependencies required).
- **Licensing:** Closed-source compatible; never link C FFmpeg libraries—always invoke the system `ffmpeg` binary via `os/exec`.
- **GIF Encoding:** Two-pass filtergraph `fps=12,scale='min(800,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle`; fallback to `fps=8,scale='min(600,iw)':-1` when output exceeds `9437184` bytes (9 MB).
- **GitHub Asset URLs:** Must format inline URLs as `https://github.com/{owner}/{repo}/blob/{branch}/pr-{prNumber}/{shortSHA}/{slug}/{filename}?raw=true` so private repository images authenticate via browser session cookies.
- **Idempotent Comment Marker:** Every PR comment must begin with `<!-- demonstra-pr-comment -->`.
- **Layout A1 Expansion Rule:** Expand (`<details open>`) any failed scenario and scenarios at 0-based index `0`, `1`, `2` (first 3 scenarios); collapse (`<details>`) passing scenarios at index `>= 3`.
- **CI Exit Code Preservation:** In `demonstra record -- <cmd>`, always run the visual upload pipeline even if `<cmd>` exits non-zero (so failed test flows still publish their recordings), then return `<cmd>`'s exact exit code; if FFmpeg or GitHub API fails after tests pass, log a warning and exit `0` unless `--strict` is set.

## Review Focus

1. **Nested Playwright suites & special characters in titles:** Playwright JSON reports nest `suites` recursively (file suite -> `describe` suite -> `specs`) and test titles often contain spaces, slashes, or quotes that must be safely slugified for git paths.
2. **Oversized GIFs (> 9 MB) & missing `ffmpeg`:** Long test recordings can produce > 9 MB GIFs at 12 fps / 800px; the converter must automatically re-encode at 8 fps / 600px, and return a clear error if `ffmpeg` fails.
3. **First run on a repository with no `demonstra-assets` branch:** Looking up `GET /repos/{owner}/{repo}/git/ref/heads/demonstra-assets` returns HTTP 404 on the first run; the client must create an orphan tree (no `base_tree`) and `POST /repos/{owner}/{repo}/git/refs`, whereas subsequent runs must preserve existing files using `base_tree` and `PATCH`.
4. **Markdown table injection & 65,536-character PR comment limit:** Pipe characters (`|`) or newlines in test titles break Markdown tables, and PRs with many scenarios can exceed GitHub's 65,536-character comment body limit if not truncated gracefully.
5. **GitHub Actions `pull_request` event payload parsing & non-zero test exit codes:** In GitHub Actions, the PR number lives inside the JSON file at `$GITHUB_EVENT_PATH` (`pull_request.number` or `issue.number`), and a failing Playwright test (`exit 1`) must still publish failure GIFs/screenshots before returning exit code `1`.

---

### Task 1: Go Module & Playwright Artifact Collector (`internal/collector`)

**Files:**
- Create: `go.mod`
- Create: `internal/collector/collector.go`
- Test: `internal/collector/collector_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  ```go
  package collector

  import "time"

  type Status string

  const (
      StatusPassed  Status = "passed"
      StatusFailed  Status = "failed"
      StatusSkipped Status = "skipped"
  )

  type Scenario struct {
      Title       string
      Slug        string
      SpecFile    string
      Line        int
      Status      Status
      Duration    time.Duration
      VideoPath   string
      Screenshots []string
  }

  func Slugify(s string) string
  func Discover(resultsDir string, reportFile string) ([]Scenario, error)
  ```

- [ ] **Step 1: Initialize `go.mod` and write the failing collector tests in `internal/collector/collector_test.go`**

Create `go.mod` with `module github.com/demonstra-me/demonstra` and `go 1.23`. In `internal/collector/collector_test.go`, write three tests:
- `TestDiscover_FromPlaywrightJSONReport`: Creates a temp directory with nested Playwright JSON report suites (`root suite -> describe suite -> spec -> tests -> results -> attachments` containing a `.webm` video and two `.png` screenshots, including a title `"Apply 20% coupon | checkout/cart"`), calls `Discover(resultsDir, reportPath)`, and asserts:
  - `len(scenarios) == 2`
  - `scenarios[0].Title == "Apply 20% coupon | checkout/cart"`
  - `scenarios[0].Slug == "apply-20-coupon-checkout-cart"`
  - `scenarios[0].SpecFile == "e2e/checkout.spec.ts"` and `scenarios[0].Line == 14`
  - `scenarios[0].Status == StatusPassed` and `scenarios[0].Duration == 6400*time.Millisecond`
  - `scenarios[0].VideoPath` ends with `"video.webm"` and `len(scenarios[0].Screenshots) == 2`
- `TestDiscover_FallbackDirectoryScan`: Calls `Discover(resultsDir, "")` where `resultsDir` has subdirectories `checkout-Apply-valid-coupon-chromium/video.webm`, `checkout-Apply-valid-coupon-chromium/test-failed-1.png`, and a second directory with `.mp4`, asserting `Discover` returns 2 scenarios sorted by directory name, marks the scenario containing `test-failed-*.png` as `StatusFailed` and the other as `StatusPassed`.
- `TestSlugify`: Asserts `Slugify("  Apply $20 Coupon / Cart! ") == "apply-20-coupon-cart"` and `Slugify("!!!") == "scenario"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/collector/... -v`
Expected: FAIL (package/functions not defined)

- [ ] **Step 3: Implement `Slugify` and `Discover` in `internal/collector/collector.go`**

- `Slugify(s string) string`: lowercases ASCII, replaces any run of non-`[a-z0-9]` characters with a single `-`, trims leading/trailing `-`, truncates to at most 64 chars without trailing `-`, and returns `"scenario"` if empty.
- `Discover(resultsDir string, reportFile string) ([]Scenario, error)`:
  - If `reportFile != ""` and the file exists on disk, parse Playwright's JSON schema (`suites` recursively, collecting each `spec` and its last `test.results` entry). Map `"passed"`/`"expected"` to `StatusPassed`, `"failed"`/`"unexpected"`/`"timedOut"` to `StatusFailed`, and others to `StatusSkipped`. Keep only scenarios that have at least one existing `.webm`/`.mp4` video or `.png` screenshot attachment on disk.
  - Otherwise, walk `resultsDir` (`os.ReadDir` / `filepath.WalkDir`), group `.webm`/`.mp4` and `.png` files by their immediate parent directory inside `resultsDir`, derive `Title` from the directory name (replacing `-` with spaces), mark `StatusFailed` if any filename contains `"failed"`, and sort scenarios deterministically.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/collector/... -v`
Expected: PASS (all 3 tests passing)

- [ ] **Step 5: Commit**

```bash
git add go.mod internal/collector/collector.go internal/collector/collector_test.go
git commit -m "feat(collector): add Playwright JSON report and directory fallback discovery"
```

---

### Task 2: FFmpeg Video-to-GIF Media Converter (`internal/media`)

**Files:**
- Create: `internal/media/converter.go`
- Test: `internal/media/converter_test.go`

**Interfaces:**
- Consumes: none
- Produces:
  ```go
  package media

  import "context"

  const DefaultMaxGIFBytes int64 = 9 * 1024 * 1024 // 9 MB (9437184 bytes)

  type CommandRunner interface {
      Run(ctx context.Context, name string, args ...string) error
  }

  type ExecRunner struct{}

  func (ExecRunner) Run(ctx context.Context, name string, args ...string) error

  type Converter struct {
      Runner   CommandRunner
      MaxBytes int64
  }

  func NewConverter(runner CommandRunner) *Converter
  func (c *Converter) ConvertToGIF(ctx context.Context, videoPath string, outGIFPath string) error
  ```

- [ ] **Step 1: Write the failing converter tests in `internal/media/converter_test.go`**

Write three tests using a fake `CommandRunner`:
- `TestConvertToGIF_PrimaryEncodingUnderLimit`: Records the command name and args passed to `Runner.Run`, writes a 1024-byte dummy GIF file to `outGIFPath`, and asserts `Runner.Run` was called exactly once with `"ffmpeg"` and filter argument `"fps=12,scale='min(800,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"`.
- `TestConvertToGIF_FallbackWhenExceedingMaxBytes`: Configures `MaxBytes: 1000`. On call 1, the fake runner writes 1500 bytes to `outGIFPath`; on call 2, it writes 500 bytes. Asserts `Runner.Run` was called twice, and the second call used the fallback filter `"fps=8,scale='min(600,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=64:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle"`.
- `TestConvertToGIF_RunnerError`: Fake runner returns an error; asserts `ConvertToGIF` returns a non-nil error containing `"ffmpeg"`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/media/... -v`
Expected: FAIL (package/types not defined)

- [ ] **Step 3: Implement `Converter` and `ExecRunner` in `internal/media/converter.go`**

- `ExecRunner.Run` executes `exec.CommandContext(ctx, name, args...)`, capturing combined output and including it in the returned error on failure.
- `NewConverter(runner CommandRunner) *Converter` defaults `Runner` to `ExecRunner{}` if `nil`, and `MaxBytes` to `DefaultMaxGIFBytes`.
- `ConvertToGIF` creates `filepath.Dir(outGIFPath)` if needed, invokes `ffmpeg -y -i <videoPath> -vf <primaryFilter> <outGIFPath>`, checks `os.Stat(outGIFPath).Size() > c.MaxBytes`, and if so re-invokes `ffmpeg` with `<fallbackFilter>`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/media/... -v`
Expected: PASS (all 3 tests passing)

- [ ] **Step 5: Commit**

```bash
git add internal/media/converter.go internal/media/converter_test.go
git commit -m "feat(media): add FFmpeg two-pass GIF converter with 9MB size fallback"
```

---

### Task 3: GitHub Git Data Asset Uploader & PR Comment Client (`internal/github`)

**Files:**
- Create: `internal/github/client.go`
- Test: `internal/github/client_test.go`

**Interfaces:**
- Consumes: `collector.Scenario` from `internal/collector`
- Produces:
  ```go
  package github

  import (
      "context"
      "net/http"

      "github.com/demonstra-me/demonstra/internal/collector"
  )

  const CommentMarker = "<!-- demonstra-pr-comment -->"

  type ScenarioAssetInput struct {
      Scenario       collector.Scenario
      GIFPath        string
      ScreenshotPath []string
  }

  type UploadedScenario struct {
      Scenario       collector.Scenario
      GIFURL         string
      ScreenshotURLs []string
  }

  type PreviousRun struct {
      ShortSHA string
      TreeURL  string
  }

  type Client struct {
      BaseURL    string
      WebBaseURL string
      Owner      string
      Repo       string
      Token      string
      Branch     string
      HTTPClient *http.Client
  }

  func NewClient(owner, repo, token, branch string) *Client
  func (c *Client) UploadPRScenarios(ctx context.Context, prNumber int, commitSHA string, inputs []ScenarioAssetInput) ([]UploadedScenario, []PreviousRun, error)
  func (c *Client) UpsertPRComment(ctx context.Context, prNumber int, markdownBody string) error
  ```

- [ ] **Step 1: Write the failing GitHub API client tests in `internal/github/client_test.go`**

Using `httptest.NewServer`, write three tests:
- `TestUploadPRScenarios_OrphanBranchCreation`: Simulates `GET /repos/acme/shop/git/ref/heads/demonstra-assets` returning `404`. Verifies the client uploads base64 blobs (`POST /repos/acme/shop/git/blobs`), creates a tree (`POST /repos/acme/shop/git/trees` with `base_tree` omitted), creates an orphan commit (`parents: []`), creates the ref (`POST /repos/acme/shop/git/refs` with `refs/heads/demonstra-assets`), and returns `GIFURL == "https://github.com/acme/shop/blob/demonstra-assets/pr-42/a8f921c/apply-coupon/flow.gif?raw=true"`.
- `TestUploadPRScenarios_ExistingBranchWithPreviousRuns`: Simulates existing ref (`GET .../git/ref/heads/demonstra-assets` -> commit SHA -> tree SHA) and `GET /repos/acme/shop/git/trees/{treeSHA}?recursive=1` containing `pr-42/1111111/apply-coupon/flow.gif`. Verifies the new tree includes `base_tree`, updates the ref via `PATCH /repos/acme/shop/git/refs/heads/demonstra-assets`, and returns `PreviousRuns` containing `ShortSHA: "1111111"`, `TreeURL: "https://github.com/acme/shop/tree/demonstra-assets/pr-42/1111111"`.
- `TestUpsertPRComment_CreateAndUpdate`: First call finds no comment with `<!-- demonstra-pr-comment -->` and `POST`s a new comment; second call sees the existing comment containing `<!-- demonstra-pr-comment -->` and `PATCH`es `/repos/acme/shop/issues/comments/{id}`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/github/... -v`
Expected: FAIL (package/types not defined)

- [ ] **Step 3: Implement `Client`, `UploadPRScenarios`, and `UpsertPRComment` in `internal/github/client.go`**

- Default `BaseURL` to `"https://api.github.com"`, `WebBaseURL` to `"https://github.com"`, `Branch` to `"demonstra-assets"`, and `ShortSHA` to the first 7 chars of `commitSHA`.
- Set headers `Authorization: Bearer <Token>`, `Accept: application/vnd.github+json`, `X-GitHub-Api-Version: 2022-11-28`.
- Upload each scenario's `GIFPath` to `pr-{prNumber}/{shortSHA}/{slug}/flow.gif` and each screenshot `i` (1-indexed) to `pr-{prNumber}/{shortSHA}/{slug}/screenshots/step-{i}.png`.
- Construct inline URLs as `{WebBaseURL}/{Owner}/{Repo}/blob/{Branch}/{path}?raw=true`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/github/... -v`
Expected: PASS (all 3 tests passing)

- [ ] **Step 5: Commit**

```bash
git add internal/github/client.go internal/github/client_test.go
git commit -m "feat(github): add orphan branch Git Data uploader and PR comment upsert"
```

---

### Task 4: Layout A1 Markdown Renderer & Job Summary Writer (`internal/render`)

**Files:**
- Create: `internal/render/markdown.go`
- Test: `internal/render/markdown_test.go`

**Interfaces:**
- Consumes: `collector.Status` from `internal/collector`, `github.UploadedScenario`, `github.PreviousRun`, `github.CommentMarker` from `internal/github`
- Produces:
  ```go
  package render

  import "github.com/demonstra-me/demonstra/internal/github"

  const MaxCommentLength = 65000

  type RenderInput struct {
      PRNumber     int
      BranchName   string
      CommitSHA    string
      AssetBranch  string
      Scenarios    []github.UploadedScenario
      PreviousRuns []github.PreviousRun
  }

  func FormatComment(in RenderInput) string
  func WriteStepSummary(summaryPath string, markdown string) error
  ```

- [ ] **Step 1: Write the failing renderer tests in `internal/render/markdown_test.go`**

Write three tests:
- `TestFormatComment_LayoutA1SmartExpansion`: Passes 4 passing scenarios and 1 failing scenario at index 4 (5th scenario), plus 1 `PreviousRun`. Asserts:
  - Output starts with `<!-- demonstra-pr-comment -->`
  - Contains the summary table with rows 1 through 5
  - Scenarios 1, 2, 3 (`index 0..2`) and Scenario 5 (`StatusFailed`) render with `<details open>`
  - Scenario 4 (`index 3`, `StatusPassed`) renders with `<details>` (without `open`)
  - Includes nested `<details><summary>View 2 Step Screenshots</summary>` for scenarios with screenshots
  - Includes `<details><summary>Previous runs on this PR (1)</summary>` with a link to the previous run's `TreeURL`
- `TestFormatComment_TableEscapingAndLengthGuardrail`: Passes a scenario title containing `"Checkout | Pipe\nNewline"` and verifies the table cell escapes `|` as `\|` and replaces newlines with spaces; passes scenarios with huge URLs exceeding 65,000 characters and asserts `len(FormatComment(in)) <= MaxCommentLength`.
- `TestWriteStepSummary`: Writes markdown to a temp file path and verifies the file content matches.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/render/... -v`
Expected: FAIL (package/functions not defined)

- [ ] **Step 3: Implement `FormatComment` and `WriteStepSummary` in `internal/render/markdown.go`**

- Render `<!-- demonstra-pr-comment -->` on line 1, followed by `## 🎬 demonstra.me Visual Review`, metadata line (`Commit \`{shortSHA}\` • Stored in \`{AssetBranch}/pr-{PRNumber}\``), and the Markdown table (`| # | Scenario | Spec File | Status | Duration |`).
- Below the table, render each scenario card. Set `<details open>` when `s.Scenario.Status == collector.StatusFailed || i < 3`; otherwise `<details>`.
- If the rendered string exceeds `MaxCommentLength`, first omit the nested screenshot blocks, and if still over `MaxCommentLength`, include only scenarios that fit within `MaxCommentLength` followed by a link to the full `demonstra-assets/pr-{PRNumber}/{shortSHA}` branch folder.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/render/... -v`
Expected: PASS (all 3 tests passing)

- [ ] **Step 5: Commit**

```bash
git add internal/render/markdown.go internal/render/markdown_test.go
git commit -m "feat(render): implement Layout A1 PR comment renderer and step summary writer"
```

---

### Task 5: CLI Orchestrator, Entrypoint & GitHub Action Example (`internal/cli` & `cmd/demonstra`)

**Files:**
- Create: `internal/cli/run.go`
- Create: `cmd/demonstra/main.go`
- Create: `examples/playwright-demo/README.md`
- Test: `internal/cli/run_test.go`

**Interfaces:**
- Consumes: `collector.Discover`, `media.Converter`, `github.Client`, `render.FormatComment`, `render.WriteStepSummary`
- Produces:
  ```go
  package cli

  import (
      "context"
      "io"
  )

  type Env struct {
      LookupEnv func(string) string
      Stdout    io.Writer
      Stderr    io.Writer
  }

  func Execute(ctx context.Context, args []string, env Env) int
  ```

- [ ] **Step 1: Write the failing end-to-end CLI tests in `internal/cli/run_test.go`**

Write three end-to-end tests exercising `Execute`:
- `TestExecute_RecordPreservesFailingTestExitCodeAndPublishes`: Sets up a mock GitHub `httptest.NewServer`, a fake `ffmpeg` executable script on `$PATH` in `t.TempDir()`, a `$GITHUB_EVENT_PATH` JSON file (`{"pull_request":{"number":42}}`), and a dummy test command that creates `test-results/login-flow/video.webm` + `test-failed-1.png` and exits with code `1`. Runs `Execute(ctx, []string{"record", "--", "sh", "-c", "..."}, env)` and asserts:
  - Exit code returned by `Execute` is `1` (preserving the test failure exit code)
  - The mock GitHub server received the blob upload, tree creation, ref creation, and PR comment POST containing `<!-- demonstra-pr-comment -->` and `login flow`
  - `$GITHUB_STEP_SUMMARY` file was populated
- `TestExecute_PublishSubcommand`: Runs `Execute(ctx, []string{"publish", "--results-dir", dir, "--repo", "acme/shop", "--pr", "7", "--sha", "abcdef1234567", "--token", "tok"}, env)` and asserts exit code `0` and comment posted to PR #7.
- `TestExecute_NonStrictGracefulDegradationOnAPIError`: When the test command exits `0` but the GitHub API returns `500`, asserts `Execute` logs a warning to `Stderr` and returns exit code `0` when `--strict` is not set, and returns non-zero when `--strict` is set.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/... -v`
Expected: FAIL (package/functions not defined)

- [ ] **Step 3: Implement `internal/cli/run.go`, `cmd/demonstra/main.go`, and `examples/playwright-demo/README.md`**

- Support subcommands `record` (executes command after `--`, then runs publish pipeline, then returns wrapped command exit code) and `publish` (runs publish pipeline on existing `--results-dir`).
- Resolve `--repo` (`GITHUB_REPOSITORY`), `--sha` (`GITHUB_SHA`), `--token` (`GITHUB_TOKEN`), `--pr` (flag or parsed from JSON file at `GITHUB_EVENT_PATH`: `pull_request.number` or `issue.number`), `--branch` (default `"demonstra-assets"`), `--results-dir` (default `"./test-results"`), `--report-file` (default `""`), `--api-url` (default `"https://api.github.com"`), `--web-url` (default `"https://github.com"`), and `--strict` (default `false`).
- Wire `cmd/demonstra/main.go` to call `os.Exit(cli.Execute(context.Background(), os.Args[1:], cli.Env{LookupEnv: os.Getenv, Stdout: os.Stdout, Stderr: os.Stderr}))`.
- Document the GitHub Actions workflow usage (`permissions: contents: write, pull-requests: write`) in `examples/playwright-demo/README.md`.

- [ ] **Step 4: Run full test suite and build binary to verify everything passes**

Run: `go test ./... -v && go build -o bin/demonstra ./cmd/demonstra`
Expected: PASS (all packages pass tests and `bin/demonstra` compiles cleanly)

- [ ] **Step 5: Commit**

```bash
git add internal/cli/run.go internal/cli/run_test.go cmd/demonstra/main.go examples/playwright-demo/README.md
git commit -m "feat(cli): add record and publish commands with GitHub Actions integration"
```
