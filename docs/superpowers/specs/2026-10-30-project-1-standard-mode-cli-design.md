# Project 1 Design Spec: Standard Mode End-to-End (`demonstra` Go CLI + GitHub PR Workflow)

**Date:** 2026-10-30  
**Project:** 1 of 3 (Standard Mode CLI & GitHub Integration)  
**Related Strategy Doc:** `docs/superpowers/specs/2026-10-30-product-roadmap-and-monetization.md`

---

## 1. Intent and Why

Distributed developers and teams reviewing pull requests—especially those generated or assisted by AI agents—need a fast, low-friction way to see how UI changes actually behave without pulling the branch locally or clicking into external dashboards.

**Project 1** delivers the complete **Standard Mode** workflow end-to-end as a standalone, closed-source Go CLI (`demonstra`) integrated with GitHub Actions and GitHub Pull Requests. When a PR opens or updates, `demonstra` runs the repository's existing E2E tests in CI, converts the captured screen recordings into crisp animated GIFs, persists those GIFs and screenshots for posterity in the user's own GitHub repository, and posts/updates a rich multi-scenario comment directly on the PR.

---

## 2. Goals, Non-Goals, and Anti-Goals

### Goals
1. **Zero-Friction CI Execution:** Run inside the user's existing GitHub Actions runner where their app (`localhost` or preview) and Playwright tests already run.
2. **Multi-Scenario Visual Capture:** Automatically discover recorded `.webm`/`.mp4` videos and `.png` screenshots per test scenario and convert videos into lightweight, high-clarity animated GIFs.
3. **In-Repo Posterity Storage:** Store generated GIFs and screenshots in a dedicated orphan branch (`demonstra-assets`) in the user's own GitHub repository so images render inline in both private and public repo PR comments and remain viewable on historical PRs forever.
4. **Layout A1 PR Comment & Job Summary:** Post (or update in-place) a single GitHub PR comment featuring a Scenario Summary Table at the top and smart expandable `<details>` scenario cards below (with inline Hero GIFs and nested step screenshots), plus output the same summary to `$GITHUB_STEP_SUMMARY`.

### Non-Goals (Deferred to Projects 2 & 3)
* **Rails SaaS Backend & Billing (Project 2):** Project 1 is strictly standalone and does not require calling a remote `demonstra.me` server or validating paid API keys yet.
* **Autonomous AI Browser Agent / `playwright-go` Execution (Project 3):** Project 1 runs existing user test suites rather than synthesizing flows from diffs with an LLM.
* **GitLab / Bitbucket Integration:** Project 1 targets GitHub Actions and GitHub Pull Requests first (though provider interfaces should be cleanly abstracted).
* **Custom Web Review UI:** Reviewers watch GIFs and leave feedback directly inside GitHub's native PR comment thread.

### Anti-Goals (What Would Make This a Failure Even If It Works)
* **Bloating `main` or Slowing Down Standard `git clone`:** Assets must live on an isolated orphan branch (`demonstra-assets`) pushed via the GitHub API or isolated plumbing, never committed into the PR's source branch.
* **Spamming the PR Thread:** Pushing 5 commits to a PR must update the existing `demonstra.me` comment (or replace its current summary while keeping history links), never flood the PR with 5 separate massive bot comments.
* **GPL/Copyleft License Contamination:** The Go binary is closed-source; it must invoke the system `ffmpeg` binary via `os/exec` rather than statically linking GPL/LGPL C libraries.
* **Failing the CI Build When Rendering Optional Visuals:** If FFmpeg or the GitHub comment API encounters a transient error after tests pass, the CLI should surface a clear warning (unless configured as strict) rather than hiding the underlying test exit code.

---

## 3. Constraints & Licensing

* **Language & Distribution:** Closed-source Go binary (`demonstra`), cross-compiled for Linux (`amd64`/`arm64`), macOS (`amd64`/`arm64`), and Windows.
* **E2E Framework Support:** First-class support for **Playwright** in Project 1 (with the collector interface designed so Cypress can be plugged in easily).
* **Video Conversion:** Uses the system `ffmpeg` binary via `os/exec` (pre-installed on `ubuntu-latest` GitHub Actions runners).
* **GitHub Limits:**
  * GitHub PR comment body limit: **65,536 characters**.
  * GitHub inline image rendering size limit via raw repo URLs: keep individual GIFs under **10 MB** (target **1–4 MB** per scenario GIF).

---

## 4. Decisions Made by Product Owner ("Theirs")

1. **Architecture Option 3 + Rails Later:** Closed-source Go CLI for CI execution; Ruby on Rails 8 reserved for Project 2 (SaaS platform).
2. **Project 1 Strict Scope:** Focus strictly on Standard Mode end-to-end (Go CLI + GitHub PR workflow) before touching Rails or AI agents.
3. **Playwright First:** Target Playwright test suites first.
4. **Keep Assets for Posterity:** Store GIFs and screenshots permanently in the user's own GitHub repo (`demonstra-assets` branch) rather than deleting them on merge.
5. **PR Comment Layout A1:**
   * Top **Scenario Summary Table** listing all scenarios (`#`, `Scenario`, `Spec File`, `Status`, `Duration`).
   * Below the table, individual **Scenario Cards** using `<details>` blocks:
     * Shows the scenario's **Hero GIF** inline.
     * Includes a nested collapsible `<details>` block for **Step Screenshots** (if any `.png` screenshots were captured for that scenario).
     * Smart expansion: first scenarios start expanded (`<details open>`) so reviewers see them immediately, while excess scenarios start collapsed (`<details>`) to keep long PRs tidy.

---

## 5. Technical Design & Builder Calls ("My Calls")

*(Note: Any item marked `[Builder Call]` was settled as a sensible technical default and can be overridden.)*

### 5.1 CLI Command Interface (`demonstra`)
* **[Builder Call] Primary Command:**
  ```bash
  demonstra record -- npx playwright test
  ```
  Or using flags / environment variables:
  ```bash
  demonstra record \
    --results-dir ./test-results \
    --report-file ./test-results/results.json \
    --branch demonstra-assets \
    -- npx playwright test
  ```
* **Two-step mode support:** Users can either let `demonstra record -- <cmd>` wrap and execute their test command directly (returning the exact exit code of `<cmd>` after uploading visuals), OR run `demonstra publish` as a separate step after their tests have already finished and produced `test-results/`.

### 5.2 Playwright Test Execution & Artifact Discovery
* **[Builder Call] Discovering Scenarios, Videos, and Screenshots:**
  * Playwright outputs a folder per test run inside `test-results/` (default) containing `video.webm` (when `video: 'on'` is configured) and `*.png` screenshots.
  * If a Playwright JSON report (`PLAYWRIGHT_JSON_OUTPUT_NAME` / `--reporter=json`) is present, `demonstra` parses it for rich metadata (human-readable scenario title, spec file path and line number, pass/fail status, duration in seconds, and exact attachment paths).
  * **Fallback Directory Scanner:** Even if the user didn't configure Playwright's JSON reporter in `playwright.config.ts`, `demonstra` automatically scans `test-results/**` for `.webm`/`.mp4` and `.png` files and derives scenario names from the folder structure so zero config edits are needed beyond `video: 'on'`.

### 5.3 Video-to-GIF Conversion Pipeline (`ffmpeg`)
* **[Builder Call] Two-Pass Palette Optimization:**
  For each discovered `.webm` or `.mp4` file, `demonstra` invokes `ffmpeg` via `os/exec` using a single filtergraph with `palettegen` and `paletteuse` for crisp text rendering and small file size:
  ```bash
  ffmpeg -y -i input.webm \
    -vf "fps=12,scale='min(800,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128:stats_mode=diff[p];[s1][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
    output.gif
  ```
* **[Builder Call] Size Guardrail:** If a generated GIF exceeds **9 MB** (approaching GitHub's 10 MB inline proxy limit), `demonstra` automatically re-encodes with a lower FPS/width (`fps=8,scale=600:-1`) so it never fails to render on GitHub.

### 5.4 Persisting Assets to `demonstra-assets` via GitHub API
* **[Builder Call] GitHub Git Data API (Zero Local Git Checkout Pollution):**
  * Instead of running `git checkout` inside the user's CI workspace (which could conflict with dirty files or shallow clones), `demonstra` uses the **GitHub REST / Git Data API** using `GITHUB_TOKEN`:
    1. Checks if `refs/heads/demonstra-assets` exists; if not, creates an initial orphan commit and ref.
    2. Uploads each `.gif` and `.png` as a Base64 Git Blob (`POST /repos/{owner}/{repo}/git/blobs`) under path:
       `pr-{pr_number}/{commit_sha_short}/{scenario_slug}/flow.gif` and `screenshots/{step}.png`.
    3. Creates a new Git Tree (`base_tree` = current `demonstra-assets` tree SHA, preserving all prior PRs and commits for posterity), creates a Commit, and updates `refs/heads/demonstra-assets`.
  * **[Builder Call] Inline Image URL Format:**
    * Uses `https://github.com/{owner}/{repo}/blob/demonstra-assets/pr-{pr_number}/{sha}/{scenario_slug}/flow.gif?raw=true`.
    * Using `/blob/...?raw=true` on `github.com` (rather than `raw.githubusercontent.com` directly) ensures GitHub automatically authenticates the image request using the logged-in reviewer's browser cookie on **both private and public repositories**.

### 5.5 Formatting and Updating the PR Comment (Layout A1)
* **[Builder Call] Comment Upsert Marker:**
  * Every PR comment posted by `demonstra` starts with a hidden marker: `<!-- demonstra-pr-comment -->`.
  * On subsequent runs for the same PR, `demonstra` lists comments on the PR (`GET /repos/{owner}/{repo}/issues/{pr_number}/comments`), finds the existing comment containing `<!-- demonstra-pr-comment -->`, and updates it via `PATCH` (or creates a new comment if none exists).
  * To preserve easy access to previous commits' recordings on the same PR (since assets are kept for posterity in `demonstra-assets`), the comment includes a collapsible `<details><summary>Previous runs on this PR</summary>...</details>` footer linking to earlier commits' folders on `demonstra-assets`.
* **[Builder Call] Smart Scenario Expansion Threshold:**
  * Any **failed** scenario is always expanded (`<details open>`).
  * Up to the first **3 scenarios** are expanded by default (`<details open>`).
  * Scenarios **4+** start collapsed (`<details>`) so a PR with 10+ scenarios remains easy to read.
* **[Builder Call] GitHub Actions Job Summary:**
  * If `$GITHUB_STEP_SUMMARY` is set in the environment, `demonstra` also appends the generated Markdown to `$GITHUB_STEP_SUMMARY` so it appears on the Actions run summary page.

### 5.6 Repository Layout for Project 1
```text
demonstra.me/
├── cmd/
│   └── demonstra/
│       └── main.go              # CLI entrypoint (Cobra / standard flags)
├── internal/
│   ├── cli/                     # Command parsing & orchestration
│   ├── collector/               # Playwright artifact & JSON report discovery
│   ├── media/                   # FFmpeg video-to-GIF encoder & size fallback
│   ├── github/                  # GitHub API client (Git Data tree uploader + PR comment upsert)
│   └── render/                  # Layout A1 Markdown renderer + Job Summary writer
├── examples/
│   └── playwright-demo/         # Sample Playwright project & GitHub Action workflow for E2E verification
├── docs/
│   └── superpowers/specs/       # Product & design specifications
└── go.mod
```
