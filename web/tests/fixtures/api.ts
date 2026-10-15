import type { Page } from '@playwright/test'

export const metadata = {
  batteries: [{ id: '1_moss_landing', name: 'Synthetic battery', file: '1_moss_landing', specs: { energy_capacity_mwh: 1, power_capacity_mw: 1 } }],
  strategies: [
    { name: 'oracle', description: 'Approximate foresight optimizer', parameters: [
      { name: 'soc_steps', type: 'integer', description: 'SOC grid steps', default: 200 },
      { name: 'power_steps', type: 'integer', description: 'Power steps', default: 10 },
    ] },
    { name: 'schedule', description: 'Daily schedule', parameters: [] },
  ],
  datasets: [{ id: 'caiso_lmp_real_time_5_min', name: 'Synthetic market', market: 'SYNTHETIC', resolution: '5 minutes' }],
  locations: [{ id: 'TEST_NODE', name: 'Synthetic node', type: 'NODE' }],
}

export async function mockMetadata(page: Page): Promise<void> {
  await page.route('**/*', async route => {
    const url = new URL(route.request().url())
    if (url.origin !== 'http://127.0.0.1:4173') {
      await route.abort('blockedbyclient')
      return
    }
    const resource = url.pathname.replace('/api/v1/', '')
    switch (resource) {
      case 'batteries':
      case 'strategies':
      case 'datasets':
      case 'locations':
        await route.fulfill({ json: { [resource]: metadata[resource] } })
        return
    }
    if (url.pathname.startsWith('/api/')) {
      await route.fulfill({ status: 501, json: { error: { code: 'UNMOCKED_REQUEST', message: 'No fixture for this request' } } })
      return
    }
    await route.continue()
  })
}
