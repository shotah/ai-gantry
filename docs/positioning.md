# Positioning

Product narrative for maintainers and anyone writing about gantry.
The [root readme](../readme.md) is the public pitch + operator contract;
this page is the longer “why / who / how we talk about it.”

---

## One-liner

**Outbound-only personal agent you own** — one Distroless Go binary, one
persona, one OpenAI-compat model, MCP tools you choose. Chat over Telegram,
Discord, or Slack. **No open ports. Ever.**

The name: a **gantry** holds and positions tools; the tools do the work.
Gantry is the frame. MCP binaries are the tools.

---

## Problem we refuse

Personal-agent stacks drift into platforms: dashboards, pairing flows,
multi-agent routers, embedding round-trips, inbound webhooks, Node gateways.
That tax shows up every turn — especially on **local models** that invent tool
names or park answers in chain-of-thought.

```text
process = persona + model + MCP set + data dir
```

Want another brain? Another process. Not another settings page.

---

## Value props (what to repeat)

1. **Outbound-only security story** — long-poll / Socket Mode / Gateway; allowlist;
   nothing listens. Health is `gantry status` (exit code), not a port.
2. **MCP is the only plugin surface** — capabilities are external stdio binaries;
   the kernel hosts them; it does not become a tool zoo.
3. **Inspectable memory** — SQLite + FTS5 you can `sqlite3`; persona files outrank
   recall; no cloud vector store in the hot path.
4. **Local-model hardening** — tool-name repair, CoT promotion, printed-call
   parsing, schema budget — so Qwen/Ollama finishes turns instead of ERROR.
5. **Boring ops** — env + mounts, static binary, Distroless or systemd.
   Published images on [Docker Hub](https://hub.docker.com/r/shotah/ai-gantry)
   (`shotah/ai-gantry`) and GHCR on every `main` / tag build.

Kernel vs appliance: sell the **kernel**. Point at
[`local-agent/`](../local-agent/) as a reference life-stack, not the product
everyone must run.

---

## Who it’s for (ICP)

| Fit | Audience |
| --- | --- |
| **Primary** | Self-hosters who want a Telegram/Discord/Slack assistant that never opens a port |
| **Primary** | Local-LLM operators (Ollama / Qwen / …) burned by stacks that assume cloud Flash + huge tool catalogs |
| **Primary** | MCP authors who need a production-shaped host for their static binaries |
| **Secondary** | Solo builders who want inspectable memory + cron/spark without a SaaS agent |

**Anti-ICP** (send them elsewhere, kindly):

- Need a web UI, team workspace, or multi-agent routing
- Want WhatsApp / Teams / Messenger inbound webhooks
- Want no-code automation canvases (n8n / Make)
- Want “Cursor/Claude for the company” — wrong category

**Pick gantry** when you want small, boring, shippable.  
**Pick something else** when you need dashboards, pairing, or platform gravity.

---

## Competition (category, not enemies)

| Camp | Examples | Our line |
| --- | --- | --- |
| Personal agent runtimes | OpenClaw-style stacks, assorted local-agent repos | Smaller surface; outbound-only; Go static + Distroless; local-model loop work |
| Orchestration libraries | LangGraph, AutoGen, CrewAI | Deployable kernel, not a Python framework |
| Automation platforms | n8n, Home Assistant + LLM | Agent loop + memory + MCP, not a workflow canvas |
| Hosted agents | Custom GPTs, SaaS “tasks” bots | You own process + data dir |

We are not competing with IDE coding agents. We compete with
**“I forked a Node bot and now I maintain a gateway.”**

---

## Distribution (what actually converts)

Priority order for strangers:

1. **Docker Hub pull** — `shotah/ai-gantry:latest` (release) / `:edge` (`main`) /
   `:0.x.y` (pin). No clone required to try the kernel.
   Walkthrough: [deploy-docker.md](deploy-docker.md).
2. **Consumer templates** — [`examples/docker/`](../examples/docker/) ·
   [`examples/hosting/`](../examples/hosting/) ([GCP](../examples/hosting/gcp/) · [AWS](../examples/hosting/aws/)) · [`examples/native/`](../examples/native/)
   (standalone repos that pull Hub / releases).
3. **Native + Ollama** — featured for people who want the full local story:
   [deploy-native.md](deploy-native.md) + [`local-agent/`](../local-agent/).
4. **GoReleaser binaries** — systemd on metal without Docker.

CI already publishes multi-arch images to Hub **and** `ghcr.io/shotah/ai-gantry`
(see `.github/workflows/docker.yml`). The Hub **overview** syncs from
[`docs/dockerhub.md`](dockerhub.md) (not the root readme — Hub caps ~25KB and
botches SVG/mermaid). Short description is set in the same workflow (≤100 chars).

**Categories** are Hub-UI only (max 3): Machine learning & AI, Developer tools,
Security. See the footer of `dockerhub.md`.

---

## Evangelism (channels that fit)

Good fits:

- Show HN / Lobsters — lead with the constraint (“no open ports”)
- r/selfhosted, r/LocalLLaMA, MCP / Telegram-bot communities
- Short technical posts from production scars (tool-name repair, thought
  signatures, Distroless MCP children, why not embeddings at personal scale)
- MCP package READMEs that say “runs under gantry” — tools as bait, frame as host

Skip early:

- Generic “AI agent platform” landing copy
- Feature-matrix wars with LangGraph
- Selling the full Garmin/Strava life OS before the Hub hello path is trivial

Success metric: strangers keep a Hub deploy running a week and file issues on
the **loop**, not only star the appliance.

---

## Website / GitHub Pages — do we need one?

**Today:** `gh-pages` already exists for the **coverage badge** only
(`badges/coverage.svg`). There is no product site.

**GitHub Pages** can host a static site from a `docs/` folder on `main`, or from
the `gh-pages` branch. Enabling a marketing site means either:

- Putting a site under something like `docs/site/` (or `/docs` Pages source) and
  keeping badge pushes careful so they don’t clobber HTML, **or**
- Using `gh-pages` for the site and moving the badge elsewhere (e.g. gist /
  shields.io coverage, or a `badges` orphan path with a disciplined publish job).

**Recommendation:** defer a standalone site until the Hub hello path and README
pitch convert people. For this audience, **GitHub README + Docker Hub** are the
landing pages. A Pages site adds value when you have a demo video, a short
comparison page, and a changelog you want outside the repo — not before.

If/when you add Pages: one page, same one-liner, embed the demo, deep-link to
Hub tags and [deploy-docker.md](deploy-docker.md). Do not invent a second
product story.

---

## Copy kit (reuse freely)

**Tagline:** A personal agent you can actually own.

**Constraint line:** No dashboard. No config UI. No open ports. Ever.

**Equation:**

```text
static binary + persona + mcp.toml + any OpenAI-compat LLM  →  outbound chat
```

**Show HN blurb (draft):**

> ai-gantry — outbound-only personal agent runtime (Go, Distroless). One
> persona, one model, MCP tools, Telegram/Discord/Slack. Pull
> `shotah/ai-gantry` from Docker Hub; no inbound ports, SQLite memory you can
> inspect. Built so local models finish tool turns instead of falling over.

**Hub short description (workflow):** keep ≤100 chars, constraint-first — see
`.github/workflows/dockerhub-description.yml`.
