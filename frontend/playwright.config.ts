import { defineConfig, devices } from '@playwright/test'

const configuredPort = process.env.GROM_PLAYWRIGHT_PORT
const port = configuredPort === undefined ? 4173 : Number(configuredPort)

if (configuredPort === '' || !Number.isInteger(port) || port < 1 || port > 65535) {
  throw new Error('GROM_PLAYWRIGHT_PORT must be an integer between 1 and 65535')
}

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: `http://127.0.0.1:${port}`,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${port} --strictPort`,
    port,
    reuseExistingServer: !process.env.CI,
  },
})
