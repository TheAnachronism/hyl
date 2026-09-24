import { useNavigate } from '@solidjs/router';
import { Show, createSignal } from 'solid-js';
import type { Me } from '../api.gen';
import { ApiError, api } from '../lib/api';
import { loadConfig, providerEnabled, setCurrentUser } from '../lib/session';

const USERNAME_PATTERN = /^[a-z0-9_]{3,30}$/;
const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

/** RegisterPage creates a local account. */
export default function RegisterPage() {
  const navigate = useNavigate();
  const [username, setUsername] = createSignal('');
  const [email, setEmail] = createSignal('');
  const [password, setPassword] = createSignal('');
  const [error, setError] = createSignal('');
  const [busy, setBusy] = createSignal(false);

  void loadConfig();

  // The server enforces the same rules; this only saves a round trip.
  function validate(): string {
    if (!USERNAME_PATTERN.test(username().toLowerCase())) {
      return 'Username must be 3-30 characters of a-z, 0-9 or _.';
    }
    if (!EMAIL_PATTERN.test(email().trim())) return 'Enter a valid email address.';
    if (password().length < 10) return 'Password must be at least 10 characters.';
    return '';
  }

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    const problem = validate();
    if (problem) {
      setError(problem);
      return;
    }
    setBusy(true);
    setError('');
    try {
      const me = await api.post<Me>('/api/auth/register', {
        username: username().toLowerCase(),
        email: email().trim(),
        password: password(),
      });
      setCurrentUser(me);
      navigate('/');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'registration failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:26rem">
        <h1 class="title is-4">Create your account</h1>

        <Show when={error()}>
          <div class="notification is-danger is-outlined">{error()}</div>
        </Show>

        <form onSubmit={(event) => void submit(event)}>
          <div class="field">
            <label class="label" for="username">
              Username
            </label>
            <div class="control">
              <input
                id="username"
                class="input"
                autocomplete="username"
                required
                value={username()}
                onInput={(event) => setUsername(event.currentTarget.value)}
              />
            </div>
            <p class="help">Lowercase letters, digits and underscore.</p>
          </div>

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

          <div class="field">
            <label class="label" for="password">
              Password
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

          <div class="field">
            <button class="button is-primary is-fullwidth" type="submit" disabled={busy()}>
              {busy() ? 'Creating…' : 'Create account'}
            </button>
          </div>
        </form>

        <p class="is-size-7 has-text-grey">
          Already registered? <a href="/login">Log in</a>
        </p>

        <Show when={providerEnabled('google') || providerEnabled('github')}>
          <hr />
          <p class="has-text-grey is-size-7 mb-2">or sign up with</p>
          <div class="buttons">
            <Show when={providerEnabled('google')}>
              <a class="button is-fullwidth" href="/auth/google">
                Google
              </a>
            </Show>
            <Show when={providerEnabled('github')}>
              <a class="button is-fullwidth" href="/auth/github">
                GitHub
              </a>
            </Show>
          </div>
        </Show>
      </div>
    </section>
  );
}
