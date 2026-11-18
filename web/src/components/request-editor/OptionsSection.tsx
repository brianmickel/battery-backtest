import type { BacktestOptions } from '../../domain/types';
import { fieldValue, type FieldControls } from './fields';
import { DraftField } from './DraftField';

interface Props extends FieldControls { options: BacktestOptions | undefined }

export function OptionsSection({ options, ...fields }: Props) {
  return (
    <fieldset>
      <legend>Output options</legend>
      <DraftField {...fields} field="options.limit_intervals" label="Interval limit (optional)" value={options?.limit_intervals}
        numeric placeholder="All intervals" />
      <label className="checkbox-label" htmlFor="include-ledger">
        <input id="include-ledger" type="checkbox" checked={fieldValue(fields.edits, 'options.include_ledger', options?.include_ledger) === 'true'}
          onChange={event => fields.onEdit('options.include_ledger', String(event.target.checked))} onBlur={fields.onBlur} />
        Include ledger for charts
      </label>
    </fieldset>
  );
}
