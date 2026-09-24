import { useSearchParams } from '@solidjs/router';
import { For, Show, createEffect, createSignal } from 'solid-js';
import type { ActivityPage, ActivitySummary } from '../api.gen';
import ActivityEntry from '../components/ActivityEntry';
import Pager from '../components/Pager';
import { ApiError, api, query } from '../lib/api';

const PAGE_SIZE = 20;

/**
 * FeedPage is the home feed: your own activities and those of everyone you
 * follow, newest first. There is no "just me" filter — one feed shows
 * everything the viewer is allowed to see.
 */
export default function FeedPage() {
  const [search] = useSearchParams();
  const page = () => {
    const raw = Array.isArray(search.page) ? search.page[0] : search.page;
    const value = Number(raw);
    return Number.isFinite(value) && value >= 1 ? Math.floor(value) : 1;
  };

  const [items, setItems] = createSignal<ActivitySummary[]>([]);
  const [totalPages, setTotalPages] = createSignal(0);
  const [total, setTotal] = createSignal(0);
  const [current, setCurrent] = createSignal(1);
  const [loading, setLoading] = createSignal(true);
  const [error, setError] = createSignal('');

  async function load(): Promise<void> {
    setLoading(true);
    setError('');
    try {
      const result = await api.get<ActivityPage>(
        `/api/activities${query({ feed: 'following', limit: PAGE_SIZE, page: page() })}`,
      );
      setItems(result.items);
      setCurrent(result.page);
      setTotalPages(result.totalPages);
      setTotal(result.total);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not load the feed.');
    } finally {
      setLoading(false);
    }
  }

  createEffect(() => {
    page();
    void load();
  });

  // Page one is the bare feed, so the common case keeps a clean URL.
  const hrefFor = (target: number) => query({ page: target === 1 ? undefined : target });

  return (
    <section class="section">
      <div class="container" style="max-width:42rem">
        <Show when={error()}>
          <div class="notification is-danger is-outlined">{error()}</div>
        </Show>

        <Show when={!loading() && !error() && items().length === 0}>
          <div class="notification is-light">
            <p>
              Nothing here yet — <a href="/upload">upload an activity</a> or follow other athletes.
            </p>
          </div>
        </Show>

        <For each={items()}>
          {(activity) => <ActivityEntry activity={activity} />}
        </For>

        <Show when={loading()}>
          <p class="has-text-grey">Loading…</p>
        </Show>

        <Pager page={current()} totalPages={totalPages()} total={total()} hrefFor={hrefFor} />
      </div>
    </section>
  );
}
