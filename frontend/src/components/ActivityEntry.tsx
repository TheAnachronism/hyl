import { For, Show, createEffect, createSignal } from 'solid-js';
import type { ActivitySummary } from '../api.gen';
import { absoluteTime, relativeTime } from '../lib/format';
import { currentUser } from '../lib/session';
import { sportLabel } from '../lib/sports';
import Avatar from './Avatar';
import CommentForm from './CommentForm';
import CommentList from './CommentList';
import MetricsGrid from './MetricsGrid';
import PeakButton from './PeakButton';
import SportBadge from './SportBadge';
import TrackPreview from './TrackPreview';

export interface ActivityEntryProps {
  activity: ActivitySummary;
}

/** ActivityEntry is the shared activity card used by the feed and profiles. */
export default function ActivityEntry(props: ActivityEntryProps) {
  const [commentsOpen, setCommentsOpen] = createSignal(false);
  const [reloadKey, setReloadKey] = createSignal(0);
  const [commentCount, setCommentCount] = createSignal(props.activity.commentCount);

  // Re-sync with a freshly fetched page.
  createEffect(() => setCommentCount(props.activity.commentCount));

  // Peaking your own activity is not offered (and the API refuses it).
  const canPeak = () => currentUser()?.id !== props.activity.userId;
  const title = () => props.activity.title || sportLabel(props.activity.sport);

  return (
    <article class="card hyl-activity-card mb-4">
      <a
        class="hyl-card-link"
        href={`/activities/${props.activity.id}`}
        aria-label={`Open ${title()}`}
      ></a>
      <div class="card-content">
        <div class="media is-align-items-center">
          <div class="media-left">
            <a class="hyl-card-above" href={`/u/${props.activity.username}`} aria-label={props.activity.username}>
              <Avatar
                src={props.activity.avatarUrl}
                name={props.activity.displayName || props.activity.username}
              />
            </a>
          </div>
          <div class="media-content">
            <p class="mb-0">
              <a
                class="has-text-weight-semibold hyl-card-above"
                href={`/u/${props.activity.username}`}
              >
                {props.activity.displayName || props.activity.username}
              </a>
            </p>
            <p class="is-size-7 has-text-grey mb-0">
              <span title={absoluteTime(props.activity.startedAt)}>
                {relativeTime(props.activity.startedAt)}
              </span>
            </p>
          </div>
          <div class="media-right">
            <SportBadge sport={props.activity.sport} />
          </div>
        </div>

        <h3 class="title is-6 mt-2 mb-1">{title()}</h3>

        <MetricsGrid activity={props.activity} />

        <TrackPreview
          track={props.activity.track}
          mapAvailable={props.activity.mapAvailable}
          href={`/activities/${props.activity.id}/map`}
        />

        <Show when={props.activity.photos.length > 0}>
          <div class="columns is-mobile is-multiline mt-2">
            <For each={props.activity.photos.slice(0, 3)}>
              {(photo) => (
                <div class="column p-1">
                  <img class="hyl-photo-thumb" src={photo.thumbUrl} alt="" loading="lazy" />
                </div>
              )}
            </For>
          </div>
        </Show>

        <Show when={props.activity.description}>
          <p class="mt-2 hyl-clamp-3 is-size-7">{props.activity.description}</p>
        </Show>

        <div class="hyl-card-actions hyl-card-above">
          <PeakButton
            activityId={props.activity.id}
            likeCount={props.activity.likeCount}
            likedByMe={props.activity.likedByMe}
            canPeak={canPeak()}
          />
          <button
            type="button"
            class="button is-small"
            onClick={() => setCommentsOpen((open) => !open)}
          >
            {commentCount()} {commentCount() === 1 ? 'comment' : 'comments'}
          </button>
        </div>

        <Show when={commentsOpen()}>
          <div class="hyl-card-above">
            <CommentList
              activityId={props.activity.id}
              reloadKey={reloadKey()}
              onDeleted={() => setCommentCount((value) => Math.max(0, value - 1))}
            />
            <CommentForm
              activityId={props.activity.id}
              onCreated={() => {
                setCommentCount((value) => value + 1);
                setReloadKey((value) => value + 1);
              }}
            />
          </div>
        </Show>
      </div>
    </article>
  );
}
