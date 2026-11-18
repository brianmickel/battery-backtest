import type { Clock } from '../domain/types';
import type { ChargeWindow, DischargeWindow } from '../services/api';
import { civilDate } from './format';

interface WindowDay {
  date: string;
  chargeWindows: ChargeWindow[];
  dischargeWindows: DischargeWindow[];
}

export function groupWindows(chargeWindows: readonly ChargeWindow[], dischargeWindows: readonly DischargeWindow[], clock: Clock): WindowDay[] {
  const days = new Map<string, WindowDay>();
  function dayAt(timestamp: string): WindowDay {
    const date = civilDate(timestamp, clock);
    const day = days.get(date) ?? { date, chargeWindows: [], dischargeWindows: [] };
    days.set(date, day);
    return day;
  }
  for (const window of chargeWindows) dayAt(window.start).chargeWindows.push(window);
  for (const window of dischargeWindows) dayAt(window.start).dischargeWindows.push(window);
  const ordered = [...days.values()].sort((a, b) => a.date < b.date ? -1 : a.date > b.date ? 1 : 0);
  for (const day of ordered) {
    day.chargeWindows.sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
    day.dischargeWindows.sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
  }
  return ordered;
}
