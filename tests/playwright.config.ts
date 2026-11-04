// Playwright config: Chromium at the three required viewport widths.
// E2E_BASE_URL points at a running server (default: local Go server).
import type { PlaywrightTestConfig } from '@playwright/test'

// Playwright sets FORCE_COLOR for its workers; an inherited NO_COLOR makes Node warn in every worker.
delete process.env.NO_COLOR

const viewports = [
  { name: 'chromium-360', width: 360, height: 780 },
  { name: 'chromium-768', width: 768, height: 1024 },
  { name: 'chromium-1280', width: 1280, height: 800 },
]

export default {
  testDir: './e2e',
  outputDir: '../.artifacts/playwright/results',
  forbidOnly: true,
  fullyParallel: true,
  reporter: [['list'], ['html', { outputFolder: '../.artifacts/playwright/report', open: 'never' }]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: viewports.map(({ name, width, height }) => ({
    name,
    use: { browserName: 'chromium', viewport: { width, height } },
  })),
} satisfies PlaywrightTestConfig
