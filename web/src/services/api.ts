import axios from 'axios';

// In development, use relative URLs to leverage Vite proxy
// In production, if VITE_API_URL is set, use it; otherwise use relative URLs
// (since frontend is served from the same server as the API)
const API_BASE_URL = import.meta.env.VITE_API_URL || '';

const api = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
});

// Log API configuration for debugging
console.log('API Base URL:', API_BASE_URL || '(relative - same origin)');
console.log('Environment:', import.meta.env.MODE);
console.log('Production:', import.meta.env.PROD);

export interface DataSourceConfig {
  type: string;
  dataset_id: string;
  location_id: string;
  start_date: string;
  end_date: string;
  timezone?: string;
}

export interface BatteryConfig {
  name?: string;
  energy_capacity_mwh: number;
  power_capacity_mw: number;
  charge_efficiency: number;
  discharge_efficiency: number;
  min_soc: number;
  max_soc: number;
  initial_soc?: number;
  degradation_cost_per_mwh?: number;
}

export interface StrategyConfig {
  name: string;
  params?: Record<string, any>;
}

export interface BacktestConfig {
  battery_file?: string;
  battery?: BatteryConfig;
  strategy: StrategyConfig;
}

export interface BacktestOptions {
  limit_intervals?: number;
  include_ledger?: boolean;
}

export interface BacktestRequest {
  api_key: string;
  data_source: DataSourceConfig;
  config: BacktestConfig;
  options?: BacktestOptions;
}

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
  action: string;
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
  status: string;
  summary: BacktestSummary;
  ledger?: LedgerRow[];
}

export interface BatteryInfo {
  id: string;
  name: string;
  file: string;
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
    default?: any;
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

export interface CompareBacktestRequest {
  api_key: string;
  data_source: DataSourceConfig;
  base_config: BacktestConfig;
  variations: Array<{
    name: string;
    config: BacktestConfig;
  }>;
}

export interface ComparisonResult {
  name: string;
  summary: BacktestSummary;
}

export interface CompareBacktestResponse {
  comparison: ComparisonResult[];
}

// API functions
export const apiService = {
  async runBacktest(request: BacktestRequest): Promise<BacktestResponse> {
    const response = await api.post<BacktestResponse>('/api/v1/backtest', request);
    return response.data;
  },

  async compareBacktests(request: CompareBacktestRequest): Promise<CompareBacktestResponse> {
    const response = await api.post<CompareBacktestResponse>('/api/v1/backtest/compare', request);
    return response.data;
  },

  async getBatteries(): Promise<BatteryInfo[]> {
    try {
      const response = await api.get<{ batteries: BatteryInfo[] }>('/api/v1/batteries');
      console.log('Batteries API response:', response.data);
      return response.data.batteries || [];
    } catch (error: any) {
      console.error('Error fetching batteries:', error);
      console.error('Response:', error.response?.data);
      throw error;
    }
  },

  async getStrategies(): Promise<StrategyInfo[]> {
    try {
      const response = await api.get<{ strategies: StrategyInfo[] }>('/api/v1/strategies');
      console.log('Strategies API response:', response.data);
      return response.data.strategies || [];
    } catch (error: any) {
      console.error('Error fetching strategies:', error);
      console.error('Response:', error.response?.data);
      throw error;
    }
  },

  async getDatasets(): Promise<DatasetInfo[]> {
    const response = await api.get<{ datasets: DatasetInfo[] }>('/api/v1/datasets');
    return response.data.datasets;
  },

  async getLocations(datasetId: string): Promise<LocationInfo[]> {
    const response = await api.get<{ locations: LocationInfo[] }>('/api/v1/locations', {
      params: { dataset_id: datasetId },
    });
    return response.data.locations;
  },

  async rankNodes(params: {
    api_key: string;
    dataset_id: string;
    start_date: string;
    end_date: string;
    location_ids?: string;
    limit?: number;
  }): Promise<Ranking[]> {
    const response = await api.get<{ rankings: Ranking[] }>('/api/v1/rank', { params });
    return response.data.rankings;
  },
};
