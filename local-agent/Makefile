.DEFAULT_GOAL := help

COMPOSE := docker compose
SERVICE := gantry
ENV_FILE := .env
ENV_EXAMPLE := .env.example
PERSONA_DIR := persona

# Bust mcp-beam / youtube-go-mcp fetch stages so `latest` re-resolves each build.
ifeq ($(OS),Windows_NT)
  TOOLS_CACHEBUST ?= $(shell powershell -NoProfile -Command "[DateTimeOffset]::UtcNow.ToUnixTimeSeconds()")
else
  TOOLS_CACHEBUST ?= $(shell date +%s)
endif

ifeq ($(OS),Windows_NT)
  ENV_COPY := powershell -NoProfile -Command "if (-not (Test-Path '$(ENV_FILE)')) { Copy-Item '$(ENV_EXAMPLE)' '$(ENV_FILE)'; Write-Host 'Created $(ENV_FILE) — edit GEMINI_API_KEY and Telegram vars' } else { Write-Host '$(ENV_FILE) already exists (use make env-force to overwrite)' }"
  ENV_FORCE := powershell -NoProfile -Command "Copy-Item '$(ENV_EXAMPLE)' '$(ENV_FILE)' -Force; Write-Host 'Overwrote $(ENV_FILE)'"
  MKDIR_DATA := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path data | Out-Null"
  RM_GARMIN_SESSION := powershell -NoProfile -Command "Remove-Item -Force -ErrorAction SilentlyContinue 'secrets/garmin/session.json'"
  # Non-empty DEPLOY_HOST in .env → push secrets after *-auth
  HAS_DEPLOY_HOST := $(shell powershell -NoProfile -Command "$$l = Get-Content .env -ErrorAction SilentlyContinue | Where-Object { $$_ -match '^DEPLOY_HOST=' } | Select-Object -First 1; if ($$l -and (($$l -split '=',2)[1].Trim())) { '1' }")
  # SOUL.example.md → SOUL.md  ($$ → $ for Make; $$_ → $_ in PowerShell)
  PERSONA_COPY := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(PERSONA_DIR)' | Out-Null; Get-ChildItem '$(PERSONA_DIR)\*.example.md' | ForEach-Object { $$dest = Join-Path $$_.DirectoryName ($$_.Name.Replace('.example.md','.md')); if (-not (Test-Path $$dest)) { Copy-Item $$_.FullName $$dest; Write-Host ('Created ' + $$dest + ' — edit personal details (gitignored)') } else { Write-Host ($$dest + ' already exists (use make persona-force to overwrite)') } }"
  PERSONA_FORCE := powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(PERSONA_DIR)' | Out-Null; Get-ChildItem '$(PERSONA_DIR)\*.example.md' | ForEach-Object { $$dest = Join-Path $$_.DirectoryName ($$_.Name.Replace('.example.md','.md')); Copy-Item $$_.FullName $$dest -Force; Write-Host ('Overwrote ' + $$dest) }"
  REMOTE := powershell -NoProfile -ExecutionPolicy Bypass -File scripts/remote.ps1
else
  ENV_COPY := @if [ -f $(ENV_FILE) ]; then \
                echo "$(ENV_FILE) already exists (use 'make env-force' to overwrite)"; \
              else \
                cp $(ENV_EXAMPLE) $(ENV_FILE) && echo "Created $(ENV_FILE) — edit GEMINI_API_KEY and Telegram vars"; \
              fi
  ENV_FORCE := cp $(ENV_EXAMPLE) $(ENV_FILE) && echo "Overwrote $(ENV_FILE)"
  MKDIR_DATA := mkdir -p data
  RM_GARMIN_SESSION := rm -f secrets/garmin/session.json
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
endif

.PHONY: help env env-force dirs init persona persona-force build up down restart logs ps status shell clean \
        strava-auth garmin-auth google-auth google-mcp-import ytmusic-auth \
        garmin-sync strava-sync ytmusic-sync google-sync secrets-sync \
        remote-check remote-sync remote-up remote-down remote-restart remote-logs remote-ps remote-status \
        remote-ssh remote-deploy

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
	@echo     docs/persona.md        SOUL/USER system prompt (examples vs personal files)
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
	@echo     strava-auth            One-time Strava OAuth; writes secrets/strava/tokens.json
	@echo     garmin-auth            One-time Garmin login; writes secrets/garmin/session.json
	@echo     google-auth            One-time Google OAuth via container; writes google-mcp creds
	@echo     google-mcp-import      Legacy: import gws export into google-mcp (prefer google-auth)
	@echo     ytmusic-auth           YouTube Music headers -^> secrets/ytmusic/headers.json
	@echo     garmin-sync            Push secrets/garmin to DEPLOY_HOST (not part of remote-deploy)
	@echo     strava-sync            Push secrets/strava to DEPLOY_HOST
	@echo     ytmusic-sync           Push secrets/ytmusic to DEPLOY_HOST
	@echo     google-sync            Push Google Workspace secrets to DEPLOY_HOST
	@echo     secrets-sync           Push all secret groups to DEPLOY_HOST
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
	@echo   Docs
	@echo   ----
	@echo     docs/telegram.md       BotFather + Telegram peers
	@echo     docs/persona.md        LOCAL_AGENT prompt (SOUL/USER) — examples are committed
	@echo     docs/models.md         Chat model / Grok swap
	@echo     docs/whatsapp.md       WhatsApp Web friend / group (optional)
	@echo     docs/google-workspace.md Go MCP + OAuth import (Gmail/Docs/…)
	@echo     docs/strava.md         Strava workouts via strava-mcp (optional)
	@echo     docs/garmin.md         Garmin sleep/weight via go-garmin MCP (optional)
	@echo     docs/cast.md           Google Cast/DLNA via mcp-beam Go MCP (optional)
	@echo     docs/ytmusic.md        YouTube Music via youtube-go-mcp (optional)
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

build: ## Build image (distroless/static + gantry + MCP tool binaries)
	$(COMPOSE) build --build-arg TOOLS_CACHEBUST=$(TOOLS_CACHEBUST) $(SERVICE)

up: ## Start gantry daemon locally (build if needed; no published ports)
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
garmin-sync: ## Push secrets/garmin/session.json to DEPLOY_HOST
	$(REMOTE) sync-secret garmin

strava-sync: ## Push secrets/strava/tokens.json to DEPLOY_HOST
	$(REMOTE) sync-secret strava

ytmusic-sync: ## Push secrets/ytmusic/headers.json to DEPLOY_HOST
	$(REMOTE) sync-secret ytmusic

google-sync: ## Push Google Workspace credential files to DEPLOY_HOST
	$(REMOTE) sync-secret google

secrets-sync: ## Push all secret groups to DEPLOY_HOST
	$(REMOTE) sync-secret all

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

strava-auth: ## One-time Strava OAuth in a throwaway container (see docs/strava.md)
	@echo Opening Strava OAuth. A URL will be printed below — open it in your browser and approve.
	$(COMPOSE) run --rm --build -p 19876:19876 --entrypoint strava-mcp $(SERVICE) auth
	$(call PUSH_SECRET_IF_REMOTE,strava)

garmin-auth: ## Garmin login; clears stale session.json then writes a fresh one (see docs/garmin.md)
	@echo Clearing stale secrets/garmin/session.json if present...
	-$(RM_GARMIN_SESSION)
	@echo Garmin interactive login (email / password / MFA). Session lands in secrets/garmin/.
	$(COMPOSE) run --rm --build -it --entrypoint garmin $(SERVICE) login
	$(call PUSH_SECRET_IF_REMOTE,garmin)

ytmusic-auth: ## YouTube Music browser headers → secrets/ytmusic/headers.json (see docs/ytmusic.md)
	@echo DevTools → Network → browse → Request Headers: copy cookie, then x-goog-authuser.
	@echo The CLI prompts for each value. See docs/ytmusic.md.
	$(COMPOSE) run --rm --build -it --entrypoint youtube-go-mcp $(SERVICE) \
	  auth --out /data/.config/ytmusic/headers.json
	$(call PUSH_SECRET_IF_REMOTE,ytmusic)

google-auth: ## Google Workspace OAuth via container; writes secrets/google-mcp (see docs/google-workspace.md)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/google-auth.ps1
else
	bash scripts/google-auth.sh
endif
	$(call PUSH_SECRET_IF_REMOTE,google)

google-mcp-import: ## Legacy: import gws export into secrets/google-mcp (prefer make google-auth)
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/google-mcp-import.ps1
else
	bash scripts/google-mcp-import.sh
endif
	$(call PUSH_SECRET_IF_REMOTE,google)

# --- Remote Ubuntu server (from Windows via OpenSSH) -------------------------

remote-check: ## Test SSH + Docker on DEPLOY_HOST
	$(REMOTE) check

remote-sync: persona ## Copy compose/.env/mcp.toml/persona to server (does not sync data volume)
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
	@echo.
