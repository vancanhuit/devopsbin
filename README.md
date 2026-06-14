# devopsbin

A small, self-contained HTTP service that exposes a grab-bag of **runtime,
health-probe, request-reflection, and fault-injection endpoints** — the kind of
utilities you reach for when wiring up, debugging, and load-testing
infrastructure (Kubernetes probes, reverse proxies, service meshes, TLS/mTLS,
ingress, observability).

Think of it as an httpbin-style "DevOps Swiss Army knife": point it behind a
proxy, hit `/scheme` to confirm TLS termination, hit `/ip` and `/headers` to
verify forwarded-header handling, hit `/status/{code}` and `/delay/{seconds}`
to exercise retry/timeout logic, and use the `/livez` `/readyz` `/startupz`
probes as ready-made health checks.

## Purpose

- **Validate deployment topologies** — direct HTTP, TLS termination at the
  binary, TLS at a reverse proxy, and mutual TLS between proxy and binary.
- **Exercise health-checking** — distinct liveness, readiness, and startup
  semantics, including dependency pings to PostgreSQL and Redis.
- **Reflect requests** — echo the caller IP, headers, user agent, and request
  scheme, honoring forwarded headers only from trusted proxies.
- **Inject faults** — return an arbitrary HTTP status or a bounded artificial
  delay to test client resilience.
- **Serve as a reference** — a compact, well-tested example of a 12-factor Go
  service with an OpenAPI-first contract and an embedded SPA console.

## Technology stack

| Area            | Choice                                                             |
| --------------- | ------------------------------------------------------------------ |
| Language        | Go 1.26                                                            |
| HTTP router     | [chi v5](https://github.com/go-chi/chi)                            |
| CLI             | [urfave/cli v3](https://github.com/urfave/cli)                     |
| Config          | [caarlos0/env](https://github.com/caarlos0/env) (env-var driven)  |
| PostgreSQL      | [pgx v5](https://github.com/jackc/pgx)                             |
| Redis           | [go-redis v9](https://github.com/redis/go-redis) (standalone / cluster / sentinel) |
| API contract    | OpenAPI 3, server types via oapi-codegen, TS client via openapi-generator |
| Logging         | stdlib `log/slog` (structured JSON)                                |
| Frontend (SPA)  | Svelte 5 + Vite + TailwindCSS (embedded into the binary)           |
| Docs UI         | Swagger UI + Redoc (embedded)                                      |
| Tooling         | [mise](https://mise.jdx.dev) tasks, Docker Compose, mkcert, Caddy  |

The server, the built SPA, and the API docs are all embedded into a single
static Go binary — there is nothing to deploy alongside it except its data
stores.

## API endpoints

All endpoints live under the `/api/v1` base path. The source of truth is
[`api/openapi.yaml`](api/openapi.yaml).

| Method | Path                  | Tag     | Description                                              |
| ------ | --------------------- | ------- | ------------------------------------------------------- |
| GET    | `/livez`              | Runtime | Process-only liveness (always 200 while running).       |
| GET    | `/readyz`             | Runtime | Readiness; 503 when a dependency is down.               |
| GET    | `/startupz`           | Runtime | Startup completion; 503 while still starting.           |
| GET    | `/version`            | Runtime | Build and version metadata.                             |
| GET    | `/uuid`               | Inspect | Generate a random UUID.                                 |
| GET    | `/ip`                 | Inspect | Caller's origin IP (trusted-proxy aware).               |
| GET    | `/headers`            | Inspect | Echo the request headers.                               |
| GET    | `/user-agent`         | Inspect | Echo the `User-Agent` header.                           |
| GET    | `/scheme`             | Inspect | Report `http` or `https` (trusted-proxy aware).         |
| ALL\*  | `/echo`               | Inspect | Echo method, path, query, headers, origin, scheme, and request body. |
| GET    | `/status/{code}`      | Status  | Return the caller-specified HTTP status code.           |
| GET    | `/delay/{seconds}`    | Latency | Respond after a bounded artificial delay.               |

\* `/echo` accepts `GET`, `POST`, `PUT`, `PATCH`, and `DELETE`; the body
methods reflect the request body back.

The root path serves the SPA console; API docs are available via Swagger UI and
Redoc.

## Configuration

All settings come from environment variables (12-factor style), grouped by a
per-section prefix. Defaults are tuned for production; override individual
variables for local development.

### App (`APP_`)

| Variable         | Default | Description                                   |
| ---------------- | ------- | --------------------------------------------- |
| `APP_VERSION`    | `dev`   | Reported version string.                      |
| `APP_GIT_SHA`    | `none`  | Reported build commit.                        |
| `APP_BUILD_TIME` | `none`  | Reported build timestamp.                     |
| `APP_LOG_LEVEL`  | `info`  | Log level: `debug`, `info`, `warn`, `error`.  |

### HTTP (`HTTP_`)

| Variable                  | Default | Description                                                                 |
| ------------------------- | ------- | --------------------------------------------------------------------------- |
| `HTTP_ADDR`               | `:8080` | Listen address.                                                             |
| `HTTP_READ_TIMEOUT`       | `5s`    | Read timeout.                                                               |
| `HTTP_WRITE_TIMEOUT`      | `10s`   | Write timeout.                                                              |
| `HTTP_IDLE_TIMEOUT`       | `60s`   | Keep-alive idle timeout.                                                    |
| `HTTP_SHUTDOWN_TIMEOUT`   | `15s`   | Graceful-shutdown grace period.                                            |
| `HTTP_REQUEST_TIMEOUT`    | `60s`   | Per-request timeout.                                                        |
| `HTTP_TLS_CERT_FILE`      | —       | PEM certificate (chain). Set with the key to serve HTTPS directly.         |
| `HTTP_TLS_KEY_FILE`       | —       | PEM private key for the certificate above.                                 |
| `HTTP_TLS_CLIENT_CA_FILE` | —       | PEM CA bundle to verify client certs (**enables mTLS**; requires TLS above). |
| `HTTP_TRUSTED_PROXIES`    | —       | Comma-separated CIDRs whose forwarded headers (`X-Forwarded-*`) are honored. |

TLS modes derived from the above:

- Both cert/key empty → **plain HTTP** (TLS terminated upstream).
- Cert + key set → **direct HTTPS**.
- Cert + key + client CA set → **mutual TLS** (`RequireAndVerifyClientCert`).

### PostgreSQL (`POSTGRES_`)

| Variable       | Default                                                                  | Description           |
| -------------- | ------------------------------------------------------------------------ | --------------------- |
| `POSTGRES_URL` | `postgres://user:password@localhost:5432/dbname?sslmode=disable`         | Connection URL.       |

### Redis (`REDIS_`)

| Variable             | Default          | Description                                                              |
| -------------------- | ---------------- | ----------------------------------------------------------------------- |
| `REDIS_MODE`         | `standalone`     | Topology: `standalone`, `cluster`, or `sentinel`.                       |
| `REDIS_ADDRS`        | `localhost:6379` | Comma-separated `host:port` nodes (one for standalone; seeds/sentinels otherwise). |
| `REDIS_MASTER_NAME`  | —                | Monitored primary name (required in `sentinel` mode).                   |
| `REDIS_DB`           | `0`              | Logical DB index (standalone/sentinel only; cluster supports only 0).   |
| `REDIS_USERNAME`     | —                | ACL username (optional).                                                |
| `REDIS_PASSWORD`     | —                | Password (never logged or serialized).                                  |
| `REDIS_TLS`          | `false`          | Enable an in-transit-encrypted connection.                              |

## CLI

The binary is a small command tree:

```sh
devopsbin run            # run the backend HTTP API server
devopsbin migrate up     # apply all pending database migrations
devopsbin migrate status # show the state of every migration
devopsbin migrate version# print the current schema version
devopsbin healthcheck    # probe /livez and exit 0 on 200 (used as the Docker HEALTHCHECK)
```

`healthcheck` supports `--url`, `--timeout`, and, for TLS/mTLS targets,
`--cacert` (verify the server) plus `--cert`/`--key` (present a client
certificate). This lets a distroless image health-check itself without curl or
wget.

`migrate` reads `POSTGRES_URL` and applies the migrations embedded in the
binary. Migrations run **only** via this explicit command — the server never
migrates on startup — so a deployment runs `devopsbin migrate up` as a discrete
step. A Postgres session-level advisory lock serializes concurrent runners, so
it is safe to invoke from multiple replicas or init containers at once.

## Database schema and demo data

The schema is defined by forward-only [goose](https://github.com/pressly/goose)
migrations under [`migrations/`](migrations), embedded into the binary and
applied with `devopsbin migrate up`:

| Table       | Purpose                                                                   |
| ----------- | ------------------------------------------------------------------------- |
| `users`     | Account holders (`username`, `password_hash`, `role` of `user`/`admin`).  |
| `accounts`  | Money accounts owned by a user, with a non-negative `balance_cents`.      |
| `transfers` | Append-only ledger of money moved between accounts (`amount_cents > 0`).  |

Data access is generated with [sqlc](https://sqlc.dev) into
`internal/store/sqlc` from the queries in `internal/store/queries`; regenerate
it with `mise run api:sqlc` (folded into `mise run api:generate`).

The final migration seeds two demo users (passwords are bcrypt hashes baked into
the seed SQL), each with a starter checking account of 1000.00:

| Username | Password    | Role    |
| -------- | ----------- | ------- |
| `alice`  | `alicepass` | `user`  |
| `admin`  | `adminpass` | `admin` |

These credentials are intentionally public — they exist only to make the demo
runnable out of the box and must never be used in a real deployment.


## Deployment topologies

The service is designed to run in several proxy/TLS arrangements. Each is
captured as a Docker Compose **profile** in [`compose.yaml`](compose.yaml) and
driven by `mise` tasks.

| Profile    | Topology                                                                                  | Redis             | Browser entry        |
| ---------- | ----------------------------------------------------------------------------------------- | ----------------- | -------------------- |
| `dev`      | Plain HTTP, binary exposed directly.                                                       | standalone        | http://localhost:8080 |
| `cluster`  | Plain HTTP against a 3-master/3-replica Redis Cluster.                                     | cluster           | (smoke-tested)       |
| `sentinel` | Plain HTTP against a Redis Sentinel-managed primary.                                       | sentinel          | (smoke-tested)       |
| `tls`      | Binary serves **HTTPS directly** (`:8443`) **and** sits behind a TLS-terminating Caddy proxy. | standalone        | https://localhost:8443 (direct) / https://localhost:9443 (proxy) |
| `mtls`     | Binary serves HTTPS and **requires a client cert**; Caddy terminates browser TLS and **re-encrypts to the backend over mTLS**. | standalone        | https://localhost:9444 (proxy) |

Notes:

- The `tls` and `mtls` profiles use **mkcert**-issued certificates. For local
  development run `mise run certs:install` once to install a browser-trusted CA;
  CI and smoke tests use a throwaway ephemeral CA instead.
- In the `mtls` profile, the backend's direct port (`https://localhost:8444`)
  **rejects** plain browser requests by design — reach it through the Caddy
  proxy on `9444`, which presents the client certificate.
- The `tls`/`mtls` profiles use dedicated data stores and pin Caddy to a static
  IP, trusted via `HTTP_TRUSTED_PROXIES`, so forwarded headers cannot be
  spoofed by other peers.

### Bringing a topology up

```sh
mise run compose:dev:up         # plain HTTP dev stack
mise run compose:cluster:up     # Redis Cluster
mise run compose:sentinel:up    # Redis Sentinel
mise run compose:tls:up         # direct HTTPS + TLS proxy (mkcert dev certs)
mise run compose:mtls:up        # mutual TLS backend + re-encrypting proxy

# tear down (…:down), e.g.
mise run compose:dev:down
```

## Development

This repo uses [`mise`](https://mise.jdx.dev) tasks; see
[`AGENTS.md`](AGENTS.md) for the full list. Common ones:

```sh
# Backend
mise run api:generate        # regenerate Go types from api/openapi.yaml
mise run api:test:unit
mise run api:lint

# Frontend
mise run web:generate        # regenerate the TypeScript client
mise run web:build
mise run web:test

# App (Go server with the embedded SPA)
mise run app:build
mise run app:run

# Smoke tests (own their compose lifecycle)
mise run smoke:dev
mise run smoke:cluster
mise run smoke:sentinel
mise run smoke:tls
mise run smoke:mtls
```

`api/openapi.yaml` is the single source of truth for the HTTP contract: edit
the spec first, then regenerate both the Go server types and the TypeScript
client. Generated code is treated as read-only build output.
