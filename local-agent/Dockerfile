# local-agent appliance (ai-gantry/local-agent): distroless/static + gantry + static Go MCP tools.
# Kernel stays lean on Hub (shotah/ai-gantry); this image bakes the tool binaries.
# Every MCP child must be static — no libc or shell in the final image.
#
# Build (from this directory):  docker compose build   # or: make build
# Auth:   docs/google-workspace.md · docs/strava.md · docs/garmin.md ·
#         docs/web-search.md · docs/cast.md · docs/youtube.md · docs/math.md

# shotah/ai-gantry GitHub release (the runtime). Override via GANTRY_VERSION.
# Also published as shotah/ai-gantry on Docker Hub / GHCR.
ARG GANTRY_VERSION=v0.0.5
ARG STRAVA_MCP_VERSION=v0.0.2
# shotah/go-garmin release (DI auth + MCP). Override via GARMIN_MCP_VERSION.
ARG GARMIN_MCP_VERSION=v0.1.2
# Our packages: default `latest` (resolved at build). Pin e.g. v0.0.2 to freeze.
# TOOLS_CACHEBUST (from make/remote) busts Docker cache so latest re-resolves.
ARG GOOGLE_MCP_VERSION=latest
ARG GEMINI_SEARCH_MCP_VERSION=latest
ARG MCP_BEAM_VERSION=latest
ARG YOUTUBE_GO_MCP_VERSION=latest
ARG MCP_GO_MATH_VERSION=latest
ARG TOOLS_CACHEBUST=0

# --- fetch gantry (static Go; shotah/ai-gantry release) -----------------------
FROM debian:trixie-slim AS gantry
ARG GANTRY_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) GANTRY_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${GANTRY_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/ai-gantry/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved gantry latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/ai-gantry/releases/download/${VER}/gantry_${V}_${GANTRY_ARCH}.tar.gz" \
    -o /tmp/gantry.tar.gz \
    && tar -xzf /tmp/gantry.tar.gz -C /tmp \
    && install -m 0755 /tmp/gantry /gantry \
    && /gantry version

# --- fetch Google MCP (static Go; shotah/google-mcp) --------------------------
FROM debian:trixie-slim AS google-mcp
ARG GOOGLE_MCP_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) GWS_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${GOOGLE_MCP_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/google-mcp/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved google-mcp latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/google-mcp/releases/download/${VER}/google-mcp_${V}_${GWS_ARCH}.tar.gz" \
    -o /tmp/google-mcp.tar.gz \
    && tar -xzf /tmp/google-mcp.tar.gz -C /tmp \
    && install -m 0755 /tmp/google-mcp /google-mcp \
    && test -x /google-mcp

# --- fetch strava-mcp (static Go; shotah/go-strava-mcp) -----------------------
FROM debian:trixie-slim AS strava
ARG STRAVA_MCP_VERSION
ARG TARGETARCH

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) STRAVA_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && V="${STRAVA_MCP_VERSION#v}" \
    && curl -fsSL \
    "https://github.com/shotah/go-strava-mcp/releases/download/${STRAVA_MCP_VERSION}/strava-mcp_${V}_${STRAVA_ARCH}.tar.gz" \
    -o /tmp/strava.tar.gz \
    && tar -xzf /tmp/strava.tar.gz -C /tmp \
    && install -m 0755 /tmp/strava-mcp /strava-mcp \
    && /strava-mcp --version

# --- fetch garmin CLI + MCP (static Go; shotah/go-garmin release) ------------
FROM debian:trixie-slim AS garmin
ARG GARMIN_MCP_VERSION
ARG TARGETARCH

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) GARMIN_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && V="${GARMIN_MCP_VERSION#v}" \
    && curl -fsSL \
    "https://github.com/shotah/go-garmin/releases/download/${GARMIN_MCP_VERSION}/garmin_${V}_${GARMIN_ARCH}.tar.gz" \
    -o /tmp/garmin.tar.gz \
    && tar -xzf /tmp/garmin.tar.gz -C /tmp \
    && install -m 0755 /tmp/garmin /garmin \
    && /garmin --version

# --- fetch Gemini Google Search MCP (static Go; shotah/mcp-gemini-search) ----
FROM debian:trixie-slim AS gemini-search
ARG GEMINI_SEARCH_MCP_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) SEARCH_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${GEMINI_SEARCH_MCP_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/mcp-gemini-search/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved mcp-gemini-search latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/mcp-gemini-search/releases/download/${VER}/mcp-gemini-google-search_${V}_${SEARCH_ARCH}.tar.gz" \
    -o /tmp/mcp-gemini-google-search.tar.gz \
    && tar -xzf /tmp/mcp-gemini-google-search.tar.gz -C /tmp \
    && install -m 0755 /tmp/mcp-gemini-google-search /mcp-gemini-google-search \
    && /mcp-gemini-google-search -version

# --- fetch mcp-beam (static Go; Chromecast + DLNA + YouTube Cast MCP) ---------
# mDNS discovery needs host networking at runtime (docs/cast.md).
FROM debian:trixie-slim AS mcp-beam
ARG MCP_BEAM_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) BEAM_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${MCP_BEAM_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/mcp-beam/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved mcp-beam latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/mcp-beam/releases/download/${VER}/mcp-beam_${V}_${BEAM_ARCH}.tar.gz" \
    -o /tmp/mcp-beam.tar.gz \
    && tar -xzf /tmp/mcp-beam.tar.gz -C /tmp \
    && install -m 0755 /tmp/mcp-beam /mcp-beam \
    && /mcp-beam --version

# --- fetch youtube-go-mcp (static Go; YouTube Data API v3) --------------------
# OAuth via YOUTUBE_OAUTH_* (docs/youtube.md). Legacy YOUTUBE_OAUTH_* still accepted.
FROM debian:trixie-slim AS youtube-go-mcp
ARG YOUTUBE_GO_MCP_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) YTM_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${YOUTUBE_GO_MCP_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/youtube-go-mcp/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved youtube-go-mcp latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/youtube-go-mcp/releases/download/${VER}/youtube-go-mcp_${V}_${YTM_ARCH}.tar.gz" \
    -o /tmp/youtube-go-mcp.tar.gz \
    && tar -xzf /tmp/youtube-go-mcp.tar.gz -C /tmp \
    && install -m 0755 /tmp/youtube-go-mcp /youtube-go-mcp \
    && /youtube-go-mcp --version

# --- fetch mcp-go-math (static Go; calculator evaluate + unit convert) --------
FROM debian:trixie-slim AS mcp-go-math
ARG MCP_GO_MATH_VERSION
ARG TARGETARCH
ARG TOOLS_CACHEBUST=0

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/* \
    && case "${TARGETARCH}" in \
    amd64|arm64) MATH_ARCH="linux_${TARGETARCH}" ;; \
    *) echo "unsupported TARGETARCH=${TARGETARCH}" >&2; exit 1 ;; \
    esac \
    && : "TOOLS_CACHEBUST=${TOOLS_CACHEBUST}" \
    && VER="${MCP_GO_MATH_VERSION}" \
    && if [ "${VER}" = "latest" ]; then \
    VER=$(curl -fsSL -o /dev/null -w '%{url_effective}' \
    "https://github.com/shotah/mcp-go-math/releases/latest" \
    | sed 's|.*/||'); \
    echo "resolved mcp-go-math latest -> ${VER}"; \
    fi \
    && V="${VER#v}" \
    && curl -fsSL \
    "https://github.com/shotah/mcp-go-math/releases/download/${VER}/mcp-go-math_${V}_${MATH_ARCH}.tar.gz" \
    -o /tmp/mcp-go-math.tar.gz \
    && tar -xzf /tmp/mcp-go-math.tar.gz -C /tmp \
    && install -m 0755 /tmp/mcp-go-math /mcp-go-math \
    && /mcp-go-math --version

# --- runtime: distroless/static + gantry + tool binaries ----------------------
# ca-certs + tzdata included; no shell, no libc. Healthchecks must be exec form.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=gantry /gantry /usr/local/bin/gantry
COPY --from=google-mcp /google-mcp /usr/local/bin/google-mcp
COPY --from=strava /strava-mcp /usr/local/bin/strava-mcp
COPY --from=garmin /garmin /usr/local/bin/garmin
COPY --from=gemini-search /mcp-gemini-google-search /usr/local/bin/mcp-gemini-google-search
COPY --from=mcp-beam /mcp-beam /usr/local/bin/mcp-beam
COPY --from=youtube-go-mcp /youtube-go-mcp /usr/local/bin/youtube-go-mcp
COPY --from=mcp-go-math /mcp-go-math /usr/local/bin/mcp-go-math

WORKDIR /data
ENTRYPOINT ["/usr/local/bin/gantry"]
CMD ["run"]
