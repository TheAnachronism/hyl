import { useNavigate } from '@solidjs/router';
import { Show, createEffect, createSignal, onCleanup } from 'solid-js';
import { TbOutlineBell, TbOutlineDeviceDesktop, TbOutlineMoon, TbOutlineSun } from 'solid-icons/tb';
import { currentUser, signOut } from '../lib/session';
import { cycleTheme, themeChoice } from '../lib/theme';
import Avatar from './Avatar';

export interface AccountMenuProps {
  /** variant picks the trigger: the navbar avatar, or a bottom-bar tab. */
  variant?: 'avatar' | 'tab';
  /** unread is the notification count shown next to Alerts. */
  unread?: number;
}

const THEME_LABELS: Record<string, string> = {
  system: 'system',
  light: 'light',
  dark: 'dark',
};

/**
 * AccountMenu is the athlete's own menu: profile, alerts, settings, theme and
 * signing out. It is the only navigation that is the same on every screen size,
 * which is why the theme switch lives in here rather than in a bar.
 */
export default function AccountMenu(props: AccountMenuProps) {
  const navigate = useNavigate();
  const [open, setOpen] = createSignal(false);
  let container!: HTMLDivElement;

  // A click anywhere else, or Escape, closes the menu.
  createEffect(() => {
    if (!open()) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!container.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    onCleanup(() => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    });
  });

  async function logOut(): Promise<void> {
    setOpen(false);
    await signOut();
    navigate('/login');
  }

  const name = () => currentUser()?.displayName || currentUser()?.username || currentUser()?.email;

  return (
    <div
      class="dropdown hyl-account"
      classList={{ 'is-active': open(), 'hyl-account-tab': props.variant === 'tab' }}
      ref={container}
    >
      <div class="dropdown-trigger">
        <Show
          when={props.variant === 'tab'}
          fallback={
            <button
              type="button"
              class="hyl-nav-profile"
              aria-haspopup="true"
              aria-expanded={open() ? 'true' : 'false'}
              aria-controls="hyl-account-menu"
              aria-label="Your account"
              onClick={() => setOpen(!open())}
            >
              <Avatar src={currentUser()?.avatarUrl} name={name()} />
            </button>
          }
        >
          <button
            type="button"
            class="hyl-bottom-tab"
            aria-haspopup="true"
            aria-expanded={open() ? 'true' : 'false'}
            aria-controls="hyl-account-menu"
            aria-label="Your account"
            onClick={() => setOpen(!open())}
          >
            <span class="hyl-bottom-icon">
              <Avatar src={currentUser()?.avatarUrl} name={name()} size={1.5} />
            </span>
            <span class="hyl-bottom-label" aria-hidden="true" />
          </button>
        </Show>
      </div>

      <div class="dropdown-menu" id="hyl-account-menu" role="menu">
        <div class="dropdown-content">
          <Show when={currentUser()}>
            {(user) => (
              <span class="dropdown-item hyl-account-name">
                <span class="has-text-weight-semibold">{user().displayName || user().username}</span>
                <span class="has-text-grey is-size-7">@{user().username}</span>
              </span>
            )}
          </Show>
          <hr class="dropdown-divider" />
          <a class="dropdown-item" role="menuitem" href={`/u/${currentUser()?.username}`} onClick={() => setOpen(false)}>
            Profile
          </a>
          <a class="dropdown-item" role="menuitem" href="/notifications" onClick={() => setOpen(false)}>
            <span class="is-flex is-align-items-center is-justify-content-space-between">
              <span>
                <span class="icon is-small mr-2">
                  <TbOutlineBell size={16} />
                </span>
                Alerts
              </span>
              <Show when={(props.unread ?? 0) > 0}>
                <span class="tag is-danger is-rounded is-small">{props.unread}</span>
              </Show>
            </span>
          </a>
          <a class="dropdown-item" role="menuitem" href="/settings" onClick={() => setOpen(false)}>
            Settings
          </a>
          <hr class="dropdown-divider" />
          <button
            type="button"
            class="dropdown-item"
            role="menuitem"
            onClick={() => cycleTheme()}
            title={`Theme: ${THEME_LABELS[themeChoice()] ?? 'system'} — click to change`}
          >
            <span class="is-flex is-align-items-center is-justify-content-space-between">
              <span>
                <span class="icon is-small mr-2">
                  <Show when={themeChoice() === 'dark'} fallback={<TbOutlineSun size={16} />}>
                    <TbOutlineMoon size={16} />
                  </Show>
                </span>
                Theme
              </span>
              <span class="has-text-grey is-size-7">{THEME_LABELS[themeChoice()] ?? 'system'}</span>
            </span>
          </button>
          <hr class="dropdown-divider" />
          <button type="button" class="dropdown-item" role="menuitem" onClick={() => void logOut()}>
            Log out
          </button>
        </div>
      </div>
    </div>
  );
}
