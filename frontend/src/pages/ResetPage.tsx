import { useSearchParams } from '@solidjs/router';
import { Show, createSignal } from 'solid-js';
import { ApiError, api } from '../lib/api';

/** ResetPage requests a reset mail and consumes the ?token= link. */
export default function ResetPage() {
  const [params] = useSearchParams<{ token?: string }>();
  const [email, setEmail] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [sent, setSent] = createSignal(false);
  const [done, setDone] = createSignal(false);
  const [busy, setBusy] = createSignal(false);

  async function requestReset(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api.post<void>('/api/auth/password/reset-request', { email: email().trim() });
      setSent(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'the request failed');
    } finally {
      setBusy(false);
    }
  }

  async function applyReset(event: SubmitEvent) {
    event.preventDefault();
    if (password().length < 10) {
      setError('Password must be at least 10 characters.');
      return;
    }
    setBusy(true);
    setError('');
    try {
      await api.post<void>('/api/auth/password/reset', {
        token: params.token,
        password: password(),
      });
      setDone(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'the reset failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:26rem">
        <h1 class="title is-4">Reset your password</h1>

        <Show when={error()}>
          <div class="notification is-danger is-outlined">{error()}</div>
        </Show>

        <Show when={done()}>
          <div class="notification is-success is-light">
            Your password has been changed. Every other session was signed out.
          </div>
          <a class="button is-primary" href="/login">
            Log in
          </a>
        </Show>

        <Show when={!done() && params.token}>
          <form onSubmit={(event) => void applyReset(event)}>
            <div class="field">
              <label class="label" for="password">
                New password
              </label>
              <div class="control">
                <input
                  id="password"
                  class="input"
                  type="password"
                  autocomplete="new-password"
                  required
                  value={password()}
                  onInput={(event) => setPassword(event.currentTarget.value)}
                />
              </div>
              <p class="help">At least 10 characters.</p>
            </div>
            <button class="button is-primary is-fullwidth" type="submit" disabled={busy()}>
              Set new password
            </button>
          </form>
        </Show>

        <Show when={!done() && !params.token}>
          <Show
            when={!sent()}
            fallback={
              <div class="notification is-info is-light">
                If an account exists for that address, a reset link is on its way. The link is valid
                for one hour.
              </div>
            }
          >
            <form onSubmit={(event) => void requestReset(event)}>
              <div class="field">
                <label class="label" for="email">
                  Email
                </label>
                <div class="control">
                  <input
                    id="email"
                    class="input"
                    type="email"
                    autocomplete="email"
                    required
                    value={email()}
                    onInput={(event) => setEmail(event.currentTarget.value)}
                  />
                </div>
              </div>
              <button class="button is-primary is-fullwidth" type="submit" disabled={busy()}>
                Send reset link
              </button>
            </form>
          </Show>
        </Show>

        <p class="is-size-7 has-text-grey mt-4">
          <a href="/login">Back to log in</a>
        </p>
      </div>
    </section>
  );
}
