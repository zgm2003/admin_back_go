# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26.1

FROM golang:${GO_VERSION}-bookworm AS build

WORKDIR /src

ENV CGO_ENABLED=0
ENV GOFLAGS=-trimpath
ENV GOPROXY=https://goproxy.io,direct

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/admin-api ./cmd/admin-api && \
    go build -ldflags="-s -w" -o /out/admin-worker ./cmd/admin-worker

FROM debian:bookworm-slim AS runtime

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd -r -g 10001 app && \
    useradd -r -u 10001 -g app -d /app -s /usr/sbin/nologin app

WORKDIR /app

COPY --from=build /out/admin-api /app/admin-api
COPY --from=build /out/admin-worker /app/admin-worker

RUN mkdir -p /app/runtime/logs /app/runtime/cert/alipay /app/exports && \
    chown -R app:app /app

ENV APP_ENV=production
ENV HTTP_ADDR=:8080
ENV TZ=Asia/Shanghai

EXPOSE 8080

USER app

CMD ["/app/admin-api"]
