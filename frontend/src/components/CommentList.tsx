import { For, Show, createEffect, createSignal } from 'solid-js';
import type { Comment, CommentPage } from '../api.gen';
import { ApiError, api, query } from '../lib/api';
import { absoluteTime, relativeTime } from '../lib/format';
import Avatar from './Avatar';

const PAGE_SIZE = 20;
const MENTION_TOKEN = /(@[A-Za-z0-9_]{3,30})/g;

interface BodyPart {
  text: string;
  /** mention is the username when the token is a confirmed mention, else null. */
  mention: string | null;
}

/**
 * bodyParts splits a stored comment body into plain text and confirmed @mention
 * links. Only usernames the API actually recorded in comment.mentions become
 * links — everything else stays text, so no HTML is ever interpreted.
 */
function bodyParts(body: string, mentions: string[]): BodyPart[] {
  const allowed = new Set(mentions.map((name) => name.toLowerCase()));
  const parts: BodyPart[] = [];
  let last = 0;
  for (const match of body.matchAll(MENTION_TOKEN)) {
    const index = match.index ?? 0;
    const token = match[0];
    if (index > last) parts.push({ text: body.slice(last, index), mention: null });
    const name = token.slice(1);
    parts.push({ text: token, mention: allowed.has(name.toLowerCase()) ? name : null });
    last = index + token.length;
  }
  if (last < body.length) parts.push({ text: body.slice(last), mention: null });
  return parts;
}

export interface CommentListProps {
  activityId: number;
  /** reloadKey forces a refetch after the owner posts a new comment. */
  reloadKey?: number;
  /** onDeleted tells the owning card to drop one from its comment count. */
  onDeleted?: () => void;
}

/** CommentList renders one activity's comments with keyset pagination. */
export default function CommentList(props: CommentListProps) {
  const [comments, setComments] = createSignal<Comment[]>([]);
  const [nextBefore, setNextBefore] = createSignal<number | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal('');

  // A reload (a posted comment) can overlap an in-flight page, so each load
  // carries an id and a superseded response is dropped rather than applied.
  let loadId = 0;

  async function load(before: number | null, append: boolean): Promise<void> {
    const id = ++loadId;
    setLoading(true);
    setError('');
    try {
      const page = await api.get<CommentPage>(
        `/api/activities/${props.activityId}/comments${query({ limit: PAGE_SIZE, before })}`,
      );
      if (id !== loadId) return;
      setComments((current) => (append ? [...current, ...page.items] : page.items));
      setNextBefore(page.nextBefore ?? null);
    } catch (err) {
      if (id !== loadId) return;
      setError(err instanceof ApiError ? err.message : 'Could not load comments.');
    } finally {
      if (id === loadId) setLoading(false);
    }
  }

  createEffect(() => {
    props.activityId;
    props.reloadKey;
    void load(null, false);
  });

  async function remove(comment: Comment): Promise<void> {
    setError('');
    try {
      await api.del<void>(`/api/comments/${comment.id}`);
      setComments((current) => current.filter((item) => item.id !== comment.id));
      props.onDeleted?.();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not delete the comment.');
    }
  }

  return (
    <div class="mt-4">
      <Show when={error()}>
        <p class="help is-danger">{error()}</p>
      </Show>
      <Show when={loading() && comments().length === 0}>
        <p class="has-text-grey is-size-7">Loading comments…</p>
      </Show>
      <Show when={!loading() && comments().length === 0}>
        <p class="has-text-grey is-size-7">No comments yet.</p>
      </Show>
      <For each={comments()}>
        {(comment) => (
          <div class="media is-align-items-flex-start">
            <div class="media-left mr-3">
              <a href={`/u/${comment.username}`}>
                <Avatar src={comment.avatarUrl} name={comment.displayName || comment.username} />
              </a>
            </div>
            <div class="media-content">
              <p class="is-size-7 mb-1">
                <a class="has-text-weight-semibold" href={`/u/${comment.username}`}>
                  {comment.displayName || comment.username}
                </a>{' '}
                <span class="has-text-grey" title={absoluteTime(comment.createdAt)}>
                  {relativeTime(comment.createdAt)}
                </span>
              </p>
              <p class="mb-1">
                <For each={bodyParts(comment.body, comment.mentions)}>
                  {(part) => (
                    <Show when={part.mention} fallback={part.text}>
                      <a href={`/u/${part.mention ?? ''}`}>{part.text}</a>
                    </Show>
                  )}
                </For>
              </p>
              <Show when={comment.canDelete}>
                <button
                  type="button"
                  class="button is-small is-ghost has-text-danger px-0"
                  onClick={() => void remove(comment)}
                >
                  Delete
                </button>
              </Show>
            </div>
          </div>
        )}
      </For>
      <Show when={nextBefore() !== null}>
        <div class="has-text-centered mt-2">
          <button
            type="button"
            class="button is-small"
            disabled={loading()}
            onClick={() => void load(nextBefore(), true)}
          >
            {loading() ? 'Loading…' : 'Older comments'}
          </button>
        </div>
      </Show>
    </div>
  );
}
