import { expect, test } from '@playwright/test'
import { ADMIN_E2E_ADMIN } from './support/admin-stack'
import { pushFirstImage } from './support/docker'
import { readRuntime } from './support/runtime'

test('an administrator completes the first-push journey through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)

  const secret = await createProjectWithWriter(page, runtime.publicURL, 'Alpha', 'alpha', 'Alpha writer', 'alpha-writer')

  const { digest, cleanup } = await pushFirstImage(runtime, 'alpha-writer', secret!)
  try {
    await page.reload()
    await expect(page.getByRole('button', { name: /app/ })).toBeVisible()
    const projectAccounting = page.locator('header p').filter({ hasText: 'Accounted registry usage:' })
    await expect(projectAccounting).toContainText('Accounted registry usage:')
    await expect(projectAccounting).not.toContainText('Accounting pending')
    await expect(projectAccounting).not.toContainText('Accounting unavailable')
    await expect(projectAccounting).not.toContainText('stale')
    await expect(projectAccounting).toContainText(/\b\d+(?:[.,]\d+)?\s(?:B|KB|MB|GB)\b/)
    expect(parseDisplayedUsageBytes(await projectAccounting.textContent())).toBeGreaterThan(0)
    await page.getByRole('button', { name: /app/ }).click()
    const repositoryAccounting = page.locator('[aria-labelledby="repository-accounting-title"]')
    await expect(repositoryAccounting).toContainText('Accounted usage')
    await expect(repositoryAccounting).not.toContainText('Accounting pending')
    await expect(repositoryAccounting).not.toContainText('Accounting unavailable')
    await expect(repositoryAccounting).not.toContainText('stale')
    await expect(repositoryAccounting).toContainText(/\d+(?:[.,]\d+)?\s(?:B|KB|MB|GB)/)
    expect(parseDisplayedUsageBytes(await repositoryAccounting.textContent())).toBeGreaterThan(0)
    await expect(page.getByText('v1', { exact: true })).toBeVisible()
    const manifestInventory = page.locator('.accordion').filter({ hasText: 'Manifest inventory' })
    await expect(manifestInventory).toBeVisible()
    await manifestInventory.getByRole('button').filter({ hasText: digest }).click()
    const manifestDialog = page.getByRole('dialog', { name: 'Manifest details' })
    await expect(manifestDialog).toContainText('v1')
    await expect(manifestDialog).toContainText(digest)
  } finally {
    await cleanup()
  }
})

function parseDisplayedUsageBytes(text: string | null): number {
  if (!text) throw new Error('accounting text was not rendered')
  const match = text.match(/(\d+(?:[.,]\d+)?)\s(B|KB|MB|GB)/)
  if (!match) throw new Error(`accounting text has no byte value: ${text}`)
  const multipliers: Record<string, number> = { B: 1, KB: 1024, MB: 1024 ** 2, GB: 1024 ** 3 }
  return Number(match[1].replace(',', '.')) * multipliers[match[2]]
}

test('an administrator confirms destructive access changes through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)

  await createProjectWithWriter(page, runtime.publicURL, 'Destructive', 'destructive', 'Destructive writer', 'destructive-writer')
  await page.goto(`${runtime.publicURL}/projects/destructive`)
  await expect(page.getByRole('heading', { name: 'Destructive', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Project settings' }).click()
  await page.getByRole('button', { name: 'Remove service account member' }).click()
  const memberDialog = page.getByRole('dialog', { name: 'Remove member' })
  await expect(memberDialog).toContainText('loses project access')
  const [memberResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && response.url().includes('/members/service_account/')),
    memberDialog.getByRole('button', { name: 'Remove member' }).click(),
  ])
  expect(memberResponse.status()).toBe(204)
  await expect(page.getByRole('button', { name: 'Remove service account member' })).toBeHidden()
  await page.getByRole('button', { name: 'Close project settings' }).click()
  await expect(page.getByRole('dialog', { name: 'Project settings' })).toBeHidden()

  await page.getByRole('link', { name: 'Service accounts' }).click()
  const account = page.locator('.account-item', { hasText: 'destructive-writer' })
  await account.getByRole('button', { name: 'Disable service account Destructive writer' }).click()
  const disableDialog = page.getByRole('form', { name: 'Disable service account' })
  await expect(disableDialog.getByRole('button', { name: 'Disable service account' })).toBeDisabled()
  await disableDialog.getByLabel('Service account name confirmation').fill('Destructive writer')
  const [disableResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && /\/service-accounts\//.test(response.url())),
    disableDialog.getByRole('button', { name: 'Disable service account' }).click(),
  ])
  expect(disableResponse.status()).toBe(204)
  await page.getByRole('combobox', { name: 'Filter service accounts by status' }).selectOption('disabled')
  await expect(account.getByText('Disabled', { exact: true })).toBeVisible()

  await page.goto(`${runtime.publicURL}/projects/destructive`)
  await page.getByRole('button', { name: 'Project settings' }).click()
  await page.getByRole('button', { name: /Danger zone/ }).click()
  await page.getByRole('button', { name: 'Delete project' }).click()
  const projectDialog = page.getByRole('dialog', { name: 'Delete project' })
  const [projectResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && response.url().endsWith('/api/v1/projects/destructive')),
    projectDialog.getByRole('button', { name: 'Delete project' }).click(),
  ])
  expect(projectResponse.status()).toBe(204)
  await expect(page.getByRole('heading', { name: 'Projects', exact: true })).toBeVisible()
})

async function signIn(page: import('@playwright/test').Page, publicURL: string) {
  await page.goto(`${publicURL}/signin`)
  await page.getByLabel('Email').fill(ADMIN_E2E_ADMIN.email)
  await page.getByRole('textbox', { name: 'Password' }).fill(ADMIN_E2E_ADMIN.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Projects', exact: true })).toBeVisible()
}

async function createProjectWithWriter(page: import('@playwright/test').Page, publicURL: string, projectName: string, slug: string, accountName: string, username: string) {
  await page.goto(`${publicURL}/projects`)
  await page.getByRole('button', { name: 'New project' }).click()
  await page.getByLabel('Name').fill(projectName)
  const nameInput = page.getByLabel('Name')
  const slugInput = page.getByLabel('Slug')
  await nameInput.blur()
  await expect(slugInput).toHaveValue(slug)
  await slugInput.fill(slug)
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByRole('link', { name: new RegExp(projectName) })).toBeVisible()

  await page.getByRole('link', { name: 'Service accounts' }).click()
  await page.getByRole('button', { name: 'New account' }).click()
  await page.getByLabel('Display name').fill(accountName)
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Description').fill('Administrative acceptance test')
  await page.getByRole('button', { name: 'Create' }).click()
  const account = page.locator('.account-item', { hasText: username })
  await expect(account).toBeVisible()
  const keysPanel = account.locator('.keys-panel')
  if (!await keysPanel.isVisible()) await account.getByRole('button', { name: 'Keys' }).click()
  await keysPanel.getByRole('button', { name: 'New key' }).click()
  await keysPanel.getByLabel('Key name').fill('Acceptance key')
  await keysPanel.getByRole('button', { name: 'Create key' }).click()
  const secret = await page.locator('.secret-value code').textContent()
  expect(secret).toBeTruthy()
  await page.getByRole('button', { name: 'Close revealed key' }).click()

  await page.goto(`${publicURL}/projects/${slug}`)
  await expect(page.getByRole('heading', { name: projectName, exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Project settings' }).click()
  await page.getByRole('button', { name: 'Add member' }).click()
  const memberDialog = page.getByRole('dialog', { name: 'Add service account' })
  const memberSelects = memberDialog.locator('select')
  await memberSelects.nth(1).selectOption({ label: `${accountName} · ${username}` })
  await memberSelects.nth(2).selectOption('writer')
  await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().includes('/members/service_account/')),
    memberDialog.getByRole('button', { name: 'Add member' }).click(),
  ])
  await expect(memberDialog).toBeHidden()
  return secret!
}
