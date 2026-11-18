import { useApiKey } from '../hooks/useApiKey';

export function APIKeyInput() {
  const { apiKey, setApiKey } = useApiKey();

  return (
    <div className="card" style={{ backgroundColor: apiKey ? '#1a2a1a' : '#2a1a1a', borderColor: apiKey ? '#44ff44' : '#ff4444' }}>
      <label htmlFor="api-key">
        Grid Status API Key <span style={{ color: '#ff4444' }}>*</span>
      </label>
      <input
        id="api-key"
        type="password"
        autoComplete="off"
        aria-required="true"
        aria-describedby="api-key-help"
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
        placeholder="Enter your Grid Status API key"
        style={{ 
          fontFamily: 'monospace',
          backgroundColor: apiKey ? '#0a1a0a' : 'transparent',
          borderColor: apiKey ? '#44ff44' : '#ff4444',
        }}
      />
      <p id="api-key-help" style={{ fontSize: '0.85em', marginTop: '0.4em', marginBottom: 0 }}>
        {apiKey ? 'Key saved in this browser. ' : 'Enter a key to run backtests. '}
        Requests send this key in a header; copied request JSON excludes it.
      </p>
    </div>
  );
}
