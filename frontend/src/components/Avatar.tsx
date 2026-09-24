import { Show } from 'solid-js';

export interface AvatarProps {
  /** src is the uploaded picture; empty renders the initial instead. */
  src?: string | null;
  /** name decides the fallback mark: display name, username or email. */
  name?: string | null;
  /** size is the diameter in rem. */
  size?: number;
  /** class carries the caller's spacing utilities. */
  class?: string;
}

/** initial is the fallback mark: the first letter of whatever identifies the
 * athlete, or a dot when nothing does. */
function initial(name: string | null | undefined): string {
  const trimmed = (name ?? '').trim();
  return trimmed === '' ? '·' : trimmed.slice(0, 1).toUpperCase();
}

/**
 * Avatar renders an athlete's picture, and always something: an athlete who has
 * not uploaded one gets the same circle with their initial, so a row of people
 * never collapses into a ragged line of holes.
 */
export default function Avatar(props: AvatarProps) {
  const size = () => `${props.size ?? 2}rem`;

  return (
    <Show
      when={props.src}
      fallback={
        <span
          class={`hyl-avatar hyl-avatar-initial ${props.class ?? ''}`}
          style={{ width: size(), height: size(), 'font-size': `calc(${size()} * 0.45)` }}
          aria-hidden="true"
        >
          {initial(props.name)}
        </span>
      }
    >
      <img
        class={`hyl-avatar ${props.class ?? ''}`}
        src={props.src ?? ''}
        alt=""
        loading="lazy"
        style={{ width: size(), height: size() }}
      />
    </Show>
  );
}
