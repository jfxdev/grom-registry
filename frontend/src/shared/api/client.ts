export class APIError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly requestId?: string,
  ) {
    super(message)
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(path, {
    ...init,
    headers,
    credentials: 'same-origin',
  })
  if (!response.ok) {
    const body = await response.json().catch(() => ({
      code: 'request_failed',
      message: `Request failed with status ${response.status}`,
    })) as { code?: string; message?: string; requestId?: string }
    throw new APIError(
      response.status,
      body.code ?? 'request_failed',
      body.message ?? 'The request could not be completed',
      body.requestId,
    )
  }
  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}
