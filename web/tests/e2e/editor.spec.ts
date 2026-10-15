import { expect, test, type Page, type Request, type Route } from '@playwright/test';
import single from '../../../testdata/single-request.json' with { type: 'json' };
import compare from '../../../testdata/compare-request.json' with { type: 'json' };
import { metadata, mockMetadata } from '../fixtures/api';

const sentinel = 'editor-test-header-only-key';
const summary = {
  total_pnl: -10, initial_soc: .5, initial_energy_mwh: 5, final_soc: .7, final_energy_mwh: 7,
  timezone: 'UTC', covered_duration_hours: 2, gap_duration_hours: 1, gap_count: 1, total_intervals: 2,
  backtest_window: { start: '2024-01-01T00:00:00Z', end: '2024-01-01T03:00:00Z' },
  energy_charged_mwh: 2, energy_discharged_mwh: 0,
  charge_windows: [
    { start: '2024-01-01T00:00:00Z', end: '2024-01-01T01:00:00Z', energy_mwh: 1, average_cost_per_mwh: -5 },
    { start: '2024-01-01T02:00:00Z', end: '2024-01-01T03:00:00Z', energy_mwh: 1, average_cost_per_mwh: 15 },
  ],
};

async function mockApp(page: Page, respond?: (route: Route, count: number) => Promise<void>) {
  const requests: Request[] = [];
  await mockMetadata(page);
  await page.route('**/api/v1/backtest**', async route => {
    requests.push(route.request());
    if (respond) { await respond(route, requests.length); return; }
    await route.fulfill({ json: route.request().url().endsWith('/compare')
      ? { comparison: compare.variations.map(variation => ({ name: variation.name, summary })) }
      : { status: 'completed', summary } });
  });
  return requests;
}

async function openApp(page: Page) {
  await page.goto('/');
  await page.getByLabel('Grid Status API Key').fill(sentinel);
}

async function importJson(page: Page, value: unknown) {
  await page.getByRole('radio', { name: 'JSON Input', exact: true }).check();
  await page.getByLabel('Request JSON', { exact: true }).fill(JSON.stringify(value, null, 2));
  await page.getByRole('button', { name: 'Apply JSON', exact: true }).click();
  await expect(page.getByText('Request applied.', { exact: true })).toBeVisible();
}

async function useForm(page: Page) {
  await page.getByRole('radio', { name: 'Form Input', exact: true }).check();
}

test('imported oracle steps survive delayed metadata and submit exactly as shown', async ({ page }) => {
  const requests = await mockApp(page);
  let releaseMetadata: () => void = () => {};
  const metadataReady = new Promise<void>(resolve => { releaseMetadata = resolve; });
  await page.route('**/api/v1/strategies', async route => {
    await metadataReady;
    await route.fulfill({ json: { strategies: metadata.strategies } });
  });
  await openApp(page);
  const imported = { ...single, config: { ...single.config, strategy: { name: 'oracle', params: { soc_steps: 37, power_steps: 3 } } } };
  await importJson(page, imported);
  await useForm(page);
  await expect(page.getByLabel('SOC steps', { exact: true })).toHaveValue('37');
  const metadataResponse = page.waitForResponse('**/api/v1/strategies');
  releaseMetadata();
  await metadataResponse;
  await expect(page.getByLabel('SOC steps', { exact: true })).toHaveValue('37');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests).toHaveLength(1);
  expect(requests[0].postDataJSON()).toEqual(imported);
  expect(requests[0].headers()['x-gridstatus-api-key']).toBe(sentinel);
  expect(requests[0].url()).not.toContain(sentinel);
  expect(requests[0].postData()).not.toContain(sentinel);
  await expect(page.getByText('Data coverage: 2 of 3 hours; 1 hour missing (1 gap).')).toBeVisible();
  await expect(page.getByText('Average Cost:', { exact: true })).toHaveCount(2);
});

test('legacy credentials commit only with a valid import and never enter copied JSON or logs', async ({ page, context }) => {
  const requests = await mockApp(page);
  const consoleMessages: string[] = [];
  page.on('console', message => consoleMessages.push(message.text()));
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  await openApp(page);
  const importedKey = 'valid-import-credential';
  await importJson(page, { ...single, api_key: importedKey });
  await expect(page.getByLabel('Grid Status API Key')).toHaveValue(importedKey);
  await expect(page.getByLabel('Request JSON', { exact: true })).not.toHaveValue(/api_key|valid-import-credential/);
  await page.getByRole('button', { name: 'Copy request JSON', exact: true }).click();
  await expect(page.getByText('Copied to clipboard.', { exact: true })).toBeVisible();
  const copied = await page.evaluate(() => navigator.clipboard.readText());
  expect(copied).not.toContain(importedKey);
  expect(JSON.parse(copied)).toEqual(single);

  const invalid = JSON.stringify({ ...single, api_key: 'invalid-import-credential', config: { ...single.config, battery: { ...single.config.battery, energy_capacity_mwh: -1 } } });
  await page.getByLabel('Request JSON', { exact: true }).fill(invalid);
  await page.getByRole('button', { name: 'Apply JSON', exact: true }).click();
  await expect(page.getByRole('alert')).toBeVisible();
  await expect(page.getByRole('alert')).not.toContainText('invalid-import-credential');
  await expect(page.getByLabel('Grid Status API Key')).toHaveValue(importedKey);
  await expect(page.getByLabel('Request JSON', { exact: true })).toHaveValue(invalid);
  await expect(page.getByRole('button', { name: 'Copy request JSON', exact: true })).toBeDisabled();
  await page.getByRole('radio', { name: 'Form Input', exact: true }).click();
  await expect(page.getByRole('radio', { name: 'JSON Input', exact: true })).toBeChecked();
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  expect(requests).toHaveLength(0);
  await page.getByRole('button', { name: 'Discard JSON edits', exact: true }).click();
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests[0].headers()['x-gridstatus-api-key']).toBe(importedKey);
  expect(requests[0].postDataJSON()).toEqual(single);
  expect(consoleMessages.join('\n')).not.toMatch(/editor-test-header-only-key|valid-import-credential|invalid-import-credential/);
});

test('preset plus explicit zero overrides round-trips through form and JSON', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  const imported = { ...single, config: { battery_file: '1_moss_landing', battery: { initial_soc: 0, degradation_cost_per_mwh: 0 }, strategy: { name: 'schedule', params: { charge_power_mw: 0, discharge_power_mw: 0 } } } };
  await importJson(page, imported);
  await useForm(page);
  await expect(page.getByRole('radio', { name: 'Preset', exact: true })).toBeChecked();
  await expect(page.getByLabel('Initial SOC (0–1)', { exact: true })).toHaveValue('0');
  await expect(page.getByLabel('Degradation cost ($/MWh)', { exact: true })).toHaveValue('0');
  await expect(page.getByLabel('Charge power (MW)', { exact: true })).toHaveValue('0');
  await page.getByRole('radio', { name: 'JSON Input', exact: true }).check();
  expect(JSON.parse(await page.getByLabel('Request JSON', { exact: true }).inputValue())).toEqual(imported);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests[0].postDataJSON()).toEqual(imported);
});

test('comparison stays in JSON mode and preserves variation order', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  await importJson(page, compare);
  await expect(page.getByRole('radio', { name: 'Form Input', exact: true })).toBeDisabled();
  await expect(page.getByText(/Comparison requests stay in JSON mode/)).toBeVisible();
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Comparison Results', exact: true })).toBeVisible();
  expect(requests[0].url()).toContain('/api/v1/backtest/compare');
  expect(requests[0].postDataJSON()).toEqual(compare);
  await expect(page.getByText('Data coverage: 2 of 3 hours; 1 hour missing (1 gap).')).toHaveCount(2);
  await importJson(page, single);
  await expect(page.getByRole('radio', { name: 'Form Input', exact: true })).toBeEnabled();
});

test('incomplete numeric, location, and date text stays visible and blocks requests', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  await importJson(page, single);
  await useForm(page);
  const capacity = page.getByLabel('Energy capacity (MWh)', { exact: false });
  for (const text of ['', '-', '0.']) {
    await capacity.fill(text);
    await capacity.press('Tab');
    await expect(capacity).toHaveValue(text);
    await expect(page.getByRole('button', { name: 'Copy request JSON', exact: true })).toBeDisabled();
    await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
    expect(requests).toHaveLength(0);
  }
  await capacity.fill('2');
  await capacity.press('Tab');
  const location = page.getByLabel('Location ID', { exact: false });
  await location.fill('');
  await location.press('Tab');
  await expect(location).toHaveValue('');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  expect(requests).toHaveLength(0);
  await location.fill('EDITED_NODE');
  await location.press('Tab');
  const end = page.getByLabel('End date (excluded, YYYY-MM-DD)', { exact: false });
  await end.fill('');
  await end.press('Tab');
  await expect(end).toHaveValue('');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  expect(requests).toHaveLength(0);
  await end.fill('2024-01-03');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests[0].postDataJSON()).toMatchObject({ data_source: { location_id: 'EDITED_NODE', end_date: '2024-01-03' }, config: { battery: { energy_capacity_mwh: 2 } } });
});

test('coupled SOC edits commit together after the entire displayed candidate is valid', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  await importJson(page, { ...single, config: { ...single.config, battery: { ...single.config.battery, initial_soc: .5, max_soc: .5 } } });
  await useForm(page);
  const minimum = page.getByLabel('Minimum SOC (0–1)', { exact: false });
  const maximum = page.getByLabel('Maximum SOC (0–1)', { exact: false });
  const initial = page.getByLabel('Initial SOC (0–1)', { exact: true });
  await minimum.fill('0.6');
  await maximum.focus();
  await expect(minimum).toHaveValue('0.6');
  await maximum.fill('0.9');
  await initial.focus();
  await expect(maximum).toHaveValue('0.9');
  await expect(page.getByRole('button', { name: 'Copy request JSON', exact: true })).toBeDisabled();
  await initial.fill('0.7');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests[0].postDataJSON()).toMatchObject({ config: { battery: { min_soc: .6, max_soc: .9, initial_soc: .7 } } });
});

test('invalid edits must be discarded or corrected before strategy, preset, or mode changes', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  await importJson(page, single);
  await useForm(page);
  const capacity = page.getByLabel('Energy capacity (MWh)', { exact: false });
  await capacity.fill('');
  await page.getByLabel('Strategy', { exact: true }).selectOption('schedule');
  await expect(page.getByLabel('Strategy', { exact: true })).toHaveValue('oracle');
  await page.getByRole('radio', { name: 'Preset', exact: true }).click();
  await expect(page.getByRole('radio', { name: 'Custom battery', exact: true })).toBeChecked();
  await page.getByRole('radio', { name: 'JSON Input', exact: true }).click();
  await expect(page.getByRole('radio', { name: 'Form Input', exact: true })).toBeChecked();
  await page.getByRole('button', { name: 'Discard form edits', exact: true }).click();
  await expect(capacity).toHaveValue('1');
  await page.getByLabel('Strategy', { exact: true }).selectOption('schedule');
  await expect(page.getByLabel('Charge start (HH:MM)', { exact: true })).toHaveValue('');
  await page.getByLabel('Strategy', { exact: true }).selectOption('oracle');
  await expect(page.getByLabel('SOC steps', { exact: true })).toHaveValue('200');
  await page.getByRole('radio', { name: 'Preset', exact: true }).check();
  await expect(page.getByRole('radio', { name: 'Preset', exact: true })).toBeChecked();
  expect(requests).toHaveLength(0);
});

test('server field errors retain the request and allow corrected resubmission', async ({ page }) => {
  const requests = await mockApp(page, async (route, count) => {
    await route.fulfill(count === 1
      ? { status: 400, json: { error: { code: 'INVALID_REQUEST', field: 'config.battery.initial_soc', message: 'Initial SOC exceeds this preset’s maximum.' } } }
      : { json: { status: 'completed', summary } });
  });
  await openApp(page);
  await importJson(page, { ...single, config: { battery_file: '1_moss_landing', battery: { initial_soc: .9 }, strategy: single.config.strategy } });
  await useForm(page);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Initial SOC exceeds this preset’s maximum.');
  const initial = page.getByLabel('Initial SOC (0–1)', { exact: true });
  await expect(initial).toHaveValue('0.9');
  await expect(initial).toHaveAttribute('aria-invalid', 'true');
  await page.getByRole('button', { name: /config.battery.initial_soc:/ }).click();
  await expect(initial).toBeFocused();
  await initial.fill('0.5');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests).toHaveLength(2);
  expect(requests[1].postDataJSON()).toMatchObject({ config: { battery: { initial_soc: .5 } } });
});

test('overlap errors focus an actionable schedule field and allow correction', async ({ page }) => {
  const requests = await mockApp(page);
  await openApp(page);
  await importJson(page, { ...single, config: { ...single.config, strategy: { name: 'schedule', params: { charge_start: '10:00', charge_end: '17:00', discharge_start: '17:00', discharge_end: '23:00' } } } });
  await useForm(page);
  const chargeEnd = page.getByLabel('Charge end (HH:MM)', { exact: true });
  await chargeEnd.fill('18:00');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  expect(requests).toHaveLength(0);
  await page.getByRole('button', { name: /Active charge and discharge windows must not overlap/ }).click();
  await expect(chargeEnd).toBeFocused();
  await expect(chargeEnd).toHaveAttribute('aria-invalid', 'true');
  await chargeEnd.fill('17:00');
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(requests).toHaveLength(1);
  expect(requests[0].postDataJSON()).toMatchObject({ config: { strategy: { params: { charge_end: '17:00' } } } });
});

test('clipboard rejection shows feedback without an unhandled rejection', async ({ page }) => {
  await mockApp(page);
  const pageErrors: string[] = [];
  page.on('pageerror', error => pageErrors.push(error.message));
  await page.addInitScript(() => {
    Object.defineProperty(navigator, 'clipboard', { value: { writeText: async () => { throw new Error('Clipboard unavailable'); } } });
  });
  await openApp(page);
  await importJson(page, single);
  await page.getByRole('button', { name: 'Copy request JSON', exact: true }).click();
  await expect(page.getByText('Could not copy. Select the JSON and copy it manually.', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  await page.getByRole('button', { name: 'Copy response JSON', exact: true }).click();
  await expect(page.getByText('Could not copy. Select the JSON and copy it manually.', { exact: true })).toHaveCount(2);
  expect(pageErrors).toEqual([]);
});

test('keyboard error recovery and narrow layout remain usable', async ({ page }, testInfo) => {
  await mockApp(page);
  await page.setViewportSize({ width: 375, height: 900 });
  await openApp(page);
  await importJson(page, single);
  await useForm(page);
  const location = page.getByLabel('Location ID', { exact: false });
  await location.focus();
  await location.press('ControlOrMeta+A');
  await location.press('Backspace');
  await location.press('Tab');
  const run = page.getByRole('button', { name: 'Run backtest', exact: true });
  await run.focus();
  await run.press('Enter');
  await expect(page.getByRole('alert')).toBeFocused();
  await page.getByRole('button', { name: /data_source.location_id:/ }).focus();
  await page.keyboard.press('Enter');
  await expect(location).toBeFocused();
  await location.fill('KEYBOARD_NODE');
  await location.press('Tab');
  await run.focus();
  await run.press('Enter');
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
  expect(await page.locator('input, select, textarea').evaluateAll(elements => elements.every(element =>
    element instanceof HTMLInputElement || element instanceof HTMLSelectElement || element instanceof HTMLTextAreaElement ? (element.labels?.length ?? 0) > 0 : false))).toBe(true);
  await page.screenshot({ path: testInfo.outputPath('editor-mobile.png'), fullPage: true });
});
