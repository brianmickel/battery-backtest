import { useState } from 'react';

const API_KEY_STORAGE_KEY = 'gridstatus_api_key';

export function useApiKey() {
  const [apiKey, setApiKeyState] = useState<string>(() => {
    // Load from localStorage on mount
    return localStorage.getItem(API_KEY_STORAGE_KEY) || '';
  });

  const setApiKey = (key: string) => {
    setApiKeyState(key);
    if (key) {
      localStorage.setItem(API_KEY_STORAGE_KEY, key);
    } else {
      localStorage.removeItem(API_KEY_STORAGE_KEY);
    }
  };

  return { apiKey, setApiKey };
}
