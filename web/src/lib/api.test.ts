import { afterEach, describe, expect, it, vi } from 'vitest'

import { call, getCsrfToken } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

function stubFetch(impl: typeof fetch) {
  const fetchMock = vi.fn(impl)
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

// stubCookie makes document.cookie return the given string so the CSRF
// middleware can read the (non-HttpOnly) token cookie under the node test env.
function stubCookie(cookie: string) {
  vi.stubGlobal('document', { cookie })
}

function headersOf(init: RequestInit): Record<string, string> {
  return (init.headers ?? {}) as Record<string, string>
}

describe('call', () => {
  it('returns a typed body for a 2xx response', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ status: 'ok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call('/livez')

    expect(fetchMock).toHaveBeenCalledOnce()
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/v1/livez')
    expect(result.ok).toBe(true)
    expect(result.status).toBe(200)
    expect(result.body).toEqual({ status: 'ok' })
    expect(result.error).toBeUndefined()
    expect(typeof result.durationMs).toBe('number')
  })

  it('reports a documented non-2xx response with its status and body', async () => {
    stubFetch(
      async () =>
        new Response(JSON.stringify({ status: 'not_ready', checks: {} }), {
          status: 503,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call('/readyz')

    expect(result.ok).toBe(false)
    expect(result.status).toBe(503)
    expect(result.body).toEqual({ status: 'not_ready', checks: {} })
    expect(result.error).toBeUndefined()
  })

  it('captures a transport failure as an error with status 0', async () => {
    stubFetch(async () => {
      throw new Error('network down')
    })

    const result = await call('/version')

    expect(result.ok).toBe(false)
    expect(result.status).toBe(0)
    expect(result.body).toBeNull()
    expect(result.error).toBeTruthy()
  })

  it('rejects an unknown endpoint without making a request', async () => {
    const fetchMock = stubFetch(async () => new Response(null, { status: 200 }))

    const result = await call('/does-not-exist')

    expect(fetchMock).not.toHaveBeenCalled()
    expect(result.ok).toBe(false)
    expect(result.status).toBe(0)
    expect(result.error).toContain('unknown endpoint')
  })

  it('issues a POST with the request body for /echo', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ method: 'POST', body: 'hello' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call('/echo', {}, { method: 'POST', body: 'hello' })

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.method).toBe('POST')
    expect(init.body).toBe('hello')
    expect(result.ok).toBe(true)
    expect(result.status).toBe(200)
    expect(result.body).toEqual({ method: 'POST', body: 'hello' })
  })

  it('issues a GET for /echo when no method is given', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ method: 'GET' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    await call('/echo')

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.method).toBe('GET')
  })

  it('sets a custom Content-Type for /echo body methods', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ method: 'POST' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    await call('/echo', {}, { method: 'POST', body: '{"a":1}', contentType: 'application/json' })

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' })
  })

  it('appends the query string to the /echo request URL', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ method: 'GET' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    await call('/echo', {}, { query: 'foo=bar&foo=baz' })

    expect(fetchMock).toHaveBeenCalledOnce()
    const url = String(fetchMock.mock.calls[0][0])
    expect(url).toContain('/echo?foo=bar&foo=baz')
  })
})

describe('getCsrfToken', () => {
  it('returns an empty string when no document is present', () => {
    expect(getCsrfToken()).toBe('')
  })

  it('reads the devopsbin_csrf cookie value', () => {
    stubCookie('other=1; devopsbin_csrf=tok-123; foo=bar')
    expect(getCsrfToken()).toBe('tok-123')
  })

  it('returns an empty string when the CSRF cookie is absent', () => {
    stubCookie('other=1; foo=bar')
    expect(getCsrfToken()).toBe('')
  })
})

describe('CSRF wiring', () => {
  it('attaches the X-CSRF-Token header to an unsafe request when the cookie is set', async () => {
    stubCookie('devopsbin_csrf=tok-abc')
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ id: '1', username: 'alice', role: 'user' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    await call(
      '/auth/login',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ username: 'alice', password: 'alicepass' }),
      }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-abc')
  })

  it('omits the X-CSRF-Token header on a safe (GET) request', async () => {
    stubCookie('devopsbin_csrf=tok-abc')
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ id: '1', username: 'alice', role: 'user' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    await call('/auth/me')

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(headersOf(init)['X-CSRF-Token']).toBeUndefined()
  })

  it('sends same-origin credentials on auth requests', async () => {
    stubCookie('devopsbin_csrf=tok-abc')
    const fetchMock = stubFetch(async () => new Response(null, { status: 204 }))

    await call('/auth/logout', {}, { method: 'POST' })

    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(init.credentials).toBe('same-origin')
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-abc')
  })
})

describe('JSON body fields', () => {
  it('serializes the request body as JSON for /auth/register', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ id: '1', username: 'bob', role: 'user' }), {
          status: 201,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/auth/register',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ username: 'bob', password: 'bobpass12' }),
      }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    const init = fetchMock.mock.calls[0][1] as RequestInit
    expect(headersOf(init)['Content-Type']).toBe('application/json')
    expect(JSON.parse(String(init.body))).toEqual({ username: 'bob', password: 'bobpass12' })
    expect(result.status).toBe(201)
    expect(result.body).toEqual({ id: '1', username: 'bob', role: 'user' })
  })
})
