import { For, Show, createSignal } from 'solid-js';
import type { Photo } from '../api.gen';

export interface PhotoGridProps {
  photos: Photo[];
}

/** PhotoGrid shows square thumbnails that open the full image in a dialog. */
export default function PhotoGrid(props: PhotoGridProps) {
  let dialog!: HTMLDialogElement;
  const [active, setActive] = createSignal<Photo | null>(null);

  function open(photo: Photo): void {
    setActive(photo);
    dialog?.showModal();
  }

  function close(): void {
    dialog?.close();
  }

  return (
    <Show when={props.photos.length > 0}>
      <div class="columns is-mobile is-multiline mt-3">
        <For each={props.photos}>
          {(photo) => (
            <div class="column is-one-quarter p-1">
              <button type="button" class="button is-ghost p-0" style="height:auto" onClick={() => open(photo)}>
                <img class="hyl-photo-thumb" src={photo.thumbUrl} alt="" loading="lazy" />
              </button>
            </div>
          )}
        </For>
      </div>
      <dialog ref={dialog} style="padding:0;border:0;background:transparent;max-width:92vw;max-height:92vh">
        <Show when={active()}>
          {(photo) => (
            <img
              src={photo().url}
              alt=""
              style="max-width:92vw;max-height:86vh;display:block"
              onClick={() => close()}
            />
          )}
        </Show>
        <div class="has-text-centered mt-2">
          <button type="button" class="button is-small" onClick={() => close()}>
            Close
          </button>
        </div>
      </dialog>
    </Show>
  );
}
