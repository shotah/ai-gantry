.DEFAULT_GOAL := help

COMPOSE := docker compose
SERVICE := gantry
ENV_FILE := .env
ENV_EXAMPLE := .env.example
PERSONA_DIR := persona

# Bust mcp-beam / youtube-go-mcp / mcp-go-math fetch stages so `latest` re-resolves each build.
ifeq ($(OS),Windows_NT)
  TOOLS_CACHEBUST ?= $(shell powershell -NoProfile -Command "[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()")
else
  TOOLS_CACHEBUST ?= $(shell date +%s)
endif

ifeq ($(OS),Windows_NT)
  ENV_COPY := powershell -NoProfile -Command "if (-not (Test-Path '$(ENV_FILE)')) { Copy-Item '$(ENV_EXAMPLE)' '$(ENV_FILE)'; Write-Host 'Created $(ENV_FILE) — edit GEMINI_API_KEY and Telegram vars' } else { Write-Host '$(ENV_FILE) already exists (use make env-force to overwrite)' }"
  ENV_FORCE := powershell -NoProfile -Command "Copy-Item '$(ENV_EXAMPLE)' '$(ENV_FILE)' -Force; Write-Host 'Overwrote $(ENV_FILE)'"
  MKDIR_DATA := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path data, 'data/.config/google-mcp/credentials', 'data/.config/strava', 'data/.config/garmin', 'data/.config/youtube' | Out-Null"
  RM_GARMIN_SESSION := powershell -NoProfile -Command "Remove-Item -Force -ErrorAction SilentlyContinue 'data/.config/garmin/session.json'"
  # Non-empty DEPLOY_HOST in .env → push secrets after *-auth
  HAS_DEPLOY_HOST := $(shell powershell -NoProfile -Command "$$l = Get-Content .env -ErrorAction SilentlyContinue | Where-Object { $$_ -match '^DEPLOY_HOST=' } | Select-Object -First 1; if ($$l -and (($$l -split '=',2)[1].Trim())) { '1' }")
  # SOUL.example.md → SOUL.md  ($$ → $ for Make; $$_ → $_ in PowerShell)
  PERSONA_COPY := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(PERSONA_DIR)' | Out-Null; Get-ChildItem '$(PERSONA_DIR)\*.example.md' | ForEach-Object { $$dest = Join-Path $$_.DirectoryName ($$_.Name.Replace('.example.md','.md')); if (-not (Test-Path $$dest)) { Copy-Item $$_.FullName $$dest; Write-Host ('Created ' + $$dest + ' — edit personal details (gitignored)') } else { Write-Host ($$dest + ' already exists (use make persona-force to overwrite)') } }"
  PERSONA_FORCE := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(PERSONA_DIR)' | Out-Null; Get-ChildItem '$(PERSONA_DIR)\*.example.md' | ForEach-Object { $$dest = Join-Path $$_.DirectoryName ($$_.Name.Replace('.example.md','.md')); Copy-Item $$_.FullName $$dest -Force; Write-Host ('Overwrote ' + $$dest) }"
  REMOTE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/remote.ps1
  REMOTE_NATIVE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/remote-native.ps1
else
  ENV_COPY := @if [ -f $(ENV_FILE) ]; then \
                echo "$(ENV_FILE) already exists (use 'make env-force' to overwrite)"; \
              else \
                cp $(ENV_EXAMPLE) $(ENV_FILE) && echo "Created $(ENV_FILE) — edit GEMINI_API_KEY and Telegram vars"; \
              fi
  ENV_FORCE := cp $(ENV_EXAMPLE) $(ENV_FILE) && echo "Overwrote $(ENV_FILE)"
  MKDIR_DATA := mkdir -p data data/.config/google-mcp/credentials data/.config/strava data/.config/garmin data/.config/youtube
  RM_GARMIN_SESSION := rm -f data/.config/garmin/session.json
  HAS_DEPLOY_HOST := $(shell awk -F= '/^[[:space:]]*DEPLOY_HOST=/{v=$$2; gsub(/^[[:space:]]+|[[:space:]]+$$/,"",v); gsub(/^["'\'']|["'\'']$$/,"",v); if (v!="") print "1"}' .env 2>/dev/null)
  PERSONA_COPY := @mkdir -p $(PERSONA_DIR); \
	for f in $(PERSONA_DIR)/*.example.md; do \
	  [ -e "$$f" ] || continue; \
	  dest="$(PERSONA_DIR)/$$(basename "$$f" | sed 's/\.example\.md$$/.md/')"; \
	  if [ ! -f "$$dest" ]; then \
	    cp "$$f" "$$dest" && echo "Created $$dest — edit personal details (gitignored)"; \
	  else echo "$$dest already exists (use make persona-force to overwrite)"; fi; \
	done
  PERSONA_FORCE := @mkdir -p $(PERSONA_DIR); \
	for f in $(PERSONA_DIR)/*.example.md; do \
	  [ -e "$$f" ] || continue; \
	  dest="$(PERSONA_DIR)/$$(basename "$$f" | sed 's/\.example\.md$$/.md/')"; \
	  cp "$$f" "$$dest" && echo "Overwrote $$dest"; \
	done
  REMOTE := bash scripts/remote.sh
  REMOTE_NATIVE := bash scripts/remote-native.sh
endif

# linux gantry binary staged for Dockerfile COPY (.cache/docker-gantry).
DOCKER_GANTRY_GOOS ?= linux
DOCKER_GANTRY_GOARCH ?= amd64
DOCKER_GANTRY_BIN := .cache/docker-gantry
REPO_ROOT := $(abspath ..)

.PHONY: help env env-force dirs init persona persona-force prepare-docker-gantry build up down restart logs ps status shell clean \
        strava-auth garmin-auth ghealth-auth google-auth google-mcp-import youtube-auth \
        youtube-whoami youtube-probe ytmusic-auth ytmusic-whoami ytmusic-probe \
        garmin-sync strava-sync ghealth-sync youtube-sync ytmusic-sync google-sync secrets-sync secrets-migrate \
        remote-check remote-sync remote-up remote-down remote-restart remote-logs remote-ps remote-status \
        remote-ssh remote-deploy \
        remote-native-env remote-native-check remote-native-fetch remote-native-sync remote-native-install \
        remote-native-up remote-native-down remote-native-restart remote-native-logs remote-native-status \
        remote-native-deploy remote-native-deploy-dev remote-native-deploy-dev-quick remote-native-build-dev

help: ## Show available commands
	@echo.
	@echo   local-agent  /  LOCAL_AGENT on ai-gantry
	@echo   =================================================
	@echo   Gemini + Telegram agent in Docker. No dashboard. No published ports.
	@echo.
	@echo   Quick start (local)
	@echo   -------------------
	@echo     make init              Create .env, data dir, persona files
	@echo     # edit .env            GEMINI_API_KEY, TELEGRAM_BOT_TOKEN,
	@echo     #                      TELEGRAM_ALLOWED_USERS
	@echo     make build             Image: distroless/static + gantry + MCP tools
	@echo     make up                Build if needed + start daemon
	@echo     make logs              Watch logs / message your Telegram bot
	@echo.
	@echo   Quick start (Ubuntu server from this PC)
	@echo   ----------------------------------------
	@echo     # set DEPLOY_HOST, DEPLOY_USER, DEPLOY_PATH in .env
	@echo     make remote-check      Verify SSH + Docker on the server
	@echo     make remote-deploy     Sync + build + up on server
	@echo     make remote-logs       Follow server logs
	@echo.
	@echo   Setup
	@echo   -----
	@echo     help                   Show this help (default)
	@echo     env                    Create .env from .env.example (skip if exists)
	@echo     env-force              Overwrite .env from .env.example
	@echo     persona                Create persona/*.md from *.example.md (skip if exists)
	@echo     persona-force          Overwrite persona *.md from examples (wipes local edits)
	@echo     dirs                   Create ./data directory
	@echo     init                   env + dirs + persona
	@echo.
	@echo     docs/persona.md        SOUL/RULES/USER/TOOLS prompt (examples vs personal)
	@echo     docs/models.md         Gemini / Grok chat provider swap
	@echo     mcp.toml               MCP server manifest (which tools LOCAL_AGENT gets)
	@echo.
	@echo   Local Docker
	@echo   ------------
	@echo     build                  Build image (distroless/static + gantry + MCP tools)
	@echo     up                     compose up -d --build
	@echo     down                   Stop and remove containers
	@echo     restart                Restart containers
	@echo     logs                   Follow container logs
	@echo     ps                     Show compose status
	@echo     status                 gantry health check inside container
	@echo     shell                  Alpine shell with ./data mounted (sqlite inspection)
	@echo     clean                  Same as down (keeps ./data)
	@echo     strava-auth            One-time Strava OAuth; writes data/.config/strava/tokens.json
	@echo     garmin-auth            One-time Garmin login; writes data/.config/garmin/session.json
	@echo     ghealth-auth           Google Health OAuth; writes data/.config/ghealth/tokens.json
	@echo     google-auth            One-time Google OAuth; writes data/.config/google-mcp/credentials
	@echo     google-mcp-import      Legacy: import gws export into google-mcp (prefer google-auth)
	@echo     youtube-auth           YouTube OAuth -^> data/.config/youtube/oauth.json (mcp.toml)
	@echo     youtube-whoami         Show Google/YouTube channel for current oauth.json
	@echo     youtube-probe          Data API probe (likes / channel / playlists)
	@echo     garmin-sync            Push data/.config/garmin to DEPLOY_HOST (not part of remote-deploy)
	@echo     strava-sync            Push data/.config/strava to DEPLOY_HOST
	@echo     ghealth-sync           Push data/.config/ghealth to DEPLOY_HOST
	@echo     youtube-sync           Push data/.config/youtube to DEPLOY_HOST
	@echo     google-sync            Push Google Workspace creds to DEPLOY_HOST
	@echo     secrets-sync           Push all secret groups to DEPLOY_HOST
	@echo     secrets-migrate        Copy legacy secrets/* into data/.config/* (once)
	@echo.
	@echo   Remote deploy  (needs OpenSSH; see docs/deploy.md)
	@echo   --------------------------------------------------
	@echo     remote-check           SSH + docker available on DEPLOY_HOST?
	@echo     remote-sync            scp compose, .env, config (NOT token/session secrets)
	@echo     remote-up              build + up -d on server
	@echo     remote-down            Stop stack on server
	@echo     remote-restart         Restart stack on server
	@echo     remote-logs            Follow logs on server
	@echo     remote-ps              compose ps on server
	@echo     remote-status          Health check on server
	@echo     remote-ssh             Interactive SSH in DEPLOY_PATH
	@echo     remote-ssh CMD='...'   Run one remote command in DEPLOY_PATH
	@echo     remote-deploy          remote-sync + remote-up
	@echo.
	@echo   Native remote (binary + systemd + Ollama — no Docker)
	@echo   ----------------------------------------------------
	@echo     # DEPLOY_HOST=/opt/gantry box
	@echo     remote-native-env      Write deploy/gantry.env from .env (Ollama)
	@echo     remote-native-check    SSH + ollama + gantry user
	@echo     remote-native-fetch    Download gantry + MCP tools (gantry tools-fetch)
	@echo     remote-native-sync     Stage under /tmp/gantry-native on host
	@echo     remote-native-install  sudo install.sh + enable unit
	@echo     remote-native-up       systemctl start gantry
	@echo     remote-native-down     systemctl stop gantry
	@echo     remote-native-restart  systemctl restart gantry
	@echo     remote-native-logs     journalctl -u gantry -f
	@echo     remote-native-status   gantry status on host
	@echo     remote-native-deploy   env + fetch release + sync + install + start
	@echo     remote-native-deploy-dev  cross-build tree + sync + install + start (no GH release)
	@echo     remote-native-deploy-dev-quick  same, reuse host MCP bins (no GitHub tools-plan)
	@echo     remote-native-build-dev   linux/amd64 -^> .cache/native/gantry only
	@echo     deploy/README.md       Native layout + cutover notes
	@echo.
	@echo   Docs
	@echo   ----
	@echo     docs/telegram.md       BotFather + Telegram peers
	@echo     docs/persona.md        LOCAL_AGENT prompt (SOUL/RULES/USER/TOOLS) — examples committed
	@echo     docs/models.md         Chat model / Grok swap
	@echo     docs/whatsapp.md       WhatsApp Web friend / group (optional)
	@echo     docs/google-workspace.md Go MCP + OAuth import (Gmail/Docs/…)
	@echo     docs/strava.md         Strava workouts via strava-mcp (optional)
	@echo     docs/garmin.md         Garmin sleep/weight via go-garmin MCP (optional)
	@echo     docs/cast.md           Google Cast/DLNA via mcp-beam Go MCP (optional)
	@echo     docs/youtube.md        YouTube Data API v3 via youtube-go-mcp (optional)
	@echo     docs/math.md           Calculator via mcp-go-math (optional)
	@echo     docs/deploy.md         Windows -^> Ubuntu remote deploy
	@echo     README.md              Overview and architecture
	@echo.
	@echo   Required .env keys: GEMINI_API_KEY  TELEGRAM_BOT_TOKEN  TELEGRAM_ALLOWED_USERS
	@echo   Remote .env keys:   DEPLOY_HOST  DEPLOY_USER  DEPLOY_PATH  [DEPLOY_SSH_KEY]
	@echo.

env: ## Create .env from .env.example (skips if exists)
	$(ENV_COPY)

env-force: ## Overwrite .env from .env.example
	$(ENV_FORCE)

dirs: ## Create data directories
	$(MKDIR_DATA)
	@echo Data dirs ready under ./data/

persona: ## Create persona/*.md from *.example.md (skips existing)
	$(PERSONA_COPY)

persona-force: ## Overwrite persona *.md from examples (destructive)
	$(PERSONA_FORCE)

init: env dirs persona ## First-time setup: .env, data dir, persona
	@echo.
	@echo Next steps:
	@echo   1. Edit .env — GEMINI_API_KEY, TELEGRAM_*, and DEPLOY_* for remote
	@echo   2. Edit $(PERSONA_DIR)/USER.md (and friends) — personal; gitignored
	@echo   3. Local:  make up
	@echo   4. Remote: make remote-deploy
	@echo.
	@echo Guides: docs/telegram.md   docs/google-workspace.md   docs/deploy.md
	@echo.

prepare-docker-gantry: ## Cross-build linux gantry into .cache/docker-gantry (Dockerfile input)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -Command "\
	  New-Item -ItemType Directory -Force -Path '.cache' | Out-Null; \
	  $$prevCgo = $$env:CGO_ENABLED; $$prevGoos = $$env:GOOS; $$prevGoarch = $$env:GOARCH; \
	  $$env:CGO_ENABLED = '0'; $$env:GOOS = '$(DOCKER_GANTRY_GOOS)'; $$env:GOARCH = '$(DOCKER_GANTRY_GOARCH)'; \
	  try { \
	    Push-Location '$(REPO_ROOT)'; \
	    go build -trimpath -ldflags '-s -w' -o '$(CURDIR)/$(DOCKER_GANTRY_BIN)' ./cmd/gantry; \
	    if ($$LASTEXITCODE -ne 0) { throw 'go build failed' } \
	  } finally { \
	    Pop-Location; \
	    $$env:CGO_ENABLED = $$prevCgo; $$env:GOOS = $$prevGoos; $$env:GOARCH = $$prevGoarch \
	  }; \
	  Write-Host 'Staged $(DOCKER_GANTRY_BIN) ($(DOCKER_GANTRY_GOOS)/$(DOCKER_GANTRY_GOARCH))'"
else
	mkdir -p .cache
	cd "$(REPO_ROOT)" && CGO_ENABLED=0 GOOS=$(DOCKER_GANTRY_GOOS) GOARCH=$(DOCKER_GANTRY_GOARCH) \
		go build -trimpath -ldflags '-s -w' -o "$(CURDIR)/$(DOCKER_GANTRY_BIN)" ./cmd/gantry
	@echo Staged $(DOCKER_GANTRY_BIN) ($(DOCKER_GANTRY_GOOS)/$(DOCKER_GANTRY_GOARCH))
endif

build: prepare-docker-gantry ## Build image (distroless/static + gantry + MCP tools via tools-fetch)
	$(COMPOSE) build --build-arg TOOLS_CACHEBUST=$(TOOLS_CACHEBUST) $(SERVICE)

up: prepare-docker-gantry ## Start gantry daemon locally (build if needed; no published ports)
	$(COMPOSE) build --build-arg TOOLS_CACHEBUST=$(TOOLS_CACHEBUST) $(SERVICE)
	$(COMPOSE) up -d $(SERVICE)

down: ## Stop and remove local containers
	$(COMPOSE) down

restart: ## Restart local daemon
	$(COMPOSE) restart $(SERVICE)

logs: ## Follow local logs
	$(COMPOSE) logs -f $(SERVICE)

ps: ## Local container status
	$(COMPOSE) ps

status: ## Local gantry health (exit-code heartbeat check)
	$(COMPOSE) exec -T $(SERVICE) /usr/local/bin/gantry status && echo OK || echo FAILED

shell: ## Alpine shell with ./data mounted (runtime image has no shell)
	docker run --rm -it -v "$(CURDIR)/data:/data" -w /data alpine:3.20 sh

clean: ## Stop local containers (keeps ./data)
	$(COMPOSE) down

# Push local secret files to DEPLOY_HOST. Not part of remote-deploy (avoids
# clobbering a fresh server session with a stale laptop copy).
# Paths: data/.config/* (same on Docker and native hosts).
garmin-sync: ## Push data/.config/garmin/session.json to DEPLOY_HOST
	$(REMOTE) sync-secret garmin

strava-sync: ## Push data/.config/strava/tokens.json to DEPLOY_HOST
	$(REMOTE) sync-secret strava

ghealth-sync: ## Push data/.config/ghealth/tokens.json to DEPLOY_HOST
	$(REMOTE) sync-secret ghealth

youtube-sync: ## Push data/.config/youtube/oauth.json to DEPLOY_HOST
	$(REMOTE) sync-secret youtube

ytmusic-sync: youtube-sync ## Deprecated alias for youtube-sync

google-sync: ## Push data/.config/google-mcp/credentials to DEPLOY_HOST
	$(REMOTE) sync-secret google

secrets-sync: ## Push all secret groups to DEPLOY_HOST
	$(REMOTE) sync-secret all

secrets-migrate: ## Copy legacy secrets/* into data/.config/* (idempotent)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/secrets-migrate.ps1
else
	bash scripts/secrets-migrate.sh
endif

# After *-auth: if DEPLOY_HOST is set, push that secret group automatically.
ifeq ($(OS),Windows_NT)
define PUSH_SECRET_IF_REMOTE
	@powershell -NoProfile -Command "if ('$(HAS_DEPLOY_HOST)' -eq '1') { Write-Host 'DEPLOY_HOST set - pushing $(1) secrets to server...'; & make --no-print-directory $(1)-sync; if ($$LASTEXITCODE -ne 0) { exit $$LASTEXITCODE } } else { Write-Host 'No DEPLOY_HOST in .env - $(1) secret stays local only (make $(1)-sync later).' }"
endef
else
define PUSH_SECRET_IF_REMOTE
	@if [ "$(HAS_DEPLOY_HOST)" = "1" ]; then \
	  echo "DEPLOY_HOST set - pushing $(1) secrets to server..."; \
	  $(MAKE) --no-print-directory $(1)-sync; \
	else \
	  echo "No DEPLOY_HOST in .env - $(1) secret stays local only (make $(1)-sync later)."; \
	fi
endef
endif

# Auth runs against the local appliance image (gantry + MCP bins on PATH).
# Do not pass --build here: the Dockerfile needs `.cache/docker-gantry` from
# `make build` / `prepare-docker-gantry`. Re-auth should not re-fetch every tool.
strava-auth: ## One-time Strava OAuth via gantry auth (see docs/strava.md)
	@echo Opening Strava OAuth. A URL will be printed below — open it in your browser and approve.
	@echo If the image is missing: make build
	$(COMPOSE) run --rm -p 19876:19876 --entrypoint /usr/local/bin/gantry $(SERVICE) auth strava
	$(call PUSH_SECRET_IF_REMOTE,strava)

garmin-auth: ## Garmin login via gantry auth; clears stale session first (see docs/garmin.md)
	@echo Clearing stale data/.config/garmin/session.json if present...
	-$(RM_GARMIN_SESSION)
	@echo Garmin interactive login (email / password / MFA). Session lands in data/.config/garmin/.
	@echo If the image is missing: make build
	$(COMPOSE) run --rm -it --entrypoint /usr/local/bin/gantry $(SERVICE) auth garmin
	$(call PUSH_SECRET_IF_REMOTE,garmin)

ghealth-auth: ## Google Health OAuth via gantry auth (see docs/google-health.md)
	@echo Needs GOOGLE_HEALTH_CLIENT_ID / GOOGLE_HEALTH_CLIENT_SECRET in .env
	@echo Opening Google Health OAuth. Approve readonly Google Health scopes in the browser.
	@echo If the image is missing: make build
	$(COMPOSE) run --rm -it --entrypoint /usr/local/bin/gantry $(SERVICE) auth ghealth
	$(call PUSH_SECRET_IF_REMOTE,ghealth)

youtube-auth: ## YouTube OAuth via gantry auth (mcp.toml auth_args; see docs/youtube.md)
	@echo Needs YOUTUBE_OAUTH_CLIENT_ID / YOUTUBE_OAUTH_CLIENT_SECRET in .env
	@echo Approve as the Google account that owns your YouTube likes/playlists (incognito helps).
	@echo See docs/youtube.md. If the image is missing: make build
	$(COMPOSE) run --rm -it --entrypoint /usr/local/bin/gantry $(SERVICE) auth youtube
	$(call PUSH_SECRET_IF_REMOTE,youtube)

youtube-whoami: ## Show token + YouTube channel for data/.config/youtube/oauth.json
	$(COMPOSE) run --rm --entrypoint youtube-go-mcp $(SERVICE) auth oauth --whoami

youtube-probe: ## Data API probe — likes / channel / playlists (needs oauth.json + client id/secret)
	$(COMPOSE) run --rm --entrypoint youtube-go-mcp $(SERVICE) auth oauth --probe-data-api

ytmusic-auth: youtube-auth ## Deprecated alias for youtube-auth
ytmusic-whoami: youtube-whoami ## Deprecated alias for youtube-whoami
ytmusic-probe: youtube-probe ## Deprecated alias for youtube-probe

google-auth: ## Google Workspace OAuth; writes data/.config/google-mcp (see docs/google-workspace.md)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/google-auth.ps1
else
	bash scripts/google-auth.sh
endif
	$(call PUSH_SECRET_IF_REMOTE,google)

google-mcp-import: ## Legacy: import gws export into data/.config/google-mcp (prefer make google-auth)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/google-mcp-import.ps1
else
	bash scripts/google-mcp-import.sh
endif
	$(call PUSH_SECRET_IF_REMOTE,google)

# --- Remote Ubuntu server (from Windows via OpenSSH) -------------------------

remote-check: ## Test SSH + Docker on DEPLOY_HOST
	$(REMOTE) check

remote-sync: persona prepare-docker-gantry ## scp compose/.env/mcp.toml/persona + staged gantry (NOT secrets)
	$(REMOTE) sync

remote-up: ## docker compose up -d on server
	$(REMOTE) up

remote-down: ## docker compose down on server
	$(REMOTE) down

remote-restart: ## Restart stack on server
	$(REMOTE) restart

remote-logs: ## Follow logs on server
	$(REMOTE) logs

remote-ps: ## docker compose ps on server
	$(REMOTE) ps

remote-status: ## Health check on server
	$(REMOTE) status

remote-ssh: ## SSH into server (cd DEPLOY_PATH). Extra args: make remote-ssh CMD='ls'
	$(REMOTE) ssh $(CMD)

remote-deploy: remote-sync remote-up ## Sync files then start on server
	@echo.
	@echo Deployed. Message your Telegram bot — allowlist is TELEGRAM_ALLOWED_USERS.

# --- Native Ubuntu host (binary + systemd + Ollama; see deploy/README.md) ----

remote-native-env: ## Write deploy/gantry.env from .env (Ollama LLM_*)
	$(REMOTE_NATIVE) env

remote-native-check: ## SSH + ollama + /opt/gantry probe
	$(REMOTE_NATIVE) check

remote-native-fetch: ## Download gantry release + MCP tools (gantry tools-fetch)
	$(REMOTE_NATIVE) fetch

remote-native-sync: ## Stage binary/env/unit under /tmp/gantry-native
	$(REMOTE_NATIVE) sync

remote-native-install: ## sudo install.sh + enable systemd unit
	$(REMOTE_NATIVE) install

remote-native-up: ## systemctl start gantry
	$(REMOTE_NATIVE) up

remote-native-down: ## systemctl stop gantry
	$(REMOTE_NATIVE) down

remote-native-restart: ## systemctl restart gantry
	$(REMOTE_NATIVE) restart

remote-native-logs: ## journalctl -u gantry -f
	$(REMOTE_NATIVE) logs

remote-native-status: ## gantry status on native host
	$(REMOTE_NATIVE) status

remote-native-deploy: ## env + fetch + sync + install + start
	$(REMOTE_NATIVE) deploy
	@echo.
	@echo Native deploy done. Stop the old Docker TIM so only one bot polls Telegram.
	@echo.

remote-native-build-dev: ## Cross-build linux/amd64 gantry into .cache/native/gantry
	$(REMOTE_NATIVE) build-dev

remote-native-deploy-dev: ## env + local linux build + sync + install + start (skip GH release)
	$(REMOTE_NATIVE) deploy-dev $(if $(filter 1 true yes,$(NATIVE_SKIP_TOOLS)),-SkipTools,)
	@echo.
	@echo Dev native deploy done (working-tree binary). logs: make remote-native-logs
	@echo.

remote-native-deploy-dev-quick: ## like deploy-dev but reuse host MCP bins (no GitHub tools-fetch)
	$(MAKE) remote-native-deploy-dev NATIVE_SKIP_TOOLS=1
