import { defineConfig, devices } from '@playwright/test'

const configuredPort = process.env.GROM_PLAYWRIGHT_PORT
const port = configuredPort === undefined ? 4173 : Number(configuredPort)
const adminE2E = process.env.GROM_RUN_ADMIN_E2E === '1'

if (configuredPort === '' || !Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('GROM_PLAYWRIGHT_PORT must be an integer between 1 and 65535')
}

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  // The public administrative suite shares one isolated Compose stack and a
  // bootstrap administrator, so its files must not mutate it concurrently.
  ...(adminE2E ? { workers: 1 } : {}),
  ...(adminE2E ? { timeout: 5 * 60_000 } : {}),
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: adminE2E ? 'off' : 'retain-on-failure',
    screenshot: adminE2E ? 'off' : 'only-on-failure',
    video: adminE2E ? 'off' : 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  globalSetup: adminE2E ? './e2e/support/admin-stack.ts' : undefined,
  webServer: adminE2E
    ? undefined
    : {
        command: `npm run dev -- --host 127.0.0.1 --port ${port} --strictPort`,
        port,
        reuseExistingServer: !process.env.CI,
      },
})
