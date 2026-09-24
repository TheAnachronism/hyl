import { Show, createSignal } from 'solid-js';
import type { CommentCreated } from '../api.gen';
import { ApiError, api } from '../lib/api';
import MentionInput from './MentionInput';

export interface CommentFormProps {
  activityId: number;
  onCreated: (created: CommentCreated) => void;
}

/** CommentForm posts one comment and explains any mention the API dropped. */
export default function CommentForm(props: CommentFormProps) {
  const [body, setBody] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');
  const [dropped, setDropped] = createSignal<string[]>([]);

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const text = body().trim();
    if (text === '' || busy()) return;
    setBusy(true);
    setError('');
    try {
      const created = await api.post<CommentCreated>(`/api/activities/${props.activityId}/comments`, {
        body: text,
      });
      setBody('');
      setDropped(created.droppedMentions);
      props.onCreated(created);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not post the comment.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <form class="mt-3" onSubmit={(event) => void submit(event)}>
      <MentionInput value={body()} onInput={setBody} disabled={busy()} />
      <Show when={dropped().length > 0}>
        <div class="notification is-warning is-light py-2 mt-2">
          {dropped().length === 1
            ? `@${dropped()[0] ?? ''} could not be mentioned.`
            : `These people could not be mentioned: ${dropped()
                .map((name) => `@${name}`)
                .join(', ')}.`}{' '}
          They do not accept mentions from everyone.
        </div>
      </Show>
      <Show when={error()}>
        <p class="help is-danger">{error()}</p>
      </Show>
      <div class="mt-2">
        <button type="submit" class="button is-small is-primary" disabled={busy() || body().trim() === ''}>
          {busy() ? 'Posting…' : 'Post'}
        </button>
      </div>
    </form>
  );
}
