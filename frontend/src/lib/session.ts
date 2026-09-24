import { createSignal } from 'solid-js';
import type { ConfigResponse, Me } from '../api.gen';
import { ApiError, api } from './api';

/** currentUser is the signed-in user; null while signed out. */
export const [currentUser, setCurrentUser] = createSignal<Me | null>(null);

/** sessionLoaded flips once the initial GET /api/me has settled. */
export const [sessionLoaded, setSessionLoaded] = createSignal(false);

/** config is the public boot configuration from GET /api/config. */
export const [config, setConfig] = createSignal<ConfigResponse | null>(null);

let sessionPromise: Promise<Me | null> | null = null;
let configPromise: Promise<ConfigResponse> | null = null;

/**
 * loadSession resolves the current session exactly once; concurrent callers
 * share the same request. Pass force to re-read after a login or logout. A
 * failed request drops the memo, so one transient error does not leave every
 * later call sharing the same rejected promise.
 */
export function loadSession(force = false): Promise<Me | null> {
  if (sessionPromise && !force) return sessionPromise;
  const promise = api
    .get<Me>('/api/me')
    .then((me) => {
      setCurrentUser(me);
      return me;
    })
    .catch((err: unknown) => {
      if (err instanceof ApiError && err.isUnauthorized) {
        setCurrentUser(null);
        return null;
      }
      throw err;
    })
    .finally(() => setSessionLoaded(true));
  sessionPromise = promise;
  promise.catch(() => {
    if (sessionPromise === promise) sessionPromise = null;
  });
  return promise;
}

/** loadConfig fetches the public boot configuration once. */
export function loadConfig(force = false): Promise<ConfigResponse> {
  if (configPromise && !force) return configPromise;
  const promise = api.get<ConfigResponse>('/api/config').then((value) => {
    setConfig(value);
    return value;
  });
  configPromise = promise;
  promise.catch(() => {
    if (configPromise === promise) configPromise = null;
  });
  return promise;
}

/** providerEnabled reports whether an OAuth provider is configured. */
export function providerEnabled(id: string): boolean {
  return config()?.providers.some((provider) => provider.id === id && provider.enabled) ?? false;
}

/** clearSession forgets the cached user after a logout. */
export function clearSession(): void {
  setCurrentUser(null);
  sessionPromise = null;
  setSessionLoaded(true);
}

/**
 * signOut ends the session on the server and then forgets it locally. A failed
 * request still clears the client state — the cookie is what grants access, and
 * the caller redirects either way.
 */
export async function signOut(): Promise<void> {
  try {
    await api.post<void>('/api/auth/logout');
  } catch (err) {
    if (!(err instanceof ApiError)) throw err;
  }
  clearSession();
}
