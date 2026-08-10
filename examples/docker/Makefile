# Consumer-repo helpers. Kernel image: shotah/ai-gantry (Docker Hub / GHCR).

.PHONY: init up down logs status pull

init: ## Seed persona/*.md and .env from *.example (skips existing)
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -Command "Get-ChildItem persona\*.example.md | ForEach-Object { $$d = Join-Path $$_.Directory (($$_.BaseName -replace '\.example$$','') + '.md'); if (-not (Test-Path $$d)) { Copy-Item $$_.FullName $$d } }; if (-not (Test-Path .env)) { Copy-Item .env.example .env }"
else
	@for f in persona/*.example.md; do \
	  dest="persona/$$(basename "$$f" .example.md).md"; \
	  test -f "$$dest" || cp "$$f" "$$dest"; \
	done
	@test -f .env || cp .env.example .env
endif
	@echo "Set GEMINI_API_KEY / TELEGRAM_* in .env, then: make up"

pull:
	docker compose pull

up: ## Pull + start (detached)
	docker compose pull
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

status:
	docker compose exec gantry /usr/local/bin/gantry status
