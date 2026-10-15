import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import type { BacktestSummary, ChargeWindow } from '../services/api';
import { DailyWindowsDisplay } from '../components/DailyWindowsDisplay';
import { MetricsDisplay } from '../components/MetricsDisplay';
import { groupWindows } from './windows';

const summary: BacktestSummary = {
  total_pnl: -12.5, initial_soc: .5, initial_energy_mwh: 5, final_soc: .7, final_energy_mwh: 7,
  timezone: 'market', covered_duration_hours: 2, gap_duration_hours: 1, gap_count: 1,
  total_intervals: 2, energy_charged_mwh: 2, energy_discharged_mwh: 0,
  backtest_window: { start: '2024-01-01T21:00:00-08:00', end: '2024-01-02T00:00:00-08:00' },
};
const windows: ChargeWindow[] = [
  { start: '2024-01-01T21:00:00-08:00', end: '2024-01-01T22:00:00-08:00', energy_mwh: 1, average_cost_per_mwh: -5 },
  { start: '2024-01-01T23:00:00-08:00', end: '2024-01-02T00:00:00-08:00', energy_mwh: 1, average_cost_per_mwh: 17.5 },
];

describe('summary presentation without a ledger', () => {
  it('renders signed PNL, initial/final inventory, assumptions, and missing coverage', () => {
    const html = renderToStaticMarkup(<MetricsDisplay summary={summary} />);
    expect(html).toContain('-$12.50');
    expect(html).toContain('Initial SOC · 5 MWh stored');
    expect(html).toContain('Final SOC · 7 MWh stored');
    expect(html).toContain('Data coverage: 2 of 3 hours; 1 hour missing (1 gap)');
    expect(html).toContain('Initial stored energy has no acquisition charge; final stored energy has zero salvage value.');
    expect(html).toContain('includes degradation');
    expect(html).toContain('2024-01-01 21:00:00 UTC-08:00');
    expect(html).not.toMatch(/ROI|upper.bound|guaranteed/i);
  });

  it('renders full coverage and a UTC clock from the same summary shape', () => {
    const html = renderToStaticMarkup(<MetricsDisplay summary={{ ...summary, timezone: 'UTC', covered_duration_hours: 3, gap_duration_hours: 0, gap_count: 0 }} />);
    expect(html).toContain('Data coverage: 3 of 3 hours; 0 hours missing (0 gaps)');
    expect(html).toContain('2024-01-02 05:00:00 UTC');
    expect(html).not.toContain('UTC-08:00');
  });
});

describe('continuous operating windows', () => {
  it('renders both separated charge runs without needing discharge windows', () => {
    const html = renderToStaticMarkup(<DailyWindowsDisplay clock="market" chargeWindows={[...windows].reverse()} />);
    expect(html.match(/Average Cost:/g)).toHaveLength(2);
    expect(html).toContain('-$5.00/MWh');
    expect(html).toContain('$17.50/MWh');
    expect(html.indexOf('21:00:00')).toBeLessThan(html.indexOf('23:00:00'));
    expect(groupWindows(windows, [], 'market')).toHaveLength(1);
    expect(groupWindows(windows, [], 'market')[0].date).toBe('2024-01-01');
    expect(groupWindows(windows, [], 'UTC')[0].date).toBe('2024-01-02');
  });

  it('does not render an empty windows section', () => {
    expect(renderToStaticMarkup(<DailyWindowsDisplay clock="market" />)).toBe('');
  });

  it('groups one DST civil day without merging runs or sorting by clock labels', () => {
    const repeated: ChargeWindow[] = [
      { ...windows[0], start: '2024-11-03T01:30:00-07:00', end: '2024-11-03T01:45:00-07:00' },
      { ...windows[1], start: '2024-11-03T01:00:00-08:00', end: '2024-11-03T01:15:00-08:00' },
    ];
    const days = groupWindows([...repeated].reverse(), [], 'market');
    expect(days).toHaveLength(1);
    expect(days[0].chargeWindows).toEqual(repeated);
    const html = renderToStaticMarkup(<DailyWindowsDisplay clock="market" chargeWindows={repeated} />);
    expect(html).toContain('01:30:00 UTC-07:00');
    expect(html).toContain('01:00:00 UTC-08:00');
  });
});
