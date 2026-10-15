import type { FormField } from '../../domain/types';
import { fieldID, fieldValue, type FieldControls } from './fields';

interface DraftFieldProps extends FieldControls {
  field: FormField;
  label: string;
  value: string | number | undefined;
  numeric?: boolean;
  placeholder?: string;
  list?: string;
  required?: boolean;
}

export function DraftField({ field, label, value, numeric, placeholder, list, required, edits, issues, onEdit, onBlur }: DraftFieldProps) {
  const errors = issues.filter(issue => issue.field === field);
  const id = fieldID(field);
  return (
    <div className="form-group">
      <label htmlFor={id}>{label}{required ? ' *' : ''}</label>
      <input id={id} type="text" inputMode={numeric ? 'decimal' : 'text'}
        value={fieldValue(edits, field, value)} placeholder={placeholder} list={list} required={required}
        aria-invalid={errors.length > 0 || undefined} aria-describedby={errors.length > 0 ? `${id}-error` : undefined}
        onChange={event => onEdit(field, event.target.value)} onBlur={onBlur} />
      {errors.length > 0 && <p id={`${id}-error`} className="field-error">{errors.map(error => error.message).join(' ')}</p>}
    </div>
  );
}
