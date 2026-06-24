# ADR 0002: Redis-backed per-IP fixed-window rate limiting

- Status: Accepted
- Date: 2025-01-15

## Context

The `GET /ratelimit` endpoint is a demo of request rate limiting: a caller
should be allowed a bounded number of requests within a short window and be
rejected with `429 Too Many Requests` (and a `Retry-After`) once that budget is
spent, with the standard `RateLimit-Limit` / `RateLimit-Remaining` /
`RateLimit-Reset` headers exposing the remaining allowance.

The service runs as **multiple stateless replicas** behind a load balancer, so
the limiter has to share state: a per-process, in-memory counter would let a
client get `N` requests _per replica_, defeating the limit. The counter must
therefore live in a store every replica can reach. The service already depends
on Redis (for sessions and the login lockout), which provides cheap atomic
counters with TTLs and is the natural fit.

The forces at play:

- **Distributed correctness.** The count must be shared and updated atomically
  across replicas; a read-modify-write race would under-count under concurrency.
- **Self-expiry.** A window must reset on its own without a sweeper; stale
  counters must not accumulate in Redis.
- **Availability vs. enforcement.** Redis is a cache dependency, not the source
  of truth. A limiter that hard-fails when Redis is briefly unavailable would
  take down a public endpoint for a cache blip — worse than not limiting.
- **Client identity.** "Per client" has to mean the real caller even behind a
  trusted proxy, consistent with how the rest of the service resolves the origin
  IP.
- **Algorithm complexity.** A sliding-window log or token bucket is more precise
  at window boundaries but needs more state and more round-trips; a fixed window
  is one counter and one round-trip.

The options considered:

- **In-memory per-replica counter.** Simplest, but wrong under more than one
  replica (the limit multiplies by the replica count).
- **Sliding-window log / token bucket in Redis.** More accurate burst control,
  but stores per-request entries or scripts more state and is overkill for a
  demo endpoint.
- **Redis fixed-window counter (`INCR` + `EXPIRE`).** One key per window, one
  atomic increment, TTL-based reset. Allows up to ~2× the nominal rate across a
  window boundary, which is acceptable here.

## Decision

Implement a **fixed-window** limiter backed by Redis, applied **only** to the
demo route `GET /ratelimit` (not the API globally).

- **Key.** `ratelimit:v1:<route>:<ip>:<windowStart>`, where `<windowStart>` is
  the request time truncated to the window (aligned wall-clock windows) and
  `<ip>` is the trusted-proxy-aware client IP. The `v1` prefix lets the key
  format evolve without colliding.
- **Counting.** A single atomic operation increments the counter and, **on the
  first increment only**, sets the window TTL (a Lua script does the `INCR` plus
  conditional `PEXPIRE` so the window is anchored to its first request and
  expires on its own). A request is allowed while the post-increment count is
  `≤ limit`, and rejected with `429` once it exceeds it.
- **Headers.** Every response carries `RateLimit-Limit`, `RateLimit-Remaining`
  (floored at 0), and `RateLimit-Reset` (seconds until the window ends, rounded
  up to at least 1); the `429` adds `Retry-After`.
- **Fail-open.** If the Redis call errors, the request is **allowed** and the
  headers report a full allowance. A cache outage degrades to no limiting rather
  than rejecting every caller.
- **Configuration.** `RATELIMIT_LIMIT` (default `5`) and `RATELIMIT_WINDOW`
  (default `10s`).

The limiter is wired as an optional server dependency (`WithRateLimiter`); when
unset (e.g. in unit tests) the endpoint never throttles. The route is public and
deliberately **not** session- or CSRF-protected.

## Consequences

Positive:

- The limit is enforced **consistently across all replicas**, because the
  counter is shared in Redis and advanced atomically.
- Windows are **self-expiring**: the TTL is set on the first increment, so no
  background cleanup is needed and counters never accumulate.
- A Redis outage **cannot take the endpoint down**: the limiter fails open, and
  the rest of the request path is unaffected.
- The limit and window are **configurable** without code changes, and only the
  demo route is affected, so the limiter cannot accidentally throttle the whole
  API.

Negative:

- A fixed window allows a burst of up to **~2× the limit** straddling a window
  boundary (the tail of one window plus the head of the next). Acceptable for a
  demo; a sliding window or token bucket would be needed for strict burst
  control.
- Failing open means that **during a Redis outage the route is unlimited**. That
  is the intended trade-off for a best-effort cache-backed control, but it is a
  trade-off: the limiter is a guardrail, not a security boundary.

Neutral:

- The window is keyed per client IP, so callers sharing a NAT/egress IP share a
  budget; this matches how the login lockout already attributes per-IP activity.
- Aligned wall-clock windows make the reset predictable and the key derivable
  from the request time alone, at the cost of the first window for a given client
  being shorter than a full window.
