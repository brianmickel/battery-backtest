import type { Clock } from '../domain/types';
import type { BacktestSummary } from '../services/api';

const currency = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

export function formatCurrency(value: number): string {
  return currency.format(value);
}

export function formatNumber(value: number, decimals = 2): string {
  return new Intl.NumberFormat('en-US', { maximumFractionDigits: decimals }).format(value);
}

export function clockLabel(clock: Clock): string {
  return clock === 'UTC' ? 'UTC' : 'Market time (offsets shown)';
}

export function civilDate(timestamp: string, clock: Clock): string {
  return clock === 'UTC' ? new Date(timestamp).toISOString().slice(0, 10) : timestamp.slice(0, 10);
}

export function formatTimestamp(timestamp: string, clock: Clock): string {
  if (clock === 'UTC') {
    return `${new Date(timestamp).toISOString().slice(0, 19).replace('T', ' ')} UTC`;
  }
  const offset = timestamp.endsWith('Z') ? '+00:00' : timestamp.slice(-6);
  return `${timestamp.slice(0, 19).replace('T', ' ')} UTC${offset}`;
}

export function timestampOffsetMinutes(reference: string): number {
  if (reference.endsWith('Z')) return 0;
  const offset = reference.slice(-6);
  return (Number(offset.slice(1, 3)) * 60 + Number(offset.slice(4, 6))) * (offset[0] === '-' ? -1 : 1);
}

// A numeric offset identifies the supplied civil clock, not an IANA timezone.
export function timestampAt(instantMs: number, reference: string, clock: Clock): string {
  if (clock === 'UTC' || reference.endsWith('Z')) return new Date(instantMs).toISOString();
  const offset = reference.slice(-6);
  const minutes = timestampOffsetMinutes(reference);
  return new Date(instantMs + minutes * 60_000).toISOString().slice(0, -1) + offset;
}

export function formatCoverage(summary: Pick<BacktestSummary, 'covered_duration_hours' | 'gap_duration_hours'>): string {
  const covered = summary.covered_duration_hours;
  const missing = summary.gap_duration_hours;
  const total = covered + missing;
  return `Data coverage: ${formatNumber(covered)} of ${formatNumber(total)} ${total === 1 ? 'hour' : 'hours'}; ${formatNumber(missing)} ${missing === 1 ? 'hour' : 'hours'} missing`;
}
