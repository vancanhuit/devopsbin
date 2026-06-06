// Typed client for the DevOpsBin backend runtime API.
//
// All endpoints live under the /api/v1 base path; in development Vite proxies
// that prefix to the Go backend (see vite.config.ts).

export const API_BASE = '/api/v1'

export type DependencyCheckStatus = 'ok' | 'error' | 'skipped'

export interface DependencyCheck {
  status: DependencyCheckStatus
  message?: string
}

export interface LivezResponse {
  status: 'ok'
}

export interface ReadyzResponse {
  status: 'ready' | 'not_ready'
  checks: Record<string, DependencyCheck>
}

export interface StartupzResponse {
  status: 'started' | 'starting'
  checks: Record<string, DependencyCheck>
}

export interface VersionResponse {
  service: string
  version: string
  git_sha: string
  build_time: string
  go_version: string
}

// CallResult captures everything the console needs to render a single request:
// the parsed body (when JSON), the HTTP status, timing, and any error.
export interface CallResult {
  ok: boolean
  status: number
  durationMs: number
  body: unknown
  error?: string
}

// call performs a GET against the given API path and returns a normalized
// CallResult. Network and parse failures are captured rather than thrown so the
// UI can render them uniformly.
export async function call(path: string): Promise<CallResult> {
  const start = performance.now()
  try {
    const res = await fetch(`${API_BASE}${path}`, {
      headers: { Accept: 'application/json' },
    })
    const durationMs = Math.round((performance.now() - start) * 100) / 100

    let body: unknown = null
    const text = await res.text()
    if (text) {
      try {
        body = JSON.parse(text)
      } catch {
        body = text
      }
    }

    return { ok: res.ok, status: res.status, durationMs, body }
  } catch (err) {
    const durationMs = Math.round((performance.now() - start) * 100) / 100
    return {
      ok: false,
      status: 0,
      durationMs,
      body: null,
      error: err instanceof Error ? err.message : String(err),
    }
  }
}
