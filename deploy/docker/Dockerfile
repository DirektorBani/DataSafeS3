FROM golang:1.26.9-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/storage-server ./cmd/storage-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget su-exec
COPY --from=build /out/storage-server /storage-server
COPY deploy/docker/docker-entrypoint.sh /docker-entrypoint.sh
# Windows checkouts can deliver CRLF. A CR on the shebang is "no such file or directory".
# The launcher is created here (always LF) so a bind-mount over /docker-entrypoint.sh
# is stripped at start as well (HA local and local-binary overlays).
RUN tr -d '\015' < /docker-entrypoint.sh > /tmp/docker-entrypoint.lf \
 && mv /tmp/docker-entrypoint.lf /docker-entrypoint.sh \
 && chmod +x /docker-entrypoint.sh \
 && printf '%s\n' \
      '#!/bin/sh' \
      'set -e' \
      'tr -d "\015" < /docker-entrypoint.sh > /tmp/docker-entrypoint.lf' \
      'chmod +x /tmp/docker-entrypoint.lf' \
      'exec /tmp/docker-entrypoint.lf "$@"' \
      > /usr/local/bin/datasafe-entrypoint \
 && chmod +x /usr/local/bin/datasafe-entrypoint
EXPOSE 9000
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O- http://127.0.0.1:9000/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/datasafe-entrypoint"]
