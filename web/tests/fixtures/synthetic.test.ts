import { describe, expect, it } from 'vitest'
import twoPrice from '../../../testdata/two-price.json'
import variableGap from '../../../testdata/variable-gap.json'
import midnight from '../../../testdata/midnight.json'
import dstFall from '../../../testdata/dst-fall.json'
import ledger from '../../../testdata/half-hour-ledger.json'
import single from '../../../testdata/single-request.json'
import compare from '../../../testdata/compare-request.json'

const minutes = (start: string, end: string) => (Date.parse(end) - Date.parse(start)) / 60_000

describe('shared synthetic fixture meanings', () => {
  it('keeps prices and chronological durations explicit', () => {
    expect(twoPrice.data.map(row => row.lmp)).toEqual([10, 100])
    expect(variableGap.data.map(row => minutes(row.interval_start_utc, row.interval_end_utc))).toEqual([15, 30])
    expect(minutes(variableGap.data[0].interval_end_utc, variableGap.data[1].interval_start_utc)).toBe(15)
    expect(minutes(midnight.data[0].interval_start_local, midnight.data[0].interval_end_local)).toBe(30)
    expect(dstFall.data.map(row => minutes(row.interval_start_local, row.interval_end_local))).toEqual([30, 30])
  })

  it('distinguishes power from interval energy', () => {
    expect(ledger[0].energy_from_grid_mwh).toBe(50)
    expect(ledger[0].energy_from_grid_mwh / 0.5).toBe(100)
    expect(ledger[0].power_mw).toBe(-100)
  })

  it('keeps credentials separate and explicit zero overrides in comparison order', () => {
    expect(single).not.toHaveProperty('api_key')
    expect(compare).not.toHaveProperty('api_key')
    expect(single.config.battery.initial_soc).toBe(0)
    expect(compare.variations.map(variation => variation.name)).toEqual(['Idle at explicit zero', 'Oracle'])
    expect(compare.variations[0].config.strategy?.params.charge_power_mw).toBe(0)
  })
})
