import { BacktestSummary } from '../services/api';

interface MetricsDisplayProps {
  summary: BacktestSummary;
}

export function MetricsDisplay({ summary }: MetricsDisplayProps) {
  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  const formatNumber = (value: number, decimals: number = 2) => {
    return new Intl.NumberFormat('en-US', {
      minimumFractionDigits: decimals,
      maximumFractionDigits: decimals,
    }).format(value);
  };

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  return (
    <div className="card" style={{ backgroundColor: '#1a2a1a' }}>
      <h2 style={{ marginTop: 0, marginBottom: '0.75em' }}>Summary Metrics</h2>
      <div style={{ 
        display: 'grid', 
        gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', 
        gap: '1em',
        marginTop: '0.5em'
      }}>
        <div>
          <div className={`metric-large ${summary.total_pnl >= 0 ? 'metric-positive' : 'metric-negative'}`}>
            {formatCurrency(summary.total_pnl)}
          </div>
          <div className="metric-label">Total P&L</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {(summary.final_soc * 100).toFixed(1)}%
          </div>
          <div className="metric-label">Final SOC</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {formatNumber(summary.energy_charged_mwh)}
          </div>
          <div className="metric-label">Energy Charged (MWh)</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {formatNumber(summary.energy_discharged_mwh)}
          </div>
          <div className="metric-label">Energy Discharged (MWh)</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {summary.total_intervals.toLocaleString()}
          </div>
          <div className="metric-label">Total Intervals</div>
        </div>

        <div>
          <div style={{ fontSize: '1rem', marginBottom: '0.25em' }}>
            {formatDate(summary.backtest_window.start)}
          </div>
          <div style={{ fontSize: '1rem' }}>
            {formatDate(summary.backtest_window.end)}
          </div>
          <div className="metric-label">Backtest Window</div>
        </div>
      </div>
    </div>
  );
}
