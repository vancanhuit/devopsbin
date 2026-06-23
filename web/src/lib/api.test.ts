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

describe('password endpoints', () => {
  it('attaches the CSRF header when changing the password', async () => {
    stubCookie('devopsbin_csrf=tok-pw')
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ id: '1', username: 'alice', role: 'user' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/auth/password/change',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ currentPassword: 'alicepass', newPassword: 'alicepass2' }),
      }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/auth/password/change')
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-pw')
    expect(JSON.parse(String(init.body))).toEqual({
      currentPassword: 'alicepass',
      newPassword: 'alicepass2',
    })
    expect(result.status).toBe(200)
  })

  it('surfaces the reset token from /auth/password/reset-request', async () => {
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ message: 'ok', token: 'reset-tok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/auth/password/reset-request',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ username: 'alice' }),
      }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/v1/auth/password/reset-request')
    expect(result.status).toBe(200)
    expect(result.body).toEqual({ message: 'ok', token: 'reset-tok' })
  })

  it('reports a 410 for an invalid reset token', async () => {
    stubFetch(
      async () =>
        new Response(JSON.stringify({ error: 'reset token is invalid or expired' }), {
          status: 410,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/auth/password/reset',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ token: 'nope', newPassword: 'whatever12' }),
      }
    )

    expect(result.ok).toBe(false)
    expect(result.status).toBe(410)
    expect(result.body).toEqual({ error: 'reset token is invalid or expired' })
  })
})

describe('admin endpoints', () => {
  it('lists users via a safe GET without a CSRF header', async () => {
    stubCookie('devopsbin_csrf=tok-admin')
    const fetchMock = stubFetch(
      async () =>
        new Response(
          JSON.stringify({
            users: [
              { id: '1', username: 'admin', role: 'admin', createdAt: '2024-01-01T00:00:00Z' },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } }
        )
    )

    const result = await call('/admin/users')

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/admin/users')
    expect(headersOf(init)['X-CSRF-Token']).toBeUndefined()
    expect(result.status).toBe(200)
    const body = result.body as { users: { id: string; username: string; role: string }[] }
    expect(body.users).toHaveLength(1)
    expect(body.users[0]).toMatchObject({ id: '1', username: 'admin', role: 'admin' })
  })

  it('reports a 403 when a non-admin lists accounts', async () => {
    stubFetch(
      async () =>
        new Response(JSON.stringify({ error: 'insufficient privileges' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call('/admin/accounts')

    expect(result.ok).toBe(false)
    expect(result.status).toBe(403)
    expect(result.body).toEqual({ error: 'insufficient privileges' })
  })

  it('substitutes the id path param and attaches the CSRF header when unlocking a user', async () => {
    stubCookie('devopsbin_csrf=tok-admin')
    const fetchMock = stubFetch(async () => new Response(null, { status: 204 }))

    const result = await call('/admin/users/{id}/unlock', { id: 'user-42' }, { method: 'POST' })

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/admin/users/user-42/unlock')
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-admin')
    expect(result.status).toBe(204)
  })

  it('surfaces the minted reset token when resetting a user password', async () => {
    stubCookie('devopsbin_csrf=tok-admin')
    const fetchMock = stubFetch(
      async () =>
        new Response(JSON.stringify({ message: 'ok', token: 'admin-reset-tok' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/admin/users/{id}/password-reset',
      { id: 'user-42' },
      { method: 'POST' }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/admin/users/user-42/password-reset')
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-admin')
    expect(result.status).toBe(200)
    expect(result.body).toEqual({ message: 'ok', token: 'admin-reset-tok' })
  })
})

describe('database endpoints', () => {
  it('lists accounts via a safe GET without a CSRF header', async () => {
    stubCookie('devopsbin_csrf=tok-user')
    const fetchMock = stubFetch(
      async () =>
        new Response(
          JSON.stringify({
            accounts: [
              {
                id: 'acc-1',
                ownerUsername: 'alice',
                name: 'Checking',
                balanceCents: 100000,
                createdAt: '2024-01-01T00:00:00Z',
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } }
        )
    )

    const result = await call('/accounts')

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/accounts')
    expect(headersOf(init)['X-CSRF-Token']).toBeUndefined()
    expect(result.status).toBe(200)
    const body = result.body as { accounts: { id: string; balanceCents: number }[] }
    expect(body.accounts).toHaveLength(1)
    expect(body.accounts[0]).toMatchObject({ id: 'acc-1', balanceCents: 100000 })
  })

  it('posts a transfer with the JSON body and attaches the CSRF header', async () => {
    stubCookie('devopsbin_csrf=tok-user')
    const fetchMock = stubFetch(
      async () =>
        new Response(
          JSON.stringify({
            transferId: 'xfer-1',
            fromAccountId: 'acc-1',
            toAccountId: 'acc-2',
            fromBalanceCents: 97500,
            toBalanceCents: 52500,
            amountCents: 2500,
            attempts: 1,
            createdAt: '2024-01-01T00:00:00Z',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } }
        )
    )

    const result = await call(
      '/transfer',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({
          fromAccountId: 'acc-1',
          toAccountId: 'acc-2',
          amountCents: 2500,
        }),
      }
    )

    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(String(url)).toContain('/api/v1/transfer')
    expect(init.method).toBe('POST')
    expect(headersOf(init)['X-CSRF-Token']).toBe('tok-user')
    expect(String(init.body)).toContain('"fromAccountId":"acc-1"')
    expect(result.status).toBe(200)
    expect(result.body).toMatchObject({
      transferId: 'xfer-1',
      fromBalanceCents: 97500,
      toBalanceCents: 52500,
      attempts: 1,
    })
  })

  it('reports a 409 on insufficient funds', async () => {
    stubCookie('devopsbin_csrf=tok-user')
    stubFetch(
      async () =>
        new Response(JSON.stringify({ error: 'insufficient funds' }), {
          status: 409,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/transfer',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({
          fromAccountId: 'acc-1',
          toAccountId: 'acc-2',
          amountCents: 999999,
        }),
      }
    )

    expect(result.ok).toBe(false)
    expect(result.status).toBe(409)
    expect(result.body).toEqual({ error: 'insufficient funds' })
  })

  it('reports a 403 when the caller does not own the source account', async () => {
    stubCookie('devopsbin_csrf=tok-user')
    stubFetch(
      async () =>
        new Response(JSON.stringify({ error: 'you do not own the source account' }), {
          status: 403,
          headers: { 'Content-Type': 'application/json' },
        })
    )

    const result = await call(
      '/transfer',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({
          fromAccountId: 'someone-elses-acc',
          toAccountId: 'acc-2',
          amountCents: 2500,
        }),
      }
    )

    expect(result.ok).toBe(false)
    expect(result.status).toBe(403)
    expect(result.body).toEqual({ error: 'you do not own the source account' })
  })
})
