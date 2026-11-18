interface Props { text: string; onChange: (text: string) => void; onApply: () => void; onDiscard: () => void }

export function JsonEditor({ text, onChange, onApply, onDiscard }: Props) {
  return (
    <section>
      <label htmlFor="request-json">Request JSON</label>
      <textarea id="request-json" value={text} onChange={event => onChange(event.target.value)} rows={22} spellCheck={false}
        aria-describedby="json-help" />
      <p id="json-help">Apply, submit, or switch to form to commit valid JSON. Comparison requests remain in JSON mode.</p>
      <div className="editor-actions">
        <button type="button" onClick={onApply}>Apply JSON</button>
        <button type="button" onClick={onDiscard}>Discard JSON edits</button>
      </div>
    </section>
  );
}
