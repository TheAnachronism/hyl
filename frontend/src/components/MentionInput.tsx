import { For, Show, createSignal, onCleanup } from 'solid-js';
import type { UserRef } from '../api.gen';
import Avatar from './Avatar';
import { api, query } from '../lib/api';

const DEBOUNCE_MS = 150;
const MAX_SUGGESTIONS = 8;
// Mirrors internal/social/mentions.go: an @ handle at the start of the text or
// after any non-word, non-@ character.
const TRAILING_MENTION = /(^|[^\w@])@([A-Za-z0-9_]*)$/;

export interface MentionInputProps {
  value: string;
  onInput: (value: string) => void;
  placeholder?: string;
  rows?: number;
  disabled?: boolean;
}

/** MentionInput is a textarea with an @-handle autocomplete dropdown. */
export default function MentionInput(props: MentionInputProps) {
  let textarea!: HTMLTextAreaElement;
  let timer: number | undefined;
  let requestSeq = 0;

  const [suggestions, setSuggestions] = createSignal<UserRef[]>([]);
  const [open, setOpen] = createSignal(false);
  const [active, setActive] = createSignal(0);

  onCleanup(() => {
    if (timer !== undefined) window.clearTimeout(timer);
  });

  const target = () => {
    const caret = textarea.selectionStart ?? props.value.length;
    const match = TRAILING_MENTION.exec(props.value.slice(0, caret));
    if (!match) return null;
    const term = match[2] ?? '';
    return { start: caret - term.length - 1, term };
  };

  const close = () => {
    if (timer !== undefined) window.clearTimeout(timer);
    setOpen(false);
    setSuggestions([]);
  };

  function schedule(): void {
    if (timer !== undefined) window.clearTimeout(timer);
    const current = target();
    if (!current) {
      close();
      return;
    }
    const term = current.term;
    timer = window.setTimeout(() => {
      const seq = (requestSeq += 1);
      api
        .get<UserRef[]>(`/api/users/search${query({ q: term })}`)
        .then((users) => {
          if (seq !== requestSeq) return;
          setSuggestions(users.slice(0, MAX_SUGGESTIONS));
          setActive(0);
          setOpen(users.length > 0);
        })
        .catch((err: unknown) => {
          if (seq !== requestSeq) return;
          console.error('mention search failed', err);
          close();
        });
    }, DEBOUNCE_MS);
  }

  function choose(user: UserRef): void {
    const current = target();
    if (!current) {
      close();
      return;
    }
    const caret = textarea.selectionStart ?? props.value.length;
    const next = `${props.value.slice(0, current.start)}@${user.username} ${props.value.slice(caret)}`;
    const position = current.start + user.username.length + 2;
    props.onInput(next);
    close();
    queueMicrotask(() => {
      textarea.focus();
      textarea.setSelectionRange(position, position);
    });
  }

  function onKeyDown(event: KeyboardEvent): void {
    const list = suggestions();
    if (!open() || list.length === 0) return;
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      setActive((index) => (index + 1) % list.length);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      setActive((index) => (index - 1 + list.length) % list.length);
    } else if (event.key === 'Enter') {
      const user = list[active()];
      if (user) {
        event.preventDefault();
        choose(user);
      }
    } else if (event.key === 'Escape') {
      close();
    }
  }

  return (
    <div style="position:relative">
      <textarea
        ref={textarea}
        class="textarea"
        rows={props.rows ?? 2}
        placeholder={props.placeholder ?? 'Add a comment…'}
        disabled={props.disabled ?? false}
        value={props.value}
        onInput={(event) => {
          props.onInput(event.currentTarget.value);
          schedule();
        }}
        onKeyDown={onKeyDown}
        onKeyUp={(event) => {
          if (event.key.startsWith('Arrow') || event.key === 'Enter' || event.key === 'Escape') return;
          schedule();
        }}
        onClick={schedule}
        onBlur={() => window.setTimeout(close, DEBOUNCE_MS)}
      />
      <Show when={open()}>
        <div class="hyl-suggestions box p-0">
          <For each={suggestions()}>
            {(user, index) => (
              <button
                type="button"
                class="button is-fullwidth is-justify-content-flex-start px-3 py-1"
                classList={{ 'is-link': index() === active() }}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(user)}
              >
                <Avatar src={user.avatarUrl} name={user.displayName || user.username} class="mr-2" />
                <span class="has-text-weight-semibold">{user.username}</span>
                <Show when={user.displayName}>
                  <span class="has-text-grey ml-2">{user.displayName}</span>
                </Show>
              </button>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
