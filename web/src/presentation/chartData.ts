import type { Clock } from '../domain/types';
import type { LedgerRow } from '../services/api';
import { timestampAt } from './format';

const BUCKET_MS = 30 * 60_000;
const HOUR_MS = 60 * 60_000;

export interface PowerBucket {
  startMs: number;
  timestamp: string;
  chargeMWh: number;
  dischargeMWh: number;
  chargeMW: number;
  dischargeMW: number;
  coveredHours: number;
}

export function powerBuckets(ledger: readonly LedgerRow[], clock: Clock): PowerBucket[] {
  const rows = [...ledger].sort((a, b) => Date.parse(a.interval_start_utc) - Date.parse(b.interval_start_utc));
  const buckets = new Map<number, PowerBucket>();
  let nextBucket: number | undefined;
  let reference = '';

  for (const row of rows) {
    const start = Date.parse(row.interval_start_utc);
    const end = Date.parse(row.interval_end_utc);
    const firstBucket = Math.floor(start / BUCKET_MS) * BUCKET_MS;
    if (nextBucket === undefined) nextBucket = firstBucket;
    for (; nextBucket < firstBucket; nextBucket += BUCKET_MS) {
      buckets.set(nextBucket, {
        startMs: nextBucket, timestamp: timestampAt(nextBucket, reference, clock),
        chargeMWh: 0, dischargeMWh: 0, chargeMW: 0, dischargeMW: 0, coveredHours: 0,
      });
    }

    for (let bucketStart = firstBucket; bucketStart < end; bucketStart += BUCKET_MS) {
      const overlap = Math.min(end, bucketStart + BUCKET_MS) - Math.max(start, bucketStart);
      const fraction = overlap / (end - start);
      const bucket = buckets.get(bucketStart) ?? {
        startMs: bucketStart, timestamp: timestampAt(bucketStart, row.interval_start_local, clock),
        chargeMWh: 0, dischargeMWh: 0, chargeMW: 0, dischargeMW: 0, coveredHours: 0,
      };
      bucket.chargeMWh += row.energy_from_grid_mwh * fraction;
      bucket.dischargeMWh += row.energy_to_grid_mwh * fraction;
      bucket.coveredHours += overlap / HOUR_MS;
      bucket.chargeMW = bucket.chargeMWh / 0.5;
      bucket.dischargeMW = -bucket.dischargeMWh / 0.5;
      buckets.set(bucketStart, bucket);
      nextBucket = bucketStart + BUCKET_MS;
    }
    reference = row.interval_end_local;
  }

  return [...buckets.values()].sort((a, b) => a.startMs - b.startMs);
}

export interface PnlPoint {
  instantMs: number;
  timestamp: string;
  cumulativePNL: number;
}

export function pnlPoints(ledger: readonly LedgerRow[], clock: Clock): PnlPoint[] {
  const rows = [...ledger].sort((a, b) => Date.parse(a.interval_end_utc) - Date.parse(b.interval_end_utc));
  const first = rows[0];
  if (!first) return [];
  return [
    {
      instantMs: Date.parse(first.interval_start_utc),
      timestamp: clock === 'UTC' ? first.interval_start_utc : first.interval_start_local,
      cumulativePNL: 0,
    },
    ...rows.map(row => ({
      instantMs: Date.parse(row.interval_end_utc),
      timestamp: clock === 'UTC' ? row.interval_end_utc : row.interval_end_local,
      cumulativePNL: row.cum_pnl,
    })),
  ];
}
