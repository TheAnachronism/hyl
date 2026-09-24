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
    let payload: ErrorResponse | undefined;
    try {
      payload = (await response.json()) as ErrorResponse;
    } catch {
      payload = undefined;
    }
    throw new ApiError(
      response.status,
      payload?.error?.code ?? 'internal',
      payload?.error?.message ?? response.statusText,
      payload,
    );
  }

  if (response.status === 204 || response.headers.get('content-length') === '0') {
    return undefined as T;
  }
  const text = await response.text();
  return (text ? JSON.parse(text) : undefined) as T;
}

/** api is the single typed entry point to the hyl REST API. */
export const api = {
  get: <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, { signal }),
  post: <T>(path: string, json?: unknown) => request<T>('POST', path, { json }),
  patch: <T>(path: string, json?: unknown) => request<T>('PATCH', path, { json }),
  put: <T>(path: string, json?: unknown) => request<T>('PUT', path, { json }),
  del: <T>(path: string, json?: unknown) => request<T>('DELETE', path, { json }),
  postForm: <T>(path: string, form: FormData) => request<T>('POST', path, { body: form }),
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
