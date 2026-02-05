import { createContext, useContext, useState, useCallback, useRef, type ReactNode, type ReactElement } from 'react';

const API_KEY_STORAGE_KEY = 'gridstatus_api_key';

type ApiKeyContextValue = {
  apiKey: string;
  setApiKey: (key: string) => void;
  /** Use when you need the latest value inside an event handler (avoids stale value before re-render). */
  getApiKey: () => string;
};

const ApiKeyContext = createContext<ApiKeyContextValue | null>(null);

export function ApiKeyProvider({ children }: { children: ReactNode }): ReactElement {
  const [apiKey, setApiKeyState] = useState<string>(() => {
    return typeof localStorage !== 'undefined' ? localStorage.getItem(API_KEY_STORAGE_KEY) || '' : '';
  });
  const apiKeyRef = useRef(apiKey);
  apiKeyRef.current = apiKey;

  const setApiKey = useCallback((key: string) => {
    apiKeyRef.current = key;
    setApiKeyState(key);
    if (typeof localStorage !== 'undefined') {
      if (key) {
        localStorage.setItem(API_KEY_STORAGE_KEY, key);
      } else {
        localStorage.removeItem(API_KEY_STORAGE_KEY);
      }
    }
  }, []);

  const getApiKey = useCallback(() => apiKeyRef.current, []);

  return (
    <ApiKeyContext.Provider value={{ apiKey, setApiKey, getApiKey }}>
      {children}
    </ApiKeyContext.Provider>
  );
}

export function useApiKey(): ApiKeyContextValue {
  const ctx = useContext(ApiKeyContext);
  if (!ctx) {
    throw new Error('useApiKey must be used within ApiKeyProvider');
  }
  return ctx;
}
