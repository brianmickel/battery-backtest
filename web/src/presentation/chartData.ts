import type { Clock } from '../domain/types';
import type { LedgerRow } from '../services/api';
import { timestampAt, timestampOffsetMinutes } from './format';

const BUCKET_MS = 30 * 60_000;
const HOUR_MS = 60 * 60_000;

interface BucketQuantities {
  startMs: number;
  chargeMWh: number;
  dischargeMWh: number;
  chargeMW: number;
  dischargeMW: number;
  coveredHours: number;
}

export interface PowerBucket extends BucketQuantities {
  timestamp: string;
  timestampSource: 'utc' | 'market-observed' | 'market-projected' | 'utc-fallback';
}

export function powerBuckets(ledger: readonly LedgerRow[], clock: Clock): PowerBucket[] {
  const rows = [...ledger].sort((a, b) => Date.parse(a.interval_start_utc) - Date.parse(b.interval_start_utc));
  const buckets = new Map<number, BucketQuantities>();
  let nextBucket: number | undefined;

  for (const row of rows) {
    const start = Date.parse(row.interval_start_utc);
    const end = Date.parse(row.interval_end_utc);
    const firstBucket = Math.floor(start / BUCKET_MS) * BUCKET_MS;
    if (nextBucket === undefined) nextBucket = firstBucket;
    for (; nextBucket < firstBucket; nextBucket += BUCKET_MS) {
      buckets.set(nextBucket, {
        startMs: nextBucket,
        chargeMWh: 0, dischargeMWh: 0, chargeMW: 0, dischargeMW: 0, coveredHours: 0,
      });
    }

    for (let bucketStart = firstBucket; bucketStart < end; bucketStart += BUCKET_MS) {
      const overlap = Math.min(end, bucketStart + BUCKET_MS) - Math.max(start, bucketStart);
      const fraction = overlap / (end - start);
      const bucket = buckets.get(bucketStart) ?? {
        startMs: bucketStart,
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
  }

  const observations = rows.flatMap(row => [
    { instantMs: Date.parse(row.interval_start_utc), timestamp: row.interval_start_local },
    { instantMs: Date.parse(row.interval_end_utc), timestamp: row.interval_end_local },
  ]).sort((a, b) => a.instantMs - b.instantMs);
  let nextObservation = 0;
  return [...buckets.values()].sort((a, b) => a.startMs - b.startMs).map((bucket): PowerBucket => {
    if (clock === 'UTC') return { ...bucket, timestamp: new Date(bucket.startMs).toISOString(), timestampSource: 'utc' };
    while (nextObservation < observations.length - 1 && observations[nextObservation].instantMs < bucket.startMs) nextObservation++;
    // At a shared endpoint the new interval's start determines the bucket label.
    while (nextObservation < observations.length - 1 && observations[nextObservation + 1].instantMs === bucket.startMs) nextObservation++;
    const next = observations[nextObservation];
    if (next.instantMs === bucket.startMs) return { ...bucket, timestamp: next.timestamp, timestampSource: 'market-observed' };
    // The rounded first bucket uses the first observation's offset; later buckets use both neighbors.
    const previous = observations[nextObservation - 1] ?? next;
    if (timestampOffsetMinutes(previous.timestamp) !== timestampOffsetMinutes(next.timestamp)) {
      return { ...bucket, timestamp: new Date(bucket.startMs).toISOString(), timestampSource: 'utc-fallback' };
    }
    return { ...bucket, timestamp: timestampAt(bucket.startMs, previous.timestamp, clock), timestampSource: 'market-projected' };
  });
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
