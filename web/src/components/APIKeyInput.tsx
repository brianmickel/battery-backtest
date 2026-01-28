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
        value={apiKey}
        onChange={(e) => setApiKey(e.target.value)}
        placeholder="Enter your Grid Status API key"
        style={{ 
          fontFamily: 'monospace',
          backgroundColor: apiKey ? '#0a1a0a' : 'transparent',
          borderColor: apiKey ? '#44ff44' : '#ff4444',
        }}
      />
      {!apiKey && (
        <p style={{ color: '#ffaaaa', fontSize: '0.85em', marginTop: '0.4em', marginBottom: 0 }}>
          API key is required to run backtests
        </p>
      )}
      {apiKey && (
        <p style={{ color: '#aaffaa', fontSize: '0.85em', marginTop: '0.4em', marginBottom: 0 }}>
          ✓ API key saved (stored locally in your browser)
        </p>
      )}
    </div>
  );
}
