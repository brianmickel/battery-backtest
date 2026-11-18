import type { Clock } from '../domain/types';
import type { ChargeWindow, DischargeWindow } from '../services/api';
import { clockLabel, formatCurrency, formatNumber, formatTimestamp } from '../presentation/format';
import { groupWindows } from '../presentation/windows';

interface DailyWindowsDisplayProps {
  clock: Clock;
  chargeWindows?: ChargeWindow[];
  dischargeWindows?: DischargeWindow[];
}

export function DailyWindowsDisplay({ chargeWindows = [], dischargeWindows = [], clock }: DailyWindowsDisplayProps) {
  if (chargeWindows.length === 0 && dischargeWindows.length === 0) {
    return null;
  }

  const days = groupWindows(chargeWindows, dischargeWindows, clock);

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Charge/Discharge Windows</h2>
      <p>{clockLabel(clock)} · Each window is one continuous operating run.</p>
      {days.map(({ date, chargeWindows: dayChargeWindows, dischargeWindows: dayDischargeWindows }) => {

        return (
          <div key={date} style={{ marginBottom: '1em', paddingBottom: '1em', borderBottom: '1px solid #333' }}>
            <h3 style={{ color: '#646cff', fontSize: '1em', marginTop: 0, marginBottom: '0.5em' }}>{date}</h3>

            {dayChargeWindows.length > 0 && (
              <div style={{ marginBottom: '0.75em' }}>
                <h4 style={{ color: '#3b82f6', fontSize: '0.9em', marginTop: 0, marginBottom: '0.4em' }}>Charge Windows</h4>
                {dayChargeWindows.map(win => (
                  <div key={win.start} style={{
                    backgroundColor: '#1a1a2a',
                    padding: '0.75em',
                    borderRadius: '4px',
                    marginBottom: '0.4em',
                    fontSize: '0.9em'
                  }}>
                    <div><strong>Time:</strong> {formatTimestamp(win.start, clock)} – {formatTimestamp(win.end, clock)}</div>
                    <div><strong>Energy:</strong> {formatNumber(win.energy_mwh)} MWh</div>
                    <div><strong>Average Cost:</strong> {formatCurrency(win.average_cost_per_mwh)}/MWh</div>
                  </div>
                ))}
              </div>
            )}

            {dayDischargeWindows.length > 0 && (
              <div>
                <h4 style={{ color: '#f97316', fontSize: '0.9em', marginTop: 0, marginBottom: '0.4em' }}>Discharge Windows</h4>
                {dayDischargeWindows.map(win => (
                  <div key={win.start} style={{
                    backgroundColor: '#2a1a1a',
                    padding: '0.75em',
                    borderRadius: '4px',
                    marginBottom: '0.4em',
                    fontSize: '0.9em'
                  }}>
                    <div><strong>Time:</strong> {formatTimestamp(win.start, clock)} – {formatTimestamp(win.end, clock)}</div>
                    <div><strong>Energy:</strong> {formatNumber(win.energy_mwh)} MWh</div>
                    <div><strong>Average Price:</strong> {formatCurrency(win.average_price_per_mwh)}/MWh</div>
                  </div>
                ))}
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
