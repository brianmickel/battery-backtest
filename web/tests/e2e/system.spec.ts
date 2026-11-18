import { expect, test, type Page } from '@playwright/test';
import { execFileSync, spawn, type ChildProcess } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import single from '../../../testdata/single-request.json' with { type: 'json' };
import compare from '../../../testdata/compare-request.json' with { type: 'json' };

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../..');
const key = 'synthetic-release-secret';
let server: ChildProcess;
let origin = '';
let scratch = '';
let serverOutput = '';

test.describe.configure({ mode: 'serial' });
test.use({ trace: 'on' });

test.beforeAll(async () => {
  test.setTimeout(120_000);
  scratch = mkdtempSync(path.join(tmpdir(), 'battery-system-'));
  const executable = path.join(scratch, 'fixture-server');
  execFileSync('go', ['build', '-o', executable, './internal/integration/server'], { cwd: root, stdio: 'pipe' });
  server = spawn(executable, ['--root', root], { cwd: root, env: { ...process.env, GIN_MODE: 'release' }, stdio: ['ignore', 'pipe', 'pipe'] });
  await new Promise<void>((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('Synthetic API did not become ready')), 30_000);
    server.on('error', error => { clearTimeout(timeout); reject(error); });
    server.on('exit', code => { clearTimeout(timeout); reject(new Error(`Synthetic API exited ${code}: ${serverOutput}`)); });
    server.stdout?.on('data', (data: Buffer) => {
      serverOutput += data.toString();
      const ready = serverOutput.match(/READY (http:\/\/127\.0\.0\.1:\d+)/);
      if (ready) { origin = ready[1]; clearTimeout(timeout); resolve(); }
    });
    server.stderr?.on('data', (data: Buffer) => { serverOutput += data.toString(); });
  });
});

test.afterAll(async () => {
  if (server && server.exitCode === null) {
    await new Promise<void>(resolve => { server.once('exit', () => resolve()); server.kill('SIGTERM'); });
  }
  if (scratch) rmSync(scratch, { recursive: true, force: true });
  expect(serverOutput).not.toContain(key);
});

async function importRequest(page: Page, request: unknown) {
  await page.getByRole('radio', { name: 'JSON Input', exact: true }).check();
  await page.getByLabel('Request JSON', { exact: true }).fill(JSON.stringify(request, null, 2));
  await page.getByRole('button', { name: 'Apply JSON', exact: true }).click();
  await expect(page.getByText('Request applied.', { exact: true })).toBeVisible();
}

test('browser → router → HTTP provider → physical ledger matches the synthetic example', async ({ page }, info) => {
  const consoleMessages: string[] = [];
  page.on('console', message => consoleMessages.push(message.text()));
  await page.goto(origin);
  await page.getByLabel('Grid Status API Key').fill(key);
  await importRequest(page, single);
  const completed = page.waitForResponse(response => response.url() === `${origin}/api/v1/backtest`);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  const response = await completed;
  expect(response.status()).toBe(200);
  expect(response.request().headers()['x-gridstatus-api-key']).toBe(key);
  expect(response.request().postDataJSON()).toEqual(single);
  expect(response.request().url()).not.toContain(key);
  expect(response.request().postData()).not.toContain(key);
  const body: unknown = await response.json();
  expect(body).toMatchObject({ status: 'completed', summary: { total_pnl: 90, total_intervals: 2, initial_soc: 0, final_soc: 0, energy_charged_mwh: 1, energy_discharged_mwh: 1 }, ledger: [{ action: 'CHARGING', power_mw: -1, pnl: -10 }, { action: 'DISCHARGING', power_mw: 1, pnl: 100, cum_pnl: 90 }] });
  await expect(page.getByRole('heading', { name: 'Summary Metrics', exact: true })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Backtest results' })).toContainText('$90.00');
  await expect(page.getByRole('region', { name: 'Backtest results' })).toContainText('MW');
  await expect.poll(async () => page.locator('.recharts-bar-rectangle path').evaluateAll(bars =>
    bars.filter(bar => bar.getBoundingClientRect().height > 30).length)).toBe(4);
  await page.locator('.recharts-bar-rectangle path').first().hover();
  await expect(page.getByText('Charge: 1 MW · 0.5 MWh', { exact: true })).toBeVisible();
  await info.attach('request.json', { body: JSON.stringify(single, null, 2), contentType: 'application/json' });
  await info.attach('response.json', { body: JSON.stringify(body, null, 2), contentType: 'application/json' });
  await page.screenshot({ path: info.outputPath('integrated-result.png'), fullPage: true });
  const resources = await page.evaluate(() => performance.getEntriesByType('resource').map(entry => {
    const resource = entry as PerformanceResourceTiming;
    return { origin: new URL(resource.name).origin, name: new URL(resource.name).pathname, bytes: resource.transferSize, durationMs: resource.duration };
  }));
  expect(resources.every(resource => resource.origin === origin)).toBe(true);
  await info.attach('browser-resources.json', { body: JSON.stringify(resources, null, 2), contentType: 'application/json' });
  expect(consoleMessages.join('\n')).not.toContain(key);
});

test('real authentication and malformed-provider failures remain visible and safe', async ({ page }) => {
  await page.goto(origin);
  await page.getByLabel('Grid Status API Key').fill('invalid-synthetic-key');
  await importRequest(page, single);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('rejected the API key');
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toHaveCount(0);
  await page.getByLabel('Grid Status API Key').fill(key);
  await importRequest(page, { ...single, data_source: { ...single.data_source, location_id: 'MALFORMED_NODE' } });
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('provider is unavailable');
  await expect(page.getByRole('alert')).not.toContainText(key);
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toHaveCount(0);
});

test('comparison order and atomic failures cross the real router', async ({ page, request }) => {
  await page.goto(origin);
  await page.getByLabel('Grid Status API Key').fill(key);
  await importRequest(page, compare);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Comparison Results' })).toBeVisible();
  await expect(page.getByRole('region', { name: 'Backtest results' }).getByRole('heading', { level: 3, name: /^(Idle at explicit zero|Oracle)$/ })).toHaveText(['Idle at explicit zero', 'Oracle']);
  await importRequest(page, { ...compare, data_source: { ...compare.data_source, location_id: 'FAIL_NODE' } });
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('provider is unavailable');
  await expect(page.getByRole('heading', { name: 'Comparison Results' })).toHaveCount(0);
  const ranking = await request.get(`${origin}/api/v1/rank`, { headers: { 'X-GridStatus-API-Key': key }, params: { dataset_id: 'synthetic_lmp', start_date: '2024-01-01', end_date: '2024-01-02', location_ids: 'TEST_NODE,FAIL_NODE', limit: 1 } });
  expect(ranking.status()).toBe(502);
  expect(await ranking.json()).toEqual({ error: { code: 'UPSTREAM_UNAVAILABLE', message: 'The market-data provider is unavailable.', location_id: 'FAIL_NODE' } });
});

test('leaving a running page cancels provider I/O and releases admission', async ({ page, request }) => {
  const before: { calls: number; cancellations: number } = await (await request.get(`${origin}/__fixture/stats`)).json();
  await page.goto(origin);
  await page.getByLabel('Grid Status API Key').fill(key);
  await importRequest(page, { ...single, data_source: { ...single.data_source, location_id: 'CANCEL_NODE' } });
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('status').filter({ hasText: 'Running backtest' })).toBeVisible();
  await expect.poll(async () => (await (await request.get(`${origin}/__fixture/stats`)).json()).calls).toBe(before.calls + 1);
  await page.goto('about:blank');
  await expect.poll(async () => (await (await request.get(`${origin}/__fixture/stats`)).json()).cancellations).toBe(before.cancellations + 1);
  await page.goto(origin);
  await importRequest(page, single);
  await page.getByRole('button', { name: 'Run backtest', exact: true }).click();
  await expect(page.getByRole('heading', { name: 'Summary Metrics' })).toBeVisible();
});
