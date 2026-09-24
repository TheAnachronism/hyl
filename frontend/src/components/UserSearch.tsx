import { useNavigate } from '@solidjs/router';
import { For, Show, createResource, createSignal } from 'solid-js';
import { TbOutlineSearch } from 'solid-icons/tb';
import type { UserRef } from '../api.gen';
import Avatar from './Avatar';
import { api, query } from '../lib/api';

/** How long typing settles before the backend is asked. */
const DEBOUNCE_MS = 180;

export interface UserSearchProps {
  /** variant picks the trigger: a navbar icon button, or a bottom-bar tab. */
  variant?: 'icon' | 'tab';
}

/**
 * UserSearch is the athlete search: a modal that asks the backend for users
 * matching what you type, and opens the one you pick.
 */
export default function UserSearch(props: UserSearchProps) {
  let dialog!: HTMLDialogElement;
  let input!: HTMLInputElement;

  const navigate = useNavigate();
  const [term, setTerm] = createSignal('');
  const [settled, setSettled] = createSignal('');
  const [active, setActive] = createSignal(0);
  let timer: number | undefined;

  const [results] = createResource(settled, async (value) => {
    const trimmed = value.trim();
    if (trimmed === '') return [];
    return api.get<UserRef[]>(`/api/users/search${query({ q: trimmed })}`);
  });

  function onInput(value: string): void {
    setTerm(value);
    setActive(0);
    window.clearTimeout(timer);
    timer = window.setTimeout(() => setSettled(value), DEBOUNCE_MS);
  }

  function open(): void {
    setTerm('');
    setSettled('');
    setActive(0);
    dialog.showModal();
    // The dialog is modal, so the field can take focus straight away.
    queueMicrotask(() => input.focus());
  }

  function close(): void {
    dialog.close();
  }

  function go(user: UserRef | undefined): void {
    if (!user) return;
    close();
    navigate(`/u/${user.username}`);
  }

  function onKey(event: KeyboardEvent): void {
    // Escape closes even while the search field has focus, where the native
    // behaviour would only clear the field.
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
      return;
    }
    const list = results() ?? [];
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActive((index) => Math.min(index + 1, list.length - 1));
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActive((index) => Math.max(index - 1, 0));
    } else if (event.key === 'Enter') {
      event.preventDefault();
      go(list[active()]);
    }
  }

  return (
    <>
      <Show
        when={props.variant === 'tab'}
        fallback={
          <button
            type="button"
            class="button is-small"
            aria-label="Search athletes"
            title="Search athletes"
            onClick={open}
          >
            <span class="icon is-small">
              <TbOutlineSearch size={16} />
            </span>
          </button>
        }
      >
        <button
          type="button"
          class="hyl-bottom-tab"
          aria-label="Search athletes"
          title="Search athletes"
          onClick={open}
        >
          <span class="hyl-bottom-icon">
            <TbOutlineSearch size={22} />
          </span>
          <span class="hyl-bottom-label" aria-hidden="true" />
        </button>
      </Show>

      <dialog
        ref={dialog}
        class="hyl-search"
        aria-label="Search athletes"
        onCancel={(event) => {
          event.preventDefault();
          close();
        }}
        onClick={(event) => {
          // A click on the backdrop (not the panel) dismisses the dialog.
          if (event.target === dialog) close();
        }}
      >
        <div class="box mb-0">
          <div class="field">
            <div class="control has-icons-left">
              <input
                ref={input}
                class="input"
                type="search"
                placeholder="Search by username or name"
                aria-label="Search by username or name"
                value={term()}
                onInput={(event) => onInput(event.currentTarget.value)}
                onKeyDown={onKey}
              />
              <span class="icon is-small is-left">
                <TbOutlineSearch size={16} />
              </span>
            </div>
          </div>

          <Show when={results.loading}>
            <p class="has-text-grey is-size-7">Searching…</p>
          </Show>

          <Show when={!results.loading && settled().trim() !== '' && (results() ?? []).length === 0}>
            <p class="has-text-grey is-size-7">No athlete matches “{settled().trim()}”.</p>
          </Show>

          <ul class="hyl-search-results">
            <For each={results() ?? []}>
              {(user, index) => (
                <li>
                  <a
                    href={`/u/${user.username}`}
                    classList={{ 'is-active': index() === active() }}
                    onMouseEnter={() => setActive(index())}
                    onClick={close}
                  >
                    <Avatar src={user.avatarUrl} name={user.displayName || user.username} class="mr-3" />
                    <span>
                      <span class="has-text-weight-semibold">{user.displayName || user.username}</span>
                      <span class="has-text-grey is-size-7 ml-2">@{user.username}</span>
                    </span>
                  </a>
                </li>
              )}
            </For>
          </ul>

          <Show when={settled().trim() === ''}>
            <p class="help">Type a username or a name; Enter opens the first match.</p>
          </Show>
        </div>
      </dialog>
    </>
  );
}
