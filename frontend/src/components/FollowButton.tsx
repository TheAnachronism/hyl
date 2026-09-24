import { Show, createEffect, createSignal } from 'solid-js';
import type { FollowResult } from '../api.gen';
import { ApiError, api } from '../lib/api';

/**
 * FollowButton renders the one control that matches the current relationship
 * and forwards the FollowResult to onChange so a profile header can refresh its
 * follower count. It never renders for your own profile.
 */
export default function FollowButton(props: {
  username: string;
  followState: string;
  onChange?: (state: string, followerCount: number) => void;
}) {
  const [state, setState] = createSignal(props.followState);
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');

  // The parent owns the relationship; mirror it whenever the prop changes.
  createEffect(() => setState(props.followState));

  async function run() {
    const base = `/api/users/${encodeURIComponent(props.username)}`;
    setBusy(true);
    setError('');
    try {
      let result: FollowResult | undefined;
      switch (state()) {
        case 'following':
          result = await api.del<FollowResult>(`${base}/follow`);
          break;
        case 'pending_outgoing':
          // Declining or withdrawing answers 204 without a body.
          result = await api.del<FollowResult>(`${base}/follow/request`);
          break;
        case 'pending_incoming':
          result = await api.post<FollowResult>(`${base}/follow/accept`);
          break;
        default:
          result = await api.post<FollowResult>(`${base}/follow`);
      }
      const next = result?.followState ?? 'none';
      setState(next);
      props.onChange?.(next, result?.followerCount ?? 0);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'the request failed');
    } finally {
      setBusy(false);
    }
  }

  const LABELS: Record<string, string> = {
    following: 'Following',
    pending_outgoing: 'Requested',
    pending_incoming: 'Accept',
  };

  return (
    <Show when={state() !== 'self'}>
      <div class="has-text-right">
        <button
          type="button"
          class={`button is-small ${state() === 'none' || state() === 'pending_incoming' ? 'is-primary' : ''}`}
          disabled={busy()}
          onClick={() => void run()}
        >
          {busy() ? 'Working…' : (LABELS[state()] ?? 'Follow')}
        </button>
        <Show when={error()}>
          <p class="help is-danger">{error()}</p>
        </Show>
      </div>
    </Show>
  );
}
