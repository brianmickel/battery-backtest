import { expect, test } from '@playwright/test'
import { mockMetadata } from './api'

test('boots the application with isolated metadata', async ({ page }) => {
  await mockMetadata(page)
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Battery Backtest Engine', exact: true })).toBeVisible()
  await expect(page.getByRole('option', { name: /Synthetic battery/ })).toHaveCount(1)
})
