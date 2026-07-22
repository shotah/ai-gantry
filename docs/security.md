# Security

ai-gantry is a **personal agent runtime** with full tool autonomy inside a
container. Security is mostly composition + least exposure, not an in-process
permission framework.

## Threat model (who / what)

| Actor | Goal we care about |
| --- | --- |
| Random Telegram user | Talk to the bot / burn LLM+tool quota |
| Compromised allowlisted account | Abuse tools (mail, calendar, fitness APIs…) as the operator |
| Malicious or buggy MCP binary | Exfil secrets, escape container, DoS the host process |
| Compromised LLM / prompt injection | Coerce tool calls or memory writes against operator intent |
| Host / volume attacker | Read `gantry.db`, `.env`, `/secrets` |
| Network observer | Sniff tokens if TLS is broken (we rely on HTTPS for LLM + Telegram) |

Out of scope for v1: multi-tenant isolation inside one process, formal sandbox
profiles per tool, enterprise SSO.

## Controls that ship

### Network posture

- **Outbound only.** No listen port. Telegram long-polls out; LLM is HTTPS out.
- **Healthcheck is exit-code**, not an HTTP endpoint (`gantry status` → SQLite
  heartbeat). Reduces accidental attack surface.

### Identity & access (Telegram)

- **Allowlist only** (`TELEGRAM_ALLOWED_USERS`). Empty list fails boot.
- No pairing codes, no “bind this chat” UX (deliberately skipped — pairing was
  an operational foot-gun elsewhere).
- Stdio channel is for local/dev; treat it as equivalent to local shell access.

### Secrets

- Secrets live in **env** and/or **read-only mounts** (`/secrets`), not in the
  image layers you push publicly.
- MCP children inherit process env unless the manifest overrides `env` — assume
  every listed server can see what the gantry sees unless you split containers.

### Tool trust boundary

- Manifest membership **is** the grant. There is no secondary ACL.
- Tool names are prefixed `{server}__{tool}` (collision safety, not auth).
- Results truncated (`TOOL_RESULT_MAX_CHARS`); iterations capped
  (`TOOL_MAX_ITERATIONS`) — availability / cost controls more than authz.

### Runtime image

- Distroless/static, **nonroot** (uid 65532), no shell.
- Static MCP binaries required — smaller, fewer shared-lib surprises.
- Persona + manifest mounts intended **read-only**.

### Data at rest

- One SQLite file: sessions, memory, heartbeat. Protect the volume like a
  mailbox dump — it is.
- Memory is correctable (`memory_forget`) and inspectable (`sqlite3`); see
  [memory.md](memory.md).

### Shutdown

- Signal → drain in-flight turn → close MCP children. Reduces half-written
  tool side effects from hard kills mid-call (best-effort, not transactional
  across external APIs).

## Tradeoffs (intentional)

| Choice | Benefit | Cost |
| --- | --- | --- |
| No inbound ports | Tiny network surface | Ops must use exit-code health + logs |
| Allowlist, no pairing | Simple, fails closed | Rotating users = edit env + restart; shared phones are risky |
| Full tool autonomy | Useful personal agent | One bad prompt/tool ≈ full API power of mounted servers |
| Secrets via env | Compose-native | Visible to all children; easy to leak via debug dumps |
| Structured memory in SQLite | Greppable, deletable, no vector SaaS | Cleartext facts on disk; volume theft = privacy incident |
| Auto-save memory **off** | Fewer hallucinated “facts” | Relies on model discipline + consolidator |
| Persona outranks memory | Identity stays operator-owned | Model must notice contradictions; not cryptographically enforced |
| Token **estimates** (chars/4) | No tokenizer dep | Rare mis-trims under weird tokenizers |
| Single-container 1:1 | Blast radius = one persona | Many secrets still co-located with that persona’s tools |
| Distroless nonroot | Harder live compromise | Debugging requires sidecar / rebuilt image |

## Residual risks

### Prompt injection → tool abuse

The model can call any mounted tool. A crafted email/doc (via a workspace MCP)
can try to steer the agent. Mitigations today: allowlist who can chat, careful
persona instructions, don't mount tools you wouldn't run yourself, keep
`TOOL_MAX_ITERATIONS` sane. **Not** mitigated: formal dual-control, human
confirm for destructive tools.

### MCP child = trusted code

A malicious binary in `mcp.toml` has the same privileges as the gantry user
inside the container (filesystem mounts, env, outbound net). Treat the
manifest like a rootkit allowlist. Prefer your own static builds pinned by
image/composition.

### Memory poisoning

Deliberate `memory_store` of false facts, or consolidator promotion of junk
episodes. Mitigations: no auto-save, `memory_forget`, persona precedence,
operator `sqlite3` edits. Risk remains if the model “trusts” hydration
silently — persona note helps but is soft.

### Data exfiltration paths

1. LLM provider sees prompts (persona, history, memory hydration, tool
   results). Choose providers/regions accordingly.
2. Tool backends (Google, Strava, …) see whatever the agent sends.
3. Logs on stderr may include tool errors / user text at debug levels —
   ship `LOG_LEVEL=info` in prod; scrub if you centralize logs.
4. Volume snapshots of `/data` include chat + memory.

### Availability / cost DoS

An allowlisted user (or stolen session) can burn LLM and upstream API quota.
Caps (history, tool chars, iterations) bound **context size**, not spend.
No per-user rate limit beyond Telegram worker concurrency (1).

### Heartbeat false health

`gantry status` only proves the process recently wrote SQLite. It does **not**
prove Telegram connectivity or LLM health. Don't treat it as end-to-end UX
liveness.

### Shutdown races

Drain waits up to ~2 minutes. External tool calls may still complete after
the user-visible reply path is torn down, or fail mid-flight leaving remote
side effects. External APIs are not two-phase commit.

## Hardening checklist (operator)

- [ ] `TELEGRAM_ALLOWED_USERS` is minimal and numeric IDs are verified
- [ ] `.env` / secrets mounts are not world-readable on the host
- [ ] `/data` volume encrypted or on a trust boundary you accept
- [ ] MCP manifest only lists servers this persona needs
- [ ] Persona instructs “confirm before irreversible sends/deletes” if you care
- [ ] `MEMORY_CONSOLIDATE_MINUTES` understood (or `0` if you don't want sleep-cycle writes)
- [ ] Image + MCP binaries rebuilt from known sources; no shell in prod
- [ ] Separate containers for separate trust domains (work vs personal)

## Related

- [architecture.md](architecture.md) — where controls sit in the process
- [design.md](design.md) — why the surface is small
- [choices.md](choices.md) — auth and packaging decisions
