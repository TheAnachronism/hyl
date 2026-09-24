import { For, Show, createSignal } from 'solid-js';
import type { Photo } from '../api.gen';
import { ApiError, api } from '../lib/api';

const MAX_PHOTOS = 10;

export interface PhotoUploadProps {
  activityId: number;
  photos: Photo[];
  onChanged: (photos: Photo[]) => void;
}

/** PhotoUpload is the owner-only multiple-file input with per-file delete. */
export default function PhotoUpload(props: PhotoUploadProps) {
  let input!: HTMLInputElement;
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');

  async function upload(files: FileList | null): Promise<void> {
    if (!files || files.length === 0) return;
    setError('');
    if (props.photos.length + files.length > MAX_PHOTOS) {
      setError(`At most ${MAX_PHOTOS} photos per activity — you already have ${props.photos.length}.`);
      if (input) input.value = '';
      return;
    }
    const form = new FormData();
    for (const file of Array.from(files)) form.append('files', file);
    setBusy(true);
    try {
      const photos = await api.postForm<Photo[]>(`/api/activities/${props.activityId}/photos`, form);
      props.onChanged(photos);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not upload the photos.');
    } finally {
      setBusy(false);
      if (input) input.value = '';
    }
  }

  async function remove(photo: Photo): Promise<void> {
    setError('');
    try {
      await api.del<void>(`/api/photos/${photo.id}`);
      props.onChanged(props.photos.filter((item) => item.id !== photo.id));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Could not delete the photo.');
    }
  }

  return (
    <div class="box mt-3">
      <div class="field">
        <label class="label is-small" for="hyl-photo-input">
          Add photos ({props.photos.length}/{MAX_PHOTOS})
        </label>
        <div class="control">
          <input
            id="hyl-photo-input"
            ref={input}
            class="input"
            type="file"
            accept="image/*"
            multiple
            disabled={busy() || props.photos.length >= MAX_PHOTOS}
            onChange={(event) => void upload(event.currentTarget.files)}
          />
        </div>
      </div>
      <Show when={error()}>
        <p class="help is-danger">{error()}</p>
      </Show>
      <Show when={busy()}>
        <p class="help">Uploading…</p>
      </Show>
      <Show when={props.photos.length > 0}>
        <div class="is-flex is-flex-wrap-wrap">
          <For each={props.photos}>
            {(photo) => (
              <div class="mr-2 mb-2 is-flex is-flex-direction-column is-align-items-center">
                <img class="hyl-thumb-sm" src={photo.thumbUrl} alt="" />
                <button
                  type="button"
                  class="button is-small is-danger is-outlined mt-1"
                  onClick={() => void remove(photo)}
                >
                  Remove
                </button>
              </div>
            )}
          </For>
        </div>
      </Show>
    </div>
  );
}
