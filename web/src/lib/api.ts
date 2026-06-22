// Thin fetch wrapper: every server call in the app goes through apiFetch so
// auth headers and error shaping have a single home (extended in M2).

export class ApiError extends Error {
  readonly status: number;
  readonly body: unknown;

  constructor(status: number, body: unknown) {
    super(`API request failed with status ${status}`);
    this.name = 'ApiError';
    this.status = status;
    this.body = body;
  }
}

// apiErrorMessage extracts the server's user-facing message from the error
// envelope ({"error":{"code","message"}}), falling back to a generic string.
export function apiErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError && typeof error.body === 'object' && error.body !== null) {
    const envelope = error.body as { error?: { message?: string } };
    if (envelope.error?.message) {
      return envelope.error.message;
    }
  }
  return fallback;
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    ...init,
    headers: { Accept: 'application/json', ...init?.headers },
  });

  const body: unknown = response.status === 204 ? null : await response.json().catch(() => null);

  if (!response.ok) {
    throw new ApiError(response.status, body);
  }

  return body as T;
}
