import { Navigate, useLocation } from '@solidjs/router';
import { Show, type Component, type JSX } from 'solid-js';
import { currentUser, sessionLoaded } from '../lib/session';

/** RequireSession sends anonymous visitors to the login page, remembering where
 * they were headed. */
export default function RequireSession(props: { children: JSX.Element }) {
  const location = useLocation();
  const next = () => encodeURIComponent(location.pathname + location.search);

  return (
    <Show when={sessionLoaded()} fallback={<div class="has-text-centered py-6">Loading…</div>}>
      <Show when={currentUser()} fallback={<Navigate href={`/login?next=${next()}`} />}>
        {props.children}
      </Show>
    </Show>
  );
}

/** withSession guards a page component behind a session. */
export function withSession(Page: Component): Component {
  return () => (
    <RequireSession>
      <Page />
    </RequireSession>
  );
}
