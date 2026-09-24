import type { Component } from 'solid-js';
import {
  TbOutlineActivity,
  TbOutlineBike,
  TbOutlineKayak,
  TbOutlineMountain,
  TbOutlineRun,
  TbOutlineSkiJumping,
  TbOutlineSwimming,
  TbOutlineWalk,
} from 'solid-icons/tb';

/**
 * The sport contract shared by the upload form, the stats filter and every
 * label in the UI. The keys are exactly the strings the backend stores, so
 * there is no translation layer anywhere.
 */
export interface Sport {
  key: string;
  label: string;
  /** usesPace selects min/km over km/h in every metric renderer. */
  usesPace: boolean;
  /** icon marks the sport on an activity card and page. */
  icon: Component<{ size?: number | string }>;
}

/** Tabler has no rowing glyph, so the paddling one stands in for it. */
export const SPORTS: Sport[] = [
  { key: 'run', label: 'Run', usesPace: true, icon: TbOutlineRun },
  { key: 'walk', label: 'Walk', usesPace: true, icon: TbOutlineWalk },
  { key: 'hike', label: 'Hike', usesPace: true, icon: TbOutlineMountain },
  { key: 'ride', label: 'Ride', usesPace: false, icon: TbOutlineBike },
  { key: 'swim', label: 'Swim', usesPace: false, icon: TbOutlineSwimming },
  { key: 'ski', label: 'Ski', usesPace: false, icon: TbOutlineSkiJumping },
  { key: 'row', label: 'Row', usesPace: false, icon: TbOutlineKayak },
  { key: 'other', label: 'Other', usesPace: false, icon: TbOutlineActivity },
];

const SPORT_BY_KEY: Record<string, Sport | undefined> = Object.fromEntries(
  SPORTS.map((sport) => [sport.key, sport]),
);

/** sportLabel renders a stored sport key. */
export function sportLabel(key: string | null | undefined): string {
  if (!key) return 'Activity';
  return SPORT_BY_KEY[key]?.label ?? key;
}

/** sportIcon renders the glyph for a stored sport key; unknown keys fall back
 * to the catch-all rather than rendering nothing. */
export function sportIcon(key: string | null | undefined): Component<{ size?: number | string }> {
  return SPORT_BY_KEY[key ?? '']?.icon ?? TbOutlineActivity;
}

/** usesPace reports whether a sport is shown as pace rather than speed. */
export function usesPace(key: string | null | undefined): boolean {
  return SPORT_BY_KEY[key ?? '']?.usesPace ?? false;
}
