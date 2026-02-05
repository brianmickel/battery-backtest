import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { apiService, BacktestRequest, CompareBacktestRequest, BatteryInfo, StrategyInfo } from '../services/api';
import { useApiKey } from '../hooks/useApiKey';

interface BacktestFormProps {
  onSubmit: (request: BacktestRequest | CompareBacktestRequest) => void;
  isLoading?: boolean;
}

export function BacktestForm({ onSubmit, isLoading }: BacktestFormProps) {
  const { apiKey, getApiKey } = useApiKey();
  
  // Input mode: 'form' or 'json'
  const [inputMode, setInputMode] = useState<'form' | 'json'>('form');
  const [jsonText, setJsonText] = useState('');
  const [jsonError, setJsonError] = useState<string | null>(null);
  const [jsonSuccess, setJsonSuccess] = useState(false);
  
  // Data source state
  const [datasetId, setDatasetId] = useState('caiso_lmp_real_time_5_min');
  const [locationId, setLocationId] = useState('TH_NP15_GEN-APND');
  const [startDate, setStartDate] = useState('2026-01-01');
  const [endDate, setEndDate] = useState('2026-01-07');
  const [timezone, setTimezone] = useState('market');
  const [dateError, setDateError] = useState<string | null>(null);

  // Battery state
  const [useBatteryPreset, setUseBatteryPreset] = useState(true);
  const [batteryFile, setBatteryFile] = useState('1_moss_landing');
  const [customBattery, setCustomBattery] = useState({
    energy_capacity_mwh: 3000,
    power_capacity_mw: 750,
    charge_efficiency: 0.94,
    discharge_efficiency: 0.94,
    min_soc: 0.10,
    max_soc: 0.90,
    initial_soc: 0.10,
    degradation_cost_per_mwh: 1.5,
  });

  // Strategy state
  const [strategyName, setStrategyName] = useState('schedule');
  const [strategyParams, setStrategyParams] = useState<Record<string, any>>({
    charge_start: '10:00',
    discharge_start: '17:00',
  });

  // Options state
  const [limitIntervals, setLimitIntervals] = useState(0);
  const [includeLedger, setIncludeLedger] = useState(true); // Default to true for charts

  // Fetch batteries and strategies
  const { data: batteries = [], error: batteriesError, isLoading: batteriesLoading } = useQuery<BatteryInfo[], Error>({
    queryKey: ['batteries'],
    queryFn: async () => {
      console.log('Fetching batteries...');
      try {
        const result = await apiService.getBatteries();
        console.log('Batteries fetched successfully:', result);
        return result;
      } catch (error) {
        console.error('Failed to fetch batteries:', error);
        throw error;
      }
    },
    retry: 2,
  });

  const { data: strategies = [], error: strategiesError, isLoading: strategiesLoading } = useQuery<StrategyInfo[], Error>({
    queryKey: ['strategies'],
    queryFn: async () => {
      console.log('Fetching strategies...');
      try {
        const result = await apiService.getStrategies();
        console.log('Strategies fetched successfully:', result);
        return result;
      } catch (error) {
        console.error('Failed to fetch strategies:', error);
        throw error;
      }
    },
    retry: 2,
  });

  const { data: locations = [] } = useQuery({
    queryKey: ['locations', datasetId],
    queryFn: () => apiService.getLocations(datasetId),
    enabled: !!datasetId,
  });

  // Get current strategy info
  const currentStrategy = strategies.find((s: StrategyInfo) => s.name === strategyName);

  // Update strategy params when strategy changes
  useEffect(() => {
    if (currentStrategy) {
      const defaults: Record<string, any> = {};
      currentStrategy.parameters.forEach((param: { name: string; default?: any }) => {
        if (param.default !== undefined) {
          defaults[param.name] = param.default;
        }
      });
      setStrategyParams(defaults);
    }
  }, [currentStrategy]);

  // Validate date range: end not in future, and (end - start) <= 2 months
  const validateDateRange = (start: string, end: string): string | null => {
    const startParsed = new Date(start);
    const endParsed = new Date(end);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    endParsed.setHours(0, 0, 0, 0);
    if (isNaN(endParsed.getTime())) return 'Invalid end date';
    if (endParsed > today) return 'End date cannot be in the future';
    if (isNaN(startParsed.getTime())) return 'Invalid start date';
    if (startParsed > endParsed) return 'Start date must be on or before end date';
    const daysDiff = Math.round((endParsed.getTime() - startParsed.getTime()) / (1000 * 60 * 60 * 24));
    const maxDays = 60; // 2 months
    if (daysDiff > maxDays) return `Time range must be 2 months or less (currently ${daysDiff} days)`;
    return null;
  };

  // Build request object from form state (keyOverride: use when submitting so we get latest key)
  const buildRequest = (keyOverride?: string): BacktestRequest => {
    const key = keyOverride ?? apiKey;
    return {
      api_key: key,
      data_source: {
        type: 'gridstatus',
        dataset_id: datasetId,
        location_id: locationId,
        start_date: startDate,
        end_date: endDate,
        timezone: timezone || 'market',
      },
      config: {
        ...(useBatteryPreset && batteryFile ? { battery_file: batteryFile } : {}),
        ...(!useBatteryPreset ? {
          battery: {
            energy_capacity_mwh: customBattery.energy_capacity_mwh,
            power_capacity_mw: customBattery.power_capacity_mw,
            charge_efficiency: customBattery.charge_efficiency,
            discharge_efficiency: customBattery.discharge_efficiency,
            min_soc: customBattery.min_soc,
            max_soc: customBattery.max_soc,
            initial_soc: customBattery.initial_soc,
            degradation_cost_per_mwh: customBattery.degradation_cost_per_mwh,
          },
        } : {}),
        strategy: {
          name: strategyName,
          params: strategyParams,
        },
      },
      options: {
        ...(limitIntervals > 0 ? { limit_intervals: limitIntervals } : {}),
        include_ledger: includeLedger,
      },
    };
  };

  // Track if we're updating from JSON to prevent loops
  const [isUpdatingFromJson, setIsUpdatingFromJson] = useState(false);

  // Build request with placeholder API key for JSON preview
  const buildRequestForPreview = (): BacktestRequest => {
    const request = buildRequest();
    // Replace actual API key with placeholder for display
    return {
      ...request,
      api_key: '{{API_KEY}}',
    };
  };

  // Update JSON text when form changes (only if in form mode and not updating from JSON)
  useEffect(() => {
    if (inputMode === 'form' && !isUpdatingFromJson) {
      try {
        const request = buildRequestForPreview();
        const newJson = JSON.stringify(request, null, 2);
        setJsonText(newJson);
        setJsonError(null);
      } catch (err) {
        setJsonError('Error generating JSON');
      }
    }
  }, [
    apiKey, datasetId, locationId, startDate, endDate, timezone,
    useBatteryPreset, batteryFile, customBattery,
    strategyName, strategyParams,
    limitIntervals, includeLedger,
    inputMode, isUpdatingFromJson
  ]);

  // Check if JSON is a compare request
  const isCompareRequest = (obj: any): obj is CompareBacktestRequest => {
    return obj && obj.base_config && Array.isArray(obj.variations);
  };

  // Replace API key placeholder in JSON string (keyOverride: use getApiKey() when submitting)
  const replaceApiKeyPlaceholder = (jsonStr: string, keyOverride?: string): string => {
    const key = keyOverride ?? apiKey;
    if (!key) {
      return jsonStr; // Keep placeholder if no API key
    }
    return jsonStr.replace(/"\{\{API_KEY\}\}"/g, JSON.stringify(key));
  };

  // Parse JSON and update form state
  const parseJsonToForm = (json: string) => {
    try {
      // First, ensure API key is set to placeholder in the JSON text
      let normalizedJson = json;
      // If the JSON has an actual API key, replace it with placeholder for display
      if (apiKey && json.includes(apiKey)) {
        normalizedJson = json.replace(new RegExp(JSON.stringify(apiKey), 'g'), '"{{API_KEY}}"');
        setJsonText(normalizedJson);
      }
      
      const parsed = JSON.parse(normalizedJson);
      
      // If it's a compare request, don't try to parse into form
      if (isCompareRequest(parsed)) {
        setJsonError(null);
        setJsonSuccess(true);
        setIsUpdatingFromJson(false);
        setTimeout(() => setJsonSuccess(false), 2000);
        return true;
      }
      
      const request: BacktestRequest = parsed;
      setJsonError(null);
      setJsonSuccess(true);
      setIsUpdatingFromJson(true);
      
      // Clear success message after 2 seconds
      setTimeout(() => setJsonSuccess(false), 2000);

      // Update data source
      if (request.data_source) {
        setDatasetId(request.data_source.dataset_id || '');
        setLocationId(request.data_source.location_id || '');
        setStartDate(request.data_source.start_date || '');
        setEndDate(request.data_source.end_date || '');
        setTimezone(request.data_source.timezone || 'market');
      }

      // Update battery config
      if (request.config) {
        if (request.config.battery_file) {
          setUseBatteryPreset(true);
          setBatteryFile(request.config.battery_file);
        } else if (request.config.battery) {
          setUseBatteryPreset(false);
          setCustomBattery({
            energy_capacity_mwh: request.config.battery.energy_capacity_mwh || 0,
            power_capacity_mw: request.config.battery.power_capacity_mw || 0,
            charge_efficiency: request.config.battery.charge_efficiency || 0,
            discharge_efficiency: request.config.battery.discharge_efficiency || 0,
            min_soc: request.config.battery.min_soc || 0,
            max_soc: request.config.battery.max_soc || 0,
            initial_soc: request.config.battery.initial_soc ?? 0,
            degradation_cost_per_mwh: request.config.battery.degradation_cost_per_mwh ?? 0,
          });
        }

        // Update strategy
        if (request.config.strategy) {
          setStrategyName(request.config.strategy.name || 'schedule');
          setStrategyParams(request.config.strategy.params || {});
        }
      }

      // Update options
      if (request.options) {
        setLimitIntervals(request.options.limit_intervals || 0);
        setIncludeLedger(request.options.include_ledger ?? true);
      }

      // Reset flag after a short delay to allow form updates to complete
      setTimeout(() => setIsUpdatingFromJson(false), 100);

      return true;
    } catch (err: any) {
      setJsonError(err.message || 'Invalid JSON');
      setJsonSuccess(false);
      setIsUpdatingFromJson(false);
      return false;
    }
  };

  // Handle JSON text changes with debouncing
  const [jsonTimeout, setJsonTimeout] = useState<ReturnType<typeof setTimeout> | null>(null);
  const handleJsonChange = (text: string) => {
    setJsonText(text);
    
    if (inputMode === 'json') {
      // Clear previous timeout
      if (jsonTimeout) {
        clearTimeout(jsonTimeout);
      }

      // Debounce parsing - wait 500ms after user stops typing
      const timeout = setTimeout(() => {
        parseJsonToForm(text);
      }, 500);
      setJsonTimeout(timeout);
    }
  };

  // Cleanup timeout on unmount
  useEffect(() => {
    return () => {
      if (jsonTimeout) {
        clearTimeout(jsonTimeout);
      }
    };
  }, [jsonTimeout]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const currentKey = getApiKey();

    let request: BacktestRequest | CompareBacktestRequest;

    if (inputMode === 'json') {
      // Parse JSON and validate
      try {
        const jsonWithApiKey = replaceApiKeyPlaceholder(jsonText, currentKey);
        const parsed = JSON.parse(jsonWithApiKey);

        const hasApiKeyInJson = parsed.api_key &&
                                parsed.api_key !== '{{API_KEY}}' &&
                                parsed.api_key !== '' &&
                                parsed.api_key !== null;

        if (!hasApiKeyInJson) {
          if (!currentKey) {
            setJsonError('Missing api_key. Please enter your API key in the API key input field at the top of the page, or provide it in the JSON.');
            return;
          }
          parsed.api_key = currentKey;
        }

        if (!parsed.api_key || parsed.api_key === '{{API_KEY}}' || parsed.api_key === '') {
          setJsonError('Missing api_key. Please enter your API key in the API key input field at the top of the page, or provide it in the JSON.');
          return;
        }

        if (isCompareRequest(parsed)) {
          if (!parsed.data_source || !parsed.base_config || !parsed.variations) {
            setJsonError('Missing required fields for compare request: data_source, base_config, and variations are required');
            return;
          }
          request = parsed as CompareBacktestRequest;
        } else {
          if (!parsed.data_source || !parsed.config) {
            setJsonError('Missing required fields: data_source and config are required');
            return;
          }
          request = parsed as BacktestRequest;
        }
        setJsonError(null);
      } catch (err: any) {
        setJsonError(err.message || 'Invalid JSON');
        return;
      }
    } else {
      if (!currentKey) {
        alert('Please enter your API key in the API key input field at the top of the page');
        return;
      }
      request = buildRequest(currentKey);
    }

    // Validate date range: end not in future, range <= 2 months
    const ds = request.data_source;
    if (ds?.start_date && ds?.end_date) {
      const err = validateDateRange(ds.start_date, ds.end_date);
      if (err) {
        if (inputMode === 'json') setJsonError(err);
        else setDateError(err);
        return;
      }
    }
    setDateError(null);
    setJsonError(null);
    onSubmit(request);
  };

  return (
    <form onSubmit={handleSubmit}>
      {/* Input Mode Toggle */}
      <div className="card" style={{ backgroundColor: '#1a1a2a' }}>
        <h2 style={{ marginBottom: '0.75em' }}>Input Method</h2>
        <div style={{ display: 'flex', gap: '1em', marginBottom: '0.5em' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.5em', cursor: 'pointer' }}>
            <input
              type="radio"
              checked={inputMode === 'form'}
              onChange={() => {
                setInputMode('form');
                // When switching to form mode, try to parse JSON and update form if valid
                if (jsonText) {
                  parseJsonToForm(jsonText);
                }
              }}
            />
            Form Input
          </label>
          <label style={{ display: 'flex', alignItems: 'center', gap: '0.5em', cursor: 'pointer' }}>
            <input
              type="radio"
              checked={inputMode === 'json'}
              onChange={() => {
                setInputMode('json');
                setIsUpdatingFromJson(false);
                // When switching to JSON mode, update JSON from current form state with placeholder
                const request = buildRequestForPreview();
                setJsonText(JSON.stringify(request, null, 2));
                setJsonError(null);
              }}
            />
            JSON Input
          </label>
        </div>
      </div>

      {inputMode === 'json' ? (
        <div className="card">
          <h2>Request JSON</h2>
          {jsonError && (
            <div className="error" style={{ marginBottom: '0.75em' }}>
              <strong>JSON Error:</strong> {jsonError}
            </div>
          )}
          {jsonSuccess && !jsonError && (
            <div className="success" style={{ marginBottom: '0.75em' }}>
              ✓ JSON parsed successfully! Form inputs have been updated.
            </div>
          )}
          <div className="form-group">
            <label>Paste or edit the request JSON:</label>
            <textarea
              value={jsonText}
              onChange={(e) => handleJsonChange(e.target.value)}
              style={{
                minHeight: '300px',
                fontFamily: 'monospace',
                fontSize: '0.85em',
                whiteSpace: 'pre',
                overflowWrap: 'normal',
                overflowX: 'auto',
              }}
              placeholder='{"api_key": "...", "data_source": {...}, "config": {...}}'
            />
          </div>
          <div style={{ fontSize: '0.85em', opacity: 0.7, marginTop: '0.5em' }}>
            <p style={{ margin: '0.25em 0' }}>💡 Tip: The JSON will automatically update the form inputs when you paste or edit it (after a short delay).</p>
            <p style={{ margin: '0.25em 0' }}>💡 Tip: Switch to "Form Input" mode to see the generated JSON from your form.</p>
            <p style={{ margin: '0.25em 0' }}>💡 Tip: Make sure to include your API key in the JSON: <code>{`{"api_key": "your-key-here"}`}</code></p>
          </div>
        </div>
      ) : (
        <>
          {/* Data Source Section */}
      <div className="card">
        <h2>Data Source</h2>
        <div className="form-row">
          <div className="form-group">
            <label>Dataset ID</label>
            <input
              type="text"
              value={datasetId}
              onChange={(e) => setDatasetId(e.target.value)}
              required
            />
          </div>
          <div className="form-group">
            <label>Location ID</label>
            <input
              type="text"
              value={locationId}
              onChange={(e) => setLocationId(e.target.value)}
              list="locations-list"
              required
            />
            <datalist id="locations-list">
              {locations.map(loc => (
                <option key={loc.id} value={loc.id}>{loc.name}</option>
              ))}
            </datalist>
          </div>
        </div>
        <div className="form-row">
          <div className="form-group">
            <label>Start Date</label>
            <input
              type="date"
              value={startDate}
              onChange={(e) => { setStartDate(e.target.value); setDateError(null); }}
              required
            />
          </div>
          <div className="form-group">
            <label>End Date</label>
            <input
              type="date"
              value={endDate}
              onChange={(e) => { setEndDate(e.target.value); setDateError(null); }}
              required
            />
          </div>
          {dateError && (
            <div className="form-group" style={{ flex: '1 1 100%', color: '#ffaaaa', fontSize: '0.9em' }}>
              {dateError}
            </div>
          )}
          <div className="form-group">
            <label>Timezone</label>
            <select value={timezone} onChange={(e) => setTimezone(e.target.value)}>
              <option value="market">Market</option>
              <option value="UTC">UTC</option>
            </select>
          </div>
        </div>
      </div>

      {/* Battery Configuration */}
      <div className="card">
        <h2>Battery Configuration</h2>
        <div className="form-group">
          <label>
            <input
              type="radio"
              checked={useBatteryPreset}
              onChange={() => setUseBatteryPreset(true)}
            />
            Use Battery Preset
          </label>
          <label>
            <input
              type="radio"
              checked={!useBatteryPreset}
              onChange={() => setUseBatteryPreset(false)}
            />
            Custom Battery
          </label>
        </div>

        {useBatteryPreset ? (
          <div className="form-group">
            <label>Battery Preset</label>
            {batteriesError && (
              <div style={{ color: '#ffaaaa', fontSize: '0.85em', marginBottom: '0.5em' }}>
                Error loading batteries: {batteriesError instanceof Error ? batteriesError.message : 'Unknown error'}
              </div>
            )}
            {batteriesLoading && (
              <div style={{ color: '#aaaaff', fontSize: '0.85em', marginBottom: '0.5em' }}>
                Loading batteries...
              </div>
            )}
            {!batteriesLoading && !batteriesError && batteries.length === 0 && (
              <div style={{ color: '#ffaa00', fontSize: '0.85em', marginBottom: '0.5em' }}>
                ⚠️ No batteries found. Check server logs.
              </div>
            )}
            <select value={batteryFile} onChange={(e) => setBatteryFile(e.target.value)} disabled={batteriesLoading || batteries.length === 0}>
              {batteries.length === 0 ? (
                <option value="">No batteries available</option>
              ) : (
                batteries.map((batt: BatteryInfo) => (
                  <option key={batt.id} value={batt.id}>
                    {batt.name} ({batt.specs.energy_capacity_mwh} MWh, {batt.specs.power_capacity_mw} MW)
                  </option>
                ))
              )}
            </select>
          </div>
        ) : (
          <div className="form-row">
            <div className="form-group">
              <label>Energy Capacity (MWh)</label>
              <input
                type="number"
                step="0.1"
                value={customBattery.energy_capacity_mwh}
                onChange={(e) => setCustomBattery({ ...customBattery, energy_capacity_mwh: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Power Capacity (MW)</label>
              <input
                type="number"
                step="0.1"
                value={customBattery.power_capacity_mw}
                onChange={(e) => setCustomBattery({ ...customBattery, power_capacity_mw: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Charge Efficiency</label>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={customBattery.charge_efficiency}
                onChange={(e) => setCustomBattery({ ...customBattery, charge_efficiency: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Discharge Efficiency</label>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={customBattery.discharge_efficiency}
                onChange={(e) => setCustomBattery({ ...customBattery, discharge_efficiency: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Min SOC</label>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={customBattery.min_soc}
                onChange={(e) => setCustomBattery({ ...customBattery, min_soc: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Max SOC</label>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={customBattery.max_soc}
                onChange={(e) => setCustomBattery({ ...customBattery, max_soc: parseFloat(e.target.value) })}
                required
              />
            </div>
            <div className="form-group">
              <label>Initial SOC (optional)</label>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                value={customBattery.initial_soc}
                onChange={(e) => setCustomBattery({ ...customBattery, initial_soc: parseFloat(e.target.value) })}
              />
            </div>
            <div className="form-group">
              <label>Degradation Cost per MWh</label>
              <input
                type="number"
                step="0.1"
                min="0"
                value={customBattery.degradation_cost_per_mwh}
                onChange={(e) => setCustomBattery({ ...customBattery, degradation_cost_per_mwh: parseFloat(e.target.value) })}
              />
            </div>
          </div>
        )}
      </div>

      {/* Strategy Configuration */}
      <div className="card">
        <h2>Strategy Configuration</h2>
        <div className="form-group">
          <label>Strategy</label>
          {strategiesError && (
            <div style={{ color: '#ffaaaa', fontSize: '0.85em', marginBottom: '0.5em' }}>
              Error loading strategies: {strategiesError instanceof Error ? strategiesError.message : 'Unknown error'}
            </div>
          )}
          {strategiesLoading && (
            <div style={{ color: '#aaaaff', fontSize: '0.85em', marginBottom: '0.5em' }}>
              Loading strategies...
            </div>
          )}
          {!strategiesLoading && !strategiesError && strategies.length === 0 && (
            <div style={{ color: '#ffaa00', fontSize: '0.85em', marginBottom: '0.5em' }}>
              ⚠️ No strategies found. Check server logs.
            </div>
          )}
          <select value={strategyName} onChange={(e) => setStrategyName(e.target.value)} disabled={strategiesLoading || strategies.length === 0}>
            {strategies.length === 0 ? (
              <option value="">No strategies available</option>
            ) : (
              strategies.map((strat: StrategyInfo) => (
                <option key={strat.name} value={strat.name}>
                  {strat.name} - {strat.description}
                </option>
              ))
            )}
          </select>
        </div>

        {currentStrategy && (
          <div>
            <p style={{ opacity: 0.8, marginBottom: '0.75em', fontSize: '0.9em' }}>{currentStrategy.description}</p>
            {currentStrategy.parameters.map((param: { name: string; type: string; description: string; default?: any }) => (
              <div key={param.name} className="form-group">
                <label>
                  {param.name}
                  {param.default !== undefined && (
                    <span style={{ opacity: 0.6, fontSize: '0.9em' }}> (default: {String(param.default)})</span>
                  )}
                </label>
                {param.type === 'int' ? (
                  <input
                    type="number"
                    value={strategyParams[param.name] ?? param.default ?? ''}
                    onChange={(e) => setStrategyParams({ ...strategyParams, [param.name]: parseInt(e.target.value) })}
                  />
                ) : param.type === 'float' ? (
                  <input
                    type="number"
                    step="0.1"
                    value={strategyParams[param.name] ?? param.default ?? ''}
                    onChange={(e) => setStrategyParams({ ...strategyParams, [param.name]: parseFloat(e.target.value) })}
                  />
                ) : (
                  <input
                    type="text"
                    value={strategyParams[param.name] ?? param.default ?? ''}
                    onChange={(e) => setStrategyParams({ ...strategyParams, [param.name]: e.target.value })}
                  />
                )}
                <small style={{ opacity: 0.7, display: 'block', marginTop: '0.25em' }}>
                  {param.description}
                </small>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Options */}
      <div className="card">
        <h2>Options</h2>
        <div className="form-row">
          <div className="form-group">
            <label>Limit Intervals (0 = all)</label>
            <input
              type="number"
              min="0"
              value={limitIntervals}
              onChange={(e) => setLimitIntervals(parseInt(e.target.value) || 0)}
            />
          </div>
          <div className="form-group">
            <label>
              <input
                type="checkbox"
                checked={includeLedger}
                onChange={(e) => setIncludeLedger(e.target.checked)}
              />
              Include Ledger (detailed interval data)
            </label>
          </div>
        </div>
      </div>

          {/* JSON Preview (read-only when in form mode) */}
          <div className="card" style={{ backgroundColor: '#0a0a0a' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '0.4em' }}>
              <h2 style={{ margin: 0, fontSize: '1.1em' }}>Request JSON Preview</h2>
              <button
                type="button"
                onClick={() => {
                  navigator.clipboard.writeText(jsonText);
                  alert('JSON copied to clipboard!');
                }}
                style={{ padding: '0.4em 0.75em', fontSize: '0.85em' }}
              >
                Copy JSON
              </button>
            </div>
            <div style={{ fontSize: '0.8em', opacity: 0.7, marginBottom: '0.4em' }}>
              This JSON is automatically generated from the form above. Switch to "JSON Input" mode to edit directly.
            </div>
            <pre style={{
              backgroundColor: '#0a0a0a',
              padding: '0.75em',
              borderRadius: '4px',
              overflow: 'auto',
              maxHeight: '250px',
              fontSize: '0.8em',
              fontFamily: 'monospace',
              margin: 0,
              border: '1px solid #333',
            }}>
              {jsonText || 'Generating...'}
            </pre>
          </div>
        </>
      )}

      <button type="submit" disabled={(!apiKey && inputMode === 'form') || isLoading} style={{ width: '100%', padding: '0.75em', fontSize: '1em', marginTop: '0.5em' }}>
        {isLoading ? 'Running Backtest...' : 'Run Backtest'}
      </button>
    </form>
  );
}
