import { afterEach, describe, expect, it, vi } from 'vitest';
import { civilDate, formatCoverage, formatCurrency, formatTimestamp, timestampAt, timestampOffsetMinutes } from './format';

afterEach(() => vi.unstubAllEnvs());

describe('explicit result clock', () => {
  it.each(['UTC', 'Pacific/Kiritimati', 'America/Los_Angeles'])('preserves market civil time under host timezone %s', timezone => {
    vi.stubEnv('TZ', timezone);
    vi.stubEnv('LANG', 'de_DE.UTF-8');
    const timestamp = '2024-01-01T23:45:00-08:00';
    expect(civilDate(timestamp, 'market')).toBe('2024-01-01');
    expect(civilDate(timestamp, 'UTC')).toBe('2024-01-02');
    expect(formatTimestamp(timestamp, 'market')).toBe('2024-01-01 23:45:00 UTC-08:00');
    expect(formatTimestamp(timestamp, 'UTC')).toBe('2024-01-02 07:45:00 UTC');
    expect(formatCurrency(-22.5)).toBe('-$22.50');
  });

  it('keeps repeated civil labels distinguishable by their explicit offset', () => {
    expect(formatTimestamp('2024-11-03T01:00:00-07:00', 'market')).toBe('2024-11-03 01:00:00 UTC-07:00');
    expect(formatTimestamp('2024-11-03T01:00:00-08:00', 'market')).toBe('2024-11-03 01:00:00 UTC-08:00');
  });

  it('projects an instant using the given offset without inferring a named zone', () => {
    const instant = Date.parse('2024-01-01T00:30:00Z');
    expect(timestampAt(instant, '2024-01-01T05:45:00+05:45', 'market')).toBe('2024-01-01T06:15:00.000+05:45');
    expect(timestampAt(instant, '2024-01-01T05:45:00+05:45', 'UTC')).toBe('2024-01-01T00:30:00.000Z');
  });

  it('compares numeric offsets independently of their UTC spelling', () => {
    expect(timestampOffsetMinutes('2024-01-01T00:00:00Z')).toBe(0);
    expect(timestampOffsetMinutes('2024-01-01T00:00:00+00:00')).toBe(0);
    expect(timestampOffsetMinutes('2024-01-01T05:45:00+05:45')).toBe(345);
    expect(timestampOffsetMinutes('2023-12-31T16:00:00-08:00')).toBe(-480);
  });
});

it('formats summary coverage without depending on ledger availability', () => {
  expect(formatCoverage({ covered_duration_hours: 2, gap_duration_hours: 1 })).toBe('Data coverage: 2 of 3 hours; 1 hour missing');
  expect(formatCoverage({ covered_duration_hours: 3, gap_duration_hours: 0 })).toBe('Data coverage: 3 of 3 hours; 0 hours missing');
});
