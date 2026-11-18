import type { BacktestConfig, BatteryConfig, DraftAction, FormField } from '../../domain/types';
import type { BatteryInfo } from '../../services/api';
import { batteryFields } from '../../domain/configuration';
import type { FieldControls } from './fields';
import { DraftField } from './DraftField';

const customBattery: BatteryConfig = {
  energy_capacity_mwh: 3000, power_capacity_mw: 750, charge_efficiency: .94, discharge_efficiency: .94,
  min_soc: .1, max_soc: .9, initial_soc: .1, degradation_cost_per_mwh: 1.5,
};
const labels: Record<keyof BatteryConfig, string> = {
  name: 'Battery name', energy_capacity_mwh: 'Energy capacity (MWh)', power_capacity_mw: 'Power capacity (MW)',
  charge_efficiency: 'Charge efficiency (0–1)', discharge_efficiency: 'Discharge efficiency (0–1)',
  min_soc: 'Minimum SOC (0–1)', max_soc: 'Maximum SOC (0–1)', initial_soc: 'Initial SOC (0–1)', degradation_cost_per_mwh: 'Degradation cost ($/MWh)',
};

interface Props extends FieldControls {
  config: BacktestConfig;
  batteries: BatteryInfo[];
  onAction: (action: DraftAction) => void;
}

export function BatterySection({ config, batteries, onAction, ...fields }: Props) {
  const preset = config.battery_file !== undefined;
  return (
    <fieldset>
      <legend>Battery</legend>
      <div className="radio-group">
        <label><input type="radio" name="battery-source" checked={preset}
          onChange={() => onAction({ type: 'select-preset', id: batteries[0]?.id ?? '1_moss_landing' })} />Preset</label>
        <label><input type="radio" name="battery-source" checked={!preset}
          onChange={() => onAction({ type: 'use-custom-battery', battery: customBattery })} />Custom battery</label>
      </div>
      <p>Choosing a preset or switching to custom replaces the battery settings.</p>
      {preset && <>
        <label htmlFor="battery-preset">Battery preset</label>
        <select id="battery-preset" value={config.battery_file} onChange={event => onAction({ type: 'select-preset', id: event.target.value })}>
          {!batteries.some(battery => battery.id === config.battery_file) && <option value={config.battery_file}>{config.battery_file}</option>}
          {batteries.map(battery => <option key={battery.id} value={battery.id}>{battery.name} ({battery.specs.energy_capacity_mwh} MWh)</option>)}
        </select>
      </>}
      <fieldset>
        <legend>{preset ? 'Battery overrides (optional)' : 'Custom battery settings'}</legend>
        {preset && <p>Leave a field blank to inherit the preset. Zero is an explicit override. Omitted preset limits are checked when you run the request.</p>}
        <div className="form-row">
          {batteryFields.map(key => {
            const field: FormField = `config.battery.${key}`;
            const required = !preset && key !== 'name' && key !== 'initial_soc' && key !== 'degradation_cost_per_mwh';
            return <DraftField key={key} {...fields} field={field} label={labels[key]} value={config.battery?.[key]}
              numeric={key !== 'name'} required={required} placeholder={preset ? 'Inherit preset' : undefined} />;
          })}
        </div>
      </fieldset>
    </fieldset>
  );
}
