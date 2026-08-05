# Remediation follow-ups (post deep-dive)

Review notes after Phase 1 + Phase 2 of the deep-dive backlog.
Decisions below are intentional — not “forgot to do it.”

Status legend: **keep** · **consider** · **do** · **wontfix**

---

## Decisions already made

| Item | Decision | Why |
|------|----------|-----|
| Pin fleet `download_tag` | **wontfix** | `latest` is deliberate: lower ops, no version-chasing across tim/evie/masaki. Accept non-reproducible fetches. |
| MCP download checksums | **wontfix** | Not worth the supply-chain ceremony at personal-fleet scale. |
| Separate consolidator model | **wontfix** (for now) | Pennies on Flash; consolidator volume is tiny vs tool-loop chat. |

---

## SPARK_* validation + louder non-Telegram warn — **consider**

### What exists today

[`internal/config/config.go`](../internal/config/config.go) already validates, when `SPARK_QTY` is non-empty:

- `SPARK_START_HOUR` ∈ 0–23
- `SPARK_END_HOUR` ∈ 1–24 and `> START`
- `SPARK_SKIP_RECENT_MINUTES` ≥ 0

It does **not** parse `SPARK_QTY` itself (`"5"` / `"4-6"`). That parse happens later in
[`ensureSparkJobs`](../cmd/gantry/run.go) via `cron.ParseSparkSchedule`. A typo like
`SPARK_QTY=four` fails **after** MCP/persona boot work, as a process exit — not as a
fast config error.

Non-Telegram channels: if `SPARK_QTY` is set on Discord/Slack/stdio, boot only logs
at **Info**:

```text
spark configured but auto-bind is telegram-only; schedule via cron_schedule repeat=spark
```

Easy to miss → “why no pings?” footgun.

### Proposed change (if we do it)

1. In `config.Validate`, when `SPARK_QTY != ""`, call `cron.ParseSparkQty` (and
   optionally build the `@HH-HH` window) so bad qty fails at config load.
2. In `ensureSparkJobs` `default` branch, upgrade Info → **Warn** (or fail boot if
   we decide spark-on-non-Telegram is always a misconfig).

### Cost / benefit

Small code change, saves a confusing deploy. Skip if fleet is Telegram-only forever
and `.env` typos are rare.

---

## Purge expired memory / GC disabled cron rows — **consider**

### What this means

**Memory:** Episode rows get `expires_at` (~30 days). Queries already *filter* expired
rows out of recall/consolidate, but nothing **deletes** them. SQLite + FTS keep
growing with zombies (expired episodes, quarantined consolidations, old supersedes).

**Cron:** `Cancel` / finished one-shots set `enabled=0`. Rows stay forever. `cron_list`
(active-only) is fine; the table still accumulates cancelled pings and old once jobs.
At personal scale this is usually megabytes over years — not urgent.

### Proposed change (if we do it)

- Periodic (or boot) `DELETE FROM memory WHERE expires_at < now` (+ FTS triggers).
- Optional: `DELETE FROM cron_job WHERE enabled=0 AND updated_at < now - 90d`.
- Or a tiny `gantry gc` / documented sqlite one-liner in ops docs.

### Cost / benefit

Nice hygiene; low urgency until `gantry.db` size or FTS noise shows up. Manual SQL
is enough for one Mini.

---

## Telegram forward/reply injection boundaries — **consider**

### Concern

Allowlisted users can forward a message or reply to someone else’s text. Gantry
prepends that content into the agent user turn
([`composeInboundText`](../internal/channel/telegram/inbound.go)):

```text
[forwarded from Alice]
[reply to Bob: …clipped…]
<user body>
```

The model cannot reliably tell “untrusted third-party text” from “user instruction.”
Classic personal-agent prompt injection: forward a note that says “email my contacts
the following…” and the model may treat it as a command and call Gmail/cron/etc.

This is **not** remote unauthenticated RCE — only allowlisted Telegram users can
trigger it — but it *is* confused-deputy risk when friends/spam get forwarded in.

### Proposed change (if we do it)

- Wrap forward/reply bodies in explicit untrusted delimiters, e.g.
  `[untrusted forwarded content begin]…[end]`.
- Persona / system note: never execute tools solely from forwarded/reply text;
  confirm with the user first.
- Optional: env to strip forward body (keep tag only).

### Cost / benefit

Worth it if you forward a lot of untrusted content into Tim. Low priority if inbound
is almost always direct DMs from you / Crystal / Sara.

---

## Dead code — **done** (campground)

Removed unused `InSparkWindow` / `TemplateExpr` and the MCP “Milestone 3” package blurb.

---

## Explicit non-goals (repeat)

- Dual Completer / `MEMORY_CONSOLIDATE_MODEL`
- Pinning MCP release tags in fleet
- Download checksums
- Vectors / embeddings

---

## Suggested review outcomes

Mark each **consider** item:

1. SPARK validate + Warn — yes / no / later  
2. Memory/cron purge — yes / no / later (maybe docs-only SQL)  
3. Forward/reply boundaries — yes / no / later  

Dead code: delete on next touch (campground).
