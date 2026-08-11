# Consumer-repo helpers. Kernel image: shotah/ai-gantry on a GCP GCE VM.

.PHONY: init

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
	@echo "Set GEMINI_API_KEY / TELEGRAM_* in .env, then follow README (GCE + compose)."
