import { useState, useEffect } from 'react';
import { APIKeyInput } from './components/APIKeyInput';
import { BacktestForm } from './components/BacktestForm';
import { MetricsDisplay } from './components/MetricsDisplay';
import { ResponseViewer } from './components/ResponseViewer';
import { ChargeDischargeChart } from './components/ChargeDischargeChart';
import { CumulativePnLChart } from './components/CumulativePnLChart';
import { DailyWindowsDisplay } from './components/DailyWindowsDisplay';
import { BacktestRequest, BacktestResponse, CompareBacktestRequest, CompareBacktestResponse } from './services/api';
import { apiService } from './services/api';

function App() {
  const [result, setResult] = useState<BacktestResponse | CompareBacktestResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isMobile, setIsMobile] = useState(window.innerWidth <= 1200);

  useEffect(() => {
    const handleResize = () => {
      setIsMobile(window.innerWidth <= 1200);
    };
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const isCompareRequest = (req: BacktestRequest | CompareBacktestRequest): req is CompareBacktestRequest => {
    return 'base_config' in req && 'variations' in req;
  };

  const handleSubmit = async (request: BacktestRequest | CompareBacktestRequest) => {
    setIsLoading(true);
    setError(null);
    setResult(null);

    try {
      if (isCompareRequest(request)) {
        const response = await apiService.compareBacktests(request);
        setResult(response);
      } else {
        const response = await apiService.runBacktest(request);
        setResult(response);
      }
    } catch (err: any) {
      setError(err.response?.data?.error?.message || err.message || 'An error occurred');
      console.error('Backtest error:', err);
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div style={{ maxWidth: '1600px', margin: '0 auto', padding: '1em' }}>
      <header style={{ marginBottom: '1em', textAlign: 'center' }}>
        <h1 style={{ fontSize: '1.75em', marginBottom: '0.25em' }}>Battery Backtest Engine</h1>
        <p style={{ opacity: 0.8, fontSize: '0.9em', margin: 0 }}>Simulate battery energy storage arbitrage strategies</p>
      </header>

      <APIKeyInput />

      <div style={{ 
        display: 'grid', 
        gridTemplateColumns: isMobile ? '1fr' : '1fr 1.5fr', 
        gap: '1em', 
        marginTop: '1em' 
      }}>
        <div>
          <BacktestForm onSubmit={handleSubmit} isLoading={isLoading} />
        </div>

        <div>
          {isLoading && (
            <div className="loading">
              <div style={{ fontSize: '1.25em', marginBottom: '0.4em' }}>⏳</div>
              <div style={{ fontSize: '0.9em' }}>Running backtest...</div>
              <div style={{ marginTop: '0.75em', opacity: 0.7, fontSize: '0.85em' }}>This may take a moment</div>
            </div>
          )}

          {error && (
            <div className="error">
              <strong>Error:</strong> {error}
            </div>
          )}

          {result && (
            <div>
              {'comparison' in result ? (
                // Compare response
                <div>
                  <h2 style={{ marginBottom: '0.75em', fontSize: '1.25em' }}>Comparison Results</h2>
                  {result.comparison.map((comp, idx) => (
                    <div key={idx} className="card" style={{ marginBottom: '0.75em' }}>
                      <h3 style={{ marginTop: 0, marginBottom: '0.5em', fontSize: '1.1em' }}>{comp.name}</h3>
                      <MetricsDisplay summary={comp.summary} />
                      {comp.summary.charge_windows && comp.summary.discharge_windows && (
                        <DailyWindowsDisplay
                          chargeWindows={comp.summary.charge_windows}
                          dischargeWindows={comp.summary.discharge_windows}
                        />
                      )}
                    </div>
                  ))}
                  <ResponseViewer data={result} />
                </div>
              ) : (
                // Regular backtest response
                <div>
                  <MetricsDisplay summary={result.summary} />
                  
                  {result.summary.charge_windows && result.summary.discharge_windows && (
                    <DailyWindowsDisplay
                      chargeWindows={result.summary.charge_windows}
                      dischargeWindows={result.summary.discharge_windows}
                    />
                  )}

                  {result.ledger && result.ledger.length > 0 ? (
                    <>
                      <ChargeDischargeChart ledger={result.ledger} />
                      <CumulativePnLChart ledger={result.ledger} />
                    </>
                  ) : (
                    <div className="card" style={{ backgroundColor: '#2a2a1a', borderColor: '#ffaa00' }}>
                      <p>
                        <strong>Note:</strong> Charts require ledger data. 
                        Enable "Include Ledger" in the form options to see charge/discharge and P&L charts.
                      </p>
                    </div>
                  )}

                  <ResponseViewer data={result} />
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

export default App;
