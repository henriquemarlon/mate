# syntax=docker/dockerfile:1

################################################################################
# Create a stage for building a statically linked Mate executable.
ARG GO_VERSION=1.25.0
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# go-sqlite3 requires CGO. Alpine's musl toolchain lets the final executable
# remain static while retaining SQLite support.
RUN apk add --no-cache build-base

# Download dependencies as a separate step to take advantage of Docker's cache.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=go.sum,target=go.sum \
    --mount=type=bind,source=go.mod,target=go.mod \
    go mod download -x

ARG VERSION=devel

# Bind the source instead of copying it so source changes do not invalidate the
# dependency layer. BuildKit runs this stage on the requested target platform,
# which keeps CGO cross-platform builds correct under emulation.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=bind,target=. \
    CGO_ENABLED=1 go build \
        -trimpath \
        -tags "osusergo netgo sqlite_omit_load_extension" \
        -ldflags="-s -w -linkmode external -extldflags '-static' -X github.com/henriquemarlon/mate/internal/infra/version.BuildVersion=${VERSION}" \
        -o /bin/mate ./cmd/mate

################################################################################
# Install the official standalone Codex package in an isolated stage.
FROM alpine:3.22 AS codex

ARG CODEX_VERSION=0.147.0

RUN apk add --no-cache ca-certificates curl \
    && curl -fsSL https://chatgpt.com/codex/install.sh -o /tmp/install-codex.sh \
    && CODEX_HOME=/opt/codex \
       CODEX_INSTALL_DIR=/usr/local/bin \
       CODEX_NON_INTERACTIVE=1 \
       CODEX_RELEASE="${CODEX_VERSION}" \
       sh /tmp/install-codex.sh \
    && rm /tmp/install-codex.sh

################################################################################
# Create the minimal runtime image with Mate's two external runtime tools:
# Codex for model execution and Poppler's pdftoppm for rendering PDF pages.
FROM alpine:3.22 AS final

RUN --mount=type=cache,target=/var/cache/apk \
    apk --update add \
        ca-certificates \
        poppler-utils \
        tzdata \
    && update-ca-certificates

# Codex stores authentication and runtime state below the user's home directory.
ARG UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/home/mate" \
    --shell "/bin/sh" \
    --uid "${UID}" \
    mate

COPY --from=build /bin/mate /usr/local/bin/mate
COPY --from=codex /opt/codex /opt/codex
RUN ln -s /opt/codex/packages/standalone/current/bin/codex /usr/local/bin/codex

USER mate
WORKDIR /home/mate

ENTRYPOINT ["mate"]
CMD ["run"]
