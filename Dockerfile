ARG GO_VERSION=1.26
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

FROM golang:${GO_VERSION} AS builder
ARG GOPROXY
ARG GOSUMDB
ENV GOPROXY=${GOPROXY} \
  GOSUMDB=${GOSUMDB}

WORKDIR /src/server

COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go mod download

COPY server ./

ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api
RUN --mount=type=cache,target=/go/pkg/mod \
  --mount=type=cache,target=/root/.cache/go-build \
  go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM alpine:3.21 AS base
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
RUN mkdir -p ./.run/uploads

FROM base AS api
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/api /app/api
# 默认头像等静态种子：放到非挂载路径，由 entrypoint 在容器启动时同步进 uploads 卷
COPY server/seed /app/uploads-seed
COPY server/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/app/docker-entrypoint.sh"]

FROM base AS worker
RUN apk add --no-cache ffmpeg
COPY --from=builder /src/server/configs /app/configs
COPY --from=builder /out/worker /app/worker
ENTRYPOINT ["/app/worker"]
