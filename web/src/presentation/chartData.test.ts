import { describe, expect, it } from 'vitest';
import variableGap from '../../../testdata/variable-gap.json';
import midnight from '../../../testdata/midnight.json';
import dstFall from '../../../testdata/dst-fall.json';
import type { LedgerRow } from '../services/api';
import { pnlPoints, powerBuckets } from './chartData';
import { formatTimestamp } from './format';
import { halfHour, fiveMinutes } from './testFixtures';

describe('average directional power buckets', () => {
  it('renders six five-minute 100 MW samples as 100 MW and 50 MWh', () => {
    const buckets = powerBuckets(fiveMinutes, 'UTC');
    expect(buckets).toHaveLength(1);
    expect(buckets[0].chargeMW).toBeCloseTo(100);
    expect(buckets[0].chargeMWh).toBeCloseTo(50);
    expect(buckets[0].coveredHours).toBeCloseTo(.5);
    expect(buckets[0].chargeMW).toBeCloseTo(powerBuckets(halfHour, 'UTC')[0].chargeMW);
  });

  it('splits a crossing interval by overlap and includes uncovered bucket time in the mean', () => {
    const row: LedgerRow = {
      ...halfHour[0],
      interval_start_utc: '2024-01-01T00:20:00Z', interval_end_utc: '2024-01-01T01:10:00Z',
      interval_start_local: '2024-01-01T00:20:00Z', interval_end_local: '2024-01-01T01:10:00Z',
      energy_from_grid_mwh: 50, power_mw: -60,
    };
    const buckets = powerBuckets([row], 'UTC');
    expect(buckets.map(bucket => bucket.chargeMWh)).toEqual([10, 30, 10]);
    expect(buckets.map(bucket => bucket.chargeMW)).toEqual([20, 60, 20]);
    expect(buckets.map(bucket => bucket.coveredHours)).toEqual([1 / 6, .5, 1 / 6]);
    expect(buckets.reduce((sum, bucket) => sum + bucket.chargeMWh, 0)).toBe(50);
  });

  it('conserves variable-duration energy and identifies partial coverage', () => {
    const rows = variableGap.data.map((row, index) => ({
      ...halfHour[0], ...row,
      energy_from_grid_mwh: index === 0 ? 15 : 0,
      energy_to_grid_mwh: index === 1 ? 30 : 0,
    }));
    const buckets = powerBuckets(rows, 'UTC');
    expect(buckets.map(bucket => bucket.chargeMW)).toEqual([30, 0]);
    expect(buckets.map(bucket => bucket.dischargeMW)).toEqual([-0, -60]);
    expect(buckets.map(bucket => bucket.coveredHours)).toEqual([.25, .5]);
    expect(buckets.reduce((sum, bucket) => sum + bucket.dischargeMWh, 0)).toBe(30);
  });

  it('keeps empty buckets inside gaps with zero coverage and energy', () => {
    const rows = [halfHour[0], {
      ...halfHour[0],
      interval_start_utc: '2024-01-01T01:30:00Z', interval_end_utc: '2024-01-01T02:00:00Z',
      interval_start_local: '2024-01-01T01:30:00Z', interval_end_local: '2024-01-01T02:00:00Z',
    }];
    const buckets = powerBuckets(rows, 'market');
    expect(buckets).toHaveLength(4);
    expect(buckets.map(bucket => bucket.coveredHours)).toEqual([.5, 0, 0, .5]);
    expect(buckets.map(bucket => bucket.chargeMWh)).toEqual([50, 0, 0, 50]);
    expect(buckets[1].timestamp).toBe('2024-01-01T00:30:00.000Z');
  });

  it('keeps the shared midnight fixture on its supplied market dates', () => {
    const rows = midnight.data.map(row => ({ ...halfHour[0], ...row, energy_from_grid_mwh: 30 }));
    const buckets = powerBuckets(rows, 'market');
    expect(buckets.map(bucket => formatTimestamp(bucket.timestamp, 'market'))).toEqual([
      '2024-01-01 23:30:00 UTC-08:00', '2024-01-02 00:00:00 UTC-08:00',
    ]);
    expect(buckets.map(bucket => bucket.chargeMWh)).toEqual([15, 15]);
  });

  it('sorts repeated DST hours by instant while preserving distinct offsets', () => {
    const rows = dstFall.data.map(row => ({ ...halfHour[0], ...row })).reverse();
    const original = structuredClone(rows);
    const buckets = powerBuckets(rows, 'market');
    expect(buckets.map(bucket => bucket.startMs)).toEqual([
      Date.parse('2024-11-03T08:30:00Z'), Date.parse('2024-11-03T09:00:00Z'),
    ]);
    expect(buckets.map(bucket => formatTimestamp(bucket.timestamp, 'market'))).toEqual([
      '2024-11-03 01:30:00 UTC-07:00', '2024-11-03 01:00:00 UTC-08:00',
    ]);
    expect(rows).toEqual(original);
  });

  it('anchors buckets to UTC even for quarter-hour market offsets', () => {
    const row = {
      ...halfHour[0],
      interval_start_utc: '2024-01-01T00:05:00Z', interval_end_utc: '2024-01-01T00:20:00Z',
      interval_start_local: '2024-01-01T05:50:00+05:45', interval_end_local: '2024-01-01T06:05:00+05:45',
    };
    const bucket = powerBuckets([row], 'market')[0];
    expect(bucket.startMs).toBe(Date.parse('2024-01-01T00:00:00Z'));
    expect(formatTimestamp(bucket.timestamp, 'market')).toBe('2024-01-01 05:45:00 UTC+05:45');
  });

  it('returns no points for an empty ledger', () => {
    expect(powerBuckets([], 'UTC')).toEqual([]);
    expect(pnlPoints([], 'market')).toEqual([]);
  });
});

it('places cumulative cashflow at interval ends in chronological order', () => {
  const rows = dstFall.data.map((row, index) => ({ ...halfHour[0], ...row, cum_pnl: index === 0 ? -5 : 10 }));
  const points = pnlPoints([...rows].reverse(), 'market');
  expect(points.map(point => point.instantMs)).toEqual([
    Date.parse('2024-11-03T08:30:00Z'), Date.parse('2024-11-03T09:00:00Z'), Date.parse('2024-11-03T09:30:00Z'),
  ]);
  expect(points.map(point => point.cumulativePNL)).toEqual([0, -5, 10]);
  expect(points[1].timestamp).toBe('2024-11-03T01:00:00-08:00');
  expect(pnlPoints(rows, 'UTC')[1].timestamp).toBe('2024-11-03T09:00:00Z');
});
