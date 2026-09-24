import { useSearchParams } from '@solidjs/router';
import { Show, createSignal, onMount } from 'solid-js';
import { ApiError, api } from '../lib/api';
import { currentUser } from '../lib/session';

/** VerifyPage consumes the ?token= link from the verification mail. */
export default function VerifyPage() {
  const [params] = useSearchParams<{ token?: string }>();
  const [state, setState] = createSignal<'working' | 'done' | 'error'>('working');
  const [message, setMessage] = createSignal('');
  const [resent, setResent] = createSignal(false);

  onMount(() => {
    const token = params.token;
    if (!token) {
      setState('error');
      setMessage('This link carries no token. Open the link from your verification mail.');
      return;
    }
    void api
      .post<void>('/api/auth/verify', { token })
      .then(() => {
        setState('done');
      })
      .catch((err: unknown) => {
        setState('error');
        setMessage(err instanceof ApiError ? err.message : 'verification failed');
      });
  });

  async function resend() {
    try {
      await api.post<void>('/api/auth/verify/resend');
      setResent(true);
    } catch (err) {
      setMessage(err instanceof ApiError ? err.message : 'sending the mail failed');
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:30rem">
        <h1 class="title is-4">Email verification</h1>

        <Show when={state() === 'working'}>
          <p>Checking your link…</p>
        </Show>

        <Show when={state() === 'done'}>
          <div class="notification is-success is-light">
            Your email address is verified.
          </div>
          <a class="button is-primary" href="/">
            Go to your feed
          </a>
        </Show>

        <Show when={state() === 'error'}>
          <div class="notification is-danger is-outlined">{message()}</div>
          <Show when={currentUser()}>
            <button class="button" type="button" disabled={resent()} onClick={() => void resend()}>
              {resent() ? 'Mail sent' : 'Send me a new link'}
            </button>
          </Show>
          <Show when={!currentUser()}>
            <a class="button" href="/login">
              Log in to request a new link
            </a>
          </Show>
        </Show>
      </div>
    </section>
  );
}
