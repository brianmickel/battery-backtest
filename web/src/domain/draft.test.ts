import { describe, expect, it, vi } from 'vitest'
import single from '../../../testdata/single-request.json'
import comparison from '../../../testdata/compare-request.json'
import { applyDraftAction, parseDraft, parseFormEdits, serializeDraft } from './draft'
import { calendarDay, validateDateRange } from './calendar'

function valid(value: unknown = single) {
  const result = parseDraft(value)
  if (!result.ok) throw new Error(JSON.stringify(result.issues))
  return result.draft
}

describe('atomic request parsing', () => {
  it('round trips all fields and comparison ordering without materializing defaults', () => {
    for (const input of [single, comparison]) expect(JSON.parse(serializeDraft(valid(input)))).toEqual(input)
    const input = { ...single, config: { battery_file: '1_moss_landing', battery: { initial_soc: 0, degradation_cost_per_mwh: 0 }, strategy: { name: 'schedule', params: { charge_power_mw: 0 } } } }
    expect(JSON.parse(serializeDraft(valid(input)))).toEqual(input)
    expect(parseDraft({ ...single, options: { limit_intervals: 0 } }).ok).toBe(true)
  })

  it('extracts legacy credentials only after complete validation', () => {
    const imported = parseDraft({ ...single, api_key: 'sentinel-key' })
    expect(imported.ok && imported.importedApiKey).toBe('sentinel-key')
    if (imported.ok) expect(serializeDraft(imported.draft)).not.toContain('sentinel-key')
    expect(parseDraft({ ...single, api_key: '{{API_KEY}}' })).not.toHaveProperty('importedApiKey')
    const failed = parseDraft({ ...single, api_key: 'sentinel-key', config: { surprise: true } })
    expect(failed.ok).toBe(false)
    expect(failed).not.toHaveProperty('importedApiKey')
    expect(JSON.stringify(failed)).not.toContain('sentinel-key')
  })

  it.each([
    { ...single, api_key: null },
    { ...single, config: null },
    { ...single, unknown: 1 },
    { ...single, config: { ...single.config, battery: { ...single.config.battery, initial_soc: null } } },
    { ...single, config: { ...single.config, battery: { ...single.config.battery, power_capacity_mw: Infinity } } },
    { ...single, config: { ...single.config, strategy: { name: 'oracle', params: { soc_steps: 2.5 } } } },
    { ...single, config: { ...single.config, strategy: { name: 'oracle', params: { charge_power_mw: 1 } } } },
    { ...single, config: { ...single.config, strategy: { name: 'schedule', params: { charge_start: '24:00' } } } },
    { ...single, options: { include_ledger: 'true' } },
    { ...comparison, variations: [] },
    { ...comparison, variations: [comparison.variations[0], comparison.variations[0]] },
  ])('rejects invalid unknown input without a draft', input => {
    const result = parseDraft(input)
    expect(result.ok).toBe(false)
    expect(result).not.toHaveProperty('draft')
  })

  it('leaves hidden preset bounds to the server but checks explicit bounds', () => {
    const input = { ...single, config: { battery_file: '1_moss_landing', battery: { initial_soc: 0.95 }, strategy: { name: 'oracle' } } }
    expect(parseDraft(input).ok).toBe(true)
    expect(parseDraft({ ...input, config: { ...input.config, battery: { initial_soc: 0.95, max_soc: 0.9 } } }).ok).toBe(false)
    expect(parseDraft({ ...single, config: { ...single.config, battery: { ...single.config.battery, min_soc: 0.5, max_soc: 0.5, initial_soc: 0.5 } } }).ok).toBe(true)
  })

  it('accepts an incomplete common patch when every variation supplies the required values', () => {
    expect(parseDraft({ ...comparison, base_config: {}, variations: [{ name: 'complete', config: single.config }] }).ok).toBe(true)
  })

  it('merges variation patches before checking coupled values and strategy compatibility', () => {
    const input = { ...comparison, variations: [{ name: 'valid', config: { battery: { min_soc: 0.2, initial_soc: 0.2 }, strategy: { name: 'schedule' } } }] }
    expect(parseDraft(input).ok).toBe(true)
    expect(parseDraft({ ...input, variations: [{ name: 'invalid', config: { battery: { min_soc: 0.2 } } }] }).ok).toBe(false)
    expect(parseDraft({ ...input, variations: [{ name: 'invalid', config: { strategy: { params: { charge_start: '10:00' } } } }] }).ok).toBe(false)
  })

  it('checks active wraparound overlap, while explicit zero makes that direction idle', () => {
    const config = { ...single.config, strategy: { name: 'schedule', params: { charge_start: '23:00', charge_end: '02:00', discharge_start: '01:00', discharge_end: '03:00' } } }
    expect(parseDraft({ ...single, config }).ok).toBe(false)
    expect(parseDraft({ ...single, config: { ...config, strategy: { ...config.strategy, params: { ...config.strategy.params, charge_power_mw: 0 } } } }).ok).toBe(true)
  })
})

describe('raw form edits', () => {
  it.each(['', '-', '0.', '2x', 'Infinity', '1e'])('preserves invalid text %j without exposing stale values', text => {
    const draft = valid()
    const before = serializeDraft(draft)
    expect(parseFormEdits(draft, { 'config.battery.power_capacity_mw': text }).ok).toBe(false)
    expect(serializeDraft(draft)).toBe(before)
  })

  it('applies coupled edits atomically and removes blank optional overrides', () => {
    const draft = valid()
    const edited = parseFormEdits(draft, { 'config.battery.min_soc': '0.5', 'config.battery.max_soc': '0.6', 'config.battery.initial_soc': '0.55', 'config.battery.degradation_cost_per_mwh': '' })
    expect(edited.ok).toBe(true)
    if (edited.ok && edited.draft.kind === 'single') {
      expect(edited.draft.config.battery).toMatchObject({ min_soc: 0.5, max_soc: 0.6, initial_soc: 0.55 })
      expect(edited.draft.config.battery).not.toHaveProperty('degradation_cost_per_mwh')
    }
    expect(parseFormEdits(draft, { 'config.strategy.params.soc_steps': '2e2' }).ok).toBe(true)
  })

  it('changes defaults only via explicit actions and does not mutate imported values', () => {
    const draft = valid({ ...single, config: { ...single.config, strategy: { name: 'oracle', params: { soc_steps: 99, power_steps: 3 } } } })
    expect(serializeDraft(draft)).toContain('99')
    const changed = applyDraftAction(draft, { type: 'select-strategy', name: 'schedule' })
    if (changed.kind === 'single') expect(changed.config.strategy).toEqual({ name: 'schedule' })
    expect(serializeDraft(draft)).toContain('99')
  })
})

describe('calendar-only dates', () => {
  it('accepts strict leap dates and rejects normalization or same-day ranges', () => {
    vi.useFakeTimers(); vi.setSystemTime(new Date('2025-01-01T00:00:00Z'))
    expect(calendarDay('2024-02-29')).toBeDefined()
    for (const value of ['2023-02-29', '2024-04-31', '2024-1-01', '0000-01-01']) expect(calendarDay(value)).toBeUndefined()
    expect(validateDateRange('2024-01-01', '2024-03-01')).toEqual([])
    expect(validateDateRange('2024-01-01', '2024-03-02')).not.toEqual([])
    expect(validateDateRange('2024-01-01', '2024-01-01')).not.toEqual([])
    expect(validateDateRange('2024-12-31', '2025-01-02')).not.toEqual([])
  })
})
