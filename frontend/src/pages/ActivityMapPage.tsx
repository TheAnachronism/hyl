import { useParams } from '@solidjs/router';
import { Show, createResource, onCleanup, onMount } from 'solid-js';
import type { ActivityDetail, ActivitySummary } from '../api.gen';
import MapView from '../components/MapView';
import { api } from '../lib/api';
import { distance, duration, elevation } from '../lib/format';
import { basemapArchiveMaxZoom } from '../lib/maps';
import { config, loadConfig } from '../lib/session';
import { visibleTheme } from '../lib/theme';

/**
 * ActivityDetail embeds ActivitySummary without a JSON tag, so Go promotes the
 * summary fields into the detail object and the wire payload is flat. This type
 * spells out that flattened shape.
 */
type ActivityDetailJSON = ActivitySummary & Pick<ActivityDetail, 'canEdit' | 'route' | 'streams'>;

/** MapNotice stands in for the map when there is nothing to draw. */
function MapNotice(props: { id: string; message: string }) {
  return (
    <section class="section">
      <div class="container has-text-centered">
        <p class="title is-5">{props.message}</p>
        <Show when={props.id}>
          <a class="button" href={`/activities/${props.id}`}>
            Back to activity
          </a>
        </Show>
      </div>
    </section>
  );
}

/** ActivityMapPage is the full-viewport map for one activity's route. */
export default function ActivityMapPage() {
  const params = useParams<{ id: string }>();
  void loadConfig();

  // A full-bleed map must not coexist with page scrolling.
  onMount(() => document.body.classList.add('hyl-no-scroll'));
  onCleanup(() => document.body.classList.remove('hyl-no-scroll'));
  const [detail] = createResource(
    () => params.id ?? '',
    (activityId) => api.get<ActivityDetailJSON>(`/api/activities/${activityId}`),
  );

  // The operator's extract may cap out well below z15; asking the archive keeps
  // the style honest instead of guessing a zoom depth.
  const [maxZoom] = createResource(
    () => config()?.pmtilesUrl ?? null,
    async (url) => {
      try {
        return await basemapArchiveMaxZoom(url);
      } catch {
        return 12;
      }
    },
  );

  return (
    <Show
      // Reading an errored resource throws, so the error is checked first.
      when={detail.error ? undefined : detail()}
      fallback={
        <MapNotice
          id={detail.error ? params.id ?? '' : ''}
          message={detail.error ? 'No map for this activity' : 'Loading map…'}
        />
      }
    >
      {(activity) => (
        <Show
          when={activity().mapAvailable && activity().route.length >= 4}
          fallback={<MapNotice id={String(activity().id)} message="No map for this activity" />}
        >
          <div class="hyl-map-page">
            <MapView
              route={activity().route}
              style={config()?.pmtilesUrl ? 'basemap' : 'fallback'}
              theme={visibleTheme()}
              maxZoom={maxZoom() ?? undefined}
            />
            <div class="hyl-map-overlay">
              <div class="notification is-light">
                <p class="has-text-weight-semibold is-clipped">{activity().title}</p>
                <p class="is-size-7 has-text-grey hyl-mono">
                  {distance(activity().distanceM)} · {duration(activity().movingTimeS)} ·{' '}
                  {elevation(activity().elevationGainM)}
                </p>
                <a class="is-size-7" href={`/activities/${activity().id}`}>
                  Back to activity
                </a>
              </div>
            </div>
          </div>
        </Show>
      )}
    </Show>
  );
}
