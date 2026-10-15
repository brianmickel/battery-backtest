import type { ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { expect, it, vi } from 'vitest';
import { halfHour, fiveMinutes } from '../presentation/testFixtures';
import type { PowerBucket } from '../presentation/chartData';
import { ChargeDischargeChart } from './ChargeDischargeChart';

vi.mock('recharts', async () => {
  const { createContext, useContext } = await import('react');
  const Buckets = createContext<PowerBucket[]>([]);
  const Empty = () => null;
  return {
    ResponsiveContainer: ({ children }: { children: ReactNode }) => children,
    BarChart: ({ data, children }: { data: PowerBucket[]; children: ReactNode }) => <Buckets value={data}>{children}</Buckets>,
    Tooltip: ({ content }: { content: (props: { active: boolean; label: number }) => ReactNode }) => {
      const buckets = useContext(Buckets);
      return <>{buckets.map(bucket => <div key={bucket.startMs}>{content({ active: true, label: bucket.startMs })}</div>)}</>;
    },
    XAxis: Empty, YAxis: Empty, CartesianGrid: Empty, Legend: Empty, ReferenceLine: Empty, Bar: Empty,
  };
});

it('passes energy-derived power, energy, clock, and coverage into chart tooltips', () => {
  const html = renderToStaticMarkup(<ChargeDischargeChart ledger={fiveMinutes} clock="UTC" />);
  expect(html).toContain('Charge: 100 MW · 50 MWh');
  expect(html).toContain('Data coverage: 30 of 30 minutes; 0 minutes missing');
  expect(html).toContain('2024-01-01 00:00:00 UTC');
  expect(html).not.toContain('600 MW');
});

it('identifies partial bucket coverage without inflating average power', () => {
  const row = { ...halfHour[0], interval_end_utc: '2024-01-01T00:15:00Z', energy_from_grid_mwh: 25 };
  const html = renderToStaticMarkup(<ChargeDischargeChart ledger={[row]} clock="market" />);
  expect(html).toContain('Charge: 50 MW · 25 MWh');
  expect(html).toContain('Data coverage: 15 of 30 minutes; 15 minutes missing');
  expect(html).toContain('UTC+00:00');
});

it('explains UTC fallback labels when supplied market offsets differ', () => {
  const row = {
    ...halfHour[0], interval_start_utc: '2024-03-10T09:30:00Z', interval_end_utc: '2024-03-10T11:00:00Z',
    interval_start_local: '2024-03-10T01:30:00-08:00', interval_end_local: '2024-03-10T04:00:00-07:00',
  };
  const html = renderToStaticMarkup(<ChargeDischargeChart ledger={[row]} clock="market" />);
  expect(html).toContain('Some bucket times use UTC because the market offset between observations is unknown.');
  expect(html).toContain('2024-03-10 01:30:00 UTC-08:00');
  expect(html).toContain('<strong>2024-03-10 10:00:00 UTC</strong>');
  expect(html).not.toContain('2024-03-10 02:00:00 UTC-08:00');
  expect(renderToStaticMarkup(<ChargeDischargeChart ledger={[row]} clock="UTC" />)).not.toContain('Some bucket times use UTC');
});

it('does not describe genuine UTC market times as uncertain offsets', () => {
  const row = { ...halfHour[0], interval_end_utc: '2024-01-01T01:00:00Z', interval_end_local: '2024-01-01T01:00:00+00:00' };
  const html = renderToStaticMarkup(<ChargeDischargeChart ledger={[row]} clock="market" />);
  expect(html).toContain('2024-01-01 00:00:00 UTC+00:00');
  expect(html).toContain('2024-01-01 00:30:00 UTC+00:00');
  expect(html).not.toContain('Some bucket times use UTC');
});
