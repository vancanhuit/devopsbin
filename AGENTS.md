# Development Guidelines

Use `mise` tasks consistently:

```sh
# Frontend
mise run web:format:check
mise run web:format
mise run web:lint
mise run web:check
mise run web:test
mise run web:build

# Backend
mise run api:generate
mise run api:format
mise run api:vulncheck
mise run api:lint
mise run api:test:unit
mise run api:test:integration
mise run api:build
mise run api:run

# Docker
mise run docker:hadolint
mise run docker:build
mise run docker:prune

# Compose
mise run compose:dev:up
mise run compose:dev:down
mise run compose:test:up
mise run compose:test:down

# Lint / smoke
mise run shellcheck:dev
mise run ruff:dev
mise run smoke:dev
```

## OpenAPI

- `api/openapi.yaml` is the single source of truth for the HTTP contract. Edit
  the spec first, then regenerate code — never hand-edit generated files.
- Regenerate both sides after any spec change: `mise run api:generate`
  (oapi-codegen → Go server types) and `mise run web:generate` (openapi-generator
  `typescript-fetch` → TS client). Commit the spec and regenerated output
  together so they never drift.
- Treat generated code as read-only build output: don't patch it, and reuse the
  generated types instead of redeclaring request/response shapes by hand.
- Target OpenAPI 3.1; keep the spec valid and lint-clean before generating.
- Model the contract explicitly: name every schema, mark `required` fields, set
  `nullable`/formats/enums precisely, and reuse `$ref` components instead of
  duplicating inline shapes.
- Define error responses (and their schema) for every operation, not just the
  happy path; give each operation a stable, unique `operationId`.
- Evolve the API backward-compatibly: add optional fields rather than changing or
  removing existing ones; version via the path prefix (`/api/v1`) for breaking
  changes.

## Go

- Run `mise run api:format` and `mise run api:lint` before committing; keep the
  build clean with `mise run api:vulncheck`.
- Accept `context.Context` as the first parameter of any function that does I/O
  and propagate it; never store a context in a struct.
- Wrap errors with `fmt.Errorf("...: %w", err)` to preserve the chain; inspect
  with `errors.Is`/`errors.As` rather than string matching. Handle every error —
  never discard one with `_` unless intentional and commented.
- Return early on errors to keep the happy path unindented.
- `defer` cleanup (`Close`, `Rollback`, `Unlock`) right after acquiring the
  resource; check the error of deferred `Close` on writes.
- Guard shared state with a mutex or channel; run tests and CI with `-race`.
- Start goroutines with a clear lifecycle (ctx cancellation or `errgroup`); never
  leak them. Don't start a goroutine you can't stop.
- Keep interfaces small and define them at the consumer, not the producer.
- Use the standard `log/slog` for structured logging; never `log.Fatal`/`panic`
  outside `main` or truly unrecoverable startup paths.

## Database / transactions

- Use the request `context.Context` for every query so cancellation and timeouts
  propagate to the database.
- Run multi-statement units of work in a single transaction (`pgx` `BeginTx` /
  `pool.Begin`). `defer tx.Rollback(ctx)` immediately — a rollback after a
  successful `Commit` is a safe no-op — and only `Commit` on the success path.
- Keep transactions short: do no network calls or slow work while holding one,
  to avoid long-held locks and connection-pool starvation.
- Always use parameterized queries (never string-concatenate SQL) to prevent
  injection.
- Set an explicit isolation level when correctness depends on it, and be ready to
  retry on serialization failures.
- Always `Close`/release rows and check `rows.Err()` after iterating.
- Make schema migrations forward-only and backward-compatible (expand/contract);
  never edit an already-applied migration.

## Redis (go-redis v9)

- Treat Redis as a cache, not a source of truth: tolerate misses and evictions,
  and always be able to rebuild a value from the database.
- Pass the request `context.Context` to every command so timeouts and
  cancellation propagate; use a bounded timeout for cache calls.
- Distinguish a cache miss from a real error: check `errors.Is(err, redis.Nil)`
  and fall through to the source instead of failing the request.
- Never let a cache failure break the request path — log and degrade gracefully
  (serve from the DB) rather than returning an error to the caller.
- Always set an explicit TTL on cache keys (`SET ... EX`); never write keys that
  live forever, and prefer a small jitter on TTLs to avoid thundering-herd
  expiry.
- Namespace keys with a clear, versioned prefix (e.g. `link:v1:<id>`) so formats
  can evolve without colliding.
- Batch round-trips with pipelines and keep values small; serialize with a single
  agreed format. Avoid unbounded keys/values.
- On writes, keep the cache consistent with the DB (write-through or invalidate
  after the committed transaction); don't update the cache before the DB commit.
- Use atomic operations (`INCR`, `SETNX`, Lua scripts) instead of read-modify-write
  races, and set a TTL on any lock keys.

## TypeScript

- Run `mise run web:check` (svelte-check + tsc) and `mise run web:lint` before
  committing; format with `mise run web:format`.
- Keep `strict` on; do not use `any` — prefer `unknown` plus narrowing, generics,
  or precise types. Avoid non-null assertions (`!`); narrow instead.
- Prefer `type`/`interface` definitions over inline shapes; derive types from a
  single source of truth and reuse the generated API client types rather than
  redeclaring response shapes.
- Use `const` by default, discriminated unions for state, and exhaustive
  `switch` (with a `never` default) so new cases are caught at compile time.
- Handle `null`/`undefined` explicitly with optional chaining and nullish
  coalescing; avoid truthiness checks that hide `0`/`""`.
- Keep modules side-effect free where possible and use `import type` for
  type-only imports.

## Svelte (5, runes)

- Use runes: `$state` for reactive state, `$derived` for computed values (never
  recompute manually), and `$effect` only for genuine side effects — not to sync
  derived state.
- Declare component inputs with `$props()` and keep props read-only; communicate
  upward via callback props or events, not by mutating parent state.
- Prefer `$derived` over `$effect` for transformations; an `$effect` that only
  assigns to another `$state` is usually a `$derived` in disguise.
- Keep markup logic minimal — move non-trivial computation into `$derived` or
  helpers; use keyed `{#each}` blocks (`{#each items as item (item.id)}`) for
  stable list updates.
- Scope styles to the component (default Svelte scoping) and use Tailwind
  utilities for layout; avoid global styles outside the app shell.
- Guard browser-only APIs and clean up listeners/timers in the `$effect` return
  function.
- Keep components small and focused; lift shared reactive logic into `.svelte.ts`
  modules using runes.
