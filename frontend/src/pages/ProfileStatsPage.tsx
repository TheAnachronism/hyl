import { useLocation, useParams, useSearchParams } from '@solidjs/router';
import { For, Show, createMemo, createResource } from 'solid-js';
import { TbOutlineChevronLeft } from 'solid-icons/tb';
import type { StatsResponse, UserProfile } from '../api.gen';
import { ApiError, api, query } from '../lib/api';
import { count, distance, duration, elevation } from '../lib/format';
import { sportLabel } from '../lib/sports';

const RANGES = [
  { key: 'month', label: 'This month' },
  { key: 'year', label: 'This year' },
  { key: 'all', label: 'All time' },
];

/** The YYYY-MM-DD form the stats endpoint expects, from local date parts. */
function isoDate(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${date.getFullYear()}-${month}-${day}`;
}

/** rangeBounds maps a range key onto the inclusive ?from=&to= window. */
function rangeBounds(key: string): { from?: string; to?: string } {
  const now = new Date();
  if (key === 'month') {
    return { from: isoDate(new Date(now.getFullYear(), now.getMonth(), 1)), to: isoDate(now) };
  }
  if (key === 'year') {
    return { from: isoDate(new Date(now.getFullYear(), 0, 1)), to: isoDate(now) };
  }
  return {};
}

/** ProfileStatsPage sums one athlete's activities by sport over a date range. */
export default function ProfileStatsPage() {
  const params = useParams<{ username: string }>();
  const location = useLocation();
  const [search] = useSearchParams<{ sport?: string; from?: string; to?: string }>();

  const [profile] = createResource(
    () => params.username,
    (username) => api.get<UserProfile>(`/api/users/${encodeURIComponent(username)}`),
  );

  const [stats] = createResource(
    () => {
      const user = profile();
      if (!user || user.isPrivate) return null;
      return { username: params.username, from: search.from, to: search.to };
    },
    (key) =>
      api.get<StatsResponse>(
        `/api/users/${encodeURIComponent(key.username)}/stats${query({
          from: key.from,
          to: key.to,
        })}`,
      ),
  );

  // Only sports the athlete actually has in the selected range get a tab, and a
  // ?sport= that the range no longer covers falls back to the full table
  // instead of showing an empty one.
  const sports = () => stats()?.sports ?? [];
  const selectedSport = () => {
    const wanted = search.sport;
    return wanted && sports().some((row) => row.sport === wanted) ? wanted : undefined;
  };
  const rows = () => {
    const wanted = selectedSport();
    return wanted ? sports().filter((row) => row.sport === wanted) : sports();
  };

  const selectedRange = createMemo(() => {
    if (!search.from) return 'all';
    if (search.from === rangeBounds('month').from) return 'month';
    if (search.from === rangeBounds('year').from) return 'year';
    return 'all';
  });

  const statsHref = (sport: string | undefined, range: string) => {
    const bounds = rangeBounds(range);
    return location.pathname + query({ sport, from: bounds.from, to: bounds.to });
  };

  return (
    <section class="section">
      <div class="container" style="max-width:48rem">
        <Show when={profile.error}>
          <div class="notification is-danger is-outlined">
            {profile.error instanceof ApiError ? profile.error.message : 'could not load the profile'}
          </div>
        </Show>

        <Show when={profile()}>
          {(user) => (
            <>
              <h1 class="title is-4">
                {user().displayName || user().username} · Statistics
              </h1>
              <p class="has-text-grey is-size-7 mb-4">
                <a class="is-inline-flex is-align-items-center" href={`/u/${user().username}`}>
                <TbOutlineChevronLeft size={16} class="mr-1" />
                Back to @{user().username}
              </a>
              </p>

              <Show
                when={!user().isPrivate}
                fallback={
                  <div class="notification is-light">
                    This profile is private. Follow @{user().username} to see their statistics.
                  </div>
                }
              >
                <div class="tabs is-toggle is-small">
                  <ul>
                    <li class={selectedSport() ? '' : 'is-active'}>
                      <a href={statsHref(undefined, selectedRange())}>All</a>
                    </li>
                    <For each={sports()}>
                      {(row) => (
                        <li class={selectedSport() === row.sport ? 'is-active' : ''}>
                          <a href={statsHref(row.sport, selectedRange())}>{sportLabel(row.sport)}</a>
                        </li>
                      )}
                    </For>
                  </ul>
                </div>

                <div class="tabs is-small">
                  <ul>
                    <For each={RANGES}>
                      {(range) => (
                        <li class={selectedRange() === range.key ? 'is-active' : ''}>
                          <a href={statsHref(search.sport, range.key)}>{range.label}</a>
                        </li>
                      )}
                    </For>
                  </ul>
                </div>

                <Show when={stats.error}>
                  <div class="notification is-danger is-outlined">
                    {stats.error instanceof ApiError ? stats.error.message : 'could not load the stats'}
                  </div>
                </Show>

                <Show when={stats()}>
                  {(value) => (
                    <div class="table-container">
                      <table class="table is-fullwidth">
                      <thead>
                        <tr>
                          <th>Sport</th>
                          <th class="has-text-right">Activities</th>
                          <th class="has-text-right">Distance</th>
                          <th class="has-text-right">Moving time</th>
                          <th class="has-text-right is-hidden-mobile">Elapsed time</th>
                          <th class="has-text-right">Elevation</th>
                        </tr>
                      </thead>
                      <tbody>
                        <For each={rows()}>
                          {(row) => (
                            <tr>
                              <td>{sportLabel(row.sport)}</td>
                              <td class="has-text-right hyl-mono">{count(row.count)}</td>
                              <td class="has-text-right hyl-mono">{distance(row.distanceM)}</td>
                              <td class="has-text-right hyl-mono">{duration(row.movingTimeS)}</td>
                              <td class="has-text-right hyl-mono is-hidden-mobile">{duration(row.elapsedTimeS)}</td>
                              <td class="has-text-right hyl-mono">{elevation(row.elevationGainM)}</td>
                            </tr>
                          )}
                        </For>
                        <Show when={rows().length === 0}>
                          <tr>
                            <td colSpan={6} class="has-text-grey">
                              No activities in this range.
                            </td>
                          </tr>
                        </Show>
                      </tbody>
                      <tfoot>
                        <tr>
                          <th>All</th>
                          <th class="has-text-right hyl-mono">{count(value().all.count)}</th>
                          <th class="has-text-right hyl-mono">{distance(value().all.distanceM)}</th>
                          <th class="has-text-right hyl-mono">{duration(value().all.movingTimeS)}</th>
                          <th class="has-text-right hyl-mono is-hidden-mobile">{duration(value().all.elapsedTimeS)}</th>
                          <th class="has-text-right hyl-mono">{elevation(value().all.elevationGainM)}</th>
                        </tr>
                      </tfoot>
                      </table>
                    </div>
                  )}
                </Show>
              </Show>
            </>
          )}
        </Show>
      </div>
    </section>
  );
}
