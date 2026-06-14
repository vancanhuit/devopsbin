// Reactive auth store backed by the cookie session on the server.
//
// The server is the source of truth: the session and CSRF tokens live in
// cookies the browser sends automatically (see api.ts). This store mirrors the
// authenticated identity for the UI and is hydrated from GET /auth/me on load.
// It never holds credentials or the session token — only the public identity.

import { call, type UserResponse } from './api'

// AuthState is the single source of truth the UI reads to decide what to show.
export interface AuthState {
  // ready is false until the initial hydrate() completes, so the UI can avoid
  // flashing a logged-out state before the session is known.
  ready: boolean
  user: UserResponse | null
}

const state = $state<AuthState>({ ready: false, user: null })

// isUserResponse narrows an unknown response body to the generated UserResponse
// shape so the store never trusts an unexpected payload.
function isUserResponse(body: unknown): body is UserResponse {
  if (typeof body !== 'object' || body === null) {
    return false
  }
  const candidate = body as Record<string, unknown>
  return (
    typeof candidate.id === 'string' &&
    typeof candidate.username === 'string' &&
    typeof candidate.role === 'string'
  )
}

// auth exposes the reactive identity plus the transitions the UI triggers.
// Components read auth.user/auth.authenticated and call login/register/logout.
export const auth = {
  get ready(): boolean {
    return state.ready
  },
  get user(): UserResponse | null {
    return state.user
  },
  get authenticated(): boolean {
    return state.user !== null
  },

  // hydrate loads the current identity from the session cookie. A 200 means a
  // live session; anything else (401 logged out, transport error) clears it.
  // Always marks the store ready so the UI can render.
  async hydrate(): Promise<void> {
    const result = await call('/auth/me')
    state.user = result.status === 200 && isUserResponse(result.body) ? result.body : null
    state.ready = true
  },

  // setUser records the identity returned by a successful login/register so the
  // UI updates without a round-trip. Cards call this after auth mutations.
  setUser(body: unknown): void {
    if (isUserResponse(body)) {
      state.user = body
      state.ready = true
    }
  },

  // clear drops the local identity. Cards call this after a successful logout or
  // when /auth/me reports no session.
  clear(): void {
    state.user = null
    state.ready = true
  },

  // login submits credentials and, on success, records the returned identity.
  // Returns the username/password failure as a boolean so the caller can show
  // an inline message; cookies are set by the server response.
  async login(username: string, password: string): Promise<boolean> {
    const result = await call(
      '/auth/login',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ username, password }),
      }
    )
    if (result.status === 200 && isUserResponse(result.body)) {
      state.user = result.body
      state.ready = true
      return true
    }
    return false
  },

  // register creates an account and, on success, records the returned identity.
  async register(username: string, password: string): Promise<boolean> {
    const result = await call(
      '/auth/register',
      {},
      {
        method: 'POST',
        contentType: 'application/json',
        body: JSON.stringify({ username, password }),
      }
    )
    if (result.status === 201 && isUserResponse(result.body)) {
      state.user = result.body
      state.ready = true
      return true
    }
    return false
  },

  // logout ends the session server-side and clears the local identity. The
  // local state is cleared regardless so the UI reflects the intent even if the
  // session had already expired.
  async logout(): Promise<void> {
    await call('/auth/logout', {}, { method: 'POST' })
    state.user = null
  },
}
