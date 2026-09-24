import { useNavigate } from '@solidjs/router';
import { For, Show, createSignal } from 'solid-js';
import type { ActivityDetail } from '../api.gen';
import { ApiError, api } from '../lib/api';
import { SPORTS } from '../lib/sports';

/** UploadPage posts one FIT/GPX file and navigates to the created activity. */
export default function UploadPage() {
  const navigate = useNavigate();
  const [file, setFile] = createSignal<File | null>(null);
  const [title, setTitle] = createSignal('');
  const [description, setDescription] = createSignal('');
  const [sport, setSport] = createSignal('');
  const [busy, setBusy] = createSignal(false);
  const [error, setError] = createSignal('');
  const [duplicateId, setDuplicateId] = createSignal<number | null>(null);

  async function submit(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const selected = file();
    if (!selected || busy()) return;
    setBusy(true);
    setError('');
    setDuplicateId(null);
    const form = new FormData();
    form.append('file', selected);
    if (title().trim() !== '') form.append('title', title().trim());
    if (description().trim() !== '') form.append('description', description().trim());
    if (sport() !== '') form.append('sport', sport());
    try {
      // POST /api/activities answers 201 with the full ActivityDetail DTO.
      const created = await api.postForm<ActivityDetail>('/api/activities', form);
      navigate(`/activities/${created.id}`);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'duplicate_activity') {
        setError(err.message);
        setDuplicateId(err.payload?.error.activityId ?? null);
      } else {
        setError(err instanceof ApiError ? err.message : 'Could not upload the activity.');
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <section class="section">
      <div class="container" style="max-width:36rem">
        <h1 class="title is-4">Upload an activity</h1>
        <form onSubmit={(event) => void submit(event)}>
          <div class="field">
            <label class="label" for="hyl-upload-file">
              Activity file
            </label>
            <div class="control">
              <input
                id="hyl-upload-file"
                class="input"
                type="file"
                accept=".fit,.gpx,.fit.gz,.gpx.gz"
                disabled={busy()}
                onChange={(event) => setFile(event.currentTarget.files?.[0] ?? null)}
              />
            </div>
            <Show when={file()}>
              {(selected) => <p class="help">{selected().name}</p>}
            </Show>
          </div>

          <div class="field">
            <label class="label" for="hyl-upload-title">
              Title
            </label>
            <div class="control">
              <input
                id="hyl-upload-title"
                class="input"
                type="text"
                maxlength="120"
                placeholder="Optional — defaults to the sport and date"
                value={title()}
                disabled={busy()}
                onInput={(event) => setTitle(event.currentTarget.value)}
              />
            </div>
          </div>

          <div class="field">
            <label class="label" for="hyl-upload-description">
              Description
            </label>
            <div class="control">
              <textarea
                id="hyl-upload-description"
                class="textarea"
                rows="3"
                value={description()}
                disabled={busy()}
                onInput={(event) => setDescription(event.currentTarget.value)}
              />
            </div>
          </div>

          <div class="field">
            <label class="label" for="hyl-upload-sport">
              Sport
            </label>
            <div class="control">
              <div class="select">
                <select
                  id="hyl-upload-sport"
                  value={sport()}
                  disabled={busy()}
                  onChange={(event) => setSport(event.currentTarget.value)}
                >
                  <option value="">From the file</option>
                  <For each={SPORTS}>{(item) => <option value={item.key}>{item.label}</option>}</For>
                </select>
              </div>
            </div>
          </div>

          <Show when={error()}>
            <div class="notification is-danger is-outlined">
              <p>{error()}</p>
              <Show when={duplicateId() !== null}>
                <p class="mt-2">
                  <a href={`/activities/${duplicateId() ?? ''}`}>Open the activity that already exists</a>
                </p>
              </Show>
            </div>
          </Show>

          <div class="field">
            <div class="control">
              <button type="submit" class="button is-primary" disabled={busy() || file() === null}>
                {busy() ? 'Uploading…' : 'Upload'}
              </button>
            </div>
          </div>
        </form>
      </div>
    </section>
  );
}
