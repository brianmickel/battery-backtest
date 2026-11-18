import type { DataSourceConfig } from '../../domain/types';
import type { DatasetInfo, LocationInfo } from '../../services/api';
import { fieldValue, type FieldControls } from './fields';
import { DraftField } from './DraftField';

interface Props extends FieldControls {
  source: DataSourceConfig;
  datasets: DatasetInfo[];
  locations: LocationInfo[];
}

export function DataSourceSection({ source, datasets, locations, ...fields }: Props) {
  return (
    <fieldset>
      <legend>Market data</legend>
      <DraftField {...fields} field="data_source.dataset_id" label="Dataset ID" value={source.dataset_id} list="dataset-options" required />
      <datalist id="dataset-options">{datasets.map(dataset => <option key={dataset.id} value={dataset.id}>{dataset.name}</option>)}</datalist>
      <DraftField {...fields} field="data_source.location_id" label="Location ID" value={source.location_id} list="location-options" required />
      <datalist id="location-options">{locations.map(location => <option key={location.id} value={location.id}>{location.name}</option>)}</datalist>
      <div className="form-row">
        <DraftField {...fields} field="data_source.start_date" label="Start date (YYYY-MM-DD)" value={source.start_date} required />
        <DraftField {...fields} field="data_source.end_date" label="End date (excluded, YYYY-MM-DD)" value={source.end_date} required />
      </div>
      <label htmlFor="request-data_source-timezone">Clock</label>
      <select id="request-data_source-timezone" value={fieldValue(fields.edits, 'data_source.timezone', source.timezone)}
        onChange={event => fields.onEdit('data_source.timezone', event.target.value)} onBlur={fields.onBlur}>
        <option value="">Default (market)</option><option value="market">Market</option><option value="UTC">UTC</option>
      </select>
    </fieldset>
  );
}
