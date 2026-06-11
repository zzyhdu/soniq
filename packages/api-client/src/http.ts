export type SoniqApiClientOptions = {
  baseUrl?: string;
  fetch?: typeof fetch;
};

export class SoniqApiError extends Error {
  readonly status: number;
  readonly statusText: string;
  readonly body: unknown;

  constructor(message: string, status: number, statusText: string, body: unknown) {
    super(message);
    this.name = 'SoniqApiError';
    this.status = status;
    this.statusText = statusText;
    this.body = body;
  }
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
    throw new SoniqApiError(errorMessage(body, response), response.status, response.statusText, body);
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
