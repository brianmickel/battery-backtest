import axios from 'axios';
import { isObject } from '../domain/validation';
import { isBacktestResponse, isBatteryInfo, isCompareResponse, isDatasetInfo, isLocationInfo, isRanking, isStrategyInfo } from './responses';

const api = axios.create({ baseURL: import.meta.env.VITE_API_URL || '' });

export type { DataSourceConfig, BatteryConfig, StrategyConfig, StrategyParamValue, BacktestConfig, BacktestOptions, BacktestRequest, CompareBacktestRequest } from '../domain/types';
import type { BacktestRequest, CompareBacktestRequest, StrategyParamValue } from '../domain/types';

export interface TimeWindow {
  start: string;
  end: string;
}

export interface ChargeWindow extends TimeWindow {
  average_cost_per_mwh: number;
  energy_mwh: number;
}

export interface DischargeWindow extends TimeWindow {
  average_price_per_mwh: number;
  energy_mwh: number;
}

export interface BacktestSummary {
  total_pnl: number;
  initial_soc: number;
  initial_energy_mwh: number;
  final_energy_mwh: number;
  timezone: "market" | "UTC";
  covered_duration_hours: number;
  gap_duration_hours: number;
  gap_count: number;
  final_soc: number;
  total_intervals: number;
  backtest_window: TimeWindow;
  energy_charged_mwh: number;
  energy_discharged_mwh: number;
  charge_windows?: ChargeWindow[];
  discharge_windows?: DischargeWindow[];
}

export interface LedgerRow {
  index: number;
  interval_start_local: string;
  interval_end_local: string;
  interval_start_utc: string;
  interval_end_utc: string;
  location: string;
  market: string;
  lmp: number;
  action: 'CHARGING' | 'DISCHARGING' | 'IDLE';
  requested_power_mw: number;
  power_mw: number;
  energy_from_grid_mwh: number;
  energy_to_grid_mwh: number;
  throughput_mwh: number;
  soc_start: number;
  soc_end: number;
  pnl: number;
  cum_pnl: number;
}

export interface BacktestResponse {
  status: 'completed';
  summary: BacktestSummary;
  ledger?: LedgerRow[];
}

export interface BatteryInfo {
  id: string;
  name: string;
  specs: {
    energy_capacity_mwh: number;
    power_capacity_mw: number;
  };
}

export interface StrategyInfo {
  name: string;
  description: string;
  parameters: Array<{
    name: string;
    type: string;
    description: string;
    default?: StrategyParamValue;
    default_source?: string;
  }>;
}

export interface DatasetInfo {
  id: string;
  name: string;
  market: string;
  resolution: string;
}

export interface LocationInfo {
  id: string;
  name: string;
  type: string;
}

export interface Ranking {
  rank: number;
  location: string;
  market: string;
  count: number;
  spread_p95_p05: number;
  min_lmp: number;
  max_lmp: number;
  oracle_profit: number;
}

export interface ComparisonResult {
  name: string;
  summary: BacktestSummary;
}

export interface CompareBacktestResponse {
  comparison: ComparisonResult[];
}


export type ErrorCode = 'INVALID_REQUEST' | 'BODY_TOO_LARGE' | 'WORK_BUDGET_EXCEEDED' | 'UPSTREAM_AUTH' | 'UPSTREAM_RATE_LIMIT' | 'UPSTREAM_UNAVAILABLE' | 'SERVER_BUSY' | 'DEADLINE_EXCEEDED' | 'INTERNAL_ERROR' | 'NOT_FOUND';
const errorCodes: readonly string[] = ['INVALID_REQUEST', 'BODY_TOO_LARGE', 'WORK_BUDGET_EXCEEDED', 'UPSTREAM_AUTH', 'UPSTREAM_RATE_LIMIT', 'UPSTREAM_UNAVAILABLE', 'SERVER_BUSY', 'DEADLINE_EXCEEDED', 'INTERNAL_ERROR', 'NOT_FOUND'];

export class ApiError extends Error {
  readonly code: string;
  readonly field?: string;
  readonly variation?: string;
  readonly location_id?: string;
  constructor(message: string, details: { code: string; field?: string; variation?: string; location_id?: string }) {
    super(message);
    this.name = 'ApiError';
    this.code = details.code;
    this.field = details.field;
    this.variation = details.variation;
    this.location_id = details.location_id;
  }
}

function safeError(error: unknown): ApiError {
  if (error instanceof ApiError) return error;
  if (axios.isAxiosError<unknown>(error) && isObject(error.response?.data)) {
    const body = error.response.data.error;
    if (isObject(body) && typeof body.code === 'string' && errorCodes.includes(body.code) && typeof body.message === 'string' && body.message.length > 0 && ['field', 'variation', 'location_id'].every(key => body[key] === undefined || typeof body[key] === 'string')) {
      return new ApiError(body.message, { code: body.code, ...(typeof body.field === 'string' ? { field: body.field } : {}), ...(typeof body.variation === 'string' ? { variation: body.variation } : {}), ...(typeof body.location_id === 'string' ? { location_id: body.location_id } : {}) });
    }
  }
  return new ApiError('The request could not be completed. Please try again.', { code: 'INTERNAL_ERROR' });
}

export function getErrorMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

async function checked<T>(operation: () => Promise<{ data: unknown }>, accepts: (value: unknown) => value is T): Promise<T> {
  try {
    const { data } = await operation();
    if (!accepts(data)) throw new ApiError('The server returned an invalid response.', { code: 'INTERNAL_ERROR' });
    return data;
  } catch (error) { throw safeError(error); }
}

function list<T>(key: string, accepts: (value: unknown) => value is T, params?: Record<string, string>): Promise<T[]> {
  return checked(async () => {
    const { data } = await api.get<unknown>(`/api/v1/${key}`, { params });
    return { data: isObject(data) ? data[key] : undefined };
  }, (value): value is T[] => Array.isArray(value) && value.every(accepts));
}

function credential(apiKey: string): { 'X-GridStatus-API-Key': string } {
  if (!apiKey.trim()) throw new ApiError('Enter your GridStatus API key.', { code: 'INVALID_REQUEST', field: 'api_key' });
  return { 'X-GridStatus-API-Key': apiKey };
}

export interface RankRequest { dataset_id: string; start_date: string; end_date: string; location_ids: string; limit?: number }

export const apiService = {
  runBacktest(request: BacktestRequest, apiKey: string, signal?: AbortSignal): Promise<BacktestResponse> {
    return checked(() => api.post<unknown>('/api/v1/backtest', request, { headers: credential(apiKey), signal }), isBacktestResponse);
  },
  compareBacktests(request: CompareBacktestRequest, apiKey: string, signal?: AbortSignal): Promise<CompareBacktestResponse> {
    return checked(() => api.post<unknown>('/api/v1/backtest/compare', request, { headers: credential(apiKey), signal }), isCompareResponse);
  },
  getBatteries(): Promise<BatteryInfo[]> { return list('batteries', isBatteryInfo); },
  getStrategies(): Promise<StrategyInfo[]> { return list('strategies', isStrategyInfo); },
  getDatasets(): Promise<DatasetInfo[]> { return list('datasets', isDatasetInfo); },
  getLocations(datasetId: string): Promise<LocationInfo[]> { return list('locations', isLocationInfo, { dataset_id: datasetId }); },
  rankNodes(params: RankRequest, apiKey: string, signal?: AbortSignal): Promise<Ranking[]> {
    return checked(() => api.get<unknown>('/api/v1/rank', { params, headers: credential(apiKey), signal }), (value): value is { rankings: Ranking[] } => isObject(value) && Array.isArray(value.rankings) && value.rankings.every(isRanking)).then(value => value.rankings);
  },
};
