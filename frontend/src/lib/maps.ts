import 'maplibre-gl/dist/maplibre-gl.css';
import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?worker&url';
import { layers, namedFlavor } from '@protomaps/basemaps';
import { addProtocol, setWorkerUrl } from 'maplibre-gl';
import type { LayerSpecification, StyleSpecification } from 'maplibre-gl';
import { PMTiles, Protocol } from 'pmtiles';
import type { Feature, LineString, Position } from 'geojson';

// maplibre-gl v6 is ESM-only and resolves its worker by URL: a bundler has to
// emit the worker as its own module, and every map shares one registration.
// Both calls are process-wide, which is why they live at module scope.
setWorkerUrl(workerUrl);
addProtocol('pmtiles', new Protocol().tile);

/** BASEMAP_ASSETS is the glyph/sprite origin; operators self-host a copy. */
export const BASEMAP_ASSETS = 'https://protomaps.github.io/basemaps-assets';

const ATTRIBUTION =
  '<a href="https://protomaps.com">Protomaps</a> © <a href="https://openstreetmap.org">OpenStreetMap</a>';

/**
 * basemapArchiveMaxZoom reads the archive's own maximum zoom.
 *
 * The style has to declare it: MapLibre overzooms up to that value and requests
 * real tiles above it, so a style claiming a deeper zoom than the operator's
 * extract actually contains renders an empty basemap at close zoom.
 */
export async function basemapArchiveMaxZoom(pmtilesUrl: string): Promise<number> {
  const header = await new PMTiles(pmtilesUrl).getHeader();
  return header.maxZoom;
}

/** ROUTE_SOURCE and its layer ids are shared by both styles. */
export const ROUTE_SOURCE = 'hyl-route';

/**
 * routeLayers draws the activity route above whatever basemap is in use. The
 * route belongs to the style rather than being added after it loads: a style
 * swap then carries the route with it, which is what makes theme changes and
 * late-arriving configuration safe.
 */
export function routeLayers(theme: 'light' | 'dark'): LayerSpecification[] {
  const dark = theme === 'dark';
  return [
    {
      id: 'hyl-route-casing',
      type: 'line',
      source: ROUTE_SOURCE,
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': dark ? '#000000' : '#ffffff', 'line-width': 6 },
    },
    {
      id: 'hyl-route',
      type: 'line',
      source: ROUTE_SOURCE,
      layout: { 'line-cap': 'round', 'line-join': 'round' },
      paint: { 'line-color': '#fc4c02', 'line-width': 3 },
    },
  ];
}

/** basemapStyle renders a self-hosted PMTiles vector basemap for one theme. */
export function basemapStyle(
  pmtilesUrl: string,
  theme: 'light' | 'dark',
  maxZoom: number,
  route: Feature<LineString>,
): StyleSpecification {
  return {
    version: 8,
    glyphs: `${BASEMAP_ASSETS}/fonts/{fontstack}/{range}.pbf`,
    sprite: `${BASEMAP_ASSETS}/sprites/v4/${theme}`,
    sources: {
      protomaps: {
        type: 'vector',
        maxzoom: maxZoom,
        url: `pmtiles://${pmtilesUrl}`,
        attribution: ATTRIBUTION,
      },
      [ROUTE_SOURCE]: { type: 'geojson', data: route },
    },
    layers: [...layers('protomaps', namedFlavor(theme), { lang: 'en' }), ...routeLayers(theme)],
  };
}

/**
 * fallbackStyle is the no-tiles style: a flat background with the route drawn
 * from the style itself, so a fresh install still shows a usable route map.
 */
export function fallbackStyle(theme: 'light' | 'dark', route: Feature<LineString>): StyleSpecification {
  const dark = theme === 'dark';
  return {
    version: 8,
    sources: { [ROUTE_SOURCE]: { type: 'geojson', data: route } },
    layers: [
      { id: 'bg', type: 'background', paint: { 'background-color': dark ? '#1b1b1f' : '#f5f5f4' } },
      ...routeLayers(theme),
    ],
  };
}

/**
 * routeFeature converts the API's flattened [lat,lon,…] list into a GeoJSON
 * LineString. A trailing odd value is ignored, and fewer than two coordinates
 * yield an empty line.
 */
export function routeFeature(route: number[]): Feature<LineString> {
  const coordinates: Position[] = [];
  for (let i = 0; i + 1 < route.length; i += 2) {
    const lat = route[i];
    const lon = route[i + 1];
    if (lat === undefined || lon === undefined) break;
    coordinates.push([lon, lat]);
  }
  return { type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates } };
}
