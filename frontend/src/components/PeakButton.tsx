import { Show, createEffect, createSignal } from 'solid-js';
import { TbFillMountain, TbOutlineMountain } from 'solid-icons/tb';
import type { LikeResult } from '../api.gen';
import { api } from '../lib/api';

export interface PeakButtonProps {
  activityId: number;
  /** likeCount and likedByMe keep the API's field names, which are stable. */
  likeCount: number;
  likedByMe: boolean;
  /**
   * canPeak is false on your own activity: the API refuses a self-peak, but the
   * peak count is still shown — it is other people's signal about the activity.
   */
  canPeak: boolean;
}

/** PeakButton records one peak; the API has no un-peak by design. */
export default function PeakButton(props: PeakButtonProps) {
  const [count, setCount] = createSignal(props.likeCount);
  const [peaked, setPeaked] = createSignal(props.likedByMe);
  const [busy, setBusy] = createSignal(false);

  // Re-sync when the parent replaces its list with a fresh response.
  createEffect(() => {
    setCount(props.likeCount);
    setPeaked(props.likedByMe);
  });

  async function peak(): Promise<void> {
    if (peaked() || busy()) return;
    setBusy(true);
    try {
      const result = await api.post<LikeResult>(`/api/activities/${props.activityId}/likes`);
      setCount(result.likeCount);
      setPeaked(result.likedByMe);
    } catch (err) {
      console.error('could not peak activity', err);
    } finally {
      setBusy(false);
    }
  }

  const label = () => (peaked() ? 'Peaked' : 'Peak');
  const icon = () => (peaked() ? <TbFillMountain size={16} /> : <TbOutlineMountain size={16} />);

  return (
    <Show
      when={props.canPeak}
      fallback={
        <span class="button is-small is-static hyl-peak hyl-peak-static" title={`${count()} peaks`}>
          <span class="icon is-small">{icon()}</span>
          <span class="hyl-mono">{count()}</span>
        </span>
      }
    >
      <button
        type="button"
        class="button is-small hyl-peak"
        classList={{ 'is-peaked': peaked() }}
        disabled={peaked() || busy()}
        aria-pressed={peaked() ? 'true' : 'false'}
        aria-label={`${label()} this activity`}
        title={`${label()} this activity`}
        onClick={() => void peak()}
      >
        <span class="icon is-small">{icon()}</span>
        <span>{label()}</span>
        <span class="hyl-mono ml-2">{count()}</span>
      </button>
    </Show>
  );
}
