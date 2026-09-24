import { For, Show, createSignal, onMount } from 'solid-js';
import type { Notification, NotificationPage } from '../api.gen';
import { ApiError, api, query } from '../lib/api';
import Avatar from '../components/Avatar';
import { absoluteTime, relativeTime } from '../lib/format';

const PAGE_SIZE = 20;

const KIND_LABEL: Record<string, string> = {
  follow: 'started following you',
  follow_request: 'asked to follow you',
  follow_accepted: 'accepted your follow request',
  like: 'peaked your activity',
  comment: 'commented on your activity',
  mention: 'mentioned you',
};

function dayLabel(iso: string): string {
  const date = new Date(iso);
  const today = new Date();
  const yesterday = new Date(today.getTime() - 86_400_000);
  if (date.toDateString() === today.toDateString()) return 'Today';
  if (date.toDateString() === yesterday.toDateString()) return 'Yesterday';
  return date.toLocaleDateString(undefined, { weekday: 'long', month: 'short', day: 'numeric' });
}

/** groupByDay keeps the API's newest-first order and splits it into day buckets. */
function groupByDay(items: Notification[]): { label: string; items: Notification[] }[] {
  const groups: { label: string; items: Notification[] }[] = [];
  for (const item of items) {
    const label = dayLabel(item.createdAt);
    const last = groups[groups.length - 1];
    if (last && last.label === label) last.items.push(item);
    else groups.push({ label, items: [item] });
  }
  return groups;
}

/** NotificationsPage lists in-app notifications grouped by day. */
export default function NotificationsPage() {
  const [items, setItems] = createSignal<Notification[]>([]);
  const [nextBefore, setNextBefore] = createSignal<number | null>(null);
  const [loading, setLoading] = createSignal(true);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');

  async function load(before: number | null, append: boolean): Promise<void> {
    setLoading(true);
    setError('');
    try {
      const page = await api.get<NotificationPage>(`/api/notifications${query({ limit: PAGE_SIZE, before })}`);
      setItems((current) => (append ? [...current, ...page.items] : page.items));
      setNextBefore(page.nextBefore ?? null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load notifications.');
    } finally {
      setLoading(false);
    }
  }

  onMount(() => void load(null, false));

  async function markAllRead(): Promise<void> {
    if (busy()) return;
    setBusy(true);
    setError('');
    try {
      await api.post<void>('/api/notifications/read', { ids: [] });
      const now = new Date().toISOString();
      setItems((current) => current.map((item) => (item.readAt ? item : { ...item, readAt: now })));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not mark the notifications as read.');
    } finally {
      setBusy(false);
    }
  }

  const unread = () => items().filter((item) => !item.readAt).length;

  return (
    <section class="section">
      <div class="container" style="max-width:40rem">
        <div class="level is-mobile">
          <div class="level-left">
            <div class="level-item">
              <h1 class="title is-4 mb-0">Notifications</h1>
            </div>
          </div>
          <div class="level-right">
            <div class="level-item">
              <button
                type="button"
                class="button is-small"
                disabled={busy() || unread() === 0}
                onClick={() => void markAllRead()}
              >
                Mark all read
              </button>
            </div>
          </div>
        </div>

        <Show when={error()}>
          <div class="notification is-danger is-outlined">{error()}</div>
        </Show>

        <Show when={!loading() && items().length === 0}>
          <div class="notification is-light">You are all caught up.</div>
        </Show>

        <For each={groupByDay(items())}>
          {(group) => (
            <>
              <h2 class="heading">{group.label}</h2>
              <For each={group.items}>
                {(item) => (
                  <a class="box is-flex is-align-items-center px-3 py-2 mb-2" href={linkFor(item)}>
                    <Avatar
                      src={item.actor.avatarUrl}
                      name={item.actor.displayName || item.actor.username}
                      class="mr-3"
                    />
                    <div class="is-flex-grow-1">
                      <p class="mb-0">
                        <span class="has-text-weight-semibold">{item.actor.displayName || item.actor.username}</span>{' '}
                        {KIND_LABEL[item.kind] ?? item.kind}
                      </p>
                      <p class="is-size-7 has-text-grey mb-0" title={absoluteTime(item.createdAt)}>
                        {relativeTime(item.createdAt)}
                      </p>
                    </div>
                    <Show when={!item.readAt}>
                      <span class="tag is-info ml-2">New</span>
                    </Show>
                  </a>
                )}
              </For>
            </>
          )}
        </For>

        <Show when={loading()}>
          <p class="has-text-grey">Loading…</p>
        </Show>

        <Show when={nextBefore() !== null}>
          <div class="has-text-centered my-4">
            <button
              type="button"
              class="button"
              disabled={loading()}
              onClick={() => void load(nextBefore(), true)}
            >
              Older notifications
            </button>
          </div>
        </Show>
      </div>
    </section>
  );
}

/** linkFor points a notification at the comment's activity or the actor. */
function linkFor(item: Notification): string {
  if (item.activityId != null) return `/activities/${item.activityId}`;
  return `/u/${item.actor.username}`;
}
