import { useMemo } from 'react';
import { BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer, ReferenceLine } from 'recharts';
import { LedgerRow } from '../services/api';

interface ChargeDischargeChartProps {
  ledger: LedgerRow[];
}

interface ChartDataPoint {
  time: string;
  charge: number;
  discharge: number;
}

export function ChargeDischargeChart({ ledger }: ChargeDischargeChartProps) {
  const chartData = useMemo(() => {
    // Group into 30-minute intervals
    const intervals = new Map<string, { charge: number; discharge: number }>();

    ledger.forEach(row => {
      const date = new Date(row.interval_start_local);
      // Round down to nearest 30 minutes
      const minutes = date.getMinutes();
      const roundedMinutes = Math.floor(minutes / 30) * 30;
      const roundedDate = new Date(date);
      roundedDate.setMinutes(roundedMinutes, 0, 0);
      
      const key = roundedDate.toISOString();
      if (!intervals.has(key)) {
        intervals.set(key, { charge: 0, discharge: 0 });
      }

      const interval = intervals.get(key)!;
      if (row.energy_from_grid_mwh > 0) {
        // Charge: power is negative, but we want to show as positive above x-axis
        interval.charge += Math.abs(row.power_mw);
      }
      if (row.energy_to_grid_mwh > 0) {
        // Discharge: power is positive, but we want to show as negative below x-axis
        interval.discharge += row.power_mw;
      }
    });

    // Convert to array and sort by time
    const data: ChartDataPoint[] = Array.from(intervals.entries())
      .map(([time, values]) => ({
        time: new Date(time).toLocaleString(),
        charge: values.charge, // Positive, will be above x-axis
        discharge: -values.discharge, // Negative, will be below x-axis
      }))
      .sort((a, b) => new Date(a.time).getTime() - new Date(b.time).getTime());

    return data;
  }, [ledger]);

  if (chartData.length === 0) {
    return <div className="card">No data available for chart</div>;
  }

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Charge/Discharge Power (30-minute intervals)</h2>
      <ResponsiveContainer width="100%" height={350}>
        <BarChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 60 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis 
            dataKey="time" 
            angle={-45}
            textAnchor="end"
            height={80}
            interval="preserveStartEnd"
          />
          <YAxis 
            label={{ value: 'Power (MW)', angle: -90, position: 'insideLeft' }}
          />
          <ReferenceLine y={0} stroke="#666" strokeDasharray="3 3" />
          <Tooltip 
            formatter={(value: number, name: string) => {
              const absValue = Math.abs(value);
              return [`${absValue.toFixed(2)} MW`, name === 'charge' ? 'Charge' : 'Discharge'];
            }}
            labelFormatter={(label) => `Time: ${label}`}
          />
          <Legend />
          <Bar dataKey="charge" fill="#3b82f6" name="Charge" />
          <Bar dataKey="discharge" fill="#f97316" name="Discharge" />
        </BarChart>
      </ResponsiveContainer>
    </div>
  );
}
