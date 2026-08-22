# Gantree (proposal)

Sibling product. **Not built yet.** Not this harness. Not anyone’s house git.

`ai-gantry` is the **AI harness** (the crane): one process, one persona, one
model, one `data/`. It talks to a human in Telegram. It does **not** grow a
settings page. Its goal is **long-horizon planning** — hold aims and
personality across days.

**Gantree** is the yard console — the product you open when you *operate*
cranes. See them. Plant a new one. Grant Google, yank Strava, notice a dead
token, recreate, read logs. Chat stays the agent’s mouth. Gantree is the
operator’s.

This page is the pitch + design for a **new public repo**. Nothing here ships
in the `gantry` binary. Hello for a single agent is still
[deploy-docker.md](deploy-docker.md).

---

## Pitch

You can already run one gantry with compose. That is not a product gap.

The gap is what happens on week two: two people, three bots, a pile of
`mcp.toml`, OAuth that only works on a laptop, a container that looks
“healthy” with zero tools, and SSH folklore you will not remember at 11pm.

Gantree is **the control plane for personal agents**. The harness stays in
`gantry`; the yard is here.

| Harness (`gantry`) | Gantree |
| --- | --- |
| Talks to the human | Talks to the operator |
| Outbound chat, no listen port | A UI you open (LAN / Tailscale / localhost) |
| Hosts MCP children | **Grants** which children exist |
| `status` is an exit code | Fleet view, per-agent doctor, logs |
| Unaware of other processes | Inventory of every gantry on the box |
| Long-horizon loop (memory / cron / `SELF.md`) | Does not sit in the token path |

If it feels like “a nicer rsync,” we missed. The CLI is a back door. The
product is the yard you can *see*.

**Tagline:** the harness is a crane; gantree is where you run the yard.

**Name:** `shotah/gantree`, CLI/binary `gantree`. Plural in copy: gantries.
Do not reuse a private inventory repo’s name.

---

## Who it’s for

An **advanced home operator**: you own the agents, you own the box, you
might run **several** (you, a partner, a tryout for a friend). You are
willing to run Docker. You might put that Docker host in the living room
**or** rent a small Linux VM so it is not your house’s power bill.

Cloud here is **not** “Gantree the company, spinning agents for
customers.” It is the same person, same product, the Mini happens to be
an `e2-small` in GCP. Billing is still your cloud account. Tenants are
still people you invited into *your* yard.

Not for: team inboxes, multi-agent routers, “ChatGPT for work,” **or**
selling per-customer agent instances as a SaaS.

---

## The thing we are not building

A third model keeps showing up: **yard-as-a-service** — Gantree in the
cloud, API that plants a gantry per customer, metering, isolation
between strangers, support, scale.

That is a different company. It wants tenancy, inbound admin APIs,
abuse, regional failover, and a harness that is no longer “one process
you SSH to.” It would fight everything this stack chose (outbound-only
agents, files on disk, no dashboard in the hot path).

If that business appears later, it is a **new product** on top of the
harness, not a mode toggle in Gantree. Gantree stays: one operator, one
Docker host, a handful of named pets.

---

## What you do in it (this is the product)

### Yard home

A board of gantries. Each card: name, alive or not, model, channel, how many
MCP servers are **published** vs **skipped**, last error, last turn time.
Click through. Not a Kubernetes dashboard. A handful of pets, named.

### Plant

New gantry wizard: **yard type first** (home Mini vs cloud VM), then slug, persona seed (or blank PERSONA.md), model (Gemini / ChatGPT /
Ollama), channel + bot token + allowlist, profile (`slim` / `life` /
`life-cast`). Writes an isolated directory, fetches bins, recreates, shows
doctor. Two minutes, not an afternoon of compose.

### Tools (the killer screen)

MCP **is** the grant. Gantree makes that visible.

- Catalog of known servers (google, search, math, garmin, …) plus “custom
  binary”
- Toggle **on** → write `[[server]]`, `tools-fetch`, recreate, wait until
  `/tools` shows the prefix
- Toggle **off** → omit from manifest, recreate (binaries can stay on disk)
- Per server: binary present?, env keys required (name only, not values),
  OAuth session yes/no, skipped-at-boot
- “Needs auth” is a button, not a wiki page: laptop localhost hop **or**
  kick `/auth` in chat and paste the code back here

Add/remove MCP is a first-class action. Editing `mcp.toml` still works; the
UI is a structured editor of the same file, not a second source of truth.

### Persona + secrets

Markdown for `PERSONA.md`; `SELF.md` with a prune hint (the
harness will keep writing it). Secrets as a form that writes `.env` /
`data/` — never git. Token push is explicit and scary. Deploy of config
never copies `data/` by default.

### Run

Start / stop / recreate. Logs. Image pin. Backup `gantry.db` + `SELF.md`.
Env change **recreates** (restart is how you keep a ghost allowlist).

---

## What it is not

Gantree does **not** become the agent’s chat UI. Humans still DM Telegram.
The console is for the person who owns the box.

Gantree does **not** punch inbound ports on the *agents*. They stay
outbound-only. The console binds to **localhost**, LAN, or Tailscale — an
operator plane, not a public website. If you “expose” it, you expose it to
yourself.

Gantree does **not** merge memories or OAuth across gantries. Isolation is
still the feature. One human, one bot, one `data/`. Delete a tryout = delete
that directory.

Gantree does **not** sell agents. No customer signup, no per-seat
metering, no “spin a bot for Acme.” A friend in your yard is still
*your* VM, *their* allowlist, *their* OAuth — a guest, not a tenant.

Gantree does **not** live inside `ai-gantry`. A dashboard in the harness
would tax every turn and break Distroless-as-the-sandbox. Different
binary, different repo, different job.

---

## Two yards, one product

Gantree is **not** “home only, cloud later.” It is one control plane that
installs onto a Docker host. You pick the host at plant time.

```text
[ browser ]
     |
     |  localhost | Tailscale | Cloudflare Tunnel (console only)
     v
[ gantree  — Vinext Node on the host ]
     |
     |  Docker API + files
     v
[ gantry ] [ gantry ] [ gantry ]     Hub image, outbound chat
```

No Kubernetes. No Cloud Run. No Lambda. Chat + SQLite want a process that
stays up — Mini or a tiny VM. Agents still open **zero** inbound ports.
Only the console is reachable, and only through a path you chose.

### Home (Mini / NUC)

`gantree init --yard home`. Compose on the box. Console on `127.0.0.1`
or the Tailscale IP. Cast / host-network (`life-cast`) is allowed here
(mDNS, TV on the LAN). OAuth: browser on the Mini **or** laptop hop then
explicit token push.

### Cloud (your GCE / EC2)

Still **your** machine. `gantree init --yard cloud --provider gcp|aws`.
Same stack on an `e2-small` / `t3.small` (bigger if you bake MCP bins).
Layout follows [examples/hosting](../examples/hosting/) — `/opt/gantree`,
compose, CI pulls the harness image. You are an advanced home user who
did not want a Mini humming in the closet.

The VM has no living-room browser and no Chromecast. So:

- Console is **not** `0.0.0.0` on the public internet. Tailscale **or**
  Cloudflare Tunnel to Gantree only.
- OAuth is **always** the laptop hop.
- `life-cast` is hidden or refused.

GCP if you already live in Gemini + Workspace. AWS if you already live
in EC2 + SSM. Both are “I rented a Linux box,” not “I run a platform.”

### What is identical

Inventory, plant wizard, MCP toggles, doctor, recreate, backups, file
layout (`gantries/<id>/`). Vinext app. Harness image pin. Isolation
(one human, one bot, one `data/`).

### What is not

| | Home | Cloud VM |
| --- | --- | --- |
| Cast / host network | yes | no |
| Open console on localhost | yes | via tunnel / Tailscale |
| OAuth | Mini browser or laptop | laptop only |
| Who patches the box | you | you + the VM image |

A Worker-hosted Gantree (UI on Cloudflare, Docker still on the Mini/VM)
is a **third** skin: same UI talking to a host agent. Not v1. Do not
pretend Workers can `docker compose up`.

---

## Shape

One process on the Docker host: `gantree serve`. Files on disk remain the
source of truth ([design.md](design.md)). The UI is the editor. Diagram and
home vs cloud: **Two yards** above.

Inventory in `gantree.toml` (or a small sqlite) — **no secrets**. Secrets in
per-gantry `.env` / `data/`.

CLI is a back door into the same HTTP API:

```text
gantree serve
gantree ls
gantree doctor kit
gantree grant kit google
gantree revoke kit strava
```

---

## Stack

**Decision:** Vinext (TypeScript) for the console, running as **Node on the
Docker host** — Mini *and* GCE/EC2. Harness stays Go. Dashboard is not Go.

Write `app/` like Next. Run `vinext`. v1 target is `--platform=node` (or
standalone). Dockerode / compose / `tools-fetch` live in route handlers,
not RSC (Vinext’s native-addon footgun).

Cloudflare Workers is **not** where Docker lives. Later, the *same* Vinext
app can sit on Workers as a portal that calls a host agent. v1 does not
need that: Tailscale or Cloudflare Tunnel in front of the Node console is
how you reach a cloud VM.

**Avoid:** Next-on-Vercel as the host (`docker.sock` does not live there).
A SPA plus a mystery API. Harness + console in one Distroless image.

---

## Profiles

Starting templates, not a plugin system. Grant is still “listed in
`mcp.toml`.”

| Profile | Starting grant | Notes |
| --- | --- | --- |
| `slim` | search + math | tryout / stranger |
| `life` | Workspace / health / maps shaped | their OAuth, not yours |
| `life-cast` | life + Cast/YouTube | **home only** (host network / mDNS) |

The Tools screen can go past the profile. Profiles are “plant with this
menu”; toggles are the real grant.

---

## Harness vs gantree

**Push into `ai-gantry` when every consumer benefits**

- `gantry doctor` / richer `status`: channel, each MCP connected vs skipped,
  auth yes/no, persona files present
- Refuse “healthy” when the manifest is all skipped
- Tool errors a model (and a UI) can tell apart: no binary vs no key vs no
  OAuth
- Stable enough file/env contract that a console can write them

**Keep in gantree**

- Fleet UI, plant wizard, MCP catalog UX, OAuth hop UX
- Docker lifecycle, image pins, SSH-or-local host
- Isolation rules, backup, operator auth to the console itself

The harness never learns instance names. Gantree never sits in the token path
of a chat turn.

---

## v1 vs later

**v1 ships two install stories, one runtime**

Linux + Docker + Vinext Node on that host. Wizard: “Mini at home” or
“GCE / EC2”.

- UI: yard, plant, MCP toggles, auth, logs, recreate
- Telegram + Hub `shotah/ai-gantry`
- Home: console on localhost / Tailscale; Cast allowed
- Cloud: console only via Tailscale or Cloudflare Tunnel; laptop OAuth;
  no Cast; compose layout like [examples/hosting](../examples/hosting/)
- Bind `127.0.0.1` by default. Never publish the console as a public
  load balancer.

**Later**

- Vinext-on-Workers as a portal in front of one or more host agents
- systemd yards, not only compose
- token-expiry / skipped-MCP nags in the console

**Not the product**

- Hosted Gantree SaaS / spinning agents for paying customers
- Cloud Run / Lambda / App Runner (wrong shape for long-poll + sqlite)
- Kubernetes
- Shared family brain
- Pairing the *agent* through the console

---

## Done looks like

**Home:** open Gantree on the Mini, three cards, enable Google for Kit,
OAuth in a browser, `/tools` shows `google__…`. No toml archaeology.

**Cloud:** `gantree init --yard cloud --provider gcp`, VM comes up,
Tailscale to the console from a laptop, plant `slim`, laptop OAuth, same
Tools screen. Agents still have no inbound ports.

A stranger does either story without reading anyone’s private git. This
harness repo still hello-paths at `docker compose up` with **zero** tools
and **no** UI.

Open a new public repo when we scaffold. Do not copy `.env` or `data/`
from any private checkout into it.

Harness: [design.md](design.md) · [features.md](features.md) ·
[mcp.md](mcp.md).
