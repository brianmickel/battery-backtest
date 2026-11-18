import type { BacktestOptions, DataSourceConfig, DraftAction, FormEdits, ParseResult, RequestDraft } from './types'
import { isObject, Validation } from './validation'
import { isPresetID, mergeConfig, parseConfig, strategyDefaults, validateConfig } from './configuration'
import { validateDateRange } from './calendar'

export type { DraftAction, FieldIssue, FormEdits, FormField, ParseResult, RequestDraft } from './types'

export function parseDraft(value: unknown): ParseResult {
  const v = new Validation()
  const compare = isObject(value) && ('base_config' in value || 'variations' in value)
  const raw = v.object(value, '', compare ? ['api_key', 'data_source', 'base_config', 'variations'] : ['api_key', 'data_source', 'config', 'options'])
  const source = v.object(raw.data_source, 'data_source', ['type', 'dataset_id', 'location_id', 'start_date', 'end_date', 'timezone'])
  if (source.type !== 'gridstatus') v.issue('data_source.type', 'Choose gridstatus.')
  const dataset = v.text(source.dataset_id, 'data_source.dataset_id', true)
  const location = v.text(source.location_id, 'data_source.location_id', true)
  const start = v.text(source.start_date, 'data_source.start_date', true)
  const end = v.text(source.end_date, 'data_source.end_date', true)
  if (source.timezone !== undefined && source.timezone !== 'market' && source.timezone !== 'UTC') v.issue('data_source.timezone', 'Choose market or UTC.')
  if (start !== undefined && end !== undefined) v.issues.push(...validateDateRange(start, end))
  let importedApiKey: string | undefined
  if (raw.api_key !== undefined) {
    const key = v.text(raw.api_key, 'api_key', true)
    if (key !== undefined && key !== '{{API_KEY}}') importedApiKey = key
  }
  const data_source: DataSourceConfig = {
    type: 'gridstatus', dataset_id: dataset ?? '', location_id: location ?? '', start_date: start ?? '', end_date: end ?? '',
    ...(source.timezone === 'market' || source.timezone === 'UTC' ? { timezone: source.timezone } : {}),
  }
  let draft: RequestDraft
  if (compare) {
    const base = parseConfig(raw.base_config, 'base_config', v)
    const variations: Array<{ name: string; config: ReturnType<typeof parseConfig> }> = []
    if (!Array.isArray(raw.variations) || raw.variations.length < 1 || raw.variations.length > 16) v.issue('variations', 'Provide 1 to 16 variations.')
    else {
      const names = new Set<string>()
      raw.variations.forEach((entry: unknown, i: number) => {
        const field = `variations[${i}]`
        const item = v.object(entry, field, ['name', 'config'])
        const name = v.text(item.name, `${field}.name`, true) ?? ''
        if (names.has(name)) v.issue(`${field}.name`, 'Variation names must be unique.')
        names.add(name)
        const config = parseConfig(item.config, `${field}.config`, v)
        validateConfig(mergeConfig(base, config), `${field}.config`, v)
        variations.push({ name, config })
      })
    }
    draft = { kind: 'compare', data_source, base_config: base, variations }
  } else {
    const config = parseConfig(raw.config, 'config', v)
    validateConfig(config, 'config', v)
    const options: BacktestOptions = {}
    if (raw.options !== undefined) {
      const input = v.object(raw.options, 'options', ['limit_intervals', 'include_ledger'])
      const limit = v.number(input.limit_intervals, 'options.limit_intervals', 0, 17280, true)
      if (limit !== undefined) options.limit_intervals = limit
      if (input.include_ledger !== undefined) {
        if (typeof input.include_ledger === 'boolean') options.include_ledger = input.include_ledger
        else v.issue('options.include_ledger', 'Expected a boolean.')
      }
    }
    draft = { kind: 'single', data_source, config, ...(raw.options !== undefined ? { options } : {}) }
  }
  if (v.issues.length) return { ok: false, issues: v.issues }
  return { ok: true, draft, ...(importedApiKey !== undefined ? { importedApiKey } : {}) }
}

export function draftRequest(draft: RequestDraft) {
  if (draft.kind === 'single') return { data_source: draft.data_source, config: draft.config, ...(draft.options !== undefined ? { options: draft.options } : {}) }
  return { data_source: draft.data_source, base_config: draft.base_config, variations: draft.variations }
}

export function serializeDraft(draft: RequestDraft): string {
  return JSON.stringify(draftRequest(draft), null, 2)
}

const numericText = /^[+-]?(?:\d+(?:\.\d+)?|\.\d+)(?:[eE][+-]?\d+)?$/
const stringFields = new Set(['config.battery_file', 'config.battery.name', 'config.strategy.params.charge_start', 'config.strategy.params.charge_end', 'config.strategy.params.discharge_start', 'config.strategy.params.discharge_end'])

export function parseFormEdits(draft: RequestDraft, edits: FormEdits): ParseResult {
  if (draft.kind !== 'single') return { ok: false, issues: [{ field: 'kind', message: 'Comparison requests are edited as JSON.' }] }
  const candidate: unknown = JSON.parse(serializeDraft(draft))
  if (!isObject(candidate)) return { ok: false, issues: [{ field: '', message: 'Invalid draft.' }] }
  const v = new Validation()
  for (const [field, text] of Object.entries(edits)) {
    if (text === undefined) continue
    const path = field.split('.')
    const key = path.pop()
    if (!key) continue
    let parent = candidate
    for (const part of path) {
      if (!isObject(parent[part])) parent[part] = {}
      const child = parent[part]
      if (isObject(child)) parent = child
    }
    const optional = field.startsWith('config.strategy.params.') || field.startsWith('options.') || field === 'config.battery.name' || field === 'config.battery.initial_soc' || field === 'config.battery.degradation_cost_per_mwh' || (draft.config.battery_file !== undefined && field.startsWith('config.battery.')) || field === 'data_source.timezone'
    if (!text.trim() && optional) { delete parent[key]; continue }
    if (field.startsWith('data_source.') || stringFields.has(field)) parent[key] = text
    else if (field === 'options.include_ledger') {
      if (text === 'true' || text === 'false') parent[key] = text === 'true'
      else v.issue(field, 'Choose true or false.')
    } else if (!numericText.test(text) || !Number.isFinite(Number(text))) v.issue(field, 'Enter a complete finite number.')
    else parent[key] = Number(text)
  }
  const result = parseDraft(candidate)
  if (v.issues.length) return { ok: false, issues: [...v.issues, ...(result.ok ? [] : result.issues)] }
  return result
}

export function applyDraftAction(draft: RequestDraft, action: DraftAction): RequestDraft {
  if (draft.kind !== 'single') return draft
  let config = draft.config
  switch (action.type) {
    case 'select-strategy': config = { ...config, strategy: strategyDefaults(action.name) }; break
    case 'select-preset':
      if (!isPresetID(action.id)) return draft
      config = { battery_file: action.id, strategy: config.strategy }; break
    case 'use-custom-battery': config = { battery: { ...action.battery }, strategy: config.strategy }; break
    default: { const exhaustive: never = action; return exhaustive }
  }
  const parsed = parseDraft({ ...draftRequest(draft), config })
  return parsed.ok ? parsed.draft : draft
}
