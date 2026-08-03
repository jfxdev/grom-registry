import { expect, test } from '@playwright/test'

test('sign-in reports an invalid credential without navigating', async ({ page }) => {
  await page.route('**/api/v1/deployment', (route) => route.fulfill({ json: { profile: 'development', warning: '' } }))
  await page.route('**/api/v1/session', (route) => {
    if (route.request().method() === 'POST') {
      return route.fulfill({ status: 401, json: { code: 'invalid_credentials', message: 'Invalid email or password' } })
    }
    return route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Unauthenticated' } })
  })
  await page.route('**/api/v1/me', (route) => route.fulfill({ status: 401, json: { code: 'unauthenticated', message: 'Unauthenticated' } }))
  await page.goto('/signin')
  await page.getByLabel('Email').fill('admin@example.com')
  await page.getByRole('textbox', { name: 'Password' }).fill('wrong-password')
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page.getByRole('alert')).toHaveText(/Invalid email or password/)
  await expect(page).toHaveURL(/\/signin$/)
})
