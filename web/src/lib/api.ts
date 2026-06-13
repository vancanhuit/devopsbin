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
  JSONApiResponse,
  LatencyApi,
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

// CallArgs holds user-supplied values for parameterized endpoints, keyed by the
// parameter name (e.g. { code: '404' } for /status/{code}). Endpoints without
// parameters ignore it.
export type CallArgs = Record<string, string>

// CallOptions holds per-invocation choices that are not path parameters, such
// as the HTTP method, request body, and raw query string for endpoints (like
// /echo) that accept them. Endpoints that ignore these leave them undefined.
export interface CallOptions {
  method?: string
  body?: string
  // contentType sets the Content-Type header for body-carrying methods (POST,
  // PUT, PATCH, DELETE). Defaults to text/plain when omitted.
  contentType?: string
  // query is a raw query string (e.g. "foo=bar&foo=baz"), with or without a
  // leading "?". It is appended verbatim so callers can send repeated keys.
  query?: string
}

// api targets the /api/v1 base path baked into the generated client; in
// development Vite proxies that prefix to the Go backend (see vite.config.ts).
const config = new Configuration()
const api = new RuntimeApi(config)
const inspect = new InspectApi(config)
const status = new StatusApi(config)
const latency = new LatencyApi(config)

// echoRaw issues the /echo request for the selected HTTP method, forwarding an
// optional request body (for body-carrying methods) and an optional raw query
// string. The echo endpoint reflects arbitrary query parameters, which the
// generated client cannot express, so the request is built directly against the
// same base path the generated client uses.
function echoRaw(opts: CallOptions): Promise<ApiResponse<unknown>> {
  const method = opts.method ?? 'GET'
  const query = (opts.query ?? '').replace(/^\?/, '')
  const url = `${config.basePath}/echo${query ? `?${query}` : ''}`

  const init: RequestInit = { method }
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method) && opts.body != null) {
    init.headers = { 'Content-Type': opts.contentType || 'text/plain' }
    init.body = opts.body
  }

  return fetch(url, init).then((res) => new JSONApiResponse(res))
}

// rawCalls maps each documented endpoint path to its generated raw call. The
// *Raw variants return an ApiResponse, exposing both the typed body via
// value() and the underlying Response for status and timing. Parameterized
// endpoints receive the user-supplied CallArgs; multi-method endpoints receive
// the CallOptions; the rest ignore both.
const rawCalls = {
  '/livez': () => api.getLivezRaw(),
  '/readyz': () => api.getReadyzRaw(),
  '/startupz': () => api.getStartupzRaw(),
  '/version': () => api.getVersionRaw(),
  '/uuid': () => inspect.getUuidRaw(),
  '/ip': () => inspect.getIpRaw(),
  '/headers': () => inspect.getHeadersRaw(),
  '/user-agent': () => inspect.getUserAgentRaw(),
  '/scheme': () => inspect.getSchemeRaw(),
  '/echo': (_args: CallArgs, opts: CallOptions) => echoRaw(opts),
  '/status/{code}': (args: CallArgs) => status.getStatusRaw({ code: Number(args.code) }),
  '/delay/{seconds}': (args: CallArgs) => latency.getDelayRaw({ seconds: Number(args.seconds) }),
} satisfies Record<string, (args: CallArgs, opts: CallOptions) => Promise<ApiResponse<unknown>>>

// EndpointPath is the set of documented API paths the console can call.
export type EndpointPath = keyof typeof rawCalls

// EndpointParam describes a single user-supplied input for a parameterized
// endpoint (e.g. the {code} path segment of /status/{code}).
export interface EndpointParam {
  // name matches the placeholder in the endpoint path, e.g. "code".
  name: string
  label: string
  // type maps to the HTML input type; "number" inputs are constrained by
  // min/max and submitted as their string value.
  type: 'number' | 'text'
  // defaultValue pre-fills the input so the endpoint is callable as-is.
  defaultValue: string
  min?: number
  max?: number
  placeholder?: string
}

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
  // params lists the inputs the console must collect before calling a
  // parameterized endpoint. Omitted for endpoints that take no input.
  params?: EndpointParam[]
  // methods lists the HTTP methods the endpoint supports when more than one is
  // available (e.g. /echo). Omitted for single-method endpoints, which use
  // `method`.
  methods?: string[]
  // supportsQuery enables a free-form query-string input for endpoints (e.g.
  // /echo) that reflect arbitrary query parameters.
  supportsQuery?: boolean
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
    path: '/scheme',
    title: 'Scheme',
    description: 'Returns the request scheme (http or https), honoring trusted-proxy forwarding.',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/echo',
    title: 'Echo',
    description: 'Reflects the request method, path, query, headers, origin IP, scheme, and body.',
    expectedStatuses: [200, 413],
    methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'],
    supportsQuery: true,
  },
  {
    method: 'GET',
    path: '/status/{code}',
    title: 'Status',
    description: 'Returns the HTTP status code given in the path.',
    expectedStatuses: [200],
    params: [
      {
        name: 'code',
        label: 'Status code',
        type: 'number',
        defaultValue: '200',
        min: 100,
        max: 599,
        placeholder: '100–599',
      },
    ],
  },
  {
    method: 'GET',
    path: '/delay/{seconds}',
    title: 'Delay',
    description: 'Waits the given number of seconds (capped at 10) before responding.',
    expectedStatuses: [200],
    params: [
      {
        name: 'seconds',
        label: 'Seconds',
        type: 'number',
        defaultValue: '1',
        min: 0,
        max: 10,
        placeholder: '0–10',
      },
    ],
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
// normalized CallResult. Parameterized endpoints receive their user-supplied
// args; multi-method endpoints receive the per-call options (method, body).
// Documented non-2xx responses (e.g. readyz 503) are reported with their
// status and parsed body; only transport failures (network errors, unknown
// paths) set `error`.
export async function call(
  path: string,
  args: CallArgs = {},
  opts: CallOptions = {}
): Promise<CallResult> {
  const raw = (
    rawCalls as Record<
      string,
      ((args: CallArgs, opts: CallOptions) => Promise<ApiResponse<unknown>>) | undefined
    >
  )[path]
  if (!raw) {
    return { ok: false, status: 0, durationMs: 0, body: null, error: `unknown endpoint: ${path}` }
  }

  const start = performance.now()
  try {
    const res = await raw(args, opts)
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
