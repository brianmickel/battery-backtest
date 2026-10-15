import { useState, useCallback, type ReactNode, type ReactElement } from 'react';
import { API_KEY_STORAGE_KEY, ApiKeyContext } from './useApiKey';

export function ApiKeyProvider({ children }: { children: ReactNode }): ReactElement {
  const [apiKey, setApiKeyState] = useState<string>(() => {
    return typeof localStorage !== 'undefined' ? localStorage.getItem(API_KEY_STORAGE_KEY) || '' : '';
  });
  const setApiKey = useCallback((key: string) => {
    setApiKeyState(key);
    if (typeof localStorage !== 'undefined') {
      if (key) {
        localStorage.setItem(API_KEY_STORAGE_KEY, key);
      } else {
        localStorage.removeItem(API_KEY_STORAGE_KEY);
      }
    }
  }, []);

  return (
    <ApiKeyContext.Provider value={{ apiKey, setApiKey }}>
      {children}
    </ApiKeyContext.Provider>
  );
}
