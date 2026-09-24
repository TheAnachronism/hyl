import { For, Show } from 'solid-js';
import type { UserRef } from '../api.gen';
import Avatar from './Avatar';

/** UserList renders a titled list of compact user references. */
export default function UserList(props: { title: string; items: UserRef[]; empty: string }) {
  return (
    <div class="box">
      <h2 class="title is-6">{props.title}</h2>
      <Show when={props.items.length > 0} fallback={<p class="has-text-grey">{props.empty}</p>}>
        <ul>
          <For each={props.items}>
            {(user) => (
              <li class="is-flex is-align-items-center py-2">
                <Avatar src={user.avatarUrl} name={user.displayName || user.username} />
                <a class="ml-3" href={`/u/${user.username}`}>
                  {user.displayName || user.username}
                </a>
              </li>
            )}
          </For>
        </ul>
      </Show>
    </div>
  );
}
