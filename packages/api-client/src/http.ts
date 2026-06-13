export type SoniqApiClientOptions = {
  baseUrl?: string;
  fetch?: typeof fetch;
};

export const CSRF_HEADER_NAME = 'X-CSRF-Token';
export const CSRF_COOKIE_NAME = 'soniq_csrf';

export type SoniqErrorCode =
  | 'unauthenticated'
  | 'invalid_credentials'
  | 'invalid_csrf_token'
  | 'user_already_exists'
  | 'validation_failed'
  | 'forbidden'
  | 'not_found'
  | 'method_not_allowed'
  | 'request_too_large'
  | 'unsupported_media_type'
  | 'conflict'
  | 'rate_limited'
  | 'internal_error'
  | 'service_unavailable';

export type UnknownSoniqErrorCode = string & {};
export type UnauthorizedHandler = (error: SoniqApiError) => void;

export class SoniqApiError extends Error {
  readonly code: SoniqErrorCode | UnknownSoniqErrorCode | undefined;
  readonly status: number;
  readonly statusText: string;
  readonly body: unknown;

  constructor(
    message: string,
    status: number,
    statusText: string,
    body: unknown,
    code?: SoniqErrorCode | UnknownSoniqErrorCode,
  ) {
    super(message);
    this.name = 'SoniqApiError';
    this.code = code;
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
}

let unauthorizedHandler: UnauthorizedHandler | null = null;

export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): () => void {
  unauthorizedHandler = handler;
  return () => {
    if (unauthorizedHandler === handler) {
      unauthorizedHandler = null;
    }
  };
}

export async function requestJSON<T>(
  path: string,
  init: RequestInit,
  options: SoniqApiClientOptions,
): Promise<T> {
  const fetchImpl = options.fetch ?? globalThis.fetch;
  const response = await fetchImpl(buildUrl(path, options.baseUrl), withCSRFHeader(init));
  const body = await parseResponseBody(response);

  if (!response.ok) {
    const error = new SoniqApiError(errorMessage(body, response), response.status, response.statusText, body, errorCode(body));
    if (error.code === 'unauthenticated') {
      unauthorizedHandler?.(error);
    }
    throw error;
  }

  return body as T;
}

async function parseResponseBody(response: Response): Promise<unknown> {
  const text = await response.text();
  if (text.length === 0) {
    return null;
  }

  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function errorCode(body: unknown): SoniqErrorCode | UnknownSoniqErrorCode | undefined {
  if (body !== null && typeof body === 'object') {
    const maybeCode = (body as { code?: unknown }).code;
    if (typeof maybeCode === 'string' && maybeCode.length > 0) {
      return maybeCode;
    }
  }

  return undefined;
}

function withCSRFHeader(init: RequestInit): RequestInit {
  if (!isUnsafeMethod(init.method)) {
    return init;
  }
  if (hasHeader(init.headers, CSRF_HEADER_NAME)) {
    return init;
  }

  const token = readCookie(CSRF_COOKIE_NAME);
  if (token === null) {
    return init;
  }

  return {
    ...init,
    headers: appendHeader(init.headers, CSRF_HEADER_NAME, token),
  };
}

function appendHeader(headers: HeadersInit | undefined, name: string, value: string): HeadersInit {
  if (headers === undefined) {
    return { [name]: value };
  }
  if (typeof Headers !== 'undefined' && headers instanceof Headers) {
    const nextHeaders = new Headers(headers);
    nextHeaders.set(name, value);
    return nextHeaders;
  }
  if (Array.isArray(headers)) {
    return [...headers, [name, value]];
  }

  return { ...headers, [name]: value };
}

function hasHeader(headers: HeadersInit | undefined, name: string): boolean {
  if (headers === undefined) {
    return false;
  }
  const normalizedName = name.toLowerCase();
  if (typeof Headers !== 'undefined' && headers instanceof Headers) {
    return headers.has(name);
  }
  if (Array.isArray(headers)) {
    return headers.some(([headerName]) => headerName.toLowerCase() === normalizedName);
  }

  return Object.keys(headers).some((headerName) => headerName.toLowerCase() === normalizedName);
}

function isUnsafeMethod(method: string | undefined): boolean {
  const normalizedMethod = method?.toUpperCase() ?? 'GET';
  return normalizedMethod !== 'GET' &&
    normalizedMethod !== 'HEAD' &&
    normalizedMethod !== 'OPTIONS' &&
    normalizedMethod !== 'TRACE';
}

function readCookie(name: string): string | null {
  if (typeof document === 'undefined') {
    return null;
  }

  const prefix = `${name}=`;
  const cookie = document.cookie
    .split(';')
    .map((part) => part.trim())
    .find((part) => part.startsWith(prefix));
  if (cookie === undefined) {
    return null;
  }

  return decodeURIComponent(cookie.slice(prefix.length));
}

function errorMessage(body: unknown, response: Response): string {
  if (typeof body === 'string' && body.length > 0) {
    return body;
  }

  if (body !== null && typeof body === 'object') {
    const maybeMessage = (body as { message?: unknown; error?: unknown }).message ??
      (body as { message?: unknown; error?: unknown }).error;
    if (typeof maybeMessage === 'string' && maybeMessage.length > 0) {
      return maybeMessage;
    }
  }

  return response.statusText || `HTTP ${response.status}`;
}

function buildUrl(path: string, baseUrl?: string): string {
  if (baseUrl === undefined || baseUrl.length === 0) {
    return path;
  }

  return `${baseUrl.replace(/\/$/, '')}${path}`;
}
