ARG GO_BUILD_IMAGE=golang:1.26.5-bookworm
ARG GO_RUNTIME_IMAGE=debian:bookworm-slim
ARG GO_MODULE_PROXY=https://goproxy.io,https://proxy.golang.org,direct
ARG BUILD_REVISION=unknown

FROM ${GO_BUILD_IMAGE} AS test

ARG GO_MODULE_PROXY

WORKDIR /src

ENV CGO_ENABLED=0
ENV GOFLAGS="-trimpath -mod=readonly"
ENV GOPROXY=${GO_MODULE_PROXY}
ENV GOSUMDB=sum.golang.org
ENV GOTOOLCHAIN=local
ENV GOWORK=off

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    go test ./...

FROM test AS build

RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/admin-api ./cmd/admin-api && \
    go build -ldflags="-s -w" -o /out/admin-worker ./cmd/admin-worker

FROM ${GO_RUNTIME_IMAGE} AS runtime

ARG BUILD_REVISION

LABEL org.opencontainers.image.revision="${BUILD_REVISION}"

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates tzdata curl && \
    rm -rf /var/lib/apt/lists/* && \
    groupadd -r -g 10001 app && \
    useradd -r -u 10001 -g app -d /app -s /usr/sbin/nologin app

WORKDIR /app

RUN mkdir -p /app/runtime/logs /app/runtime/cert/alipay /app/exports && \
    chown -R app:app /app

COPY --chown=app:app --from=build /out/admin-api /app/admin-api
COPY --chown=app:app --from=build /out/admin-worker /app/admin-worker

ENV APP_ENV=production
ENV HTTP_ADDR=:8080
ENV TZ=Asia/Shanghai

EXPOSE 8080

USER app

CMD ["/app/admin-api"]
