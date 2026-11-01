# 🎬 demonstra.me

[![CI](https://github.com/demonstra-me/demonstra/actions/workflows/ci.yml/badge.svg)](https://github.com/demonstra-me/demonstra/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/demonstra-me/demonstra?logo=github)](https://github.com/demonstra-me/demonstra/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/demonstra-me/demonstra?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Effortless, automated visual pull request reviews with inline full-color WebP recordings and interactive MP4 timeline scrubbers.**

Reviewing PRs should not require pulling branches locally, configuring databases, or reading through hundred-line diffs just to see what changed on screen. **`demonstra`** automatically records user flows during CI tests, generates optimized visual assets, and publishes an interactive visual review directly onto your GitHub Pull Request.

---

## 📽️ Live Demo

[![Demonstra Visual Review](https://github.com/gregoriomelo/todomvc_rails/raw/demonstra-assets/pr-1/f74eec7/create-items-toggle-completion-and-filter/flow.webp)](https://github.com/gregoriomelo/todomvc_rails/blob/demonstra-assets/pr-1/f74eec7/create-items-toggle-completion-and-filter/video.mp4)

🎬 **[▶️ Click to Open Interactive Player with Timeline Scrubbing & Pause](https://github.com/gregoriomelo/todomvc_rails/blob/demonstra-assets/pr-1/f74eec7/create-items-toggle-completion-and-filter/video.mp4)**  
👉 **[View real GitHub PR #1 Comment](https://github.com/gregoriomelo/todomvc_rails/pull/1#issuecomment-6559366455)**

---

## ✨ Features

- **🌐 Zero External SaaS Required (Standard Mode):** Runs completely inside your CI runner. Encodes media via system FFmpeg and stores visual assets in an orphan git branch (`demonstra-assets`) within your own repository.
- **🎨 Visual Tutorial Engine (Option C):** Visual feedback that makes videos feel like human walkthroughs:
  - 💥 **Concentric Radar Pulses:** Expanding animated shockwaves on click coordinates.
  - ✨ **Glowing Halos:** High-contrast outline highlights on active inputs, buttons, and links.
  - ⌨️ **Keystroke & Action HUD:** Floating pill displaying real-time typed text and keybadges (`<kbd>↵ Enter</kbd> Submit`, `<span>⌨️ Type</span> "..."`, `<span>🖱️ Click</span> Button`).
- **🚀 Dual-Format Output (WebP + MP4):**
  - **Inline WebP:** 24-bit full-color animated WebP auto-plays directly inside the GitHub PR comment (no GIF color banding or dithering).
  - **Click-to-Scrub MP4:** Streamable H.264 MP4 (~100 KB) with `+faststart` metadata for instant seeking, pause, playback speed, and fullscreen.
- **📋 Smart Layout A1:**
  - Compact summary table with scenarios, spec files, line numbers, and pass/fail indicators.
  - Auto-expanding cards for failed scenarios or the first 3 flows.
  - Collapsible nested step screenshots and previous run history.

---

## ⚡ Quickstart

### Method 1: Official GitHub Action (Recommended)

Add **Demonstra** to your existing test workflow in 3 lines:

```yaml
name: E2E Tests & Visual Review

on:
  pull_request:
    types: [opened, synchronize, reopened]

permissions:
  contents: write
  pull-requests: write

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - name: Install dependencies
        run: npm ci && npx playwright install --with-deps chromium

      # Single composite step installs FFmpeg, Demonstra CLI, runs tests & posts visual review
      - name: Demonstra Visual PR Review
        uses: demonstra-me/demonstra@v1
        with:
          command: npx playwright test
          github-token: ${{ secrets.GITHUB_TOKEN }}
```

### Method 2: Automatic Scaffolding (`demonstra init`)

Run `demonstra init` in your project root to auto-detect your test framework (Playwright, Cypress) and scaffold the workflow and visual interaction helpers:

```bash
# Detects framework and scaffolds .github/workflows/demonstra.yml and e2e/support/interact.js
demonstra init
```

Enable visual feedback in Playwright tests (`e2e/todos.spec.js`):

```javascript
import { test, expect } from '@playwright/test';
import { enableVisualInteractions } from './support/interact.js';

test.beforeEach(async ({ page }) => {
  await enableVisualInteractions(page);
});
```

---

## 🥊 Comparison

| Feature | demonstra.me | Chromatic / Percy | Cypress Cloud | PageBolt / Screencli |
| :--- | :---: | :---: | :---: | :---: |
| **Inline PR Walkthrough** | ✅ **Yes (WebP + MP4)** | ❌ External Dashboard | ❌ External Dashboard | ⚠️ External MP4 |
| **Interactive Scrubber** | ✅ **Yes (H.264 MP4)** | ❌ Static Diffs | ✅ Video Player | ⚠️ Separate URL |
| **Visual Tutorial Feedback** | ✅ **Radar + HUD Pill** | ❌ None | ❌ None | ❌ None |
| **Runs on Localhost in CI** | ✅ **Yes** | ⚠️ Partial | ✅ Yes | ❌ Requires Public URL |
| **Storage Cost** | **$0** (In-Repo Branch) | 💰 Per-screenshot SaaS | 💰 Per-test SaaS | 💰 Subscription |
| **SaaS Dependency** | ❌ None (Standard Mode) | ⚠️ Required | ⚠️ Required | ⚠️ Required |

---

## 🛠️ CLI Reference

### `demonstra init`
Scaffolds workflow templates and visual interaction helpers.
```bash
demonstra init [--dir .] [--framework playwright|cypress|generic] [--force]
```

### `demonstra record`
Executes an E2E test command, captures visual artifacts, encodes WebP/MP4/GIF, and posts the PR comment.
```bash
demonstra record \
  --results-dir ./test-results \
  --report-file ./test-results/results.json \
  --branch demonstra-assets \
  --strict \
  -- npx playwright test
```

### `demonstra publish`
Publishes existing test results to GitHub without running a test command.
```bash
demonstra publish \
  --results-dir ./test-results \
  --repo owner/repo \
  --pr 42 \
  --sha 7437a83 \
  --token $GITHUB_TOKEN
```

---

## 💻 Local Development

### Requirements
- Go 1.22+
- FFmpeg (`brew install ffmpeg` on macOS, `sudo apt-get install ffmpeg` on Ubuntu)

### Building & Testing
```bash
# Clone the repository
git clone https://github.com/demonstra-me/demonstra.git
cd demonstra

# Run all unit tests with race detector
go test -v -race ./...

# Build binary
go build -o bin/demonstra ./cmd/demonstra

# Test CLI
./bin/demonstra --version
./bin/demonstra init --help
```

---

## 📄 License

MIT © [demonstra.me](https://demonstra.me)
