import type { BacktestResponse, BacktestSummary, BatteryInfo, ChargeWindow, CompareBacktestResponse, DatasetInfo, DischargeWindow, LedgerRow, LocationInfo, Ranking, StrategyInfo, TimeWindow } from './api'
import { isObject } from '../domain/validation'
import { calendarDay } from '../domain/calendar'

const finite = (value: unknown): value is number => typeof value === 'number' && Number.isFinite(value)
function timestamp(value: unknown): value is string {
  return typeof value === 'string' &&
    /^\d{4}-\d{2}-\d{2}T(?:[01]\d|2[0-3]):[0-5]\d:[0-5]\d(?:\.\d{1,9})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$/.test(value) &&
    calendarDay(value.slice(0, 10)) !== undefined && Number.isFinite(Date.parse(value))
}
function isTimeWindow(value: unknown): value is TimeWindow {
  return isObject(value) && timestamp(value.start) && timestamp(value.end)
}
function isChargeWindow(value: unknown): value is ChargeWindow {
  return isObject(value) && isTimeWindow(value) && finite(value.average_cost_per_mwh) && finite(value.energy_mwh)
}
function isDischargeWindow(value: unknown): value is DischargeWindow {
  return isObject(value) && isTimeWindow(value) && finite(value.average_price_per_mwh) && finite(value.energy_mwh)
}
function isSummary(value: unknown): value is BacktestSummary {
  return isObject(value) &&
    ['total_pnl', 'initial_soc', 'initial_energy_mwh', 'final_soc', 'final_energy_mwh', 'energy_charged_mwh', 'energy_discharged_mwh', 'covered_duration_hours', 'gap_duration_hours'].every(key => finite(value[key])) &&
    finite(value.total_intervals) && Number.isInteger(value.total_intervals) && value.total_intervals > 0 &&
    finite(value.gap_count) && Number.isInteger(value.gap_count) && value.gap_count >= 0 &&
    (value.timezone === 'market' || value.timezone === 'UTC') && isTimeWindow(value.backtest_window) &&
    (value.charge_windows === undefined || (Array.isArray(value.charge_windows) && value.charge_windows.every(isChargeWindow))) &&
    (value.discharge_windows === undefined || (Array.isArray(value.discharge_windows) && value.discharge_windows.every(isDischargeWindow)))
}
function isLedgerRow(value: unknown): value is LedgerRow {
  return isObject(value) && finite(value.index) && Number.isInteger(value.index) &&
    ['interval_start_local', 'interval_end_local', 'interval_start_utc', 'interval_end_utc'].every(key => timestamp(value[key])) &&
    typeof value.location === 'string' && typeof value.market === 'string' &&
    (value.action === 'CHARGING' || value.action === 'DISCHARGING' || value.action === 'IDLE') &&
    ['lmp', 'requested_power_mw', 'power_mw', 'energy_from_grid_mwh', 'energy_to_grid_mwh', 'throughput_mwh', 'soc_start', 'soc_end', 'pnl', 'cum_pnl'].every(key => finite(value[key]))
}
export function isBacktestResponse(value: unknown): value is BacktestResponse {
  return isObject(value) && value.status === 'completed' && isSummary(value.summary) &&
    (value.ledger === undefined || (Array.isArray(value.ledger) && value.ledger.every(isLedgerRow)))
}
export function isCompareResponse(value: unknown): value is CompareBacktestResponse {
  return isObject(value) && Array.isArray(value.comparison) && value.comparison.length > 0 && value.comparison.every(row => isObject(row) && typeof row.name === 'string' && isSummary(row.summary))
}
export function isRanking(value: unknown): value is Ranking {
  return isObject(value) && typeof value.location === 'string' && typeof value.market === 'string' &&
    ['rank', 'count', 'spread_p95_p05', 'min_lmp', 'max_lmp', 'oracle_profit'].every(key => finite(value[key]))
}
export function isBatteryInfo(value: unknown): value is BatteryInfo {
  return isObject(value) && typeof value.id === 'string' && typeof value.name === 'string' && isObject(value.specs) && finite(value.specs.energy_capacity_mwh) && finite(value.specs.power_capacity_mw)
}
export function isStrategyInfo(value: unknown): value is StrategyInfo {
  return isObject(value) && typeof value.name === 'string' && typeof value.description === 'string' && Array.isArray(value.parameters) && value.parameters.every(p => isObject(p) && typeof p.name === 'string' && typeof p.type === 'string' && typeof p.description === 'string' && (p.default === undefined || typeof p.default === 'string' || finite(p.default)) && (p.default_source === undefined || typeof p.default_source === 'string'))
}
export function isDatasetInfo(value: unknown): value is DatasetInfo {
  return isObject(value) && ['id', 'name', 'market', 'resolution'].every(key => typeof value[key] === 'string')
}
export function isLocationInfo(value: unknown): value is LocationInfo {
  return isObject(value) && ['id', 'name', 'type'].every(key => typeof value[key] === 'string')
}
