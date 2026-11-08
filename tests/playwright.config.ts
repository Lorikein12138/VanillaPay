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
const globalAnnouncement = /@global-announcement/
const publicSiteSettings = /@public-site-settings/

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
  projects: [
    ...viewports.map(({ name, width, height }) => ({
      name,
      testIgnore: '**/console/**',
      use: { browserName: 'chromium' as const, viewport: { width, height } },
    })),
    {
      // Existing console settings specs also write global branding and security.
      // Run them once, after the public and merchant viewport projects finish.
      name: 'console',
      testMatch: '**/console/console.spec.ts',
      grepInvert: [globalAnnouncement, publicSiteSettings],
      dependencies: viewports.map(({ name }) => name),
      use: { browserName: 'chromium', viewport: { width: 1280, height: 800 } },
    },
    {
      // A real platform popup also reaches other logged-in merchants. Run this
      // case after the shared-database projects have finished.
      name: 'console-announcements',
      testMatch: '**/console/console.spec.ts',
      grep: globalAnnouncement,
      dependencies: ['console'],
      use: { browserName: 'chromium', viewport: { width: 1280, height: 800 } },
    },
    {
      // Public site settings also affect every visitor. Run after the announcement project and its dependencies.
      name: 'public-site-settings',
      testMatch: '**/console/console.spec.ts',
      grep: publicSiteSettings,
      dependencies: ['console-announcements'],
      use: { browserName: 'chromium', viewport: { width: 1280, height: 800 } },
    },
  ],
} satisfies PlaywrightTestConfig
