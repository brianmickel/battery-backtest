import { ChargeWindow, DischargeWindow } from '../services/api';

interface DailyWindowsDisplayProps {
  chargeWindows?: ChargeWindow[];
  dischargeWindows?: DischargeWindow[];
}

export function DailyWindowsDisplay({ chargeWindows = [], dischargeWindows = [] }: DailyWindowsDisplayProps) {
  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  if (chargeWindows.length === 0 && dischargeWindows.length === 0) {
    return null;
  }

  // Get unique days
  const days = new Set<string>();
  chargeWindows.forEach(w => {
    const day = new Date(w.start).toDateString();
    days.add(day);
  });
  dischargeWindows.forEach(w => {
    const day = new Date(w.start).toDateString();
    days.add(day);
  });

  const sortedDays = Array.from(days).sort((a, b) => 
    new Date(a).getTime() - new Date(b).getTime()
  );

  return (
    <div className="card">
      <h2 style={{ marginBottom: '0.75em' }}>Daily Charge/Discharge Windows</h2>
      {sortedDays.map(day => {
        const dayChargeWindows = chargeWindows.filter(w => 
          new Date(w.start).toDateString() === day
        );
        const dayDischargeWindows = dischargeWindows.filter(w => 
          new Date(w.start).toDateString() === day
        );

        return (
          <div key={day} style={{ marginBottom: '1em', paddingBottom: '1em', borderBottom: '1px solid #333' }}>
            <h3 style={{ color: '#646cff', fontSize: '1em', marginTop: 0, marginBottom: '0.5em' }}>{day}</h3>
            
            {dayChargeWindows.length > 0 && (
              <div style={{ marginBottom: '0.75em' }}>
                <h4 style={{ color: '#3b82f6', fontSize: '0.9em', marginTop: 0, marginBottom: '0.4em' }}>Charge Windows</h4>
                {dayChargeWindows.map((win, idx) => (
                  <div key={idx} style={{ 
                    backgroundColor: '#1a1a2a', 
                    padding: '0.75em', 
                    borderRadius: '4px',
                    marginBottom: '0.4em',
                    fontSize: '0.9em'
                  }}>
                    <div><strong>Time:</strong> {formatDate(win.start)} - {formatDate(win.end)}</div>
                    <div><strong>Energy:</strong> {win.energy_mwh.toFixed(2)} MWh</div>
                    <div><strong>Average Cost:</strong> {formatCurrency(win.average_cost_per_mwh)}/MWh</div>
                  </div>
                ))}
              </div>
            )}

            {dayDischargeWindows.length > 0 && (
              <div>
                <h4 style={{ color: '#f97316', fontSize: '0.9em', marginTop: 0, marginBottom: '0.4em' }}>Discharge Windows</h4>
                {dayDischargeWindows.map((win, idx) => (
                  <div key={idx} style={{ 
                    backgroundColor: '#2a1a1a', 
                    padding: '0.75em', 
                    borderRadius: '4px',
                    marginBottom: '0.4em',
                    fontSize: '0.9em'
                  }}>
                    <div><strong>Time:</strong> {formatDate(win.start)} - {formatDate(win.end)}</div>
                    <div><strong>Energy:</strong> {win.energy_mwh.toFixed(2)} MWh</div>
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
