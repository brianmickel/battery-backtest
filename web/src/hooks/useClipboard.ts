import { useState } from 'react';

export function useClipboard() {
  const [feedback, setFeedback] = useState<{ value: string; message: string } | null>(null);
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setFeedback({ value, message: 'Copied to clipboard.' });
    } catch {
      setFeedback({ value, message: 'Could not copy. Select the JSON and copy it manually.' });
    }
  }
  return { copy, messageFor: (value: string) => feedback?.value === value ? feedback.message : '' };
}
