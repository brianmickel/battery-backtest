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
