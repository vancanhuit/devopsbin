import { afterEach, describe, expect, it, vi } from 'vitest'

import { call } from './api'

afterEach(() => {
  vi.unstubAllGlobals()
})

function stubFetch(impl: typeof fetch) {
  const fetchMock = vi.fn(impl)
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
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
