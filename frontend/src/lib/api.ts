import type { ErrorResponse } from '../api.gen';

/** ApiError carries the server's machine code alongside the HTTP status. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly payload: ErrorResponse | undefined;

  constructor(status: number, code: string, message: string, payload?: ErrorResponse) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.payload = payload;
  }

  get isUnauthorized(): boolean {
    return this.status === 401;
  }
}

function csrfFromCookie(): string | null {
  const match = document.cookie.match(/(?:^|;\s*)_csrf=([^;]*)/);
  return match?.[1] ? decodeURIComponent(match[1]) : null;
}

// The CSRF token is a double-submit cookie: echo sets `_csrf` on any request,
// and mutating requests must echo it in a header. On the very first request of a
// fresh session the cookie may not exist yet, so fetch it once.
async function csrfToken(): Promise<string | null> {
  const existing = csrfFromCookie();
  if (existing) return existing;
  await fetch('/api/csrf', { credentials: 'same-origin' });
  return csrfFromCookie();
}

interface RequestOptions {
  json?: unknown;
  body?: BodyInit;
  signal?: AbortSignal;
}

/** apiErrorFrom turns a failed response into the ApiError carrying its envelope. */
async function apiErrorFrom(response: Response): Promise<ApiError> {
  let payload: ErrorResponse | undefined;
  try {
    payload = (await response.json()) as ErrorResponse;
  } catch {
    payload = undefined;
  }
  return new ApiError(
    response.status,
    payload?.error?.code ?? 'internal',
    payload?.error?.message ?? response.statusText,
    payload,
  );
}

async function request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
  const headers = new Headers();
  let body = options.body;

  if (options.json !== undefined) {
    headers.set('Content-Type', 'application/json');
    body = JSON.stringify(options.json);
  }
  if (method !== 'GET' && method !== 'HEAD') {
    const token = await csrfToken();
    if (token) headers.set('X-CSRF-Token', token);
  }

  const response = await fetch(path, {
    method,
    headers,
    body,
    credentials: 'same-origin',
    signal: options.signal ?? null,
  });

  if (!response.ok) {
    throw await apiErrorFrom(response);
  }

  if (response.status === 204 || response.headers.get('content-length') === '0') {
    return undefined as T;
  }
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/**
 * postForRedirect POSTs to an endpoint that answers with a redirect the caller
 * must apply itself. The redirect is not chased: an OAuth consent screen is
 * cross-origin, so fetch could not follow it, and a manual redirect hides its
 * Location from a document. The target is returned when the browser exposes it,
 * and null when the redirect arrives opaque.
 */
async function postForRedirect(path: string, json?: unknown): Promise<string | null> {
  const headers = new Headers();
  if (json !== undefined) headers.set('Content-Type', 'application/json');
  const token = await csrfToken();
  if (token) headers.set('X-CSRF-Token', token);

  const response = await fetch(path, {
    method: 'POST',
    headers,
    body: json === undefined ? undefined : JSON.stringify(json),
    credentials: 'same-origin',
    redirect: 'manual',
  });

  // The redirect itself is the success case; only a real 4xx/5xx is an error.
  if (response.status >= 400) throw await apiErrorFrom(response);
  return response.headers.get('Location');
}

/** api is the single typed entry point to the hyl REST API. */
export const api = {
  get: <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, { signal }),
  post: <T>(path: string, json?: unknown) => request<T>('POST', path, { json }),
  patch: <T>(path: string, json?: unknown) => request<T>('PATCH', path, { json }),
  put: <T>(path: string, json?: unknown) => request<T>('PUT', path, { json }),
  del: <T>(path: string, json?: unknown) => request<T>('DELETE', path, { json }),
  postForm: <T>(path: string, form: FormData) => request<T>('POST', path, { body: form }),
  postRedirect: (path: string, json?: unknown) => postForRedirect(path, json),
};

/** query builds a `?a=1&b=2` string, skipping empty values. */
export function query(params: Record<string, string | number | undefined | null>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === null || value === '') continue;
    search.set(key, String(value));
  }
  const encoded = search.toString();
  return encoded ? `?${encoded}` : '';
}
