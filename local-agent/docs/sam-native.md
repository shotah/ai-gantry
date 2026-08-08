# SAM — native Beelink agent / vision lab

**SAM** stays on **gantry-fleet** (Docker + Gemini) for day-to-day.
**SAM** (`@sam_ai_agent_test_bot`) runs from **this** `local-agent/` tree on the
Beelink via systemd + Ollama **`gemma4:12b`** (Google, US — vision + tools).

> Current mode: **agent** (lean MCP: google, google-search, garmin, strava, math).
>
> Model notes:
> - **US-origin** vendor (Meta / Google / IBM / …) — not Qwen for this lab pin.
> - `gemma4:12b` supports tools + vision. Prior OCR-only pin used
>   `TOOLS_ENABLED=false` + empty `mcp.toml` (also true for `gemma3:12b`, which
>   rejects tool schemas entirely).
> - Lean catalog on purpose — full SAM surface is in `mcp.full.toml` (often too
>   heavy for 12B schema following).
> - Optional heavier MoE: `gemma4:26b` (~3.8B active). Dense `gemma4:31b` /
>   Qwen 27B usually too slow on the SER10 890M.

```text
Laptop (this repo)  --make remote-native-*-->  Beelink /opt/gantry
Telegram @sam_ai_agent_test_bot  <-->  gantry  <-->  Ollama + MCP
```

---

## One-time on the Beelink (SSH)

```bash
ollama --version
ollama pull gemma4:12b
ollama run gemma4:12b "Say hi in one sentence."
ollama list
```

**Kill leftover native SAM** if he still owns this box — SAM day-to-day is on
the Mini/fleet:

```bash
sudo systemctl stop gantry
sudo systemctl disable gantry
ps aux | grep -E '[g]antry' || echo 'no gantry process'
```

Then deploy SAM from the laptop (below).

---

## From the laptop (PowerShell)

```powershell
cd C:\workspace\ai-gantry\local-agent

# 1) Confirm .env: SAM token, NATIVE_LLM_MODEL=gemma4:12b, TOOLS_ENABLED=true,
#    GEMINI_* + Google OAuth + USER_GOOGLE_EMAIL, DEPLOY_HOST = Beelink.

# 2) Rewrite deploy/gantry.env from .env
make remote-native-env

# 3) SSH / Ollama / gantry user probe
make remote-native-check

# 4) Ship binary + mcp.toml + persona
# Full (refresh MCP bins from GitHub):
make remote-native-deploy-dev
# Quick (reuse /opt/gantry/bin — preferred if tools already installed):
make remote-native-deploy-dev-quick

# 5) Watch
make remote-native-logs
```

---

## Ollama performance (Beelink iGPU)

`deploy/ollama-gantry.conf` ships as a systemd drop-in. Agent defaults:

- `OLLAMA_CONTEXT_LENGTH=49152` (was 16k in OCR-only mode)
- `OLLAMA_FLASH_ATTENTION=1` + `OLLAMA_KV_CACHE_TYPE=q8_0`

```bash
systemctl cat ollama | grep -E 'CONTEXT|FLASH|KV_CACHE|KEEP_ALIVE'
ollama ps
```

---

## Telegram smoke tests (agent)

1. `/start` on `t.me/sam_ai_agent_test_bot` (allowlisted), then `/new`.
2. `/tools` — expect `google__…`, `google-search__web_search`, `garmin__…`, etc.
3. Calendar: “What’s on my calendar tomorrow?”
4. Search: “Search the web for Seattle weather this weekend.”
5. Garmin: “How did I sleep last night?”
6. Auth fails → on host: `gantry auth google` / `gantry auth garmin` (creds under
   `/opt/gantry/data/.config/`).

---

## OCR-only mode (previous)

Empty `mcp.toml`, `TOOLS_ENABLED=false`, `MEMORY_ENABLED=false`,
`CRON_ENABLED=false`, `OLLAMA_CONTEXT_LENGTH=16384`, OCR-heavy persona.
Restore from git history / prior chat if you need that lab again.

---

## Swap notes

| Artifact | Role |
| --- | --- |
| `.env` | SAM agent lab (this tree) |
| `.env.SAM-gemini.bak` | Prior SAM Gemini local-agent env |
| `mcp.toml` | Lean agent catalog |
| `mcp.full.toml` | Full SAM-scale MCP list |

Widen tools: copy entries from `mcp.full.toml` into `mcp.toml`, then
`make remote-native-env` and redeploy. Expect weaker tool-picking on 12B as the
catalog grows.

---

## Security

Bot tokens and API keys in chat history should be treated as exposed — rotate
if this transcript is shared. Do not commit `.env`.
