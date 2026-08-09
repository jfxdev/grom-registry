import { expect, test, type Browser, type Page } from '@playwright/test'
import { ADMIN_E2E_ADMIN } from './support/admin-stack'
import { pushImage } from './support/docker'
import { readRuntime } from './support/runtime'

test.describe.configure({ mode: 'serial' })

test('an administrator removes access, revokes a key, and disables a service account through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)
  const account = await createProjectWithWriter(page, runtime.publicURL, 'Access changes', 'access-changes', 'Access writer', 'access-writer')

  await page.goto(`${runtime.publicURL}/projects/access-changes`)
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
  const accountRow = page.locator('.account-item', { hasText: account.username })
  const keys = accountRow.locator('.keys-panel')
  if (!await keys.isVisible()) await accountRow.getByRole('button', { name: 'Keys' }).click()
  await expect(keys).toBeVisible()
  await expect(keys.getByText('Acceptance key', { exact: true })).toBeVisible()
  const [revokeResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && response.url().includes('/tokens/')),
    keys.getByRole('button', { name: 'Revoke access key' }).click(),
  ])
  expect(revokeResponse.status()).toBe(204)
  await expect(keys.getByText('Revoked', { exact: true })).toBeVisible()

  await accountRow.getByRole('button', { name: `Disable service account ${account.name}` }).click()
  const disableForm = page.getByRole('form', { name: 'Disable service account' })
  await expect(disableForm.getByRole('button', { name: 'Disable service account' })).toBeDisabled()
  await disableForm.getByLabel('Service account name confirmation').fill(account.name)
  const [disableResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && /\/service-accounts\//.test(response.url())),
    disableForm.getByRole('button', { name: 'Disable service account' }).click(),
  ])
  expect(disableResponse.status()).toBe(204)
  await page.getByRole('combobox', { name: 'Filter service accounts by status' }).selectOption('disabled')
  await expect(accountRow.getByText('Disabled', { exact: true })).toBeVisible()
})

test('an administrator reviews and deletes an artifact through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)
  const account = await createProjectWithWriter(page, runtime.publicURL, 'Artifact deletion', 'artifact-deletion', 'Artifact writer', 'artifact-writer')
  await createEmptyRepository(page, runtime.publicURL, 'artifact-deletion', 'app')
  const pushed = await pushImage(runtime, account.username, account.secret, { project: 'artifact-deletion', repository: 'app', tag: 'v1' })
  try {
    await openRepository(page, runtime.publicURL, 'artifact-deletion', 'app')
    await page.getByRole('button', { name: 'Delete v1' }).click()
    const deletion = page.getByRole('dialog', { name: 'Delete artifact' })
    await expect(deletion).toContainText(pushed.digest)
    await expect(deletion).toContainText('v1')
    const [response] = await Promise.all([
      page.waitForResponse((candidate) => candidate.request().method() === 'POST' && candidate.url().includes('/artifact-deletions')),
      deletion.getByRole('button', { name: 'Delete artifact' }).click(),
    ])
    expect(response.status()).toBe(200)
    await expect(async () => {
      await page.reload()
      await expect(page.getByRole('heading', { name: 'app', exact: true })).toBeVisible({ timeout: 1_000 })
      await expect(page.getByRole('button', { name: 'Delete v1' })).toBeHidden({ timeout: 1_000 })
    }).toPass({ timeout: 30_000 })
  } finally {
    await pushed.cleanup()
  }
})

test('an administrator executes a reviewed retention lifecycle through the public UI', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)
  const account = await createProjectWithWriter(page, runtime.publicURL, 'Lifecycle', 'lifecycle', 'Lifecycle writer', 'lifecycle-writer')
  await createEmptyRepository(page, runtime.publicURL, 'lifecycle', 'app')
  const first = await pushImage(runtime, account.username, account.secret, { project: 'lifecycle', repository: 'app', tag: 'v1', variant: 'a' })
  const second = await pushImage(runtime, account.username, account.secret, { project: 'lifecycle', repository: 'app', tag: 'v2', variant: 'b' })
  try {
    await openRepository(page, runtime.publicURL, 'lifecycle', 'app')
    await page.getByRole('button', { name: 'Policies' }).click()
    const policies = page.getByRole('dialog', { name: 'Policies for app' })
    await policies.locator('select').selectOption('retention')
    await policies.getByRole('button', { name: 'Add policy' }).click()
    await policies.getByLabel('Expire after days').fill('')
    await policies.getByLabel('Keep last').fill('1')
    const [policyResponse] = await Promise.all([
      page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().includes('/policies')),
      policies.getByRole('button', { name: 'Save policies' }).click(),
    ])
    expect(policyResponse.status()).toBe(200)
    await page.getByRole('button', { name: 'Review lifecycle' }).click()
    const preview = page.getByRole('dialog', { name: 'app' })
    await expect(preview).toContainText('1 eligible')
    const runButton = preview.getByRole('button', { name: /Delete \d+ eligible/ })
    await expect(runButton).toBeDisabled()
    await preview.getByLabel('Execution reason').fill('Administrative acceptance test')
    await expect(runButton).toBeEnabled()
    const [runResponse] = await Promise.all([
      page.waitForResponse((response) => response.request().method() === 'POST' && response.url().includes('/lifecycle-runs')),
      runButton.click(),
    ])
    expect(runResponse.status()).toBe(200)
    await expect(preview).toContainText('Run completed')
    await expect(page.getByText('Lifecycle · 1 candidates', { exact: true })).toBeVisible()
  } finally {
    await first.cleanup()
    await second.cleanup()
  }
})

test('an administrator archives/removes empty repositories and observes project deletion conflicts', async ({ page }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)
  await createProject(page, runtime.publicURL, 'Repository removal', 'repository-removal')
  await createEmptyRepository(page, runtime.publicURL, 'repository-removal', 'empty')
  await openRepository(page, runtime.publicURL, 'repository-removal', 'empty')
  const [archiveResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/archive')),
    page.getByRole('button', { name: 'Archive' }).click(),
  ])
  expect(archiveResponse.status()).toBe(204)
  await page.getByRole('button', { name: 'Remove logical record' }).click()
  const removeDialog = page.getByRole('dialog', { name: 'Remove logical repository' })
  const [removeResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && response.url().includes('/repositories/')),
    removeDialog.getByRole('button', { name: 'Remove record' }).click(),
  ])
  expect(removeResponse.status()).toBe(204)
  await expect(page.getByText('Repository not found')).toBeVisible()

  await createEmptyRepository(page, runtime.publicURL, 'repository-removal', 'still-present')
  await page.goto(`${runtime.publicURL}/projects/repository-removal`)
  await page.getByRole('button', { name: 'Project settings' }).click()
  await page.getByRole('button', { name: 'Danger zone' }).click()
  await page.getByRole('button', { name: 'Delete project' }).click()
  const projectDialog = page.getByRole('dialog', { name: 'Delete project' })
  const [conflictResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && response.url().endsWith('/api/v1/projects/repository-removal')),
    projectDialog.getByRole('button', { name: 'Delete project' }).click(),
  ])
  expect(conflictResponse.status()).toBe(409)
  await expect(projectDialog).toContainText(/repository|repositories/i)
  await expect(page.getByRole('heading', { name: 'Repository removal', exact: true })).toBeVisible()
})

test('an administrator disables a live user session and deletes a recovery point through the public UI', async ({ page, browser }) => {
  const runtime = await readRuntime()
  await signIn(page, runtime.publicURL)
  const username = 'disabled-e2e'
  await page.getByRole('link', { name: 'Users' }).click()
  await page.getByRole('button', { name: 'New user' }).click()
  await page.getByLabel('Email').fill('disabled-e2e@grom.local')
  await page.getByLabel('Username').fill(username)
  await page.getByRole('button', { name: 'Create user' }).click()
  const registrationURL = await page.locator('.reveal-value code').textContent()
  expect(registrationURL).toBeTruthy()
  await page.getByRole('button', { name: 'Close registration link' }).click()
  const userPage = await completeRegistration(browser, registrationURL!)
  try {
    await expect(userPage.getByRole('heading', { name: 'Projects', exact: true })).toBeVisible()
    await page.reload()
    const disableUser = page.getByRole('button', { name: `Disable user ${username}` })
    await expect(disableUser).toBeVisible()
    await disableUser.click()
    const [disableResponse] = await Promise.all([
      page.waitForResponse((response) => response.request().method() === 'DELETE' && /\/api\/v1\/users\//.test(response.url())),
      page.getByRole('form', { name: `Disable ${username}` }).getByRole('button', { name: 'Disable user', exact: true }).click(),
    ])
    expect(disableResponse.status()).toBe(204)
    await userPage.goto(`${runtime.publicURL}/projects`)
    await expect(userPage).toHaveURL(/signin/)
    await expect(page.getByLabel('Inactive user')).toBeVisible()
  } finally {
    await userPage.context().close()
  }

  await page.goto(`${runtime.publicURL}/backups`)
  await page.getByRole('button', { name: 'Create backup' }).click()
  const [backupResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && response.url().endsWith('/api/v1/backups')),
    page.getByRole('button', { name: 'Begin backup' }).click(),
  ])
  expect(backupResponse.status()).toBe(202)
  const backupRow = page.locator('.backup-row').filter({ has: page.getByRole('button', { name: /Delete backup/ }) }).first()
  await expect(backupRow).toBeVisible({ timeout: 90_000 })
  await backupRow.getByRole('button', { name: /Delete backup/ }).click()
  const backupDeletion = page.getByRole('form', { name: /Delete this recovery point/ })
  await expect(backupDeletion.getByRole('button', { name: 'Delete snapshot' })).toBeDisabled()
  await backupDeletion.getByLabel('Type DELETE to confirm').fill('DELETE')
  const [backupDeleteResponse] = await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'DELETE' && /\/api\/v1\/backups\//.test(response.url())),
    backupDeletion.getByRole('button', { name: 'Delete snapshot' }).click(),
  ])
  expect(backupDeleteResponse.status()).toBe(204)
  await expect(backupRow).toBeHidden()
})

async function signIn(page: Page, publicURL: string) {
  await page.goto(`${publicURL}/signin`)
  await page.getByLabel('Email').fill(ADMIN_E2E_ADMIN.email)
  await page.getByRole('textbox', { name: 'Password' }).fill(ADMIN_E2E_ADMIN.password)
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('heading', { name: 'Projects', exact: true })).toBeVisible()
}

async function createProject(page: Page, publicURL: string, name: string, slug: string) {
  await page.goto(`${publicURL}/projects`)
  await page.getByRole('button', { name: 'New project' }).click()
  await page.getByLabel('Name').fill(name)
  await page.getByLabel('Name').blur()
  await page.getByLabel('Slug').fill(slug)
  await page.getByRole('button', { name: 'Create project' }).click()
  await expect(page.getByRole('link', { name: new RegExp(name) })).toBeVisible()
}

async function createProjectWithWriter(page: Page, publicURL: string, name: string, slug: string, accountName: string, username: string) {
  await createProject(page, publicURL, name, slug)
  await page.getByRole('link', { name: 'Service accounts' }).click()
  await page.getByRole('button', { name: 'New account' }).click()
  await page.getByLabel('Display name').fill(accountName)
  await page.getByLabel('Username').fill(username)
  await page.getByLabel('Description').fill('Administrative acceptance test')
  await page.getByRole('button', { name: 'Create' }).click()
  const row = page.locator('.account-item', { hasText: username })
  await expect(row).toBeVisible()
  const panel = row.locator('.keys-panel')
  if (!await panel.isVisible()) await row.getByRole('button', { name: 'Keys' }).click()
  await expect(panel).toBeVisible()
  await panel.getByRole('button', { name: 'New key' }).click()
  await panel.getByLabel('Key name').fill('Acceptance key')
  await panel.getByRole('button', { name: 'Create key' }).click()
  const secret = await panel.locator('.secret-value code').textContent()
  expect(secret).toBeTruthy()
  await panel.getByRole('button', { name: 'Close revealed key' }).click()
  await page.goto(`${publicURL}/projects/${slug}`)
  await page.getByRole('button', { name: 'Project settings' }).click()
  await page.getByRole('button', { name: 'Add member' }).click()
  const dialog = page.getByRole('dialog', { name: 'Add service account' })
  const selects = dialog.locator('select')
  await selects.nth(1).selectOption({ label: `${accountName} · ${username}` })
  await selects.nth(2).selectOption('writer')
  await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'PUT' && response.url().includes('/members/service_account/')),
    dialog.getByRole('button', { name: 'Add member' }).click(),
  ])
  return { name: accountName, username, secret: secret! }
}

async function createEmptyRepository(page: Page, publicURL: string, project: string, name: string) {
  await page.goto(`${publicURL}/projects/${project}`)
  await page.getByRole('button', { name: 'New repository' }).click()
  const dialog = page.getByRole('dialog', { name: 'Create repository' })
  await dialog.getByLabel('Path').fill(name)
  await Promise.all([
    page.waitForResponse((response) => response.request().method() === 'POST' && response.url().includes('/repositories')),
    dialog.getByRole('button', { name: 'Create repository' }).click(),
  ])
}

async function openRepository(page: Page, publicURL: string, project: string, repository: string) {
  await page.goto(`${publicURL}/projects/${project}`)
  const repositoryButton = page.getByRole('button', { name: new RegExp(`^${repository}(?:\\s|$)`) })
  await expect(async () => {
    await page.reload()
    await expect(repositoryButton).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 30_000 })
  await repositoryButton.click()
  await expect(page.getByRole('heading', { name: repository, exact: true })).toBeVisible()
}

async function completeRegistration(browser: Browser, registrationURL: string) {
  const context = await browser.newContext()
  const page = await context.newPage()
  await page.goto(registrationURL)
  await page.getByRole('textbox', { name: 'New password', exact: true }).fill('disabled-e2e-password')
  await page.getByLabel('Confirm new password').fill('disabled-e2e-password')
  await page.getByRole('button', { name: 'Reset password' }).click()
  await page.getByRole('link', { name: 'Continue to sign in' }).click()
  await page.getByLabel('Email').fill('disabled-e2e@grom.local')
  await page.getByRole('textbox', { name: 'Password' }).fill('disabled-e2e-password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  return page
}
