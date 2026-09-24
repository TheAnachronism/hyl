import { useLocation } from '@solidjs/router';
import { Show, createEffect, createResource, createSignal } from 'solid-js';
import { TbOutlineMountain, TbOutlineUpload } from 'solid-icons/tb';
import AccountMenu from './AccountMenu';
import UserSearch from './UserSearch';
import { api } from '../lib/api';
import { currentUser, loadConfig, providerEnabled } from '../lib/session';

/**
 * NavBar renders the one navigation a screen gets: the top navbar when the
 * window is wide enough for it, and the bottom bar otherwise. They are never
 * both on screen.
 *
 * The brand mark is the feed link in both bars — there is no "Feed" item beside
 * it — and everything that is not a place to go (alerts, theme, signing out)
 * lives in the account menu, so both bars carry the same four things: home,
 * upload, search and the account.
 */
export default function NavBar() {
  const location = useLocation();
  const [resending, setResending] = createSignal(false);
  const [resent, setResent] = createSignal(false);

  // The bottom bar is fixed, so the page has to reserve room for it (the rule
  // itself only applies where the bar exists).
  createEffect(() => {
    document.body.classList.toggle('has-bottom-bar', Boolean(currentUser()));
  });

  void loadConfig();

  // Keyed on the current path so navigating anywhere refreshes the badge.
  const [unread] = createResource(
    () => (currentUser() ? location.pathname : null),
    async () => {
      try {
        const result = await api.get<{ unread: number }>('/api/notifications/count');
        return result.unread;
      } catch {
        return 0;
      }
    },
    { initialValue: 0 },
  );

  const home = () => location.pathname === '/';
  const active = (href: string) => (href === '/' ? home() : location.pathname.startsWith(href));

  async function resendVerification() {
    setResending(true);
    try {
      await api.post<void>('/api/auth/verify/resend');
      setResent(true);
    } catch {
      setResent(false);
    } finally {
      setResending(false);
    }
  }

  return (
    <>
      {/* Wide windows: the top navbar. Hidden as soon as the bottom bar takes
          over, which is what keeps the two from ever appearing together. */}
      <nav class="navbar hyl-navbar" role="navigation" aria-label="main navigation">
        <div class="navbar-brand">
          <a
            class="navbar-item hyl-brand has-text-weight-bold is-inline-flex is-align-items-center"
            classList={{ 'hyl-nav-current': home() }}
            href="/"
            aria-label="hyl — your feed"
          >
            <TbOutlineMountain size={20} class="mr-1" />
            hyl
          </a>
        </div>

        <div class="navbar-menu">
          <div class="navbar-start">
            <Show when={currentUser()}>
              <a class="navbar-item" classList={{ 'hyl-nav-current': active('/upload') }} href="/upload">
                Upload
              </a>
            </Show>
          </div>
          <div class="navbar-end">
            <Show
              when={currentUser()}
              fallback={
                <div class="navbar-item">
                  <div class="buttons">
                    <a class="button" href="/register">
                      Sign up
                    </a>
                    <a class="button is-primary" href="/login">
                      Log in
                    </a>
                  </div>
                </div>
              }
            >
              <div class="navbar-item">
                <UserSearch />
              </div>
              <div class="navbar-item hyl-nav-account">
                <AccountMenu unread={unread()} />
              </div>
            </Show>
          </div>
        </div>
      </nav>

      <Show when={currentUser()}>
        {/* Narrow windows: the bottom bar, carrying the same four things. */}
        <nav class="hyl-bottom-bar" aria-label="main navigation">
          <div class="hyl-bottom-row">
            <a
              href="/"
              class="hyl-bottom-tab"
              classList={{ 'is-current': home() }}
              aria-current={home() ? 'page' : undefined}
              aria-label="hyl"
            >
              <span class="hyl-bottom-icon">
                <TbOutlineMountain size={22} />
              </span>
              <span class="hyl-bottom-label">hyl</span>
            </a>
            <a
              href="/upload"
              class="hyl-bottom-tab"
              classList={{ 'is-current': active('/upload') }}
              aria-current={active('/upload') ? 'page' : undefined}
            >
              <span class="hyl-bottom-icon">
                <TbOutlineUpload size={22} />
              </span>
              <span class="hyl-bottom-label">Upload</span>
            </a>
            <UserSearch variant="tab" />
            <AccountMenu variant="tab" unread={unread()} />
          </div>
        </nav>
      </Show>

      <Show when={currentUser() && !currentUser()?.emailVerified && !resent()}>
        <div class="notification is-warning is-light mb-0" style="border-radius:0">
          <div class="container is-flex is-align-items-center is-justify-content-space-between">
            <span>
              Your email address is not verified yet. Some providers and password recovery need it.
            </span>
            <button
              type="button"
              class="button is-small ml-3"
              disabled={resending()}
              onClick={() => void resendVerification()}
            >
              {resending() ? 'Sending…' : 'Resend mail'}
            </button>
          </div>
        </div>
      </Show>

      <Show when={currentUser() && resent()}>
        <div class="notification is-success is-light mb-0" style="border-radius:0">
          <div class="container">Verification mail sent. Check your inbox.</div>
        </div>
      </Show>
    </>
  );
}
