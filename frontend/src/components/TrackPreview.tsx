import { Show } from 'solid-js';
import { projectRoute } from '../lib/mercator';

/** The preview is a 16:10 stage, which is what the card's map box reserves. */
const VIEW_WIDTH = 480;
const VIEW_HEIGHT = 300;
const PADDING = 12;
const GRID_STEP = 48;

// <pattern> ids have to be unique per document, so every instance gets its own.
let patternSeq = 0;

export interface TrackPreviewProps {
  /** track is the flattened [lat,lon,…] list exactly as the API sends it. */
  track: number[];
  mapAvailable: boolean;
  href: string;
}

/**
 * TrackPreview draws a route as a static SVG line over a subtle grid.
 *
 * The route always fits the box — never cropped — and is rotated a quarter turn
 * when that suits the 16:10 frame better. When there is no drawable route
 * (indoor activity, hidden route, or trimmed below the display threshold) it
 * renders nothing at all, so the card closes up instead of reserving an empty
 * map box.
 */
export default function TrackPreview(props: TrackPreviewProps) {
  const patternId = `hyl-grid-${(patternSeq += 1)}`;
  const drawable = () => props.mapAvailable && props.track.length >= 4;

  const points = () => {
    const projector = projectRoute(props.track, VIEW_WIDTH, VIEW_HEIGHT, PADDING);
    const out: string[] = [];
    for (let i = 0; i + 1 < props.track.length; i += 2) {
      const lat = props.track[i];
      const lon = props.track[i + 1];
      if (lat === undefined || lon === undefined) continue;
      const [x, y] = projector(lat, lon);
      out.push(`${x.toFixed(1)},${y.toFixed(1)}`);
    }
    return out.join(' ');
  };

  return (
    <Show when={drawable()}>
      <a href={props.href} aria-label="Route map" class="hyl-track-preview">
        <svg
          viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`}
          preserveAspectRatio="xMidYMid meet"
          role="img"
          aria-label="Route"
        >
          <defs>
            <pattern id={patternId} width={GRID_STEP} height={GRID_STEP} patternUnits="userSpaceOnUse">
              <path d={`M ${GRID_STEP} 0 H 0 V ${GRID_STEP}`} class="hyl-grid-line" fill="none" />
            </pattern>
          </defs>
          <rect width={VIEW_WIDTH} height={VIEW_HEIGHT} class="hyl-track-bg" />
          <rect width={VIEW_WIDTH} height={VIEW_HEIGHT} fill={`url(#${patternId})`} />
          <polyline
            class="hyl-track"
            points={points()}
            fill="none"
            stroke-width="2.5"
            stroke-linejoin="round"
            stroke-linecap="round"
            shape-rendering="geometricPrecision"
          />
        </svg>
      </a>
    </Show>
  );
}
