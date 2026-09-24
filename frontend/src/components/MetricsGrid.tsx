import { For } from 'solid-js';
import type { ActivitySummary } from '../api.gen';
import { bpm, cadence, distance, duration, elevation, paceOrSpeed, watts } from '../lib/format';
import { usesPace } from '../lib/sports';

interface Metric {
  label: string;
  value: string;
}

/** MetricsGrid renders the compact metric row shared by cards and the activity page. */
export default function MetricsGrid(props: { activity: ActivitySummary }) {
  const metrics = (): Metric[] => {
    const activity = props.activity;
    const rows: Metric[] = [
      { label: 'Distance', value: distance(activity.distanceM) },
      { label: 'Moving', value: duration(activity.movingTimeS) },
      { label: 'Elapsed', value: duration(activity.elapsedTimeS) },
      { label: 'Elevation', value: elevation(activity.elevationGainM) },
    ];
    // Optional series render only when the file carried them; the format
    // helpers turn a null through to DASH, so test for presence not truthiness.
    if (activity.avgSpeedMps != null) {
      rows.push({ label: 'Avg', value: paceOrSpeed(activity.avgSpeedMps, usesPace(activity.sport)) });
    }
    if (activity.avgHeartRate != null) {
      rows.push({ label: 'Avg HR', value: bpm(activity.avgHeartRate) });
    }
    if (activity.avgCadence != null) {
      rows.push({ label: 'Cadence', value: cadence(activity.avgCadence) });
    }
    if (activity.avgPowerW != null) {
      rows.push({ label: 'Power', value: watts(activity.avgPowerW) });
    }
    return rows;
  };

  return (
    <div class="columns is-mobile is-multiline is-gapless hyl-mono">
      <For each={metrics()}>
        {(metric) => (
          <div class="column is-half-mobile is-one-quarter-tablet">
            <p class="heading is-size-7 mb-0">{metric.label}</p>
            <p class="has-text-weight-semibold is-size-6">{metric.value}</p>
          </div>
        )}
      </For>
    </div>
  );
}
