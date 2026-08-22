# Positioning

Product narrative for maintainers and anyone writing about gantry.
The [root readme](../readme.md) is the public pitch and Docker hello path.
Harness contract: [design.md](design.md). This page is the longer
“why / who / how we talk about it.”

---

## How we argue (repeat this)

Not “fewer features than a platform.” **We spent the engineering budget on
the harness** — the runtime around the model that you actually talk to.

The harness is the product. The management plane is infrastructure
([gantree.md](gantree.md) — proposal). Dashboards are not an argument
against this binary; they are an argument for keeping them *out of the
thing you talk to all day*.

Category: **AI harness**. Goal: **long-horizon planning**.

Aim:

> Make a local harness small enough to understand, efficient enough to run
> continuously, resilient enough for imperfect local models, and stateful
> enough for long-horizon planning: a useful personality and standing goals
> rather than a stateless chatbot every time context gets expensive.

Gantry is not trying to be the most complete local-agent platform. The
focus is making the **harness** exceptionally good: tool calling, MCP,
context economics, memory that outlives a session, reliable operation with
local models — so an agent can hold a plan across days, not just a turn.

---

## One-liner

**Run your own agent** — pull a container, point it at a local model or paste
an API key, chat from your phone. One persona, one OpenAI-compat model, MCP
tools if you want them. Telegram, Discord, or Slack. **No open ports.**
Gantry is the **AI harness** that makes that agent **long-horizon**.

The name: a **gantry** holds and positions tools; the tools do the work.
Gantry is the frame — the harness. MCP binaries are the tools.

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
   the harness hosts them; it does not become a tool zoo.
3. **Inspectable memory** — SQLite + FTS5 you can `sqlite3`; persona files outrank
   recall; no cloud vector store in the hot path. This is how long-horizon
   facts survive `/new`.
4. **Local-model hardening** — tool-name repair, CoT promotion, printed-call
   parsing, schema budget — so Qwen/Ollama finishes turns instead of ERROR.
5. **Boring ops** — env + mounts, static binary, Distroless or systemd.
   Published images on [Docker Hub](https://hub.docker.com/r/shotah/ai-gantry)
   (`shotah/ai-gantry`) and GHCR on every `main` / tag build.
6. **Long-horizon by construction** — cron, watches, spark, `SELF.md`, history
   fold. A turn is execution; the horizon is the product.

Sell the **harness**. A life-stack (Workspace, Strava, …) is a consumer you
own. Operating N isolated agents is a proposed sibling console
([gantree.md](gantree.md)), not a folder in this checkout. Hello path: Hub
compose or [`examples/`](../examples/).

---

## Who it’s for (ICP)

Lead with the Docker hello. The hardening story is why it *stays* running, not
a filter on who is allowed to try it.

| Fit | Audience |
| --- | --- |
| **Primary** | Anyone who wants to `docker compose up` a long-horizon personal agent and drop in Ollama or an API key |
| **Primary** | Self-hosters who want Telegram/Discord/Slack without opening a port |
| **Primary** | Local-LLM operators (Ollama / Qwen / …) burned by stacks that assume cloud Flash + huge tool catalogs |
| **Secondary** | MCP authors who need a production-shaped host for their static binaries |
| **Secondary** | Solo builders who want inspectable memory + cron/spark without a SaaS agent |

**Anti-ICP** (send them elsewhere, kindly):

- Need a web UI, team workspace, or multi-agent routing
- Want WhatsApp / Teams / Messenger inbound webhooks
- Want no-code automation canvases (n8n / Make)
- Want “Cursor/Claude for the company” — wrong category (that is a coding
  harness; this is a personal long-horizon harness)

**Pick gantry** when you want the harness to be excellent, the binary small,
and the agent still itself next week.  
**Pick something else** when you need a platform on day one. A yard console
is a sibling, not a missing tab in this process.

---

## Competition (category, not enemies)

| Camp | Examples | Our line |
| --- | --- | --- |
| Agent harnesses (personal) | OpenClaw-style stacks, assorted appliance repos | Smaller surface; outbound-only; Go static + Distroless; local-model loop; long-horizon (memory / cron / watches / `SELF.md`) |
| Agent harnesses (coding) | Cursor, Claude Code, Codex CLI | Wrong category. We are a personal long-horizon harness, not an IDE loop |
| Orchestration libraries | LangGraph, AutoGen, CrewAI | Deployable harness, not a Python framework |
| Automation platforms | n8n, Home Assistant + LLM | Agent loop + memory + MCP, not a workflow canvas |
| Hosted agents | Custom GPTs, SaaS “tasks” bots | You own process + data dir |

We are not competing with IDE coding agents. We compete with
**“I forked a Node bot and now I maintain a gateway.”** The industry name for
what we ship is **AI harness**; the goal we optimize for is **long-horizon
planning**.

---

## Distribution (what actually converts)

Priority order for strangers:

1. **Docker Hub pull** — `shotah/ai-gantry:latest` (release) / `:edge` (`main`) /
   `:0.x.y` (pin). No clone required to try the harness.
   Walkthrough: [deploy-docker.md](deploy-docker.md).
2. **Consumer templates** — [`examples/docker/`](../examples/docker/) ·
   [`examples/hosting/`](../examples/hosting/) ([GCP](../examples/hosting/gcp/) · [AWS](../examples/hosting/aws/)) · [`examples/native/`](../examples/native/)
   (standalone repos that pull Hub / releases).
3. **Native + Ollama** — featured for people who want the full local story:
   [deploy-native.md](deploy-native.md) + [`examples/native/`](../examples/native/).
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
- MCP package READMEs that say “runs under gantry” — tools as bait, harness as host

Skip early:

- Generic “AI agent platform” landing copy (say **harness** and **long-horizon**)
- Feature-matrix wars with LangGraph
- Selling the full Garmin/Strava life OS before the Hub hello path is trivial

Success metric: strangers keep a Hub deploy running a week and file issues on
the **loop**, not only star the appliance. A week is the first long-horizon
check.

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

**Tagline:** Run your own agent.

**Category:** AI harness for long-horizon planning.

**Constraint line:** No dashboard. No config UI. No open ports. Ever.

**Equation:**

```text
container + persona + any OpenAI-compat LLM  →  outbound chat
```

**Show HN blurb (draft):**

> ai-gantry — a long-horizon AI harness you run in Docker. Point it at Ollama
> or paste a Gemini/xAI key; chat over Telegram/Discord/Slack. No inbound
> ports. Tools optional. Memory, cron, and personality survive `/new`. The
> same binary is hardened so small local models finish tool turns instead of
> falling over.

**Hub short description (workflow):** keep ≤100 chars, constraint-first — see
`.github/workflows/dockerhub-description.yml`.
