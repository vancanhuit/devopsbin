// Typed client for the DevOpsBin backend runtime API.
//
// This wraps the auto-generated OpenAPI client (see ./generated, produced by
// openapi-generator-cli via `mise run web:generate`) so the console gets typed
// responses while still rendering documented non-2xx results (e.g. readyz 503)
// uniformly. Do not edit files under `./generated/` by hand; re-run the
// generator after changing `api/openapi.yaml`.

import {
  Configuration,
  ResponseError,
  InspectApi,
  RuntimeApi,
  StatusApi,
  type ApiResponse,
} from './generated'
import type { VersionResponse } from './generated'

export type {
  DependencyCheck,
  LivezResponse,
  ReadyzResponse,
  StartupzResponse,
  VersionResponse,
} from './generated'
// CallResult captures everything the console needs to render a single request:
// the parsed body, the HTTP status, timing, and any transport-level error.
export interface CallResult {
  ok: boolean
  status: number
  durationMs: number
  body: unknown
  error?: string
}

// api targets the /api/v1 base path baked into the generated client; in
// development Vite proxies that prefix to the Go backend (see vite.config.ts).
const config = new Configuration()
const api = new RuntimeApi(config)
const inspect = new InspectApi(config)
const status = new StatusApi(config)

// rawCalls maps each documented endpoint path to its generated raw call. The
// *Raw variants return an ApiResponse, exposing both the typed body via
// value() and the underlying Response for status and timing.
const rawCalls = {
  '/livez': () => api.getLivezRaw(),
  '/readyz': () => api.getReadyzRaw(),
  '/startupz': () => api.getStartupzRaw(),
  '/version': () => api.getVersionRaw(),
  '/uuid': () => inspect.getUuidRaw(),
  '/ip': () => inspect.getIpRaw(),
  '/headers': () => inspect.getHeadersRaw(),
  '/user-agent': () => inspect.getUserAgentRaw(),
  '/echo': () => inspect.getEchoRaw(),
  '/status/200': () => status.getStatusRaw({ code: 200 }),
} satisfies Record<string, () => Promise<ApiResponse<unknown>>>

// EndpointPath is the set of documented API paths the console can call.
export type EndpointPath = keyof typeof rawCalls

// Endpoint describes a documented endpoint for both invocation and display.
// This is the single source of truth consumed by the UI; the path is typed
// against rawCalls so the metadata cannot drift from the callable endpoints.
export interface Endpoint {
  method: string
  path: EndpointPath
  title: string
  description: string
  // Status codes the endpoint may return that should still be treated as a
  // documented (expected) response, e.g. readyz returns 503 when not ready.
  expectedStatuses: number[]
}

export const endpoints: readonly Endpoint[] = [
  {
    method: 'GET',
    path: '/livez',
    title: 'Liveness',
    description: 'Process-only liveness check. Always 200 while the process runs.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/readyz',
    title: 'Readiness',
    description: 'Reports whether the service is ready to receive traffic (503 when not).',
    expectedStatuses: [200, 503],
  },
  {
    method: 'GET',
    path: '/startupz',
    title: 'Startup',
    description: 'Reports whether startup has completed (503 while still starting).',
    expectedStatuses: [200, 503],
  },
  {
    method: 'GET',
    path: '/version',
    title: 'Version',
    description: 'Build and version metadata for the running binary.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/uuid',
    title: 'UUID',
    description: 'Returns a randomly generated version 4 UUID.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/ip',
    title: 'IP',
    description: "Returns the caller's origin IP address.",
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/headers',
    title: 'Headers',
    description: 'Echoes the HTTP headers received with the request.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/user-agent',
    title: 'User-Agent',
    description: 'Echoes the User-Agent header received with the request.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/echo',
    title: 'Echo',
    description: 'Reflects the request method, path, query, headers, and origin IP.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/status/200',
    title: 'Status',
    description: 'Returns the HTTP status code given in the path (here, 200).',
    expectedStatuses: [200],
  },
]

function elapsedMs(start: number): number {
  return Math.round((performance.now() - start) * 100) / 100
}

async function parseBody(res: Response): Promise<unknown> {
  const text = await res.text()
  if (!text) {
    return null
  }
  try {
    return JSON.parse(text)
  } catch {
    return text
  }
}

// call performs the documented request for the given API path and returns a
// normalized CallResult. Documented non-2xx responses (e.g. readyz 503) are
// reported with their status and parsed body; only transport failures (network
// errors, unknown paths) set `error`.
export async function call(path: string): Promise<CallResult> {
  const raw = (rawCalls as Record<string, (() => Promise<ApiResponse<unknown>>) | undefined>)[path]
  if (!raw) {
    return { ok: false, status: 0, durationMs: 0, body: null, error: `unknown endpoint: ${path}` }
  }

  const start = performance.now()
  try {
    const res = await raw()
    return {
      ok: res.raw.ok,
      status: res.raw.status,
      durationMs: elapsedMs(start),
      body: await res.value(),
    }
  } catch (err) {
    // The generated client throws ResponseError for non-2xx responses; surface
    // those like any other documented response rather than a transport error.
    if (err instanceof ResponseError) {
      return {
        ok: false,
        status: err.response.status,
        durationMs: elapsedMs(start),
        body: await parseBody(err.response),
      }
    }
    return {
      ok: false,
      status: 0,
      durationMs: elapsedMs(start),
      body: null,
      error: err instanceof Error ? err.message : String(err),
    }
  }
}

// getVersion fetches the running binary's build metadata via the typed client.
// Used by the console footer to display the app version; callers treat it as
// best-effort and ignore failures.
export async function getVersion(): Promise<VersionResponse> {
  return api.getVersion()
}
