FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /build

COPY go.mod go.sum ./

RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 \
    GOOS=$TARGETOS \
    GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/tsnet-proxy .

FROM alpine:latest

RUN apk add --no-cache ca-certificates \
    && adduser -D -u 65532 tsnet-proxy \
    && mkdir -p /var/lib/tsnet-proxy \
    && chown tsnet-proxy:tsnet-proxy /var/lib/tsnet-proxy

COPY --from=builder /out/tsnet-proxy /usr/local/bin/tsnet-proxy

USER tsnet-proxy

ENV TSNET_PROXY_STATE_DIR=/var/lib/tsnet-proxy

VOLUME /var/lib/tsnet-proxy

ENTRYPOINT [ "tsnet-proxy" ]
