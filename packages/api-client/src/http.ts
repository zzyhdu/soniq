export type SoniqApiClientOptions = {
  baseUrl?: string;
  fetch?: typeof fetch;
};

export type SoniqErrorCode =
  | 'unauthenticated'
  | 'invalid_credentials'
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
  const response = await fetchImpl(buildUrl(path, options.baseUrl), init);
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
