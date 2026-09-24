import { useLocation, useNavigate, useParams, useSearchParams } from '@solidjs/router';
import { For, Show, createEffect, createResource, createSignal } from 'solid-js';
import { TbOutlineSearch } from 'solid-icons/tb';
import type { ActivityPage, UserProfile } from '../api.gen';
import ActivityEntry from '../components/ActivityEntry';
import Avatar from '../components/Avatar';
import FollowButton from '../components/FollowButton';
import Pager from '../components/Pager';
import { ApiError, api, query } from '../lib/api';
import { count } from '../lib/format';

/** How long typing settles before the list is refetched. */
const SEARCH_DEBOUNCE_MS = 250;

/** ProfilePage renders one athlete's header and their activity list. */
export default function ProfilePage() {
  const params = useParams<{ username: string }>();
  const location = useLocation();
  const navigate = useNavigate();
  const [search] = useSearchParams<{ page?: string; q?: string }>();

  const term = () => (Array.isArray(search.q) ? search.q[0] : search.q) ?? '';
  const page = () => {
    const raw = Array.isArray(search.page) ? search.page[0] : search.page;
    const value = Number(raw);
    return Number.isFinite(value) && value >= 1 ? Math.floor(value) : 1;
  };

  // The field is its own draft: typing is debounced into the URL, while a URL
  // that changes underneath us (back button, a shared link) refills the field.
  const [draft, setDraft] = createSignal(term());
  let navigated = term();
  let timer: number | undefined;

  createEffect(() => {
    const url = term();
    if (url !== navigated) {
      navigated = url;
      setDraft(url);
    }
  });

  /** Typing searches on its own; there is no button to press. */
  function onSearchInput(value: string): void {
    setDraft(value);
    window.clearTimeout(timer);
    const next = value.trim();
    const go = () => {
      navigated = next;
      // replace, so a search does not fill the history with one entry per key.
      navigate(location.pathname + query({ q: next === '' ? undefined : next }), { replace: true });
    };
    if (next === '') {
      go();
      return;
    }
    timer = window.setTimeout(go, SEARCH_DEBOUNCE_MS);
  }

  const [profile, { refetch: refetchProfile }] = createResource(
    () => params.username,
    (username) => api.get<UserProfile>(`/api/users/${encodeURIComponent(username)}`),
  );

  // A private profile has no activities to show, so the list is not requested.
  const [activities, { refetch: refetchActivities }] = createResource(
    () => {
      const user = profile();
      if (!user || user.isPrivate) return null;
      return {
        username: params.username,
        page: page(),
        q: term() === '' ? undefined : term(),
      };
    },
    (key) =>
      api.get<ActivityPage>(
        `/api/users/${encodeURIComponent(key.username)}/activities${query({ page: key.page, q: key.q })}`,
      ),
  );

  const hrefFor = (target: number) =>
    query({ q: term() || undefined, page: target === 1 ? undefined : target });

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
              <div class="box">
                <div class="is-flex is-align-items-center mb-3">
                  <Avatar
                    src={user().avatarUrl}
                    name={user().displayName || user().username}
                    size={4}
                  />
                  <div class="ml-4">
                    <h1 class="title is-4 mb-0">{user().displayName || user().username}</h1>
                    <p class="has-text-grey">@{user().username}</p>
                  </div>
                  <div class="ml-auto">
                    <FollowButton
                      username={user().username}
                      followState={user().followState}
                      onChange={() => void refetchProfile()}
                    />
                  </div>
                </div>

                <Show
                  when={user().isPrivate}
                  fallback={
                    <>
                      <p class="mb-3">{user().bio || 'No bio yet.'}</p>
                      <p class="has-text-grey is-size-7 mb-3">
                        <strong class="has-text-grey">{count(user().followerCount)}</strong> followers ·{' '}
                        <strong class="has-text-grey">{count(user().followingCount)}</strong> following ·{' '}
                        <strong class="has-text-grey">{count(user().activityCount)}</strong> activities
                      </p>
                      <div class="buttons">
                        <a class="button is-small" href={`/u/${user().username}/stats`}>
                          Statistics
                        </a>
                        {/* Settings has no nav tab of its own; the account menu
                            in the navbar reaches it too. */}
                        <Show when={user().isMe}>
                          <a class="button is-small" href="/settings">
                            Settings
                          </a>
                        </Show>
                      </div>
                    </>
                  }
                >
                  <div class="notification is-light">
                    This profile is private. Follow @{user().username} to see their activities.
                  </div>
                </Show>
              </div>

              <Show when={!user().isPrivate}>
                <div class="field">
                  <div class="control has-icons-left">
                    <input
                      class="input"
                      type="search"
                      name="q"
                      placeholder={`Search ${user().username}'s activities`}
                      aria-label={`Search ${user().username}'s activities`}
                      value={draft()}
                      onInput={(event) => onSearchInput(event.currentTarget.value)}
                    />
                    <span class="icon is-small is-left">
                      <TbOutlineSearch size={16} />
                    </span>
                  </div>
                </div>

                <Show when={activities.error}>
                  <div class="notification is-danger is-outlined">
                    {activities.error instanceof ApiError
                      ? activities.error.message
                      : 'could not load the activities'}
                  </div>
                </Show>

                <Show
                  when={activities()?.items.length}
                  fallback={
                    <Show when={activities()}>
                      <div class="notification is-light">
                        <Show
                          when={term() !== ''}
                          fallback={<p>No activities to show yet.</p>}
                        >
                          <p>No activities match “{term()}”.</p>
                        </Show>
                      </div>
                    </Show>
                  }
                >
                  <For each={activities()?.items ?? []}>
                    {(activity) => <ActivityEntry activity={activity} />}
                  </For>
                </Show>

                <Pager
                  page={activities()?.page ?? 1}
                  totalPages={activities()?.totalPages ?? 0}
                  total={activities()?.total ?? 0}
                  hrefFor={hrefFor}
                />
              </Show>
            </>
          )}
        </Show>
      </div>
    </section>
  );
}
