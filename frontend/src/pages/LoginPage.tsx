import { useNavigate, useSearchParams } from '@solidjs/router';
import { Show, createSignal } from 'solid-js';
import type { Me } from '../api.gen';
import { ApiError, api } from '../lib/api';
import { loadConfig, providerEnabled, setCurrentUser } from '../lib/session';

const PROVIDERS = [
  { id: 'google', label: 'Google' },
  { id: 'github', label: 'GitHub' },
];

/** LoginPage signs a user in with credentials or an OAuth provider. */
export default function LoginPage() {
  const navigate = useNavigate();
  const [params] = useSearchParams<{ next?: string }>();
  const [login, setLogin] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  void loadConfig();

  const next = () => params.next ?? '/';

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    setBusy(true);
    setError('');
    try {
      const me = await api.post<Me>('/api/auth/login', {
        login: login(),
        password: password(),
      });
      setCurrentUser(me);
      navigate(next());
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'sign-in failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:26rem">
        <h1 class="title is-4">Log in</h1>

        <Show when={error()}>
          <div class="notification is-danger is-outlined">{error()}</div>
        </Show>

        <form onSubmit={(event) => void submit(event)}>
          <div class="field">
            <label class="label" for="login">
              Username or email
            </label>
            <div class="control">
              <input
                id="login"
                class="input"
                autocomplete="username"
                required
                value={login()}
                onInput={(event) => setLogin(event.currentTarget.value)}
              />
            </div>
          </div>

          <div class="field">
            <label class="label" for="password">
              Password
            </label>
            <div class="control">
              <input
                id="password"
                class="input"
                type="password"
                autocomplete="current-password"
                required
                value={password()}
                onInput={(event) => setPassword(event.currentTarget.value)}
              />
            </div>
          </div>

          <div class="field">
            <button class="button is-primary is-fullwidth" type="submit" disabled={busy()}>
              {busy() ? 'Signing in…' : 'Log in'}
            </button>
          </div>
        </form>

        <p class="is-size-7 has-text-grey">
          <a href="/reset">Forgot your password?</a> · <a href="/register">Create an account</a>
        </p>

        <Show when={providerEnabled('google') || providerEnabled('github')}>
          <hr />
          <p class="has-text-grey is-size-7 mb-2">or continue with</p>
          <div class="buttons">
            <Show when={providerEnabled('google')}>
              <a class="button is-fullwidth" href={`/auth/google?next=${encodeURIComponent(next())}`}>
                Google
              </a>
            </Show>
            <Show when={providerEnabled('github')}>
              <a class="button is-fullwidth" href={`/auth/github?next=${encodeURIComponent(next())}`}>
                GitHub
              </a>
            </Show>
          </div>
        </Show>
      </div>
    </section>
  );
}
