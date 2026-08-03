import { expect, test } from '@playwright/test'
import { pushFirstImage } from './support/docker'
import { readRuntime } from './support/runtime'

const adminEmail = 'admin-e2e@grom.local'
const adminPassword = 'admin-e2e-password'

test('an administrator completes the first-push journey through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await page.goto(`${runtime.publicURL}/signin`)
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByRole('textbox', { name: 'Password' }).fill(adminPassword)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Projects', exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'New project' }).click()
  await page.getByLabel('Name').fill('Alpha')
  await page.getByLabel('Slug').fill('alpha')
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByRole('link', { name: /Alpha/ })).toBeVisible()

  await page.getByRole('link', { name: 'Service accounts' }).click()
  await page.getByRole('button', { name: 'New account' }).click()
  await page.getByLabel('Display name').fill('Alpha writer')
  await page.getByLabel('Username').fill('alpha-writer')
  await page.getByLabel('Description').fill('First-push acceptance test')
  await page.getByRole('button', { name: 'Create' }).click()
  await page.getByRole('button', { name: /Keys/ }).click()
  await page.getByRole('button', { name: 'New key' }).click()
  await page.getByLabel('Key name').fill('First push')
  await page.getByRole('button', { name: 'Create key' }).click()
  const secret = await page.locator('.secret-value code').textContent()
  expect(secret).toBeTruthy()
  await page.getByRole('button', { name: 'Close revealed key' }).click()

  await page.goto(`${runtime.publicURL}/projects/alpha`)
  await page.getByRole('button', { name: /Members/ }).click()
  await page.getByRole('button', { name: 'Add service account' }).click()
  const memberDialog = page.getByRole('dialog', { name: 'Add service account' })
  await memberDialog.locator('label').filter({ hasText: 'Principal' }).locator('select').selectOption({ label: 'Alpha writer · alpha-writer' })
  await memberDialog.locator('label').filter({ hasText: 'Role' }).locator('select').selectOption('writer')
  await memberDialog.getByRole('button', { name: 'Add member' }).click()
  await expect(page.getByText('writer')).toBeVisible()

  const cleanup = await pushFirstImage(runtime, 'alpha-writer', secret!)
  try {
    await page.getByRole('button', { name: /Repositories/ }).click()
    await expect(page.getByRole('button', { name: /app/ })).toBeVisible()
    await page.getByRole('button', { name: /app/ }).click()
    await expect(page.getByText('v1', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Manifest inventory' })).toBeVisible()
    await page.locator('.operation-history [role="button"]').first().click()
    await expect(page.getByRole('dialog', { name: 'Manifest details' })).toContainText('v1')
  } finally {
    await cleanup()
  }
})
