import { useSearchParams } from '@solidjs/router';
import { For, Show, createEffect, createResource, createSignal } from 'solid-js';
import type {
  APIKey,
  APIKeyCreated,
  Connection,
  ConnectionsResponse,
  ImportRule,
  LinkedIdentity,
  Me,
  PrivacyZone,
} from '../api.gen';
import AvatarUpload from '../components/AvatarUpload';
import { ApiError, api } from '../lib/api';
import { absoluteTime, dateOnly, relativeTime, shortDate } from '../lib/format';
import { sportLabel } from '../lib/sports';
import { config, currentUser, setCurrentUser } from '../lib/session';

const TABS = [
  { key: 'account', label: 'Account' },
  { key: 'privacy', label: 'Privacy' },
  { key: 'connections', label: 'Connections' },
  { key: 'developer', label: 'Developer' },
];

/** CONNECTION_LABELS names the provider a stored connection talks to. */
const CONNECTION_LABELS: Record<string, string> = {
  intervals_oauth: 'intervals.icu (OAuth)',
  intervals_apikey: 'intervals.icu (API key)',
  strava_oauth: 'Strava',
};

const PROVIDER_LABELS: Record<string, string> = { google: 'Google', github: 'GitHub' };

interface Option {
  value: string;
  label: string;
}

const PROFILE_VISIBILITY: Option[] = [
  { value: 'everyone', label: 'Everyone' },
  { value: 'followers', label: 'Followers only' },
];
const ACTIVITIES_VISIBILITY: Option[] = [
  { value: 'everyone', label: 'Everyone' },
  { value: 'followers', label: 'Followers only' },
  { value: 'only_me', label: 'Only me' },
];
const FOLLOW_POLICY: Option[] = [
  { value: 'everyone', label: 'Anyone can follow me' },
  { value: 'on_request', label: 'Approve follow requests' },
];
const MENTION_POLICY: Option[] = [
  { value: 'everyone', label: 'Anyone' },
  { value: 'followers', label: 'People who follow me' },
  { value: 'nobody', label: 'Nobody' },
];
const TRIM_SCOPE: Option[] = [
  { value: 'all', label: 'Hide a radius around the start and end of every route' },
  { value: 'zones', label: 'Hide my privacy zones only' },
];

/** The API error envelope, surfaced verbatim so 409s read as explanations. */
function message(err: unknown): string {
  return err instanceof ApiError ? err.message : 'the request failed';
}

function RadioGroup(props: {
  legend: string;
  name: string;
  value: string;
  options: Option[];
  onChange: (value: string) => void;
}) {
  return (
    <div class="field">
      <label class="label">{props.legend}</label>
      <div class="control">
        <For each={props.options}>
          {(option) => (
            <label class="radio mr-4">
              <input
                type="radio"
                name={props.name}
                value={option.value}
                checked={props.value === option.value}
                onChange={() => props.onChange(option.value)}
              />{' '}
              {option.label}
            </label>
          )}
        </For>
      </div>
    </div>
  );
}

function AccountSection() {
  const [error, setError] = createSignal('');
  const [notice, setNotice] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  const [identities, { refetch: refetchIdentities }] = createResource(
    () => currentUser()?.id,
    () => api.get<LinkedIdentity[]>('/api/me/identities'),
  );

  async function saveProfile(event: SubmitEvent) {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    setBusy(true);
    setError('');
    setNotice('');
    try {
      setCurrentUser(
        await api.patch<Me>('/api/me', {
          displayName: String(data.get('displayName') ?? '').trim(),
          bio: String(data.get('bio') ?? '').trim(),
        }),
      );
      setNotice('Profile saved.');
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function saveEmail(event: SubmitEvent) {
    event.preventDefault();
    const data = new FormData(event.currentTarget as HTMLFormElement);
    setBusy(true);
    setError('');
    setNotice('');
    try {
      setCurrentUser(
        await api.post<Me>('/api/me/email', {
          email: String(data.get('email') ?? '').trim(),
          password: String(data.get('password') ?? ''),
        }),
      );
      setNotice('Email changed. Check your inbox for a fresh verification link.');
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function savePassword(event: SubmitEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const data = new FormData(form);
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api.post<void>('/api/me/password', {
        currentPassword: String(data.get('currentPassword') ?? ''),
        newPassword: String(data.get('newPassword') ?? ''),
      });
      form.reset();
      setNotice('Password changed. Every other session was signed out.');
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function unlink(provider: string) {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api.del<void>(`/api/me/identities/${provider}`);
      void refetchIdentities();
      setNotice(`Unlinked ${PROVIDER_LABELS[provider] ?? provider}.`);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Show when={error()}>
        <div class="notification is-danger is-outlined">{error()}</div>
      </Show>
      <Show when={notice()}>
        <div class="notification is-success is-light">{notice()}</div>
      </Show>

      <form onSubmit={(event) => void saveProfile(event)}>
        <div class="field">
          <label class="label" for="displayName">
            Display name
          </label>
          <div class="control">
            <input
              id="displayName"
              name="displayName"
              class="input"
              maxlength="60"
              value={currentUser()?.displayName ?? ''}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="bio">
            Bio
          </label>
          <div class="control">
            <textarea
              id="bio"
              name="bio"
              class="textarea"
              maxlength="500"
              value={currentUser()?.bio ?? ''}
            />
          </div>
        </div>
        <button class="button is-primary" type="submit" disabled={busy()}>
          Save profile
        </button>
      </form>

      <hr />
      <AvatarUpload />

      <hr />
      <h2 class="title is-6">Email</h2>
      <form onSubmit={(event) => void saveEmail(event)}>
        <div class="field">
          <label class="label" for="email">
            Email address
          </label>
          <div class="control">
            <input
              id="email"
              name="email"
              class="input"
              type="email"
              autocomplete="email"
              required
              value={currentUser()?.email ?? ''}
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="email-password">
            Current password
          </label>
          <div class="control">
            <input
              id="email-password"
              name="password"
              class="input"
              type="password"
              autocomplete="current-password"
              required
            />
          </div>
        </div>
        <button class="button" type="submit" disabled={busy()}>
          Change email
        </button>
      </form>

      <hr />
      <h2 class="title is-6">Password</h2>
      <form onSubmit={(event) => void savePassword(event)}>
        <div class="field">
          <label class="label" for="currentPassword">
            Current password
          </label>
          <div class="control">
            <input
              id="currentPassword"
              name="currentPassword"
              class="input"
              type="password"
              autocomplete="current-password"
              required
            />
          </div>
        </div>
        <div class="field">
          <label class="label" for="newPassword">
            New password
          </label>
          <div class="control">
            <input
              id="newPassword"
              name="newPassword"
              class="input"
              type="password"
              autocomplete="new-password"
              required
            />
          </div>
          <p class="help">At least 10 characters. Every other session is signed out.</p>
        </div>
        <button class="button" type="submit" disabled={busy()}>
          Change password
        </button>
      </form>

      <hr />
      <h2 class="title is-6">Linked accounts</h2>
      <For each={identities() ?? []}>
        {(identity) => (
          <div class="is-flex is-align-items-center mb-2">
            <span class="tag mr-2">
              {PROVIDER_LABELS[identity.provider] ?? identity.provider}
            </span>
            <span class="has-text-grey is-size-7 mr-3">{identity.email ?? ''}</span>
            <button
              type="button"
              class="button is-small"
              disabled={busy()}
              onClick={() => void unlink(identity.provider)}
            >
              Unlink
            </button>
          </div>
        )}
      </For>
      <div class="buttons mt-2">
        <For each={config()?.providers ?? []}>
          {(provider) => (
            <Show
              when={
                provider.enabled &&
                !(identities() ?? []).some((identity) => identity.provider === provider.id)
              }
            >
              <a class="button is-small" href={`/api/me/identities/${provider.id}/link`} rel="external">
                Link {PROVIDER_LABELS[provider.id] ?? provider.id}
              </a>
            </Show>
          )}
        </For>
      </div>
      <Show when={(identities() ?? []).length === 0 && (config()?.providers ?? []).length === 0}>
        <p class="has-text-grey is-size-7">
          No external providers are configured on this instance.
        </p>
      </Show>
    </>
  );
}

function PrivacySection() {
  const [error, setError] = createSignal('');
  const [notice, setNotice] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  // The tab edits a draft copy of the privacy settings and writes nothing until
  // Save is pressed, so a half-made choice is never committed.
  const [profileVisibility, setProfileVisibility] = createSignal('everyone');
  const [activitiesVisibility, setActivitiesVisibility] = createSignal('followers');
  const [followPolicy, setFollowPolicy] = createSignal('everyone');
  const [mentionPolicy, setMentionPolicy] = createSignal('followers');
  const [trimScope, setTrimScope] = createSignal('all');
  const [trimRadiusM, setTrimRadiusM] = createSignal(200);

  // A new server value (a save, or a session reload) reseeds the draft.
  createEffect(() => {
    const user = currentUser();
    if (!user) return;
    setProfileVisibility(user.profileVisibility);
    setActivitiesVisibility(user.activitiesVisibility);
    setFollowPolicy(user.followPolicy);
    setMentionPolicy(user.mentionPolicy);
    setTrimScope(user.trimScope);
    setTrimRadiusM(user.trimRadiusM);
  });

  const dirty = () => {
    const user = currentUser();
    if (!user) return false;
    return (
      user.profileVisibility !== profileVisibility() ||
      user.activitiesVisibility !== activitiesVisibility() ||
      user.followPolicy !== followPolicy() ||
      user.mentionPolicy !== mentionPolicy() ||
      user.trimScope !== trimScope() ||
      user.trimRadiusM !== trimRadiusM()
    );
  };

  const [zones, { refetch: refetchZones }] = createResource(
    () => currentUser()?.id,
    () => api.get<PrivacyZone[]>('/api/me/privacy-zones'),
  );

  async function save() {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      setCurrentUser(
        await api.patch<Me>('/api/me', {
          profileVisibility: profileVisibility(),
          activitiesVisibility: activitiesVisibility(),
          followPolicy: followPolicy(),
          mentionPolicy: mentionPolicy(),
          trimScope: trimScope(),
          trimRadiusM: trimRadiusM(),
        }),
      );
      setNotice('Privacy settings saved.');
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function addZone(event: SubmitEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const data = new FormData(form);
    const label = String(data.get('label') ?? '').trim();
    const lat = Number(data.get('lat'));
    const lon = Number(data.get('lon'));
    const radiusM = Number(data.get('radiusM'));
    if (!label || !Number.isFinite(lat) || !Number.isFinite(lon) || !Number.isFinite(radiusM)) {
      setError('A zone needs a label, a latitude, a longitude and a radius.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await api.post<PrivacyZone>('/api/me/privacy-zones', { label, lat, lon, radiusM });
      form.reset();
      void refetchZones();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function removeZone(id: number) {
    setBusy(true);
    setError('');
    try {
      await api.del<void>(`/api/me/privacy-zones/${id}`);
      void refetchZones();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Show when={error()}>
        <div class="notification is-danger is-outlined">{error()}</div>
      </Show>

      <RadioGroup
        legend="Profile visibility"
        name="profileVisibility"
        value={profileVisibility()}
        options={PROFILE_VISIBILITY}
        onChange={setProfileVisibility}
      />
      <RadioGroup
        legend="Activity visibility"
        name="activitiesVisibility"
        value={activitiesVisibility()}
        options={ACTIVITIES_VISIBILITY}
        onChange={setActivitiesVisibility}
      />
      <RadioGroup
        legend="Who may follow you"
        name="followPolicy"
        value={followPolicy()}
        options={FOLLOW_POLICY}
        onChange={setFollowPolicy}
      />
      <RadioGroup
        legend="Who may mention you"
        name="mentionPolicy"
        value={mentionPolicy()}
        options={MENTION_POLICY}
        onChange={setMentionPolicy}
      />

      <hr />
      <h2 class="title is-6">Route privacy</h2>
      <RadioGroup
        legend="Trimming"
        name="trimScope"
        value={trimScope()}
        options={TRIM_SCOPE}
        onChange={setTrimScope}
      />

      <Show when={trimScope() === 'all'}>
        <div class="field">
          <label class="label" for="trimRadiusM">
            Hide radius (metres)
          </label>
          <div class="control">
            <input
              id="trimRadiusM"
              class="input"
              type="number"
              min="0"
              max="5000"
              value={trimRadiusM()}
              onChange={(event) => {
                const value = event.currentTarget.valueAsNumber;
                if (Number.isFinite(value)) setTrimRadiusM(value);
              }}
            />
          </div>
          <p class="help">Between 0 and 5000 metres; the default is 200.</p>
        </div>
      </Show>

      <Show when={trimScope() === 'zones'}>
        <h3 class="title is-6">Privacy zones</h3>
        <For each={zones() ?? []}>
          {(zone) => (
            <div class="is-flex is-align-items-center mb-2">
              <span class="has-text-weight-semibold mr-3">{zone.label}</span>
              <span class="has-text-grey is-size-7 hyl-mono mr-3">
                {zone.lat.toFixed(4)}, {zone.lon.toFixed(4)} · {zone.radiusM} m
              </span>
              <button
                type="button"
                class="button is-small"
                disabled={busy()}
                onClick={() => void removeZone(zone.id)}
              >
                Delete
              </button>
            </div>
          )}
        </For>
        <Show when={(zones() ?? []).length === 0}>
          <p class="has-text-grey is-size-7 mb-3">No privacy zones yet.</p>
        </Show>

        <form class="box" onSubmit={(event) => void addZone(event)}>
          <div class="field">
            <label class="label" for="zone-label">
              Label
            </label>
            <div class="control">
              <input id="zone-label" name="label" class="input" required />
            </div>
          </div>
          <div class="field is-grouped">
            <div class="control is-expanded">
              <label class="label" for="zone-lat">
                Latitude
              </label>
              <input
                id="zone-lat"
                name="lat"
                class="input"
                type="number"
                step="any"
                required
              />
            </div>
            <div class="control is-expanded">
              <label class="label" for="zone-lon">
                Longitude
              </label>
              <input
                id="zone-lon"
                name="lon"
                class="input"
                type="number"
                step="any"
                required
              />
            </div>
            <div class="control is-expanded">
              <label class="label" for="zone-radius">
                Radius (m)
              </label>
              <input
                id="zone-radius"
                name="radiusM"
                class="input"
                type="number"
                min="1"
                required
              />
            </div>
          </div>
          <button class="button is-primary" type="submit" disabled={busy()}>
            Add zone
          </button>
        </form>
      </Show>

      <Show when={notice()}>
        <p class="help is-success">{notice()}</p>
      </Show>

      <div class="buttons mt-4">
        <button
          type="button"
          class="button is-primary"
          disabled={busy() || !dirty()}
          onClick={() => void save()}
        >
          Save privacy settings
        </button>
      </div>
    </>
  );
}

/** ConnectionsSection configures the import providers, the import rules and
 * the outbound sync. */
function ConnectionsSection() {
  const [error, setError] = createSignal('');
  const [notice, setNotice] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [copy, setCopy] = createSignal<'idle' | 'done'>('idle');

  const [data, { refetch }] = createResource(
    () => currentUser()?.id,
    () => api.get<ConnectionsResponse>('/api/connections'),
  );

  const connection = (kind: string): Connection | undefined =>
    data()?.connections.find((item) => item.kind === kind);

  async function run(action: () => Promise<unknown>, success: string) {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await action();
      setNotice(success);
      await refetch();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function connectIntervalsKey(event: SubmitEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const formData = new FormData(form);
    const apiKey = String(formData.get('apiKey') ?? '').trim();
    const athleteId = String(formData.get('athleteId') ?? '').trim();
    await run(
      () => api.post<Connection>('/api/connections/intervals/apikey', { apiKey, athleteId }),
      'intervals.icu connected.',
    );
    form.reset();
  }

  async function setRule(rule: ImportRule, enabled: boolean) {
    const rules = (data()?.importRules ?? []).map((row) =>
      row.connectionKind === rule.connectionKind && row.sport === rule.sport
        ? { ...row, enabled }
        : row,
    );
    await run(() => api.put<ImportRule[]>('/api/import-rules', { rules }), 'Import rules saved.');
  }

  async function updateConnection(kind: string, values: Record<string, unknown>) {
    await run(() => api.patch<Connection>(`/api/connections/${kind}`, values), 'Connection updated.');
  }

  async function disconnect(kind: string) {
    await run(() => api.del<void>(`/api/connections/${kind}`), 'Connection removed.');
  }

  async function syncNow() {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api.post<{ status: string }>('/api/sync/run');
      setNotice('Sync queued — the worker imports within a few seconds.');
      // The worker is asynchronous, so poll once after it has had time to run.
      window.setTimeout(() => void refetch(), 2500);
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  // Import rules arrive as one row per connection and sport; the table below
  // groups them so an operator sees a matrix rather than a flat list.
  const sportRows = (kind: string): ImportRule[] =>
    (data()?.importRules ?? []).filter((rule) => rule.connectionKind === kind);

  return (
    <>
      <Show when={error()}>
        <div class="notification is-danger is-outlined">{error()}</div>
      </Show>
      <Show when={notice()}>
        <div class="notification is-success is-light">{notice()}</div>
      </Show>

      <h2 class="title is-6">intervals.icu</h2>
      <Show
        when={connection('intervals_apikey') || connection('intervals_oauth')}
        fallback={
          <Show
            when={config()?.intervalsOAuth}
            fallback={
              <p class="content is-size-7">
                Paste your personal intervals.icu API key, which intervals.icu shows under Settings, Developer.
              </p>
            }
          >
            <p class="content is-size-7">
              Connect with an API key, or authorise hyl through intervals.icu.
            </p>
          </Show>
        }
      >
        <div class="box">
          <p>
            Connected as {connection('intervals_apikey')?.athleteId ?? connection('intervals_oauth')?.athleteId ?? '—'}
          </p>
          <Show when={connection('intervals_oauth')?.reauthorize || connection('intervals_apikey')?.reauthorize}>
            <div class="notification is-warning is-light">
              intervals.icu refused the stored credential. Reconnect to keep importing.
            </div>
          </Show>
          <Show when={connection('intervals_oauth') || connection('intervals_apikey')}>
            {(conn) => (
              <>
                <Show when={conn().lastError && conn().lastError !== 'reauthorize'}>
                  <p class="help is-danger">Last error: {conn().lastError}</p>
                </Show>
                <Show when={conn().lastSuccessAt}>
                  <p class="help">
                    Last successful sync {relativeTime(conn().lastSuccessAt)} (
                    {absoluteTime(conn().lastSuccessAt)}).
                  </p>
                </Show>
              </>
            )}
          </Show>
          <div class="buttons">
            <button class="button is-small" type="button" disabled={busy()} onClick={() => void syncNow()}>
              Sync now
            </button>
            <Show when={config()?.intervalsOAuth}>
              <a class="button is-small" href="/api/connections/intervals/oauth/start" rel="external">
                Reconnect
              </a>
            </Show>
            <button
              class="button is-small is-danger is-outlined"
              type="button"
              disabled={busy()}
              onClick={() =>
                void disconnect(connection('intervals_apikey') ? 'intervals_apikey' : 'intervals_oauth')
              }
            >
              Disconnect
            </button>
          </div>

          <table class="table is-fullwidth is-size-7">
            <thead>
              <tr>
                <th>Sport</th>
                <th>Import</th>
              </tr>
            </thead>
            <tbody>
              <For each={sportRows(connection('intervals_apikey') ? 'intervals_apikey' : 'intervals_oauth')}>
                {(rule) => (
                  <tr>
                    <td>{rule.sport === 'all' ? 'All sports' : sportLabel(rule.sport)}</td>
                    <td>
                      <input
                        type="checkbox"
                        checked={rule.enabled}
                        disabled={busy()}
                        onChange={(event) => void setRule(rule, event.currentTarget.checked)}
                      />
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={!(connection('intervals_apikey') || connection('intervals_oauth'))}>
        <form onSubmit={(event) => void connectIntervalsKey(event)}>
          <div class="field">
            <label class="label is-small" for="intervals-key">
              API key
            </label>
            <div class="control">
              <input id="intervals-key" name="apiKey" class="input is-small" required />
            </div>
          </div>
          <div class="field">
            <label class="label is-small" for="intervals-athlete">
              Athlete id
            </label>
            <div class="control">
              <input id="intervals-athlete" name="athleteId" class="input is-small" placeholder="0 (your own account)" />
            </div>
          </div>
          <div class="buttons">
            <button class="button is-small is-primary" type="submit" disabled={busy()}>
              Connect intervals.icu
            </button>
            <Show when={config()?.intervalsOAuth}>
              <a class="button is-small" href="/api/connections/intervals/oauth/start" rel="external">
                Connect with OAuth
              </a>
            </Show>
          </div>
        </form>
      </Show>

      <hr />
      <h2 class="title is-6">Strava</h2>
      <Show
        when={config()?.strava}
        fallback={
          <p class="content is-size-7">
            Strava export is not configured on this instance (HYL_STRAVA_CLIENT_ID and
            HYL_STRAVA_CLIENT_SECRET are unset).
          </p>
        }
      >
        <Show
          when={connection('strava_oauth')}
          fallback={
            <a class="button is-small" href="/api/connections/strava/oauth/start" rel="external">
              Connect Strava
            </a>
          }
        >
          {(conn) => (
            <div class="box">
              <p>
                Connected as athlete {conn().athleteId ?? '—'}.
                <Show when={conn().lastSuccessAt}>
                  {' '}
                  Last success {relativeTime(conn().lastSuccessAt)}.
                </Show>
              </p>
              <Show when={conn().reauthorize}>
                <div class="notification is-warning is-light">
                  Strava refused the stored token. Reconnect to keep exporting.
                </div>
              </Show>
              <Show when={conn().lastError && conn().lastError !== 'reauthorize'}>
                <p class="help is-danger">Last error: {conn().lastError}</p>
              </Show>
              <div class="field">
                <label class="checkbox">
                  <input
                    type="checkbox"
                    class="switch"
                    checked={conn().autoExport}
                    disabled={busy()}
                    onChange={(event) =>
                      void updateConnection(conn().kind, { autoExport: event.currentTarget.checked })
                    }
                  />{' '}
                  Send new activities to Strava automatically
                </label>
                <p class="help">
                  Covers activities you upload here and any imported from intervals.icu.
                </p>
              </div>
              <div class="field">
                <label class="label is-small" for="export-message">
                  Strava description
                </label>
                <div class="control">
                  <input
                    id="export-message"
                    class="input is-small"
                    value={conn().exportMessage}
                    onChange={(event) =>
                      void updateConnection(conn().kind, { exportMessage: event.currentTarget.value })
                    }
                  />
                </div>
              </div>
              <div class="buttons">
                <a class="button is-small" href="/api/connections/strava/oauth/start" rel="external">
                  Reconnect
                </a>
                <button
                  class="button is-small is-danger is-outlined"
                  type="button"
                  disabled={busy()}
                  onClick={() => void disconnect('strava_oauth')}
                >
                  Disconnect
                </button>
              </div>
            </div>
          )}
        </Show>
      </Show>

      <hr />
      <h2 class="title is-6">Recent sync runs</h2>
      <Show
        when={(data()?.lastRuns ?? []).length > 0}
        fallback={<p class="content is-size-7">No sync has run yet.</p>}
      >
        <div class="table-container">
          <table class="table is-fullwidth is-size-7">
            <thead>
              <tr>
                <th>Started</th>
                <th>Connection</th>
                <th>Imported</th>
                <th>Skipped</th>
                <th>Result</th>
              </tr>
            </thead>
            <tbody>
              <For each={data()?.lastRuns ?? []}>
                {(run) => (
                  <tr>
                    <td>{relativeTime(run.startedAt)}</td>
                    <td>{CONNECTION_LABELS[run.connectionKind] ?? run.connectionKind}</td>
                    <td>{run.imported}</td>
                    <td>{run.skipped}</td>
                    <td>
                      <Show when={run.error} fallback="ok">
                        <span class="has-text-danger">{run.error}</span>
                      </Show>
                    </td>
                  </tr>
                )}
              </For>
            </tbody>
          </table>
        </div>
      </Show>
    </>
  );
}

function DeveloperSection() {
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [created, setCreated] = createSignal<APIKeyCreated | null>(null);
  const [copied, setCopied] = createSignal(false);

  const [keys, { refetch: refetchKeys }] = createResource(
    () => currentUser()?.id,
    () => api.get<APIKey[]>('/api/me/api-keys'),
  );

  async function createKey(event: SubmitEvent) {
    event.preventDefault();
    const form = event.currentTarget as HTMLFormElement;
    const name = String(new FormData(form).get('name') ?? '').trim();
    if (!name) return;
    setBusy(true);
    setError('');
    try {
      setCreated(await api.post<APIKeyCreated>('/api/me/api-keys', { name }));
      setCopied(false);
      form.reset();
      void refetchKeys();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function revokeKey(id: number) {
    setBusy(true);
    setError('');
    try {
      await api.del<void>(`/api/me/api-keys/${id}`);
      void refetchKeys();
    } catch (err) {
      setError(message(err));
    } finally {
      setBusy(false);
    }
  }

  async function copyKey(key: string) {
    try {
      await navigator.clipboard.writeText(key);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <>
      <Show when={error()}>
        <div class="notification is-danger is-outlined">{error()}</div>
      </Show>

      <Show when={created()}>
        {(value) => (
          <div class="notification is-success is-light">
            <div class="is-flex is-align-items-center is-justify-content-space-between">
              <p class="mb-0">Copy this key now — it is shown exactly once.</p>
              <button
                type="button"
                class="delete"
                aria-label="dismiss"
                onClick={() => setCreated(null)}
              />
            </div>
            <div class="is-flex is-align-items-center mt-2">
              <code class="hyl-mono" style="word-break:break-all">
                {value().key}
              </code>
              <button
                type="button"
                class="button is-small ml-3"
                onClick={() => void copyKey(value().key)}
              >
                {copied() ? 'Copied' : 'Copy'}
              </button>
            </div>
          </div>
        )}
      </Show>

      <div class="table-container">
        <table class="table is-fullwidth">
          <thead>
            <tr>
              <th>Name</th>
              <th class="is-hidden-mobile">Prefix</th>
              <th>Created</th>
              <th>Last used</th>
              <th />
            </tr>
          </thead>
          <tbody>
            <For each={keys() ?? []}>
              {(key) => (
                <tr>
                  <td>{key.name}</td>
                  <td class="hyl-mono is-hidden-mobile">{key.prefix}</td>
                  <td>{dateOnly(key.createdAt)}</td>
                  <td>{dateOnly(key.lastUsedAt)}</td>
                  <td class="has-text-right">
                    <button
                      type="button"
                      class="button is-small"
                      disabled={busy()}
                      onClick={() => void revokeKey(key.id)}
                    >
                      Revoke
                    </button>
                  </td>
                </tr>
              )}
            </For>
            <Show when={(keys() ?? []).length === 0}>
              <tr>
                <td colSpan={5} class="has-text-grey">
                  No API keys yet.
                </td>
              </tr>
            </Show>
          </tbody>
        </table>
      </div>

      <form onSubmit={(event) => void createKey(event)}>
        <div class="field is-grouped">
          <div class="control is-expanded">
            <input
              name="name"
              class="input"
              placeholder="Key name"
              aria-label="Key name"
              required
            />
          </div>
          <div class="control">
            <button class="button is-primary" type="submit" disabled={busy()}>
              Create key
            </button>
          </div>
        </div>
      </form>
      <p class="help">
        Keys authenticate the public developer API with <code>Authorization: Bearer …</code>.
      </p>
    </>
  );
}

/** SettingsPage groups account, privacy and developer settings under tabs. */
export default function SettingsPage() {
  const [search] = useSearchParams<{ tab?: string }>();
  const tab = () => search.tab ?? 'account';

  return (
    <section class="section">
      <div class="container" style="max-width:48rem">
        <h1 class="title is-4">Settings</h1>

        <div class="tabs is-toggle">
          <ul>
            <For each={TABS}>
              {(item) => (
                <li class={tab() === item.key ? 'is-active' : ''}>
                  <a href={`/settings?tab=${item.key}`}>{item.label}</a>
                </li>
              )}
            </For>
          </ul>
        </div>

        <Show when={tab() === 'account'}>
          <AccountSection />
        </Show>
        <Show when={tab() === 'privacy'}>
          <PrivacySection />
        </Show>
        <Show when={tab() === 'connections'}>
          <ConnectionsSection />
        </Show>
        <Show when={tab() === 'developer'}>
          <DeveloperSection />
        </Show>

      </div>
    </section>
  );
}
