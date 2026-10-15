export type Clock = 'market' | 'UTC'
export type StrategyName = 'schedule' | 'oracle'
export type StrategyParamValue = string | number

export interface DataSourceConfig {
  type: 'gridstatus'
  dataset_id: string
  location_id: string
  start_date: string
  end_date: string
  timezone?: Clock
}

export interface BatteryConfig {
  name?: string
  energy_capacity_mwh?: number
  power_capacity_mw?: number
  charge_efficiency?: number
  discharge_efficiency?: number
  min_soc?: number
  max_soc?: number
  initial_soc?: number
  degradation_cost_per_mwh?: number
}

export interface StrategyParams {
  charge_start?: string
  charge_end?: string
  discharge_start?: string
  discharge_end?: string
  charge_power_mw?: number
  discharge_power_mw?: number
  soc_steps?: number
  power_steps?: number
}

export interface StrategyConfig {
  name?: StrategyName
  params?: StrategyParams
}

export interface BacktestConfig {
  battery_file?: string
  battery?: BatteryConfig
  strategy?: StrategyConfig
}

export interface BacktestOptions {
  limit_intervals?: number
  include_ledger?: boolean
}

export interface BacktestRequest {
  data_source: DataSourceConfig
  config: BacktestConfig
  options?: BacktestOptions
}

export interface CompareBacktestRequest {
  data_source: DataSourceConfig
  base_config: BacktestConfig
  variations: Array<{ name: string; config: BacktestConfig }>
}

export type RequestDraft =
  | ({ kind: 'single' } & BacktestRequest)
  | ({ kind: 'compare' } & CompareBacktestRequest)

export interface FieldIssue { field: string; message: string }
export type ParseResult =
  | { ok: true; draft: RequestDraft; importedApiKey?: string }
  | { ok: false; issues: FieldIssue[] }

export type FormField =
  | `data_source.${Exclude<keyof DataSourceConfig, 'type'>}`
  | 'config.battery_file'
  | `config.battery.${keyof BatteryConfig}`
  | `config.strategy.params.${keyof StrategyParams}`
  | `options.${keyof BacktestOptions}`
export type FormEdits = Partial<Record<FormField, string>>
export type DraftAction =
  | { type: 'select-strategy'; name: StrategyName }
  | { type: 'select-preset'; id: string }
  | { type: 'use-custom-battery'; battery: BatteryConfig }
