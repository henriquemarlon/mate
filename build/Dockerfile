FROM golang:1.25-alpine AS builder

ARG VERSION=devel

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w -X github.com/henriquemarlon/mate/internal/infra/version.BuildVersion=${VERSION}" \
    -o /app/mate ./cmd/mate

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /app/mate /usr/local/bin/mate

ENTRYPOINT ["mate"]
