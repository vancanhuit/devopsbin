# syntax=docker/dockerfile:1

FROM debian:13-slim AS builder

ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update  \
    && apt-get -y upgrade \
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
ENV PATH="/mise/shims:$PATH"
ENV GOPATH="/go"

RUN curl https://mise.run | sh

WORKDIR /app

COPY mise.toml mise.toml
COPY mise.lock mise.lock
COPY mise/ mise/

RUN --mount=type=cache,target=/mise/cache mise trust && mise install

COPY go.mod go.mod
COPY go.sum go.sum
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY web/package.json web/package.json
COPY web/bun.lock web/bun.lock
RUN --mount=type=cache,target=/root/.bun/install/cache cd web && bun install --frozen-lockfile

COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY web/ web/

# Build metadata injected from the host (see compose.yaml build.args). These
# are read by mise/tasks/api/build and baked into the binary via ldflags.
ARG VERSION=unknown
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
ENV VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    BUILD_TIME=${BUILD_TIME}

ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    mise run api:build

FROM gcr.io/distroless/static-debian13 AS runtime
COPY --from=builder /app/bin/devopsbin /devopsbin

ENTRYPOINT ["/devopsbin"]