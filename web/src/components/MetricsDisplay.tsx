import type { BacktestSummary } from '../services/api';
import { clockLabel, formatCoverage, formatCurrency, formatNumber, formatTimestamp } from '../presentation/format';

interface MetricsDisplayProps {
  summary: BacktestSummary;
}

export function MetricsDisplay({ summary }: MetricsDisplayProps) {
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
          <div className="metric-label">Net Dispatch P&amp;L</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {formatNumber(summary.initial_soc * 100, 1)}%
          </div>
          <div className="metric-label">Initial SOC · {formatNumber(summary.initial_energy_mwh)} MWh stored</div>
        </div>

        <div>
          <div className="metric-large" style={{ fontSize: '2rem' }}>
            {formatNumber(summary.final_soc * 100, 1)}%
          </div>
          <div className="metric-label">Final SOC · {formatNumber(summary.final_energy_mwh)} MWh stored</div>
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
            {formatNumber(summary.total_intervals, 0)}
          </div>
          <div className="metric-label">Total Intervals</div>
        </div>

        <div>
          <div style={{ fontSize: '1rem', marginBottom: '0.25em' }}>
            {formatTimestamp(summary.backtest_window.start, summary.timezone)}
          </div>
          <div style={{ fontSize: '1rem' }}>
            {formatTimestamp(summary.backtest_window.end, summary.timezone)}
          </div>
          <div className="metric-label">Backtest Window · {clockLabel(summary.timezone)}</div>
        </div>
      </div>
      <p>{formatCoverage(summary)} ({formatNumber(summary.gap_count, 0)} {summary.gap_count === 1 ? 'gap' : 'gaps'}).</p>
      <p>Net dispatch cash flow includes degradation. Initial stored energy has no acquisition charge; final stored energy has zero salvage value.</p>
    </div>
  );
}
