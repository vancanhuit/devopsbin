# syntax=docker/dockerfile:1

FROM --platform=${BUILDPLATFORM} debian:13-slim AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update  \
    && apt-get -y --no-install-recommends install  \
        # install any other dependencies you might need
        curl git tar xz-utils unzip ca-certificates build-essential \
    && apt-get -y autoremove \
    && apt-get -y autoclean \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV MISE_DATA_DIR="/mise"
ENV MISE_CONFIG_DIR="/mise"
ENV MISE_CACHE_DIR="/mise/cache"
ENV MISE_INSTALL_PATH="/usr/local/bin/mise"
ENV MISE_VERSION="2026.6.1"
ENV PATH="/mise/shims:$PATH"
ENV GOPATH="/go"

RUN curl https://mise.run | sh

WORKDIR /app

COPY mise.toml mise.toml
COPY mise.lock mise.lock

RUN --mount=type=cache,target=/mise/cache \
    mise trust && mise install

COPY go.mod go.mod
COPY go.sum go.sum
# Module source is platform-independent; sharing=locked serializes the two
# per-platform builder passes so the first populates the module cache and the
# second reuses it instead of racing to download the same modules.
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download

COPY web/package.json web/package.json
COPY web/bun.lock web/bun.lock
WORKDIR /app/web
# Bun packages are platform-independent too; lock the install cache so the
# per-platform passes install once and share the result.
RUN --mount=type=cache,target=/root/.bun/install/cache,sharing=locked \
    bun install --frozen-lockfile
WORKDIR /app

COPY mise/ mise/

COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/ web/

# Build metadata (injected from the host; see compose.yaml build.args) and the
# target platform are scoped to the build RUN below as inline environment
# instead of ENV layers. COMMIT/BUILD_TIME change on every commit, so promoting
# them to ENV would create a layer that busts each build and is re-exported into
# the buildx cache; keeping them inline limits the churn to the one RUN that
# actually bakes them into the binary via ldflags.
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    VERSION="${VERSION}" COMMIT="${COMMIT}" BUILD_TIME="${BUILD_TIME}" \
    mise run api:build

FROM gcr.io/distroless/static-debian13 AS runtime
COPY --from=builder /app/bin/devopsbin /devopsbin

# distroless ships no shell or curl/wget, so probe via the binary's own
# healthcheck subcommand (exec form -- there is no shell to interpret it).
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD ["/devopsbin", "healthcheck"]

ENTRYPOINT ["/devopsbin"]
