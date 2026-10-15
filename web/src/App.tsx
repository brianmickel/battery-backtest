import { useEffect, useRef, useState } from 'react';
import { APIKeyInput } from './components/APIKeyInput';
import { BacktestForm } from './components/BacktestForm';
import { MetricsDisplay } from './components/MetricsDisplay';
import { ResponseViewer } from './components/ResponseViewer';
import { ChargeDischargeChart } from './components/ChargeDischargeChart';
import { CumulativePnLChart } from './components/CumulativePnLChart';
import { DailyWindowsDisplay } from './components/DailyWindowsDisplay';
import type { BacktestResponse, CompareBacktestResponse } from './services/api';
import { apiService, ApiError, getErrorMessage } from './services/api';
import { draftRequest } from './domain/draft';
import type { FieldIssue, RequestDraft } from './domain/types';

function App() {
  const [result, setResult] = useState<BacktestResponse | CompareBacktestResponse | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const activeRequest = useRef<AbortController | null>(null);
  useEffect(() => () => activeRequest.current?.abort(), []);

  async function handleSubmit(draft: RequestDraft, apiKey: string): Promise<FieldIssue[]> {
    activeRequest.current?.abort();
    const controller = new AbortController();
    activeRequest.current = controller;
    setIsLoading(true);
    setResult(null);
    const request = draftRequest(draft);
    try {
      const response = request.variations !== undefined
        ? await apiService.compareBacktests(request, apiKey, controller.signal)
        : await apiService.runBacktest(request, apiKey, controller.signal);
      setResult(response);
      return [];
    } catch (error) {
      return [{ field: error instanceof ApiError ? error.field ?? '' : '', message: getErrorMessage(error, 'The request could not be completed. Please try again.') }];
    } finally {
      setIsLoading(false);
      activeRequest.current = null;
    }
  }

  return (
    <main className="app-shell">
      <header style={{ marginBottom: '1em', textAlign: 'center' }}>
        <h1 style={{ fontSize: '1.75em', marginBottom: '0.25em' }}>Battery Backtest Engine</h1>
        <p style={{ opacity: 0.8, fontSize: '0.9em', margin: 0 }}>Simulate battery energy storage arbitrage strategies</p>
      </header>
      <APIKeyInput />
      <div className="app-columns">
        <BacktestForm onSubmit={handleSubmit} isLoading={isLoading} />
        <section aria-label="Backtest results">
          {isLoading && <p className="loading" role="status">Running backtest…</p>}
          {result && ('comparison' in result ? <>
            <h2>Comparison Results</h2>
            {result.comparison.map(comparison => <section key={comparison.name} className="card">
              <h3>{comparison.name}</h3>
              <MetricsDisplay summary={comparison.summary} />
              <DailyWindowsDisplay clock={comparison.summary.timezone} chargeWindows={comparison.summary.charge_windows} dischargeWindows={comparison.summary.discharge_windows} />
            </section>)}
            <ResponseViewer data={result} />
          </> : <>
            <MetricsDisplay summary={result.summary} />
            <DailyWindowsDisplay clock={result.summary.timezone} chargeWindows={result.summary.charge_windows} dischargeWindows={result.summary.discharge_windows} />
            {result.ledger && result.ledger.length > 0 ? <>
              <ChargeDischargeChart ledger={result.ledger} clock={result.summary.timezone} />
              <CumulativePnLChart ledger={result.ledger} clock={result.summary.timezone} />
            </> : <p className="card">Charts require ledger data. Enable “Include ledger for charts” to display them.</p>}
            <ResponseViewer data={result} />
          </>)}
        </section>
      </div>
    </main>
  );
}

export default App;
