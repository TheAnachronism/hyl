import type { Feature, LineString } from 'geojson';
import { LngLatBounds, Map as MapLibreMap } from 'maplibre-gl';
import type { StyleSpecification } from 'maplibre-gl';
import { createEffect, onCleanup, onMount } from 'solid-js';
import { basemapStyle, fallbackStyle, routeFeature } from '../lib/maps';
import { config } from '../lib/session';

interface MapViewProps {
  /** route is the flattened [lat,lon,…] list from the activity detail payload. */
  route: number[];
  /** style forces one renderer; without it the presence of tiles decides. */
  style?: 'basemap' | 'fallback';
  theme: 'light' | 'dark';
  /**
   * maxZoom is the archive's own maximum zoom. The style must declare it so
   * MapLibre overzooms instead of requesting tiles the extract does not have.
   */
  maxZoom?: number;
  /** interactive defaults to true; panning and zooming are on. */
  interactive?: boolean;
}

const FIT_OPTIONS = { padding: 32, duration: 0 } as const;

/** routeBounds wraps a flattened coordinate list in a LngLatBounds. */
function routeBounds(route: number[]): LngLatBounds | null {
  const bounds = new LngLatBounds();
  let hasPoint = false;
  for (let i = 0; i + 1 < route.length; i += 2) {
    const lat = route[i];
    const lon = route[i + 1];
    if (lat === undefined || lon === undefined) break;
    bounds.extend([lon, lat]);
    hasPoint = true;
  }
  return hasPoint ? bounds : null;
}

/**
 * MapView renders one activity route over a self-hosted basemap, or over the
 * plain fallback background when the instance has no tiles. The parent supplies
 * the box; the map fills it.
 */
export default function MapView(props: MapViewProps) {
  let container: HTMLDivElement | undefined;
  let map: MapLibreMap | undefined;
  let appliedTheme: 'light' | 'dark' | undefined;
  let appliedMode: 'basemap' | 'fallback' | undefined;
  let appliedMaxZoom: number | undefined;
  let appliedRoute: number[] | undefined;

  const mode = (): 'basemap' | 'fallback' =>
    props.style ?? (config()?.pmtilesUrl ? 'basemap' : 'fallback');

  // The route is part of the style, so every input that shapes the style is
  // read here; the effect below rebuilds it whenever one of them changes.
  const styleFor = (): StyleSpecification => {
    const url = config()?.pmtilesUrl;
    const route = routeFeature(props.route);
    return mode() === 'basemap' && url
      ? basemapStyle(url, props.theme, props.maxZoom ?? 12, route)
      : fallbackStyle(props.theme, route);
  };

  onMount(() => {
    if (!container) return;
    const created = new MapLibreMap({
      container,
      style: styleFor(),
      bounds: routeBounds(props.route) ?? undefined,
      fitBoundsOptions: FIT_OPTIONS,
      interactive: props.interactive ?? true,
      attributionControl: { compact: true },
    });
    map = created;
    appliedTheme = props.theme;
    appliedMode = mode();
    appliedMaxZoom = props.maxZoom;
    appliedRoute = props.route;
    onCleanup(() => {
      map = undefined;
      created.remove();
    });
  });

  // A theme switch, tiles appearing once /api/config lands, the archive's real
  // maximum zoom arriving, or a different activity all replace the style.
  createEffect(() => {
    const nextMode = mode();
    const nextTheme = props.theme;
    const nextMaxZoom = props.maxZoom;
    const nextRoute = props.route;
    const current = map;
    if (!current) return;
    const unchanged =
      nextMode === appliedMode &&
      nextTheme === appliedTheme &&
      nextMaxZoom === appliedMaxZoom &&
      nextRoute === appliedRoute;
    if (unchanged) return;

    appliedMode = nextMode;
    appliedTheme = nextTheme;
    appliedMaxZoom = nextMaxZoom;
    appliedRoute = nextRoute;
    current.setStyle(styleFor(), { diff: false });

    // setStyle keeps the camera, but a new activity needs a new frame.
    const bounds = routeBounds(nextRoute);
    if (bounds) current.fitBounds(bounds, FIT_OPTIONS);
  });

  return <div class="hyl-map-full" ref={container} />;
}
