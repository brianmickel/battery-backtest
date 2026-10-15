import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiService } from '../services/api';
import { applyDraftAction, parseDraft, parseFormEdits, serializeDraft } from '../domain/draft';
import type { DraftAction, FieldIssue, FormEdits, FormField, ParseResult, RequestDraft } from '../domain/types';
import { useApiKey } from '../hooks/useApiKey';
import { BatterySection } from './request-editor/BatterySection';
import { DataSourceSection } from './request-editor/DataSourceSection';
import { JsonEditor } from './request-editor/JsonEditor';
import { OptionsSection } from './request-editor/OptionsSection';
import { RequestPreview } from './request-editor/RequestPreview';
import { StrategySection } from './request-editor/StrategySection';
import { fieldID } from './request-editor/fields';

interface BacktestFormProps {
  onSubmit: (draft: RequestDraft, apiKey: string) => Promise<FieldIssue[]>;
  isLoading?: boolean;
}

function initialDraft(): RequestDraft {
  const end = new Date().toISOString().slice(0, 10);
  const start = new Date(Date.parse(`${end}T00:00:00Z`) - 7 * 86_400_000).toISOString().slice(0, 10);
  return {
    kind: 'single',
    data_source: { type: 'gridstatus', dataset_id: 'caiso_lmp_real_time_5_min', location_id: 'TH_NP15_GEN-APND', start_date: start, end_date: end, timezone: 'market' },
    config: { battery_file: '1_moss_landing', strategy: { name: 'schedule' } },
    options: { include_ledger: true },
  };
}

function parseJson(text: string): ParseResult {
  try {
    const value: unknown = JSON.parse(text);
    return parseDraft(value);
  } catch {
    return { ok: false, issues: [{ field: '', message: 'Enter valid JSON before applying or submitting the request.' }] };
  }
}

export function BacktestForm({ onSubmit, isLoading = false }: BacktestFormProps) {
  const { apiKey, setApiKey } = useApiKey();
  const [draft, setDraft] = useState<RequestDraft>(initialDraft);
  const [mode, setMode] = useState<'form' | 'json'>('form');
  const [edits, setEdits] = useState<FormEdits>({});
  const [jsonText, setJsonText] = useState(() => serializeDraft(draft));
  const [issues, setIssues] = useState<FieldIssue[]>([]);
  const [notice, setNotice] = useState('');
  const errorSummary = useRef<HTMLDivElement>(null);
  const shouldFocusErrors = useRef(false);
  useEffect(() => {
    if (shouldFocusErrors.current) errorSummary.current?.focus();
    shouldFocusErrors.current = false;
  }, [issues]);

  const batteries = useQuery({ queryKey: ['batteries'], queryFn: apiService.getBatteries });
  const strategies = useQuery({ queryKey: ['strategies'], queryFn: apiService.getStrategies });
  const datasets = useQuery({ queryKey: ['datasets'], queryFn: apiService.getDatasets });
  const locations = useQuery({ queryKey: ['locations', draft.data_source.dataset_id], queryFn: () => apiService.getLocations(draft.data_source.dataset_id) });

  const candidate = mode === 'form' ? parseFormEdits(draft, edits) : parseJson(jsonText);

  function showIssues(next: FieldIssue[], focus = false) {
    shouldFocusErrors.current = focus;
    setIssues(next);
    setNotice('');
  }

  function commit(parsed: ParseResult): boolean {
    if (!parsed.ok) return false;
    setDraft(parsed.draft);
    setEdits({});
    setJsonText(serializeDraft(parsed.draft));
    setIssues([]);
    if (parsed.importedApiKey !== undefined) setApiKey(parsed.importedApiKey);
    return true;
  }

  function editField(field: FormField, text: string) {
    setEdits(current => ({ ...current, [field]: text }));
    setIssues([]);
    setNotice('');
  }

  function commitFormOnBlur() {
    const parsed = parseFormEdits(draft, edits);
    if (!commit(parsed) && !parsed.ok) showIssues(parsed.issues);
  }

  function applyAction(action: DraftAction) {
    const parsed = parseFormEdits(draft, edits);
    if (!parsed.ok) { showIssues(parsed.issues, true); return; }
    commit({ ok: true, draft: applyDraftAction(parsed.draft, action) });
    setNotice('');
  }

  function switchMode(nextMode: 'form' | 'json') {
    if (nextMode === mode) return;
    const parsed = mode === 'form' ? parseFormEdits(draft, edits) : parseJson(jsonText);
    if (!parsed.ok) { showIssues(parsed.issues, true); return; }
    commit(parsed);
    if (parsed.draft.kind === 'compare') {
      setMode('json');
      setNotice('Comparison requests are edited as JSON.');
    } else {
      setMode(nextMode);
      setNotice('');
    }
  }

  function applyJson() {
    const parsed = parseJson(jsonText);
    if (!parsed.ok) { showIssues(parsed.issues, true); return; }
    commit(parsed);
    setNotice('Request applied.');
  }

  async function submit() {
    if (isLoading) return;
    const parsed = mode === 'form' ? parseFormEdits(draft, edits) : parseJson(jsonText);
    if (!parsed.ok) { showIssues(parsed.issues, true); return; }
    commit(parsed);
    const key = parsed.importedApiKey ?? apiKey;
    if (!key.trim()) { showIssues([{ field: 'api_key', message: 'Enter your Grid Status API key.' }], true); return; }
    setNotice('');
    const submissionIssues = await onSubmit(parsed.draft, key);
    if (submissionIssues.length > 0) showIssues(submissionIssues, true);
  }

  const fields = { edits, issues, onEdit: editField, onBlur: commitFormOnBlur };
  return (
    <form className="card request-editor" noValidate aria-busy={isLoading} onSubmit={event => { event.preventDefault(); void submit(); }}>
      <h2>Backtest request</h2>
      <fieldset>
        <legend>Input mode</legend>
        <div className="radio-group">
          <label><input type="radio" name="input-mode" checked={mode === 'form'} disabled={draft.kind === 'compare'}
            aria-describedby={draft.kind === 'compare' ? 'compare-mode-help' : undefined} onChange={() => switchMode('form')} />Form Input</label>
          <label><input type="radio" name="input-mode" checked={mode === 'json'} onChange={() => switchMode('json')} />JSON Input</label>
        </div>
        {draft.kind === 'compare' && <p id="compare-mode-help">Comparison requests stay in JSON mode to preserve all variations. Apply a single request to enable Form Input.</p>}
      </fieldset>

      {issues.length > 0 && <div id="request-errors" className="error" role="alert" tabIndex={-1} ref={errorSummary}>
        <h3>Review the request</h3>
        <ul>{issues.map((issue, index) => <li key={`${issue.field}-${index}`}>
          <button type="button" className="error-link" onClick={() => {
            const target = mode === 'json' && issue.field !== 'api_key' ? 'request-json' : fieldID(issue.field);
            (document.getElementById(target) ?? errorSummary.current)?.focus();
          }}>{issue.field ? `${issue.field}: ` : ''}{issue.message}</button>
        </li>)}</ul>
      </div>}
      {notice && <p role="status">{notice}</p>}
      {[batteries, strategies, datasets, locations].some(query => query.isError) &&
        <p role="status">Some selection lists could not load. Your request is unchanged; you can still enter IDs or edit JSON.</p>}

      {mode === 'json' ? <JsonEditor text={jsonText} onChange={text => { setJsonText(text); setIssues([]); setNotice(''); }}
        onApply={applyJson} onDiscard={() => { setJsonText(serializeDraft(draft)); setIssues([]); setNotice('JSON edits discarded.'); }} />
        : draft.kind === 'single' && <>
          <DataSourceSection {...fields} source={draft.data_source} datasets={datasets.data ?? []} locations={locations.data ?? []} />
          <BatterySection {...fields} config={draft.config} batteries={batteries.data ?? []} onAction={applyAction} />
          <StrategySection {...fields} strategy={draft.config.strategy} strategies={strategies.data ?? []} onAction={applyAction} />
          <OptionsSection {...fields} options={draft.options} />
          <button type="button" onClick={() => { setEdits({}); setIssues([]); setNotice('Form edits discarded.'); }}>Discard form edits</button>
        </>}

      <RequestPreview candidate={candidate} />
      <button type="submit" className="run-request" disabled={isLoading}>{isLoading ? 'Running backtest…' : 'Run backtest'}</button>
    </form>
  );
}
