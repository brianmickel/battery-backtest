import { useState, useCallback, useRef, type ReactNode, type ReactElement } from 'react';
import { API_KEY_STORAGE_KEY, ApiKeyContext } from './useApiKey';

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
