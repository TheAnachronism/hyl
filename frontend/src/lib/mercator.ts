/**
 * Web Mercator projection helpers shared by the static SVG track previews.
 * Everything here is pure — no DOM access — so the maths can be exercised
 * directly.
 */

const DEG = 180 / Math.PI;

/** ROTATION_GAIN is how much bigger the drawing must get to justify a quarter turn. */
const ROTATION_GAIN = 1.15;

/** EPSILON stands in for a missing extent (unit-square units). */
const EPSILON = 1e-12;

/** MAX_LAT is the latitude where Web Mercator reaches the top/bottom of the unit square. */
const MAX_LAT = 85.05112877980659;

/** project maps lon/lat degrees onto the unit square (x,y ∈ 0..1), clamping latitude. */
function project(lon: number, lat: number): [number, number] {
  const clamped = Math.max(-MAX_LAT, Math.min(MAX_LAT, lat)) / DEG;
  const y = (1 - Math.log(Math.tan(clamped) + 1 / Math.cos(clamped)) / Math.PI) / 2;
  return [(lon + 180) / 360, y];
}

/**
 * projectRoute builds a projector for a flattened [lat,lon,…] list.
 *
 * The whole route always fits — nothing is cropped — and it is centred in the
 * box. A route that is much taller than it is wide would fit into a landscape
 * frame as a thin sliver, so it is rotated a quarter turn whenever that fills
 * the frame better; that is the only case where the rotation applies, and a
 * route that already suits the frame is left alone.
 *
 * Coordinates are north-up: mercator y grows southwards, which is also how SVG
 * grows, so no axis is inverted.
 */
export function projectRoute(
  points: number[],
  width: number,
  height: number,
  padding: number,
): (lat: number, lon: number) => [number, number] {
  let minX = Infinity;
  let maxX = -Infinity;
  let minY = Infinity;
  let maxY = -Infinity;

  for (let i = 0; i + 1 < points.length; i += 2) {
    const lat = points[i];
    const lon = points[i + 1];
    if (lat === undefined || lon === undefined) continue;
    const [x, y] = project(lon, lat);
    if (x < minX) minX = x;
    if (x > maxX) maxX = x;
    if (y < minY) minY = y;
    if (y > maxY) maxY = y;
  }

  // No drawable coordinate at all: park everything in the middle of the box.
  if (!Number.isFinite(minX)) return () => [width / 2, height / 2];

  // A route with no extent at all (a single place, repeated) has nothing to
  // scale; keep it in the middle rather than letting it drift to a corner.
  if (maxX - minX < EPSILON && maxY - minY < EPSILON) return () => [width / 2, height / 2];

  // A route may be perfectly straight along one axis. The epsilon keeps such an
  // axis from constraining the scale: it contributes no extent, so the other
  // axis decides how big the drawing gets.
  const spanX = Math.max(maxX - minX, EPSILON);
  const spanY = Math.max(maxY - minY, EPSILON);
  const usableWidth = Math.max(1, width - 2 * padding);
  const usableHeight = Math.max(1, height - 2 * padding);

  const scaleDirect = Math.min(usableWidth / spanX, usableHeight / spanY);
  const scaleRotated = Math.min(usableWidth / spanY, usableHeight / spanX);
  // Rotate only when the quarter turn buys a materially bigger drawing: a
  // near-square route fits either way, and turning it sideways would just make
  // a familiar shape harder to recognise.
  const rotate = scaleRotated > scaleDirect * ROTATION_GAIN;

  const scale = rotate ? scaleRotated : scaleDirect;
  const drawnWidth = scale * (rotate ? spanY : spanX);
  const drawnHeight = scale * (rotate ? spanX : spanY);
  const offsetX = (width - drawnWidth) / 2;
  const offsetY = (height - drawnHeight) / 2;

  return (lat, lon) => {
    const [x, y] = project(lon, lat);
    let u = x - minX; // 0..spanX, eastwards
    let v = y - minY; // 0..spanY, southwards
    if (rotate) {
      // A quarter turn keeps the shape intact: (u,v) → (spanY - v, u).
      const rotatedU = spanY - v;
      const rotatedV = u;
      u = rotatedU;
      v = rotatedV;
    }
    return [offsetX + scale * u, offsetY + scale * v];
  };
}
