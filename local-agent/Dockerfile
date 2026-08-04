# syntax=docker/dockerfile:1
# local-agent appliance: distroless/static + gantry + MCP tools from mcp.toml.
# Gantry binary is staged by `make prepare-docker-gantry` into .cache/docker-gantry
# (linux static build from this monorepo). MCP binaries are installed by
# `gantry tools-fetch` — same path as native deploy.
#
# Build:  make build   # or: make up
# Auth:   docs/google-workspace.md · docs/strava.md · docs/garmin.md ·
#         docs/web-search.md · docs/cast.md · docs/youtube.md · docs/math.md ·
#         docs/flights.md · docs/rentals.md · docs/cars.md

# Busts the tools-fetch layer so download_tag=latest re-resolves.
ARG TOOLS_CACHEBUST=0

# --- gantry (pre-built on the workstation / CI) ------------------------------
FROM debian:trixie-slim AS gantry
COPY .cache/docker-gantry /gantry
RUN chmod 0755 /gantry && /gantry version

# --- MCP tools via gantry tools-fetch + local-agent/mcp.toml -----------------
FROM debian:trixie-slim AS tools
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=gantry /gantry /usr/local/bin/gantry
COPY mcp.toml /mcp.toml
RUN case "${TARGETARCH}" in \
    amd64|arm64) ARCH="${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && mkdir -p /out/bin \
    && gantry tools-fetch \
        --manifest /mcp.toml \
        --os linux \
        --arch "${ARCH}" \
        --outdir /out/bin \
        --cache /tmp/mcp-cache \
    && ls -la /out/bin

# --- runtime: distroless/static + gantry + tool binaries ---------------------
# ca-certs + tzdata included; no shell, no libc. Healthchecks must be exec form.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=gantry /gantry /usr/local/bin/gantry
COPY --from=tools /out/bin/ /usr/local/bin/

WORKDIR /data
ENTRYPOINT ["/usr/local/bin/gantry"]
CMD ["run"]
