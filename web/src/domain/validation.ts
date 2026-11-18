import type { FieldIssue } from './types'

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

export class Validation {
  readonly issues: FieldIssue[] = []

  issue(field: string, message: string): void { this.issues.push({ field, message }) }

  object(value: unknown, field: string, allowed: readonly string[]): Record<string, unknown> {
    if (!isObject(value)) {
      this.issue(field, 'Expected an object.')
      return {}
    }
    for (const key of Object.keys(value)) {
      if (!allowed.includes(key)) this.issue(field ? `${field}.${key}` : key, 'Unknown field.')
      else if (value[key] === null) this.issue(field ? `${field}.${key}` : key, 'Null is not allowed; omit an optional field instead.')
    }
    return value
  }

  text(value: unknown, field: string, required = false): string | undefined {
    if (value === undefined && !required) return undefined
    if (typeof value !== 'string' || !value.trim()) {
      this.issue(field, 'Enter a nonempty string.')
      return undefined
    }
    return value
  }

  number(value: unknown, field: string, min: number, max: number, integer = false): number | undefined {
    if (value === undefined) return undefined
    if (typeof value !== 'number' || !Number.isFinite(value) || value < min || value > max || (integer && !Number.isInteger(value))) {
      this.issue(field, `Enter ${integer ? 'an integer' : 'a finite number'} from ${min} to ${max}.`)
      return undefined
    }
    return value
  }
}
