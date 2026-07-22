# syntax=docker/dockerfile:1

# Build on Alpine (musl toolchain is fine — we link statically).
FROM golang:1.26-alpine AS build
WORKDIR /src

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

ENV CGO_ENABLED=0
RUN go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/gantry \
    ./cmd/gantry

# Runtime: distroless/static — ca-certs + tzdata, no shell, uid 65532.
# Pin the Debian track (not :nonroot alone) so the base OS doesn't drift under us.
# MCP child binaries copied into persona images must also be static (CGO off)
# or link only against libs present here (effectively: none).
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/gantry /usr/local/bin/gantry

USER nonroot
WORKDIR /data

ENTRYPOINT ["/usr/local/bin/gantry"]
CMD ["run"]
