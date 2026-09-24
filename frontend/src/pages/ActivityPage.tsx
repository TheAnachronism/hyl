import { useNavigate, useParams } from '@solidjs/router';
import { For, Show, createResource, createSignal } from 'solid-js';
import type { ActivityDetail, ExportState } from '../api.gen';
import CommentForm from '../components/CommentForm';
import CommentList from '../components/CommentList';
import PeakButton from '../components/PeakButton';
import MetricsGrid from '../components/MetricsGrid';
import PhotoGrid from '../components/PhotoGrid';
import PhotoUpload from '../components/PhotoUpload';
import SportBadge from '../components/SportBadge';
import TrackPreview from '../components/TrackPreview';
import Avatar from '../components/Avatar';
import { ApiError, api } from '../lib/api';
import { shortDate } from '../lib/format';
import { currentUser } from '../lib/session';
import { sportLabel } from '../lib/sports';

/** LoadState keeps the resource from ever holding a thrown error. */
type LoadState =
  | { status: 'ready'; activity: ActivityDetail }
  | { status: 'missing' }
  | { status: 'error'; message: string };

const VISIBILITY_OPTIONS = [
  { value: 'default', label: 'Default (profile setting)' },
  { value: 'everyone', label: 'Everyone' },
  { value: 'followers', label: 'Followers' },
  { value: 'only_me', label: 'Only me' },
];

/** EXPORT_STATUS_LABELS names the export queue states the API reports. */
const EXPORT_STATUS_LABELS: Record<string, string> = {
  pending: 'Queued for Strava',
  sent: 'Sent to Strava',
  error: 'Strava export failed',
};

interface Series {
  label: string;
  values: (number | undefined)[];
  format: (value: number) => string;
}

const SPARK_WIDTH = 160;
const SPARK_HEIGHT = 36;

/** Sparkline draws one dependency-free sparkline plus its range. */
function Sparkline(props: Series) {
  const geometry = () => {
    // Absent samples arrive as JSON null (Go's nil slice elements), so test the
    // value rather than the generated `number | undefined` type.
    const defined = props.values.filter((value): value is number => typeof value === 'number');
    if (defined.length < 2) return null;
    let min = Infinity;
    let max = -Infinity;
    for (const value of defined) {
      if (value < min) min = value;
      if (value > max) max = value;
    }
    const span = max - min || 1;
    const step = SPARK_WIDTH / (props.values.length - 1);
    const points: string[] = [];
    props.values.forEach((value, index) => {
      if (typeof value !== 'number') return;
      const x = index * step;
      const y = SPARK_HEIGHT - ((value - min) / span) * SPARK_HEIGHT;
      points.push(`${x.toFixed(1)},${y.toFixed(1)}`);
    });
    return { points: points.join(' '), range: `${props.format(min)} – ${props.format(max)}` };
  };

  return (
    <div class="column is-half-mobile is-one-third-tablet">
      <p class="heading is-size-7 mb-1">{props.label}</p>
      <Show when={geometry()}>
        {(shape) => (
          <>
            <svg
              viewBox={`0 0 ${SPARK_WIDTH} ${SPARK_HEIGHT}`}
              preserveAspectRatio="none"
              style="display:block;width:100%;height:2rem"
            >
              <polyline
                class="hyl-track"
                points={shape().points}
                fill="none"
                stroke-width="2"
                stroke-linejoin="round"
                stroke-linecap="round"
              />
            </svg>
            <p class="is-size-7 has-text-grey hyl-mono mb-0">{shape().range}</p>
          </>
        )}
      </Show>
    </div>
  );
}

/** ActivityPage shows one activity, its streams, photos and comments. */
export default function ActivityPage() {
  const params = useParams();
  const navigate = useNavigate();
  const activityId = () => {
    const value = Number(params.id);
    return Number.isFinite(value) && value > 0 ? value : null;
  };

  // Solid re-throws a resource's error when its accessor is read, which would
  // abort rendering, so the fetcher resolves into an explicit state instead.
  const [detail, { mutate }] = createResource(activityId, async (id): Promise<LoadState> => {
    try {
      return { status: 'ready', activity: await api.get<ActivityDetail>(`/api/activities/${id}`) };
    } catch (err) {
      if (err instanceof ApiError && (err.status === 404 || err.status === 403)) return { status: 'missing' };
      return { status: 'error', message: err instanceof ApiError ? err.message : 'Could not load the activity.' };
    }
  });

  const ready = () => {
    const state = detail();
    return state?.status === 'ready' ? state.activity : null;
  };
  const missing = () => activityId() === null || detail()?.status === 'missing';
  const failure = () => {
    const state = detail();
    return state?.status === 'error' ? state.message : '';
  };

  // The owner's edits are gathered locally and only sent when Save is pressed,
  // so a half-finished title never reaches the API.
  const [editing, setEditing] = createSignal(false);
  const [draftTitle, setDraftTitle] = createSignal('');
  const [draftDescription, setDraftDescription] = createSignal('');
  const [draftVisibility, setDraftVisibility] = createSignal('default');
  const [draftRouteHidden, setDraftRouteHidden] = createSignal(false);

  const [extraComments, setExtraComments] = createSignal(0);
  const [reloadKey, setReloadKey] = createSignal(0);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');

  // The export queue is the owner's business alone, so the state is only read
  // for an activity they own; for anyone else the resource stays idle.
  const [exportState, { refetch: refetchExport }] = createResource(
    () => {
      const current = ready();
      return current && current.userId === currentUser()?.id ? current.id : null;
    },
    (id) => api.get<ExportState>(`/api/activities/${id}/export`),
  );
  const [exportBusy, setExportBusy] = createSignal(false);
  const [exportNotice, setExportNotice] = createSignal('');
  const [exportError, setExportError] = createSignal('');

  async function retryExport(): Promise<void> {
    const current = ready();
    if (!current || exportBusy()) return;
    setExportBusy(true);
    setExportNotice('');
    setExportError('');
    try {
      await api.post<{ status: string }>(`/api/activities/${current.id}/export`, { target: 'strava' });
      setExportNotice('Export queued. It will be sent to Strava shortly.');
    } catch (err) {
      // A 409 means it is already queued or already sent: that is the state the
      // user wanted, not a failure.
      if (err instanceof ApiError && err.status === 409) {
        setExportNotice(err.message);
      } else {
        setExportError(err instanceof ApiError ? err.message : 'Could not queue the export.');
      }
    } finally {
      setExportBusy(false);
      void refetchExport();
    }
  }

  const streams = (): Series[] => {
    const data = ready()?.streams;
    if (!data) return [];
    const candidates: Series[] = [
      { label: 'Heart rate', values: data.hr, format: (value) => `${Math.round(value)} bpm` },
      { label: 'Speed', values: data.speedMps, format: (value) => `${(value * 3.6).toFixed(1)} km/h` },
      { label: 'Elevation', values: data.elevationM, format: (value) => `${Math.round(value)} m` },
      { label: 'Cadence', values: data.cadence, format: (value) => `${Math.round(value)} spm` },
      { label: 'Power', values: data.power, format: (value) => `${Math.round(value)} W` },
    ];
    return candidates.filter((series) => series.values.some((value) => typeof value === 'number'));
  };

  /** patch reports success so an editor only closes once the API accepted it. */
  async function patch(body: Record<string, unknown>): Promise<boolean> {
    const current = ready();
    if (!current) return false;
    setBusy(true);
    setError('');
    try {
      mutate({ status: 'ready', activity: await api.patch<ActivityDetail>(`/api/activities/${current.id}`, body) });
      return true;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not update the activity.');
      return false;
    } finally {
      setBusy(false);
    }
  }

  // Editing is explicit: the owner controls, the photo uploader and the delete
  // button only exist while the editor is open, and nothing is sent until Save
  // is pressed.
  function startEditing(): void {
    const current = ready();
    if (!current) return;
    setDraftTitle(current.title);
    setDraftDescription(current.description);
    setDraftVisibility(current.visibility);
    setDraftRouteHidden(current.routeHidden);
    setError('');
    setEditing(true);
  }

  async function saveEdits(): Promise<void> {
    const saved = await patch({
      title: draftTitle().trim(),
      description: draftDescription(),
      visibility: draftVisibility(),
      routeHidden: draftRouteHidden(),
    });
    if (saved) setEditing(false);
  }

  async function remove(): Promise<void> {
    const current = ready();
    if (!current || busy()) return;
    if (!window.confirm('Delete this activity? This cannot be undone.')) return;
    setBusy(true);
    setError('');
    try {
      await api.del<void>(`/api/activities/${current.id}`);
      navigate('/');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not delete the activity.');
      setBusy(false);
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:48rem">
        <Show when={detail.loading}>
          <p class="has-text-grey">Loading…</p>
        </Show>

        <Show when={missing()}>
          <div class="notification is-light">
            <h1 class="title is-5">This activity is private or does not exist</h1>
            <p>
              <a href="/">Back to the feed</a>
            </p>
          </div>
        </Show>

        <Show when={failure()}>
          <div class="notification is-danger is-outlined">{failure()}</div>
        </Show>

        <Show when={ready()}>
          {(activity) => (
            <>
              <div class="is-flex is-align-items-center is-justify-content-space-between mb-1">
                <a class="is-inline-flex is-align-items-center" href={`/u/${activity().username}`}>
                  <Avatar
                    src={activity().avatarUrl}
                    name={activity().displayName || activity().username}
                    class="mr-2"
                  />
                  <span class="has-text-weight-semibold">{activity().displayName || activity().username}</span>
                </a>
                <div class="is-flex is-align-items-center" style="gap:0.5rem">
                  <SportBadge sport={activity().sport} />
                  <Show when={activity().canEdit && !editing()}>
                    <button type="button" class="button is-small" onClick={startEditing}>
                      Edit activity
                    </button>
                  </Show>
                </div>
              </div>
              <p class="subtitle is-7 has-text-grey mb-2">{shortDate(activity().startedAt)}</p>
              <Show
                when={editing()}
                fallback={<h1 class="title is-4 mb-1">{activity().title || sportLabel(activity().sport)}</h1>}
              >
                <div class="field">
                  <label class="label is-small" for="hyl-activity-title">
                    Title
                  </label>
                  <div class="control">
                    <input
                      id="hyl-activity-title"
                      class="input"
                      type="text"
                      maxlength={120}
                      value={draftTitle()}
                      onInput={(event) => setDraftTitle(event.currentTarget.value)}
                    />
                  </div>
                </div>
              </Show>

              <Show when={exportState()?.status}>
                <div class="notification is-light py-3">
                  <div class="is-flex is-align-items-center is-justify-content-space-between">
                    <span class="is-size-7">
                      {EXPORT_STATUS_LABELS[exportState()?.status ?? ''] ?? exportState()?.status}
                    </span>
                    <Show when={exportState()?.status === 'error'}>
                      <button
                        type="button"
                        class="button is-small ml-3"
                        disabled={exportBusy()}
                        onClick={() => void retryExport()}
                      >
                        {exportBusy() ? 'Retrying…' : 'Retry export'}
                      </button>
                    </Show>
                  </div>
                  <Show when={exportState()?.status === 'error' && exportState()?.lastError}>
                    <p class="help is-danger mt-1 mb-0">{exportState()?.lastError}</p>
                  </Show>
                  <Show when={exportNotice()}>
                    <p class="help mt-1 mb-0">{exportNotice()}</p>
                  </Show>
                  <Show when={exportError()}>
                    <p class="help is-danger mt-1 mb-0">{exportError()}</p>
                  </Show>
                </div>
              </Show>

              <MetricsGrid activity={activity()} />

              <TrackPreview
                track={activity().track}
                mapAvailable={activity().mapAvailable}
                href={`/activities/${activity().id}/map`}
              />
              <Show when={activity().mapAvailable}>
                <a class="button is-small mt-2" href={`/activities/${activity().id}/map`}>
                  Open the map
                </a>
              </Show>

              <Show when={streams().length > 0}>
                <div class="columns is-mobile is-multiline mt-4">
                  <For each={streams()}>
                    {(series) => <Sparkline label={series.label} values={series.values} format={series.format} />}
                  </For>
                </div>
              </Show>

              <Show
                when={editing()}
                fallback={
                  <Show when={activity().description}>
                    <p class="mt-4" style="white-space:pre-wrap">
                      {activity().description}
                    </p>
                  </Show>
                }
              >
                <div class="field mt-4">
                  <label class="label is-small" for="hyl-activity-description">
                    Description
                  </label>
                  <div class="control">
                    <textarea
                      id="hyl-activity-description"
                      class="textarea"
                      rows={4}
                      maxlength={2000}
                      value={draftDescription()}
                      onInput={(event) => setDraftDescription(event.currentTarget.value)}
                    />
                  </div>
                </div>
              </Show>

              <PhotoGrid photos={activity().photos} />
              <Show when={activity().canEdit && editing()}>
                <PhotoUpload
                  activityId={activity().id}
                  photos={activity().photos}
                  onChanged={(photos) =>
                    mutate({ status: 'ready', activity: { ...activity(), photos, photoCount: photos.length } })
                  }
                />
              </Show>

              <Show when={activity().canEdit && editing()}>
                <div class="box mt-4">
                  <h2 class="title is-6">Activity settings</h2>
                  <div class="field">
                    <label class="label is-small" for="hyl-activity-visibility">
                      Who can see this activity
                    </label>
                    <div class="control">
                      <div class="select is-small">
                        <select
                          id="hyl-activity-visibility"
                          value={draftVisibility()}
                          disabled={busy()}
                          onChange={(event) => setDraftVisibility(event.currentTarget.value)}
                        >
                          <For each={VISIBILITY_OPTIONS}>
                            {(option) => <option value={option.value}>{option.label}</option>}
                          </For>
                        </select>
                      </div>
                    </div>
                  </div>
                  <div class="field">
                    <label class="checkbox" for="hyl-activity-route">
                      <input
                        id="hyl-activity-route"
                        type="checkbox"
                        class="switch"
                        checked={!draftRouteHidden()}
                        disabled={busy()}
                        onChange={(event) => setDraftRouteHidden(!event.currentTarget.checked)}
                      />
                      Show route
                    </label>
                    <p class="help">
                      <Show
                        when={activity().hasGps}
                        fallback="This activity has no GPS track, so there is nothing to show."
                      >
                        Turn this off to hide the map for everyone.
                      </Show>
                    </p>
                  </div>
                  <Show when={error()}>
                    <p class="help is-danger">{error()}</p>
                  </Show>
                  <div class="buttons mt-4">
                    <button
                      type="button"
                      class="button is-small is-primary"
                      disabled={busy()}
                      onClick={() => void saveEdits()}
                    >
                      Save changes
                    </button>
                    <button
                      type="button"
                      class="button is-small"
                      disabled={busy()}
                      onClick={() => setEditing(false)}
                    >
                      Cancel
                    </button>
                    <button
                      type="button"
                      class="button is-small is-danger is-outlined"
                      disabled={busy()}
                      onClick={() => void remove()}
                    >
                      Delete activity
                    </button>
                  </div>
                </div>
              </Show>

              <div class="level is-mobile mt-4">
                <div class="level-left">
                  <div class="level-item">
                    <PeakButton
                      activityId={activity().id}
                      likeCount={activity().likeCount}
                      likedByMe={activity().likedByMe}
                      canPeak={currentUser()?.id !== activity().userId}
                    />
                  </div>
                </div>
              </div>

              <div class="box mt-4">
                <h2 class="title is-6">
                  {activity().commentCount + extraComments()}{' '}
                  {activity().commentCount + extraComments() === 1 ? 'comment' : 'comments'}
                </h2>
                <CommentList
                  activityId={activity().id}
                  reloadKey={reloadKey()}
                  onDeleted={() => setExtraComments((value) => value - 1)}
                />
                <CommentForm
                  activityId={activity().id}
                  onCreated={() => {
                    setReloadKey((value) => value + 1);
                    setExtraComments((value) => value + 1);
                  }}
                />
              </div>
            </>
          )}
        </Show>
      </div>
    </section>
  );
}
