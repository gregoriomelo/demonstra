# demonstra.me — Playwright + GitHub Actions Quickstart

## 1. Enable Video Recording in `playwright.config.ts`

```typescript
import { defineConfig } from '@playwright/test';

export default defineConfig({
  reporter: [['list'], ['json', { outputFile: 'test-results/results.json' }]],
  use: {
    video: 'on',
    screenshot: 'on',
  },
});
```

## 2. Add `demonstra` to Your GitHub Actions Workflow

```yaml
name: E2E & Visual Review

on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  e2e:
    runs-on: ubuntu-latest
    permissions:
      contents: write        # Required to store GIFs in the demonstra-assets branch
      pull-requests: write   # Required to post/update the PR visual review comment

    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 20

      - run: npm ci
      - run: npx playwright install --with-deps chromium

      - name: Run Playwright with demonstra.me
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          demonstra record \
            --results-dir ./test-results \
            --report-file ./test-results/results.json \
            -- npx playwright test
```
