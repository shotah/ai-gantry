# Consumer-repo helpers. Kernel binary: github.com/shotah/ai-gantry releases.

.PHONY: init

init: ## Seed persona/*.md and gantry.env from *.example (skips existing)
ifeq ($(OS),Windows_NT)
	@powershell -NoProfile -Command "Get-ChildItem persona\*.example.md | ForEach-Object { $$d = Join-Path $$_.Directory (($$_.BaseName -replace '\.example$$','') + '.md'); if (-not (Test-Path $$d)) { Copy-Item $$_.FullName $$d } }; if (-not (Test-Path gantry.env)) { Copy-Item gantry.env.example gantry.env }"
else
	@for f in persona/*.example.md; do \
	  dest="persona/$$(basename "$$f" .example.md).md"; \
	  test -f "$$dest" || cp "$$f" "$$dest"; \
	done
	@test -f gantry.env || cp gantry.env.example gantry.env
endif
	@echo "Set TELEGRAM_* / LLM_* in gantry.env, copy this tree to the Linux host, then: sudo ./install.sh"
