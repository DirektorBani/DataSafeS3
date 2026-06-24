FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/storage-server ./cmd/storage-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget su-exec
COPY --from=build /out/storage-server /storage-server
COPY deploy/docker/docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh
EXPOSE 9000
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O- http://127.0.0.1:9000/healthz || exit 1
ENTRYPOINT ["/docker-entrypoint.sh"]
