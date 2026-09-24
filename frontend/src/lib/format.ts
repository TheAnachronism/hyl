/** Formatting helpers shared by every metric the UI renders. */

export const DASH = '—';

/** distance renders metres as kilometres with one or two decimals. */
export function distance(meters: number | null | undefined): string {
  if (meters === null || meters === undefined) return DASH;
  const km = meters / 1000;
  if (km < 10) return `${km.toFixed(2)} km`;
  if (km < 100) return `${km.toFixed(1)} km`;
  return `${Math.round(km)} km`;
}

/** distanceValue renders just the number, for tight layouts. */
export function distanceValue(meters: number | null | undefined): string {
  if (meters === null || meters === undefined) return DASH;
  const km = meters / 1000;
  return km < 100 ? km.toFixed(2) : Math.round(km).toString();
}

/** duration renders seconds as h:mm:ss (or m:ss under an hour). */
export function duration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || seconds < 0) return DASH;
  const total = Math.round(seconds);
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
  return `${m}:${String(s).padStart(2, '0')}`;
}

/** paceOrSpeed renders min/km for pace sports and km/h for the rest. */
export function paceOrSpeed(speedMps: number | null | undefined, usesPace: boolean): string {
  if (speedMps === null || speedMps === undefined || speedMps <= 0) return DASH;
  if (usesPace) {
    const secondsPerKm = 1000 / speedMps;
    return `${duration(secondsPerKm)} /km`;
  }
  return `${(speedMps * 3.6).toFixed(1)} km/h`;
}

/** elevation renders metres. */
export function elevation(meters: number | null | undefined): string {
  if (meters === null || meters === undefined) return DASH;
  return `${Math.round(meters)} m`;
}

/** bpm renders an optional heart rate. */
export function bpm(value: number | null | undefined): string {
  if (value === null || value === undefined) return DASH;
  return `${Math.round(value)} bpm`;
}

/** cadence renders an optional cadence. */
export function cadence(value: number | null | undefined): string {
  if (value === null || value === undefined) return DASH;
  return `${Math.round(value)} spm`;
}

/** watts renders an optional power value. */
export function watts(value: number | null | undefined): string {
  if (value === null || value === undefined) return DASH;
  return `${Math.round(value)} W`;
}

/** count renders a plain integer. */
export function count(value: number | null | undefined): string {
  if (value === null || value === undefined) return DASH;
  return String(Math.round(value));
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });

/** relativeTime renders a timestamp as "3 hours ago". */
export function relativeTime(iso: string | null | undefined): string {
  if (!iso) return DASH;
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return DASH;
  const deltaSeconds = Math.round((then - Date.now()) / 1000);
  const abs = Math.abs(deltaSeconds);
  if (abs < 45) return rtf.format(Math.round(deltaSeconds), 'second');
  if (abs < 3600) return rtf.format(Math.round(deltaSeconds / 60), 'minute');
  if (abs < 86400) return rtf.format(Math.round(deltaSeconds / 3600), 'hour');
  if (abs < 2592000) return rtf.format(Math.round(deltaSeconds / 86400), 'day');
  if (abs < 31536000) return rtf.format(Math.round(deltaSeconds / 2592000), 'month');
  return rtf.format(Math.round(deltaSeconds / 31536000), 'year');
}

/** absoluteTime renders the full local timestamp used in titles. */
export function absoluteTime(iso: string | null | undefined): string {
  if (!iso) return DASH;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return DASH;
  return date.toLocaleString();
}

/** shortDate renders "Tue 2 Sep 2026, 15:04". */
export function shortDate(iso: string | null | undefined): string {
  if (!iso) return DASH;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return DASH;
  return date.toLocaleString(undefined, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

/** dateOnly renders "2 Sep 2026", for tables where the time of day is noise. */
export function dateOnly(iso: string | null | undefined): string {
  if (!iso) return DASH;
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return DASH;
  return date.toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' });
}
