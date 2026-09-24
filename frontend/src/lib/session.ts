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
 * share the same request. Pass force to re-read after a login or logout.
 */
export function loadSession(force = false): Promise<Me | null> {
  if (sessionPromise && !force) return sessionPromise;
  sessionPromise = api
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
  return sessionPromise;
}

/** loadConfig fetches the public boot configuration once. */
export function loadConfig(force = false): Promise<ConfigResponse> {
  if (configPromise && !force) return configPromise;
  configPromise = api.get<ConfigResponse>('/api/config').then((value) => {
    setConfig(value);
    return value;
  });
  return configPromise;
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
