import { Show, createSignal } from 'solid-js';
import type { Me } from '../api.gen';
import { ApiError, api } from '../lib/api';
import { currentUser, setCurrentUser } from '../lib/session';
import Avatar from './Avatar';

/** AvatarUpload replaces or removes the signed-in user's avatar. */
export default function AvatarUpload() {
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');
  const [notice, setNotice] = createSignal('');

  async function upload(input: HTMLInputElement) {
    const file = input.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.append('file', file);
    setBusy(true);
    setError('');
    setNotice('');
    try {
      setCurrentUser(await api.postForm<Me>('/api/me/avatar', form));
      input.value = '';
      setNotice('Avatar updated.');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'the upload failed');
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    setError('');
    setNotice('');
    try {
      await api.del<void>('/api/me/avatar');
      setCurrentUser(await api.get<Me>('/api/me'));
      setNotice('Avatar removed.');
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'the request failed');
    } finally {
      setBusy(false);
    }
  }

  return (
    <div class="field">
      <label class="label">Avatar</label>
      <div class="is-flex is-align-items-center">
        <Avatar
          src={currentUser()?.avatarUrl}
          name={currentUser()?.displayName || currentUser()?.username || currentUser()?.email}
          size={3}
        />
        <div class="ml-4">
          <div class="file is-small">
            <label class="file-label">
              <input
                class="file-input"
                type="file"
                accept="image/*"
                disabled={busy()}
                onChange={(event) => void upload(event.currentTarget)}
              />
              <span class="file-cta">
                <span class="file-label">{busy() ? 'Uploading…' : 'Choose image'}</span>
              </span>
            </label>
          </div>
          <Show when={currentUser()?.avatarUrl}>
            <button
              type="button"
              class="button is-small mt-2"
              disabled={busy()}
              onClick={() => void remove()}
            >
              Remove avatar
            </button>
          </Show>
        </div>
      </div>
      <Show when={error()}>
        <p class="help is-danger">{error()}</p>
      </Show>
      <Show when={notice()}>
        <p class="help is-success">{notice()}</p>
      </Show>
    </div>
  );
}
