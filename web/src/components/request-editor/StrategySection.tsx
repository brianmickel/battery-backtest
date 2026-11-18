import type { DraftAction, FormField, StrategyConfig, StrategyParams } from '../../domain/types';
import type { StrategyInfo } from '../../services/api';
import { oracleFields, scheduleFields } from '../../domain/configuration';
import type { FieldControls } from './fields';
import { DraftField } from './DraftField';

const labels: Record<keyof StrategyParams, string> = {
  charge_start: 'Charge start (HH:MM)', charge_end: 'Charge end (HH:MM)',
  discharge_start: 'Discharge start (HH:MM)', discharge_end: 'Discharge end (HH:MM)',
  charge_power_mw: 'Charge power (MW)', discharge_power_mw: 'Discharge power (MW)',
  soc_steps: 'SOC steps', power_steps: 'Power steps',
};

interface Props extends FieldControls {
  strategy: StrategyConfig | undefined;
  strategies: StrategyInfo[];
  onAction: (action: DraftAction) => void;
}

export function StrategySection({ strategy, strategies, onAction, ...fields }: Props) {
  const metadata = strategies.find(item => item.name === strategy?.name);
  const parameters = strategy?.name === 'oracle' ? oracleFields : scheduleFields;
  return (
    <fieldset>
      <legend>Strategy</legend>
      <label htmlFor="strategy-name">Strategy</label>
      <select id="strategy-name" value={strategy?.name} onChange={event => {
        const name = event.target.value;
        if (name === 'schedule' || name === 'oracle') onAction({ type: 'select-strategy', name });
      }}>
        <option value="schedule">Schedule</option><option value="oracle">Oracle (approximate foresight)</option>
      </select>
      <p>Leave optional parameters blank to use their defaults. Zero power disables that direction.</p>
      <div className="form-row">
        {parameters.map(key => {
          const parameter = metadata?.parameters.find(item => item.name === key);
          const placeholder = parameter?.default_source === 'battery_power_capacity_mw' ? 'Battery power capacity'
            : parameter?.default_source === 'discharge_start' ? 'Discharge start'
            : parameter?.default === undefined ? 'Default' : String(parameter.default);
          const field: FormField = `config.strategy.params.${key}`;
          return <DraftField key={key} {...fields} field={field} label={labels[key]} value={strategy?.params?.[key]}
            numeric={!key.endsWith('_start') && !key.endsWith('_end')} placeholder={placeholder} />;
        })}
      </div>
    </fieldset>
  );
}
