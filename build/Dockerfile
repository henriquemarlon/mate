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
# Create the minimal runtime image with Mate's one external runtime tool:
# Poppler's pdftoppm for PDF rendering. Model execution goes over HTTPS.
FROM alpine:3.22 AS final

LABEL org.opencontainers.image.source="https://github.com/henriquemarlon/mate"
LABEL org.opencontainers.image.description="Turn locally synced GoodNotes PDFs into auditable study material"

RUN --mount=type=cache,target=/var/cache/apk \
    apk --update add \
        ca-certificates \
        poppler-utils \
        tzdata \
    && update-ca-certificates

ARG UID=10001
RUN adduser \
    --disabled-password \
    --gecos "" \
    --home "/home/mate" \
    --shell "/bin/sh" \
    --uid "${UID}" \
    mate

COPY --from=build /bin/mate /usr/local/bin/mate

USER mate
WORKDIR /home/mate

ENTRYPOINT ["mate"]
CMD ["run"]
