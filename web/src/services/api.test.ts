import { describe, expect, it, vi } from 'vitest'
import single from '../../../testdata/single-request.json'
import comparison from '../../../testdata/compare-request.json'
import ledger from '../../../testdata/half-hour-ledger.json'
import { parseDraft } from '../domain/draft'

const transport = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn() }))
vi.mock('axios', () => ({ default: { create: () => transport, isAxiosError: (value: unknown) => typeof value === 'object' && value !== null && 'isAxiosError' in value } }))
import { apiService, ApiError, getErrorMessage } from './api'

const summary = {
  total_pnl: -1000, initial_soc: 0, initial_energy_mwh: 0, final_soc: 0.5, final_energy_mwh: 50,
  total_intervals: 1, timezone: 'UTC', backtest_window: { start: '2024-01-01T00:00:00Z', end: '2024-01-01T00:30:00Z' },
  energy_charged_mwh: 50, energy_discharged_mwh: 0, covered_duration_hours: 0.5, gap_duration_hours: 0, gap_count: 0,
}

function singleRequest() {
  const parsed = parseDraft(single)
  if (!parsed.ok || parsed.draft.kind !== 'single') throw new Error('invalid fixture')
  return { data_source: parsed.draft.data_source, config: parsed.draft.config, options: parsed.draft.options }
}

describe('credential-scoped transport', () => {
  it('posts single and compare bodies unchanged with only a credential header', async () => {
    const key = 'sentinel-secret'
    const request = singleRequest()
    transport.post.mockResolvedValueOnce({ data: { status: 'completed', summary, ledger } })
    await apiService.runBacktest(request, key)
    expect(transport.post).toHaveBeenLastCalledWith('/api/v1/backtest', request, { headers: { 'X-GridStatus-API-Key': key }, signal: undefined })
    const parsed = parseDraft(comparison)
    if (!parsed.ok || parsed.draft.kind !== 'compare') throw new Error('invalid comparison fixture')
    const body = { data_source: parsed.draft.data_source, base_config: parsed.draft.base_config, variations: parsed.draft.variations }
    transport.post.mockResolvedValueOnce({ data: { comparison: [{ name: 'first', summary }, { name: 'second', summary }] } })
    await apiService.compareBacktests(body, key)
    expect(transport.post).toHaveBeenLastCalledWith('/api/v1/backtest/compare', body, { headers: { 'X-GridStatus-API-Key': key }, signal: undefined })
    expect(JSON.stringify(body)).not.toContain(key)
  })

  it('keeps rank credentials out of URLs and forwards cancellation', async () => {
    transport.get.mockResolvedValueOnce({ data: { rankings: [] } })
    const signal = new AbortController().signal
    const params = { dataset_id: 'test', location_ids: 'TEST_NODE', start_date: '2024-01-01', end_date: '2024-01-02' }
    await apiService.rankNodes(params, 'rank-secret', signal)
    expect(transport.get).toHaveBeenLastCalledWith('/api/v1/rank', { params, headers: { 'X-GridStatus-API-Key': 'rank-secret' }, signal })
  })

  it('uses the actual metadata shapes without logging responses', async () => {
    const log = vi.spyOn(console, 'log')
    const batteries = [{ id: 'battery', name: 'Battery', specs: { energy_capacity_mwh: 1, power_capacity_mw: 1 } }]
    transport.get.mockResolvedValueOnce({ data: { batteries } })
    expect(await apiService.getBatteries()).toEqual(batteries)
    expect(log).not.toHaveBeenCalled()
  })

  it.each([
    { status: 'completed', summary: { ...summary, gap_count: undefined } },
    { status: 'completed', summary: { ...summary, final_soc: '0.5' } },
    { status: 'completed', summary, ledger: [{ ...ledger[0], power_mw: Infinity }] },
    { status: 'failed', summary },
    null,
  ])('rejects malformed success responses safely', async data => {
    transport.post.mockResolvedValueOnce({ data })
    await expect(apiService.runBacktest(singleRequest(), 'sentinel-secret')).rejects.toMatchObject({ code: 'INTERNAL_ERROR', message: 'The server returned an invalid response.' })
  })

  it('preserves structured field errors but discards raw transport errors', async () => {
    transport.post.mockRejectedValueOnce({ isAxiosError: true, response: { data: { error: { code: 'INVALID_REQUEST', message: 'Initial SOC exceeds the preset maximum.', field: 'config.battery.initial_soc' } } } })
    await expect(apiService.runBacktest(singleRequest(), 'secret')).rejects.toMatchObject({ code: 'INVALID_REQUEST', field: 'config.battery.initial_soc' })
    transport.post.mockRejectedValueOnce({ isAxiosError: true, message: 'request header secret', response: { data: { error: { code: 'UNKNOWN', message: 'secret' } } } })
    await expect(apiService.runBacktest(singleRequest(), 'secret')).rejects.toMatchObject({ message: 'The request could not be completed. Please try again.' })
    expect(getErrorMessage(new Error('secret'), 'Safe fallback')).toBe('Safe fallback')
    expect(getErrorMessage(new ApiError('Safe message', { code: 'SERVER_BUSY' }), 'fallback')).toBe('Safe message')
  })
})
