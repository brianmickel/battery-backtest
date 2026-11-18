import type { ParseResult } from '../../domain/types';
import { serializeDraft } from '../../domain/draft';
import { useClipboard } from '../../hooks/useClipboard';

export function RequestPreview({ candidate }: { candidate: ParseResult }) {
  const { copy, messageFor } = useClipboard();
  const json = candidate.ok ? serializeDraft(candidate.draft) : '';
  return (
    <section aria-labelledby="preview-title">
      <h3 id="preview-title">Request preview</h3>
      {candidate.ok ? <pre className="request-preview" data-testid="request-preview">{json}</pre>
        : <p role="status">Preview unavailable: the displayed edits are incomplete or invalid.</p>}
      <button type="button" disabled={!candidate.ok} onClick={() => void copy(json)}>Copy request JSON</button>
      <p role="status">{messageFor(json)}</p>
    </section>
  );
}
