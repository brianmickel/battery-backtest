import type { FieldIssue, FormEdits, FormField } from '../../domain/types';

export interface FieldControls {
  edits: FormEdits;
  issues: FieldIssue[];
  onEdit: (field: FormField, text: string) => void;
  onBlur: () => void;
}

export function fieldID(field: string): string {
  return field === 'api_key' ? 'api-key' : `request-${field.split('.').join('-')}`;
}

export function fieldValue(edits: FormEdits, field: FormField, value: string | number | boolean | undefined): string {
  return edits[field] ?? (value === undefined ? '' : String(value));
}
