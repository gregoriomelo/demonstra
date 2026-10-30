# demonstra.me — Product Roadmap, Competitive Landscape & Monetization

**Date:** 2026-10-30  
**Status:** Living Product Strategy Document

---

## 1. Product Vision

**demonstra.me** makes reviewing and approving pull requests effortless for distributed teams and AI-assisted development workflows. When a PR is opened or updated, `demonstra.me` runs in CI alongside the application, captures visual recordings (animated GIFs and step-by-step screenshots) of the affected user flows, stores the visual artifacts in the user's repository for posterity, and posts an interactive visual walkthrough directly as a comment on the GitHub/GitLab pull request.

---

## 2. Three-Phase Product Roadmap

### Project 1: Standard Mode End-to-End (Go CLI + GitHub PR Workflow)
* **Scope:** Strict standalone pipeline with zero external SaaS runtime dependency so it can be used immediately on real GitHub repositories.
* **Capabilities:**
  * Closed-source Go CLI (`demonstra`) that wraps existing E2E test commands (starting with Playwright, extensible to Cypress).
  * Collects recorded `.webm`/`.mp4` videos and `.png` screenshots per test scenario.
  * Converts videos into optimized, crisp animated GIFs using the CI runner's system `ffmpeg` binary (`os/exec`).
  * Commits GIFs and screenshots to an orphan branch (`demonstra-assets`) in the user's own GitHub repository so assets render inline in both public and private repos and are preserved for posterity.
  * Posts/updates a structured Markdown PR comment (**Layout A1**: Scenario Summary Table + Smart Expandable Scenario Cards with inline Hero GIFs and nested step screenshots) and writes to `$GITHUB_STEP_SUMMARY`.

### Project 2: The Rails 8 SaaS Control Plane (`demonstra.me`)
* **Scope:** Commercial SaaS web platform and API built with **Ruby on Rails 8** + PostgreSQL + Stripe.
* **Capabilities:**
  * User, organization, and repository management.
  * API key generation and CLI authentication (`DEMONSTRA_API_KEY`).
  * Run metering, usage quotas, and run metadata history.
  * Stripe subscription billing and credit top-ups.
  * Promotional footer removal for paid tiers.

### Project 3: Pro Mode — Autonomous AI Browser Agent
* **Scope:** Zero-test visual verification powered by AI + `playwright-go`.
* **Capabilities:**
  * When a PR has no Playwright/Cypress test covering the change, the Go CLI extracts the PR context (git diff, PR description, linked GitHub/Jira/Linear issue) and sends it to the Rails AI Agent API.
  * The AI Agent determines the user flow to verify and drives a headless browser in CI via `playwright-go` with video recording enabled.
  * Produces the exact same visual PR comment and repo-stored GIFs as Standard Mode.

---

## 3. Competitive Landscape & Differentiation

| Category | Examples | What They Do | How `demonstra.me` Wins |
|---|---|---|---|
| **Visual Regression Tools** | Chromatic, Percy, Argos CI | Static pixel-by-pixel before/after screenshot diffs on an external dashboard. | Shows animated multi-step user flows (GIFs) directly inside the GitHub PR comment—no external dashboard required. |
| **Cloud E2E Dashboards** | Cypress Cloud, Currents.dev, Replay.io | Cloud storage for test traces/videos aimed at debugging failed CI runs. | Built for human sign-off on *passing* features; $0 cloud video storage cost by storing assets in `demonstra-assets`. |
| **Enterprise AI QA Suites** | Meticulous.ai, Momentic, Octomind | Heavy ($500–$2,000+/mo) platforms aimed at replacing QA teams or test frameworks. | Lightweight, developer-first pricing; works with your existing Playwright tests today and adds AI agent mode when needed. |
| **Emerging PR Video Bots** | PageBolt CI, Screencli | Point an AI browser at a public preview URL (e.g., Vercel) to record an MP4. | Runs inside CI against `localhost` (no public preview URL needed), leverages existing Playwright tests, and stores assets in-repo. |

---

## 4. Monetization Model ("Public Free / Private Paid + AI Credits")

Because video encoding runs in the user's CI and assets live in their repository, **Standard Mode has ~99% gross margin**, while **Pro Mode (AI Agent)** scales with LLM token usage.

### Tier 1: Free ($0 / month)
* **Unlimited Standard Mode runs** on **public** GitHub repositories.
* Every public PR comment includes a subtle *"Recorded with [demonstra.me](https://demonstra.me)"* footer for viral open-source growth.
* Up to **20 Standard Mode PR runs / month** on private repositories.

### Tier 2: Team / Standard ($29 / month per organization)
* **Unlimited Standard Mode runs** on **private** repositories.
* Removes promotional branding footer from PR comments.
* Run history and team management in the Rails SaaS dashboard.
* Includes **15 Pro AI Agent runs / month** so teams can experience autonomous PR recording.

### Tier 3: Pro / Agentic ($99 / month per organization)
* Everything in Team / Standard.
* **150 Pro AI Agent runs / month** included (autonomous flow discovery and recording from PR diffs/issues).
* **Overages / Credit Packs:** **$0.50–$1.00 per additional AI Agent run** so heavy agent usage always remains profitable.

### Future Enterprise Upsells ($199–$499 / month)
* **Visual Approval Gate:** GitHub Required Check (`demonstra/visual-signoff`) blocking PR merge until a reviewer/PM approves the recorded visual flow.
* **AI Executive Changelog:** Plain-English visual summary next to each scenario GIF for product managers.
* **Managed Cloud HD Video Hosting:** Optional Cloudflare R2 / S3 hosting for 60fps MP4 videos when organizations prefer not to store assets in a git branch.
