# Warder's core API.
#
# One image serves both HTTP surfaces. Which one is public is decided by the
# environment, not by the image: each deployment binds its own surface to the
# port the platform assigns and leaves the other on loopback. See render.yaml.

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
FROM golang:1.26-alpine AS build

WORKDIR /src

# Dependencies first, so a source change does not re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# CGO off produces a static binary that runs on a distroless base with no libc
# to match. -trimpath keeps build machine paths out of the artifact.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w -X github.com/Tobe0504/Warder/internal/cli.Version=${VERSION}" \
        -o /out/warder-api \
        ./cmd/warder-api

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------
#
# Distroless: no shell, no package manager, no busybox. A process that manages
# credentials should not be sharing its filesystem with tools that make an
# intrusion convenient. It also means there is nothing to `docker exec` into,
# which is the point.
FROM gcr.io/distroless/static-debian12:nonroot

# Runs as uid 65532. Nothing here needs root, and nothing writes to disk:
# plaintext is never cached, so there is no volume to mount.
USER nonroot:nonroot

COPY --from=build /out/warder-api /usr/local/bin/warder-api

# Documentation only; the platform maps whatever it assigns.
EXPOSE 8080 8081

ENTRYPOINT ["/usr/local/bin/warder-api"]
CMD ["serve"]
