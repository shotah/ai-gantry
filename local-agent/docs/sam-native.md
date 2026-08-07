# SAM — native Beelink OCR / vision lab

**Tim** stays on **gantry-fleet** (Docker + Gemini) for day-to-day.
**SAM** (`@sam_ai_agent_test_bot`) runs from **this** `local-agent/` tree on the
Beelink via systemd + Ollama **`gemma3:12b`** (Google, US — vision/OCR).

> Model constraints for this lab:
> - Need a **US-origin** vendor (Meta / Google / IBM / …) — not Qwen.
> - `llama3.2-vision` fails on Ollama ≥0.30 (`unknown architecture: mllama`).
> - Practical US swap on current Ollama: **Gemma 3** (Google). Alt: `llava:13b`
>   (UW–Madison / Microsoft lineage) or IBM `granite3.2-vision` if you prefer.
> - **`gemma3:12b` does not support tools.** Ollama returns 400 if the chat
>   request includes any tool schemas — including gantry’s built-in
>   `memory_*` / `cron_*` and any MCP. Set **`TOOLS_ENABLED=false`** (hard
>   omit). Also keep `mcp.toml` empty, `MEMORY_ENABLED=false`, and
>   `CRON_ENABLED=false` so leftover Tim cron jobs do not fire.

```text
Laptop (this repo)  --make remote-native-*-->  Beelink /opt/gantry
Telegram @sam_ai_agent_test_bot  <-->  gantry  <-->  Ollama vision
```

---

## One-time on the Beelink (SSH)

```bash
# Ollama already installed? skip install.
# Need a recent build (Beelink was on 0.32.x — fine for gemma3).
ollama --version

# US vision model (Google Gemma 3 — multimodal)
ollama pull gemma3:12b

# Optional: more quality on 87GB RAM
# ollama pull gemma3:27b
# Alt US lineage: ollama pull llava:13b

# Sanity (text-only; photo test is via Telegram after deploy)
ollama run gemma3:12b "Say hi in one sentence."

# Drop broken / unused pulls:
# ollama rm llama3.2-vision:11b
# ollama rm qwen2.5vl:7b

ls -la /opt/gantry
ollama list
```

**Kill leftover native Tim** (he often comes back after reboot if still enabled).
Tim day-to-day is on the Mini/fleet — this box should only run SAM:

```bash
sudo systemctl stop gantry
sudo systemctl disable gantry
sudo systemctl status gantry --no-pager || true
# confirm nothing is still bound to the old bot:
ps aux | grep -E '[g]antry' || echo 'no gantry process'
```

Then deploy SAM (laptop `make remote-native-deploy-dev`), which re-enables the
unit under the SAM Telegram token + vision model.

---

## From the laptop (PowerShell)

```powershell
cd C:\workspace\ai-gantry\local-agent

# 1) Confirm .env: SAM bot token, TELEGRAM_ALLOWED_USERS, NATIVE_LLM_MODEL,
#    DEPLOY_HOST = Beelink IP (not the Mini).

# 2) Write deploy/gantry.env from .env (Ollama LLM_* + SAM Telegram)
make remote-native-env

# 3) SSH / Ollama / gantry user probe
make remote-native-check

# 4) Ship binary + lean mcp.toml + persona (dev loop from this tree)
make remote-native-deploy-dev

# Or release binary instead of local cross-build:
# make remote-native-deploy

# 5) Watch
make remote-native-logs
```

Quick persona/env-only iterate (reuse MCP bins on host):

```powershell
make remote-native-deploy-dev-quick
```

---

## Telegram smoke test

1. Open `t.me/sam_ai_agent_test_bot` and `/start` (must be allowlisted user).
2. Send a **photo** of a receipt/form with caption:
   `OCR this — return JSON {merchant, date, total, lines[]}`.
3. Expect structured text from the vision model (no OCR MCP required).

---

## Swap notes

| Artifact | Role |
| --- | --- |
| `.env` | SAM (this tree) |
| `.env.tim-gemini.bak` | Prior Tim Gemini local-agent env |
| `mcp.toml` | Empty (gemma3 rejects tools) |
| `mcp.full.toml` | Archived Tim-scale MCP list |

Restore full tools later only with a **tools-capable** model: set `TOOLS_ENABLED=true`,
`copy mcp.full.toml mcp.toml`, re-enable `MEMORY_ENABLED` / `CRON_ENABLED`, then
`make remote-native-env` and redeploy.

---

## Security

Bot tokens and API keys in chat history should be treated as exposed — rotate
the SAM bot token via BotFather if this transcript is shared. Do not commit `.env`.
