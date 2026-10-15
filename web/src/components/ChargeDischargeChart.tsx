import { useMemo } from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, ReferenceLine } from 'recharts';
import type { Clock } from '../domain/types';
import type { LedgerRow } from '../services/api';
import { powerBuckets } from '../presentation/chartData';
import { clockLabel, formatNumber, formatTimestamp } from '../presentation/format';

interface ChargeDischargeChartProps {
  ledger: LedgerRow[];
  clock: Clock;
}

export function ChargeDischargeChart({ ledger, clock }: ChargeDischargeChartProps) {
  const chartData = useMemo(() => powerBuckets(ledger, clock), [ledger, clock]);
  const byStart = new Map(chartData.map(bucket => [bucket.startMs, bucket]));

  if (chartData.length === 0) return <div className="card">No data available for chart</div>;

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Average Charge/Discharge Power</h2>
      <p>30-minute buckets · {clockLabel(clock)}. Charge is positive; discharge is negative. Missing coverage contributes zero to the bucket average.</p>
      <ResponsiveContainer width="100%" height={350}>
        <BarChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 60 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis dataKey="startMs" angle={-45} textAnchor="end" height={80} interval="preserveStartEnd"
            tickFormatter={(instant: number) => {
              const bucket = byStart.get(instant);
              return bucket ? formatTimestamp(bucket.timestamp, clock) : '';
            }} />
          <YAxis label={{ value: 'Average power (MW)', angle: -90, position: 'insideLeft' }} />
          <ReferenceLine y={0} stroke="#666" strokeDasharray="3 3" />
          <Tooltip content={({ active, label }) => {
            const bucket = typeof label === 'number' ? byStart.get(label) : undefined;
            if (!active || !bucket) return null;
            return (
              <div className="card" style={{ margin: 0 }}>
                <strong>{formatTimestamp(bucket.timestamp, clock)}</strong>
                <div>Charge: {formatNumber(bucket.chargeMW)} MW · {formatNumber(bucket.chargeMWh)} MWh</div>
                <div>Discharge: {formatNumber(Math.abs(bucket.dischargeMW))} MW · {formatNumber(bucket.dischargeMWh)} MWh</div>
                <div>Data coverage: {formatNumber(bucket.coveredHours * 60)} of 30 minutes; {formatNumber(Math.max(0, 0.5 - bucket.coveredHours) * 60)} minutes missing</div>
              </div>
            );
          }} />
          <Legend />
          <Bar dataKey="chargeMW" fill="#3b82f6" name="Charge (average MW)" />
          <Bar dataKey="dischargeMW" fill="#f97316" name="Discharge (average MW)" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
