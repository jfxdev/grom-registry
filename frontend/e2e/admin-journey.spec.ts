import { expect, test } from '@playwright/test'
import { ADMIN_E2E_ADMIN } from './support/admin-stack'
import { pushFirstImage } from './support/docker'
import { readRuntime } from './support/runtime'

test('an administrator completes the first-push journey through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await page.goto(`${runtime.publicURL}/signin`)
  await page.getByLabel('Email').fill(ADMIN_E2E_ADMIN.email)
  await page.getByRole('textbox', { name: 'Password' }).fill(ADMIN_E2E_ADMIN.password)
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
  const newKey = page.getByRole('button', { name: 'New key' })
  if (!await newKey.isVisible()) {
    await page.getByRole('button', { name: /Keys/ }).click()
  }
  await newKey.click()
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

  const { digest, cleanup } = await pushFirstImage(runtime, 'alpha-writer', secret!)
  try {
    await page.getByRole('button', { name: /Repositories/ }).click()
    await expect(page.getByRole('button', { name: /app/ })).toBeVisible()
    await page.getByRole('button', { name: /app/ }).click()
    await expect(page.getByText('v1', { exact: true })).toBeVisible()
    await expect(page.getByRole('heading', { name: 'Manifest inventory' })).toBeVisible()
    await page.locator('.operation-history [role="button"]').first().click()
    const manifestDialog = page.getByRole('dialog', { name: 'Manifest details' })
    await expect(manifestDialog).toContainText('v1')
    await expect(manifestDialog).toContainText(digest)
  } finally {
    await cleanup()
  }
})
