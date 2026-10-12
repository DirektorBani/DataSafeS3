FROM golang:1.26.9-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# franz-go lives in a second static binary. The server does not import it.
# The child is started only when STORAGE_KAFKA_BROKERS is set. No CGO.
# release.yml passes VERSION and GIT_COMMIT. A build with no args reads VERSION
# and reports commit "unknown".
ARG VERSION=dev
ARG GIT_COMMIT=unknown
RUN set -eu; \
    ver="$VERSION"; \
    if [ "$ver" = "dev" ] && [ -f VERSION ]; then ver="$(tr -d '[:space:]' < VERSION)"; fi; \
    commit="${GIT_COMMIT:-unknown}"; \
    if [ -z "$commit" ]; then commit=unknown; fi; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -tags datasafe_sinkexec \
      -ldflags="-s -w -X github.com/DirektorBani/datasafe/internal/buildinfo.Version=$${ver} -X github.com/DirektorBani/datasafe/internal/buildinfo.Commit=$${commit}" \
      -o /out/storage-server ./cmd/storage-server \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" \
      -o /out/datasafe-kafka-bridge ./cmd/datasafe-kafka-bridge

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget su-exec
COPY --from=build /out/storage-server /storage-server
COPY --from=build /out/datasafe-kafka-bridge /usr/lib/datasafe/datasafe-kafka-bridge
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
