import type { BacktestConfig, BatteryConfig, StrategyConfig, StrategyParams, StrategyName } from './types'
import { Validation } from './validation'

export const batteryFields = ['name', 'energy_capacity_mwh', 'power_capacity_mw', 'charge_efficiency', 'discharge_efficiency', 'min_soc', 'max_soc', 'initial_soc', 'degradation_cost_per_mwh'] as const
export const scheduleFields = ['charge_start', 'charge_end', 'discharge_start', 'discharge_end', 'charge_power_mw', 'discharge_power_mw'] as const
export const oracleFields = ['soc_steps', 'power_steps'] as const

export function isPresetID(value: string): boolean {
  return /^[A-Za-z0-9_-]+$/.test(value)
}

function battery(value: unknown, field: string, validation: Validation): BatteryConfig {
  const raw = validation.object(value, field, batteryFields)
  const result: BatteryConfig = {}
  if (raw.name !== undefined) {
    if (typeof raw.name !== 'string') validation.issue(`${field}.name`, 'Expected a string.')
    else result.name = raw.name
  }
  for (const key of batteryFields) {
    if (key === 'name' || raw[key] === undefined) continue
    const parsed = validation.number(raw[key], `${field}.${key}`, -Number.MAX_VALUE, Number.MAX_VALUE)
    if (parsed !== undefined) result[key] = parsed
  }
  return result
}

function strategy(value: unknown, field: string, validation: Validation): StrategyConfig {
  const raw = validation.object(value, field, ['name', 'params'])
  const result: StrategyConfig = {}
  if (raw.name !== undefined) {
    if (raw.name === 'schedule' || raw.name === 'oracle') result.name = raw.name
    else validation.issue(`${field}.name`, 'Choose schedule or oracle.')
  }
  if (raw.params === undefined) return result
  const params = validation.object(raw.params, `${field}.params`, [...scheduleFields, ...oracleFields])
  const parsed: StrategyParams = {}
  for (const key of ['charge_start', 'charge_end', 'discharge_start', 'discharge_end'] as const) {
    if (params[key] === undefined) continue
    if (typeof params[key] !== 'string') validation.issue(`${field}.params.${key}`, 'Expected a time string.')
    else parsed[key] = params[key]
  }
  for (const key of ['charge_power_mw', 'discharge_power_mw'] as const) {
    const number = validation.number(params[key], `${field}.params.${key}`, -Number.MAX_VALUE, Number.MAX_VALUE)
    if (number !== undefined) parsed[key] = number
  }
  const soc = validation.number(params.soc_steps, `${field}.params.soc_steps`, Number.MIN_SAFE_INTEGER, Number.MAX_SAFE_INTEGER, true)
  const power = validation.number(params.power_steps, `${field}.params.power_steps`, Number.MIN_SAFE_INTEGER, Number.MAX_SAFE_INTEGER, true)
  if (soc !== undefined) parsed.soc_steps = soc
  if (power !== undefined) parsed.power_steps = power
  if (result.name !== undefined) {
    const incompatible = result.name === 'schedule' ? oracleFields : scheduleFields
    for (const key of incompatible) if (params[key] !== undefined) validation.issue(`${field}.params.${key}`, `This parameter is not supported by ${result.name}.`)
  }
  result.params = parsed
  return result
}

export function parseConfig(value: unknown, field: string, validation: Validation): BacktestConfig {
  const raw = validation.object(value, field, ['battery_file', 'battery', 'strategy'])
  const result: BacktestConfig = {}
  if (raw.battery_file !== undefined) {
    const id = validation.text(raw.battery_file, `${field}.battery_file`)
    if (id !== undefined) {
      result.battery_file = id
      if (!isPresetID(id)) validation.issue(`${field}.battery_file`, 'Use a preset ID, not a file path.')
    }
  }
  if (raw.battery !== undefined) result.battery = battery(raw.battery, `${field}.battery`, validation)
  if (raw.strategy !== undefined) result.strategy = strategy(raw.strategy, `${field}.strategy`, validation)
  return result
}

export function mergeConfig(base: BacktestConfig, patch: BacktestConfig): BacktestConfig {
  const changedStrategy = patch.strategy?.name !== undefined && patch.strategy.name !== base.strategy?.name
  return {
    ...base, ...patch,
    ...(base.battery || patch.battery ? { battery: { ...base.battery, ...patch.battery } } : {}),
    ...(base.strategy || patch.strategy ? { strategy: changedStrategy ? patch.strategy : {
      ...base.strategy, ...patch.strategy,
      ...(base.strategy?.params || patch.strategy?.params ? { params: { ...base.strategy?.params, ...patch.strategy?.params } } : {}),
    } } : {}),
  }
}

function minute(value: string): number { return Number(value.slice(0, 2)) * 60 + Number(value.slice(3)) }
function contains(start: number, end: number, point: number): boolean {
  return start < end ? point >= start && point < end : start > end && (point >= start || point < end)
}

export function validateConfig(config: BacktestConfig, field: string, validation: Validation): void {
  const b = config.battery ?? {}
  for (const key of batteryFields) {
    if (key === 'name' || b[key] === undefined) continue
    const max = ['charge_efficiency', 'discharge_efficiency', 'min_soc', 'max_soc', 'initial_soc'].includes(key) ? 1 : Number.MAX_VALUE
    validation.number(b[key], `${field}.battery.${key}`, 0, max)
    if (b[key] === 0 && ['energy_capacity_mwh', 'power_capacity_mw', 'charge_efficiency', 'discharge_efficiency'].includes(key)) validation.issue(`${field}.battery.${key}`, 'Must be greater than zero.')
  }
  if (config.battery_file === undefined) {
    for (const key of ['energy_capacity_mwh', 'power_capacity_mw', 'charge_efficiency', 'discharge_efficiency', 'min_soc', 'max_soc'] as const) {
      if (b[key] === undefined) validation.issue(`${field}.battery.${key}`, 'Required for a custom battery.')
    }
  }
  if (b.min_soc !== undefined && b.max_soc !== undefined && b.min_soc > b.max_soc) validation.issue(`${field}.battery.max_soc`, 'Maximum SOC must be at least minimum SOC.')
  if (b.initial_soc !== undefined && ((b.min_soc !== undefined && b.initial_soc < b.min_soc) || (b.max_soc !== undefined && b.initial_soc > b.max_soc))) validation.issue(`${field}.battery.initial_soc`, 'Initial SOC must lie within the explicit SOC bounds.')
  const settings = config.strategy
  if (!settings?.name) {
    validation.issue(`${field}.strategy.name`, 'Choose a strategy.')
    return
  }
  const params = settings.params ?? {}
  const incompatible = settings.name === 'schedule' ? oracleFields : scheduleFields
  for (const key of incompatible) if (params[key] !== undefined) validation.issue(`${field}.strategy.params.${key}`, `This parameter is not supported by ${settings.name}.`)
  if (settings.name === 'oracle') {
    validation.number(params.soc_steps, `${field}.strategy.params.soc_steps`, 2, 1000, true)
    validation.number(params.power_steps, `${field}.strategy.params.power_steps`, 1, 50, true)
    return
  }
  for (const key of ['charge_start', 'charge_end', 'discharge_start', 'discharge_end'] as const) {
    const value = params[key]
    if (value !== undefined && !/^([01]\d|2[0-3]):[0-5]\d$/.test(value)) validation.issue(`${field}.strategy.params.${key}`, 'Enter a time from 00:00 to 23:59.')
  }
  for (const key of ['charge_power_mw', 'discharge_power_mw'] as const) validation.number(params[key], `${field}.strategy.params.${key}`, 0, Number.MAX_VALUE)
  const chargeStart = minute(params.charge_start ?? '10:00')
  const chargeEnd = minute(params.charge_end ?? params.discharge_start ?? '17:00')
  const dischargeStart = minute(params.discharge_start ?? '17:00')
  const dischargeEnd = minute(params.discharge_end ?? '23:59')
  if (params.charge_power_mw === 0 || params.discharge_power_mw === 0) return
  if (chargeStart !== chargeEnd && dischargeStart !== dischargeEnd && (contains(chargeStart, chargeEnd, dischargeStart) || contains(dischargeStart, dischargeEnd, chargeStart))) validation.issue(`${field}.strategy.params.charge_end`, 'Active charge and discharge windows must not overlap.')
}

export function strategyDefaults(name: StrategyName): StrategyConfig {
  return name === 'oracle' ? { name, params: { soc_steps: 200, power_steps: 10 } } : { name }
}
