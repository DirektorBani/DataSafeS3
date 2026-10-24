# DataSafeS3 storage server.
#
# The final stage has no RUN: it only COPYs files prepared in the build stage
# (Go cross-compiles, so the build stage always runs on the build host's
# architecture). That is what lets the same image be assembled without a
# Docker daemon and without emulation (scripts/build-images.sh, tools/imgbuild).
#
# BASE_REGISTRY lets a build use a mirror instead of Docker Hub, for example
#   --build-arg BASE_REGISTRY=dh-mirror.gitverse.ru/library/
ARG BASE_REGISTRY=
ARG ALPINE_VERSION=3.24
ARG GO_VERSION=1.26.10

FROM --platform=$BUILDPLATFORM ${BASE_REGISTRY}golang:${GO_VERSION}-alpine AS build
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# franz-go lives in a second static binary. The server does not import it.
# The child is started only when STORAGE_KAFKA_BROKERS is set. No CGO.
# VERSION and GIT_COMMIT come from the release pipeline. A build with no args
# reads VERSION and reports commit "unknown". The link step is a script so
# Docker does not eat the shell variables (a $$ in RUN becomes the build
# container's pid).
ARG VERSION=dev
ARG GIT_COMMIT=unknown
RUN chmod +x deploy/docker/link-version.sh \
 && TARGETARCH="$TARGETARCH" VERSION="$VERSION" GIT_COMMIT="$GIT_COMMIT" ./deploy/docker/link-version.sh \
 && tr -d '\015' < deploy/docker/docker-entrypoint.sh > /out/docker-entrypoint.sh \
 && tr -d '\015' < deploy/docker/entrypoint-launcher.sh > /out/datasafe-entrypoint \
 && chmod 0755 /out/docker-entrypoint.sh /out/datasafe-entrypoint /out/su-exec

FROM ${BASE_REGISTRY}alpine:${ALPINE_VERSION}
# ca-certificates-bundle and busybox wget come with the Alpine base image, so
# nothing is installed here. su-exec is our own static binary (cmd/su-exec).
COPY --from=build /out/storage-server /storage-server
COPY --from=build /out/datasafe-kafka-bridge /usr/lib/datasafe/datasafe-kafka-bridge
COPY --from=build /out/su-exec /usr/local/bin/su-exec
COPY --from=build /out/docker-entrypoint.sh /docker-entrypoint.sh
COPY --from=build /out/datasafe-entrypoint /usr/local/bin/datasafe-entrypoint
EXPOSE 9000
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O- http://127.0.0.1:9000/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/datasafe-entrypoint"]
