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
  AdminApi,
  AuthApi,
  InspectApi,
  JSONApiResponse,
  LatencyApi,
  RuntimeApi,
  StatusApi,
  type ApiResponse,
  type Middleware,
} from './generated'
import type {
  LoginRequest,
  PasswordChangeRequest,
  PasswordResetRequest,
  PasswordResetRequestRequest,
  RegisterRequest,
  VersionResponse,
} from './generated'

export type {
  DependencyCheck,
  LivezResponse,
  ReadyzResponse,
  StartupzResponse,
  UserResponse,
  VersionResponse,
} from './generated'

// CSRF_COOKIE_NAME is the cookie the backend issues to carry the session-bound
// CSRF token (see AUTH_CSRF_COOKIE_NAME). The double-submit pattern requires the
// client to echo this value in the X-CSRF-Token header on state-changing
// requests; the server rejects a missing or mismatched token with 403.
export const CSRF_COOKIE_NAME = 'devopsbin_csrf'

// CSRF_HEADER_NAME is the header the backend reads the CSRF token from.
export const CSRF_HEADER_NAME = 'X-CSRF-Token'

// UNSAFE_METHODS are the HTTP methods the backend treats as state-changing and
// therefore guards with the CSRF check.
const UNSAFE_METHODS = ['POST', 'PUT', 'PATCH', 'DELETE']

// getCsrfToken reads the CSRF token from the (non-HttpOnly) CSRF cookie so it
// can be echoed back in the request header. Returns an empty string when the
// cookie is absent (e.g. logged out), in which case unsafe requests omit the
// header and the server responds 403 — the expected logged-out behaviour.
export function getCsrfToken(): string {
  if (typeof document === 'undefined') {
    return ''
  }
  const prefix = `${CSRF_COOKIE_NAME}=`
  for (const part of document.cookie.split('; ')) {
    if (part.startsWith(prefix)) {
      return decodeURIComponent(part.slice(prefix.length))
    }
  }
  return ''
}

// csrfMiddleware attaches the CSRF token header to every state-changing request
// issued through the generated client. Centralizing it here means individual
// callers (and cards) never deal with CSRF directly.
const csrfMiddleware: Middleware = {
  async pre(context) {
    const method = (context.init.method ?? 'GET').toUpperCase()
    if (!UNSAFE_METHODS.includes(method)) {
      return
    }
    const token = getCsrfToken()
    if (token) {
      context.init.headers = { ...context.init.headers, [CSRF_HEADER_NAME]: token }
    }
    return { url: context.url, init: context.init }
  },
}
// CallResult captures everything the console needs to render a single request:
// the parsed body, the HTTP status, timing, any response headers of interest,
// and any transport-level error.
export interface CallResult {
  ok: boolean
  status: number
  durationMs: number
  body: unknown
  // headers carries the response headers (lower-cased names) so cards can
  // surface documented ones (see Endpoint.resultHeaders). Browsers hide
  // Set-Cookie, so cookie-based responses simply omit it.
  headers?: Record<string, string>
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
// The shared Configuration sends cookies (same-origin) so the session and CSRF
// cookies ride along, and attaches the CSRF header to state-changing requests.
const config = new Configuration({
  credentials: 'same-origin',
  middleware: [csrfMiddleware],
})
const api = new RuntimeApi(config)
const inspect = new InspectApi(config)
const status = new StatusApi(config)
const latency = new LatencyApi(config)
const auth = new AuthApi(config)
const admin = new AdminApi(config)

// echoRaw issues the /echo request for the selected HTTP method, forwarding an
// optional request body (for body-carrying methods) and an optional raw query
// string. The echo endpoint reflects arbitrary query parameters, which the
// generated client cannot express, so the request is built directly against the
// same base path the generated client uses.
function echoRaw(opts: CallOptions): Promise<ApiResponse<unknown>> {
  const method = opts.method ?? 'GET'
  const query = (opts.query ?? '').replace(/^\?/, '')
  const url = `${config.basePath}/echo${query ? `?${query}` : ''}`

  const init: RequestInit = { method, credentials: 'same-origin' }
  const headers: Record<string, string> = {}
  if (['POST', 'PUT', 'PATCH', 'DELETE'].includes(method)) {
    const token = getCsrfToken()
    if (token) {
      headers[CSRF_HEADER_NAME] = token
    }
    if (opts.body != null) {
      headers['Content-Type'] = opts.contentType || 'text/plain'
      init.body = opts.body
    }
  }
  if (Object.keys(headers).length > 0) {
    init.headers = headers
  }

  return fetch(url, init).then((res) => new JSONApiResponse(res))
}

// jsonBody parses the JSON request body a card builds from its bodyFields. It
// returns an empty object when the body is absent or unparseable so the
// generated client still issues the request and the server reports the
// validation error.
function jsonBody(opts: CallOptions): Record<string, unknown> {
  if (opts.body == null || opts.body === '') {
    return {}
  }
  try {
    const parsed: unknown = JSON.parse(opts.body)
    return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : {}
  } catch {
    return {}
  }
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
  '/auth/register': (_args: CallArgs, opts: CallOptions) =>
    auth.postAuthRegisterRaw({ registerRequest: jsonBody(opts) as unknown as RegisterRequest }),
  '/auth/login': (_args: CallArgs, opts: CallOptions) =>
    auth.postAuthLoginRaw({ loginRequest: jsonBody(opts) as unknown as LoginRequest }),
  '/auth/logout': () => auth.postAuthLogoutRaw({}),
  '/auth/me': () => auth.getAuthMeRaw(),
  '/auth/password/change': (_args: CallArgs, opts: CallOptions) =>
    auth.postAuthPasswordChangeRaw({
      passwordChangeRequest: jsonBody(opts) as unknown as PasswordChangeRequest,
    }),
  '/auth/password/reset-request': (_args: CallArgs, opts: CallOptions) =>
    auth.postAuthPasswordResetRequestRaw({
      passwordResetRequestRequest: jsonBody(opts) as unknown as PasswordResetRequestRequest,
    }),
  '/auth/password/reset': (_args: CallArgs, opts: CallOptions) =>
    auth.postAuthPasswordResetRaw({
      passwordResetRequest: jsonBody(opts) as unknown as PasswordResetRequest,
    }),
  '/admin/users': () => admin.getAdminUsersRaw(),
  '/admin/accounts': () => admin.getAdminAccountsRaw(),
  '/admin/transfers': () => admin.getAdminTransfersRaw(),
  '/admin/users/{id}/unlock': (args: CallArgs) => admin.postAdminUserUnlockRaw({ id: args.id }),
  '/admin/users/{id}/password-reset': (args: CallArgs) =>
    admin.postAdminUserPasswordResetRaw({ id: args.id }),
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

// BodyField describes a single user-supplied input that contributes to a JSON
// request body (e.g. the username/password of /auth/login). Cards render an
// input per field and assemble them into a JSON object sent as the body.
export interface BodyField {
  // name is the JSON property the input populates, e.g. "username".
  name: string
  label: string
  // type maps to the HTML input type; "password" masks the value and "number"
  // is submitted as a JSON number.
  type: 'text' | 'password' | 'number'
  placeholder?: string
  // defaultValue pre-fills the input; omitted fields start empty.
  defaultValue?: string
}

// Endpoint describes a documented endpoint for both invocation and display.
// This is the single source of truth consumed by the UI; the path is typed
// against rawCalls so the metadata cannot drift from the callable endpoints.
export interface Endpoint {
  method: string
  path: EndpointPath
  title: string
  description: string
  // tag groups related endpoints into a labeled section in the console and
  // mirrors the OpenAPI tag (Runtime, Inspect, Status, Latency, Auth).
  tag: string
  // Status codes the endpoint may return that should still be treated as a
  // documented (expected) response, e.g. readyz returns 503 when not ready.
  expectedStatuses: number[]
  // params lists the inputs the console must collect before calling a
  // parameterized endpoint. Omitted for endpoints that take no input.
  params?: EndpointParam[]
  // bodyFields lists the inputs assembled into a JSON request body (e.g. the
  // credentials for /auth/login). Omitted for endpoints with no JSON body.
  bodyFields?: BodyField[]
  // methods lists the HTTP methods the endpoint supports when more than one is
  // available (e.g. /echo). Omitted for single-method endpoints, which use
  // `method`.
  methods?: string[]
  // supportsQuery enables a free-form query-string input for endpoints (e.g.
  // /echo) that reflect arbitrary query parameters.
  supportsQuery?: boolean
  // requiresAuth marks endpoints that need a valid session; the card shows a
  // hint badge and the request will 401 when logged out.
  requiresAuth?: boolean
  // requiresRole marks endpoints that additionally need a specific role.
  requiresRole?: 'admin'
  // resultHeaders lists response header names (lower-cased) worth surfacing in
  // the result panel. Omitted when no headers are interesting.
  resultHeaders?: string[]
}

export const endpoints: readonly Endpoint[] = [
  {
    method: 'GET',
    path: '/livez',
    title: 'Liveness',
    description: 'Process-only liveness check. Always 200 while the process runs.',
    tag: 'Runtime',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/readyz',
    title: 'Readiness',
    description: 'Reports whether the service is ready to receive traffic (503 when not).',
    tag: 'Runtime',
    expectedStatuses: [200, 503],
  },
  {
    method: 'GET',
    path: '/startupz',
    title: 'Startup',
    description: 'Reports whether startup has completed (503 while still starting).',
    tag: 'Runtime',
    expectedStatuses: [200, 503],
  },
  {
    method: 'GET',
    path: '/version',
    title: 'Version',
    description: 'Build and version metadata for the running binary.',
    tag: 'Runtime',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/uuid',
    title: 'UUID',
    description: 'Returns a randomly generated version 4 UUID.',
    tag: 'Inspect',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/ip',
    title: 'IP',
    description: "Returns the caller's origin IP address.",
    tag: 'Inspect',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/headers',
    title: 'Headers',
    description: 'Echoes the HTTP headers received with the request.',
    tag: 'Inspect',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/user-agent',
    title: 'User-Agent',
    description: 'Echoes the User-Agent header received with the request.',
    tag: 'Inspect',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/scheme',
    title: 'Scheme',
    description: 'Returns the request scheme (http or https), honoring trusted-proxy forwarding.',
    tag: 'Inspect',
    expectedStatuses: [200],
  },
  {
    method: 'GET',
    path: '/echo',
    title: 'Echo',
    description: 'Reflects the request method, path, query, headers, origin IP, scheme, and body.',
    tag: 'Inspect',
    expectedStatuses: [200, 413],
    methods: ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'],
    supportsQuery: true,
  },
  {
    method: 'GET',
    path: '/status/{code}',
    title: 'Status',
    description: 'Returns the HTTP status code given in the path.',
    tag: 'Status',
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
    tag: 'Latency',
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
  {
    method: 'POST',
    path: '/auth/register',
    title: 'Register',
    description: 'Creates a user and opens an authenticated session (sets cookies).',
    tag: 'Auth',
    expectedStatuses: [201, 400, 409],
    bodyFields: [
      { name: 'username', label: 'Username', type: 'text', placeholder: '3–32 characters' },
      { name: 'password', label: 'Password', type: 'password', placeholder: '8–128 characters' },
    ],
  },
  {
    method: 'POST',
    path: '/auth/login',
    title: 'Login',
    description: 'Verifies credentials and opens an authenticated session (sets cookies).',
    tag: 'Auth',
    expectedStatuses: [200, 400, 401, 423],
    resultHeaders: ['retry-after'],
    bodyFields: [
      { name: 'username', label: 'Username', type: 'text', defaultValue: 'alice' },
      { name: 'password', label: 'Password', type: 'password', defaultValue: 'alicepass' },
    ],
  },
  {
    method: 'POST',
    path: '/auth/logout',
    title: 'Logout',
    description: 'Deletes the current session and clears the auth cookies.',
    tag: 'Auth',
    expectedStatuses: [204, 401, 403],
    requiresAuth: true,
  },
  {
    method: 'GET',
    path: '/auth/me',
    title: 'Me',
    description: 'Returns the user bound to the current session.',
    tag: 'Auth',
    expectedStatuses: [200, 401],
    requiresAuth: true,
  },
  {
    method: 'POST',
    path: '/auth/password/change',
    title: 'Change password',
    description:
      'Changes the current user’s password, rotates the session, and revokes other sessions.',
    tag: 'Auth',
    expectedStatuses: [200, 400, 401, 403],
    requiresAuth: true,
    bodyFields: [
      { name: 'currentPassword', label: 'Current password', type: 'password' },
      {
        name: 'newPassword',
        label: 'New password',
        type: 'password',
        placeholder: '8–128 characters',
      },
    ],
  },
  {
    method: 'POST',
    path: '/auth/password/reset-request',
    title: 'Request password reset',
    description:
      'Issues a single-use reset token for a username. Always 200; the token is returned only when the user exists (demo only).',
    tag: 'Auth',
    expectedStatuses: [200, 400],
    bodyFields: [{ name: 'username', label: 'Username', type: 'text', defaultValue: 'alice' }],
  },
  {
    method: 'POST',
    path: '/auth/password/reset',
    title: 'Reset password',
    description:
      'Consumes a reset token and sets a new password, revoking all of the user’s sessions.',
    tag: 'Auth',
    expectedStatuses: [200, 400, 410],
    bodyFields: [
      { name: 'token', label: 'Reset token', type: 'text' },
      {
        name: 'newPassword',
        label: 'New password',
        type: 'password',
        placeholder: '8–128 characters',
      },
    ],
  },
  {
    method: 'GET',
    path: '/admin/users',
    title: 'List users',
    description:
      'Lists all users (id, username, role, created time). Requires an admin session; non-admins get 403.',
    tag: 'Admin',
    expectedStatuses: [200, 401, 403],
    requiresAuth: true,
    requiresRole: 'admin',
  },
  {
    method: 'GET',
    path: '/admin/accounts',
    title: 'List accounts',
    description:
      'Lists every account across users with its owner. Requires an admin session; non-admins get 403.',
    tag: 'Admin',
    expectedStatuses: [200, 401, 403],
    requiresAuth: true,
    requiresRole: 'admin',
  },
  {
    method: 'GET',
    path: '/admin/transfers',
    title: 'List transfers',
    description:
      'Lists the transfers ledger, most recent first. Requires an admin session; non-admins get 403.',
    tag: 'Admin',
    expectedStatuses: [200, 401, 403],
    requiresAuth: true,
    requiresRole: 'admin',
  },
  {
    method: 'POST',
    path: '/admin/users/{id}/unlock',
    title: 'Unlock user',
    description:
      'Clears a user’s brute-force login lockout. Requires an admin session and CSRF token; non-admins get 403.',
    tag: 'Admin',
    expectedStatuses: [204, 401, 403, 404],
    requiresAuth: true,
    requiresRole: 'admin',
    params: [{ name: 'id', label: 'User id', type: 'text', defaultValue: '' }],
  },
  {
    method: 'POST',
    path: '/admin/users/{id}/password-reset',
    title: 'Reset user password',
    description:
      'Mints a single-use reset token for a user (returned in the response, demo only). Requires an admin session and CSRF token.',
    tag: 'Admin',
    expectedStatuses: [200, 401, 403, 404],
    requiresAuth: true,
    requiresRole: 'admin',
    params: [{ name: 'id', label: 'User id', type: 'text', defaultValue: '' }],
  },
]

function elapsedMs(start: number): number {
  return Math.round((performance.now() - start) * 100) / 100
}

// headerMap copies the response headers into a plain object (lower-cased names)
// so cards can read documented ones. Set-Cookie is never exposed to JS by the
// browser, so cookie-bearing responses simply omit it.
function headerMap(res: Response): Record<string, string> {
  const out: Record<string, string> = {}
  res.headers.forEach((value, key) => {
    out[key] = value
  })
  return out
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
      headers: headerMap(res.raw),
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
        headers: headerMap(err.response),
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
