import { useMemo } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, Legend, ResponsiveContainer } from 'recharts';
import { LedgerRow } from '../services/api';

interface CumulativePnLChartProps {
  ledger: LedgerRow[];
}

export function CumulativePnLChart({ ledger }: CumulativePnLChartProps) {
  const chartData = useMemo(() => {
    return ledger.map(row => ({
      time: new Date(row.interval_start_local).toLocaleString(),
      cumPnl: row.cum_pnl,
      pnl: row.pnl,
    }));
  }, [ledger]);

  if (chartData.length === 0) {
    return <div className="card">No data available for chart</div>;
  }

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Cumulative P&L</h2>
      <ResponsiveContainer width="100%" height={350}>
        <LineChart data={chartData} margin={{ top: 20, right: 30, left: 20, bottom: 60 }}>
          <CartesianGrid strokeDasharray="3 3" />
          <XAxis 
            dataKey="time" 
            angle={-45}
            textAnchor="end"
            height={80}
            interval="preserveStartEnd"
          />
          <YAxis 
            label={{ value: 'Cumulative P&L ($)', angle: -90, position: 'insideLeft' }}
            tickFormatter={(value) => formatCurrency(value)}
          />
          <Tooltip 
            formatter={(value: number) => formatCurrency(value)}
            labelFormatter={(label) => `Time: ${label}`}
          />
          <Legend />
          <Line 
            type="monotone" 
            dataKey="cumPnl" 
            stroke={chartData[chartData.length - 1]?.cumPnl >= 0 ? '#4ade80' : '#f87171'}
            strokeWidth={2}
            dot={false}
            name="Cumulative P&L"
          />
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}
