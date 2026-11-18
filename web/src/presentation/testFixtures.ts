import halfHourFixture from '../../../testdata/half-hour-ledger.json';
import type { LedgerRow } from '../services/api';

export const halfHour: LedgerRow[] = halfHourFixture.map(row => {
  const action = row.action;
  if (action !== 'CHARGING' && action !== 'DISCHARGING' && action !== 'IDLE') {
    throw new Error('Shared ledger fixture has an invalid action');
  }
  return { ...row, action };
});

export const fiveMinutes: LedgerRow[] = Array.from({ length: 6 }, (_, index) => {
  const start = new Date(Date.parse(halfHour[0].interval_start_utc) + index * 300_000).toISOString();
  const end = new Date(Date.parse(start) + 300_000).toISOString();
  return {
    ...halfHour[0], index,
    interval_start_utc: start, interval_end_utc: end,
    interval_start_local: start, interval_end_local: end,
    energy_from_grid_mwh: 100 / 12, throughput_mwh: 100 / 12,
  };
});
