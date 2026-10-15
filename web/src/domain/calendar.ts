import type { FieldIssue } from './types'

export function calendarDay(value: string): number | undefined {
  if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined
  const year = Number(value.slice(0, 4))
  const month = Number(value.slice(5, 7))
  const day = Number(value.slice(8, 10))
  if (year < 1) return undefined
  const instant = new Date(0)
  instant.setUTCFullYear(year, month - 1, day)
  instant.setUTCHours(0, 0, 0, 0)
  if (instant.getUTCFullYear() !== year || instant.getUTCMonth() !== month - 1 || instant.getUTCDate() !== day) return undefined
  return instant.getTime() / 86_400_000
}

export function validateDateRange(start: string, end: string): FieldIssue[] {
  const first = calendarDay(start)
  const last = calendarDay(end)
  const issues: FieldIssue[] = []
  if (first === undefined) issues.push({ field: 'data_source.start_date', message: 'Enter a valid YYYY-MM-DD date.' })
  if (last === undefined) issues.push({ field: 'data_source.end_date', message: 'Enter a valid YYYY-MM-DD date.' })
  if (first === undefined || last === undefined) return issues
  if (last <= first) issues.push({ field: 'data_source.end_date', message: 'End date must follow start date (the end date is excluded).' })
  if (last - first > 60) issues.push({ field: 'data_source.end_date', message: 'Choose no more than 60 calendar days.' })
  if (last > Math.floor(Date.now() / 86_400_000)) issues.push({ field: 'data_source.end_date', message: 'End date cannot be after today in UTC.' })
  return issues
}
