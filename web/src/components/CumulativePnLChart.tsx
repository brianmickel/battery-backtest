import { useMemo } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import type { Clock } from '../domain/types';
import type { LedgerRow } from '../services/api';
import { pnlPoints } from '../presentation/chartData';
import { clockLabel, formatCurrency, formatTimestamp } from '../presentation/format';

interface CumulativePnLChartProps {
  ledger: LedgerRow[];
  clock: Clock;
}

export function CumulativePnLChart({ ledger, clock }: CumulativePnLChartProps) {
  const chartData = useMemo(() => pnlPoints(ledger, clock), [ledger, clock]);
  const labels = new Map(chartData.map(point => [point.instantMs, formatTimestamp(point.timestamp, clock)]));

  if (chartData.length === 0) return <div className="card">No data available for chart</div>;

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Cumulative Net Dispatch P&amp;L</h2>
      <p>Cash flow at each interval end · {clockLabel(clock)}</p>
      <ResponsiveContainer width="100%" height={350}>
        <LineChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 60 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="instantMs" type="number" scale="time" domain={['dataMin', 'dataMax']}
            ticks={chartData.map(point => point.instantMs)}
            tickFormatter={(instant: number) => labels.get(instant) ?? ''}
            angle={-45} textAnchor="end" height={80} interval="preserveStartEnd" />
          <YAxis label={{ value: 'Cumulative P&L ($)', angle: -90, position: 'insideLeft' }}
            tickFormatter={formatCurrency} />
          <Tooltip formatter={(value) => formatCurrency(Number(value))}
            labelFormatter={(label) => typeof label === 'number' ? labels.get(label) ?? '' : ''} />
          <Legend />
          <Line type="stepAfter" dataKey="cumulativePNL"
            stroke={chartData[chartData.length - 1]?.cumulativePNL >= 0 ? '#4ade80' : '#f87171'}
            strokeWidth={2} dot={false} name="Net dispatch P&L" />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
