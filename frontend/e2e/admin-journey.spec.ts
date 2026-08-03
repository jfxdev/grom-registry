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
  await page.getByLabel('Name').press('Tab')
  await expect(page.getByLabel('Slug')).toHaveValue('alpha')
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByRole('link', { name: /Alpha/ })).toBeVisible()

  await page.getByRole('link', { name: 'Service accounts' }).click()
  await page.getByRole('button', { name: 'New account' }).click()
  await page.getByLabel('Display name').fill('Alpha writer')
  await page.getByLabel('Username').fill('alpha-writer')
  await page.getByLabel('Description').fill('First-push acceptance test')
  await page.getByRole('button', { name: 'Create' }).click()
  const account = page.locator('.account-item', { hasText: 'alpha-writer' })
  await expect(account).toBeVisible()
  const keysPanel = account.locator('.keys-panel')
  if (!await keysPanel.isVisible()) {
    await account.getByRole('button', { name: 'Keys' }).click()
  }
  await expect(keysPanel).toBeVisible()
  await keysPanel.getByRole('button', { name: 'New key' }).click()
  await keysPanel.getByLabel('Key name').fill('First push')
  await keysPanel.getByRole('button', { name: 'Create key' }).click()
  const secret = await page.locator('.secret-value code').textContent()
  expect(secret).toBeTruthy()
  await page.getByRole('button', { name: 'Close revealed key' }).click()

  await page.goto(`${runtime.publicURL}/projects/alpha`)
  await expect(page.getByRole('heading', { name: 'Alpha', exact: true })).toBeVisible()
  await page.getByRole('button', { name: /Members/ }).click()
  await page.getByRole('button', { name: 'Add service account' }).click()
  const memberDialog = page.getByRole('dialog', { name: 'Add service account' })
  await memberDialog.getByLabel('Principal', { exact: true }).selectOption({ label: 'Alpha writer · alpha-writer' })
  await memberDialog.locator('select').last().selectOption('writer')
  const [memberResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().includes('/members/service_account/')),
    memberDialog.getByRole('button', { name: 'Add member' }).click(),
  ])
  expect(memberResponse.url()).toContain('/api/v1/projects/alpha/members/service_account/')
  expect(await memberResponse.json()).toEqual({ status: 'saved' })
  await expect(memberDialog).toBeHidden()
  await expect(page.locator('.data-row').filter({ hasText: 'service account' }).getByText('writer', { exact: true })).toBeVisible()

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
