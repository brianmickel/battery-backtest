import { createContext, useContext } from 'react';

export const API_KEY_STORAGE_KEY = 'gridstatus_api_key';

export type ApiKeyContextValue = {
  apiKey: string;
  setApiKey: (key: string) => void;
  /** Use when you need the latest value inside an event handler (avoids stale value before re-render). */
  getApiKey: () => string;
};

export const ApiKeyContext = createContext<ApiKeyContextValue | null>(null);

export function useApiKey(): ApiKeyContextValue {
  const ctx = useContext(ApiKeyContext);
  if (!ctx) {
    throw new Error('useApiKey must be used within ApiKeyProvider');
  }
  return ctx;
}
