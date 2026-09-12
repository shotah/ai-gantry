# Todo

Open work. A line is done when tests say so (`go test` on the package
you touched). Mouths: [channels.md](channels.md). Prompt order today:
[architecture.md](architecture.md#prompt-assembly-order). Sibling
lists: pendant [todo.md](https://github.com/shotah/gantry-pendant/blob/main/docs/todo.md)
· [audit_todo.md](https://github.com/shotah/gantry-pendant/blob/main/docs/audit_todo.md)
· cab [todo.md](https://github.com/shotah/gantry-cab/blob/main/docs/todo.md).

---

## User-role clock leak

Kit (the running agent) called this out: a **context auto-hydration
block** sat in the Completer's **raw user message** again this turn,
and asked to "strip context header from client message body on send"
(pendant `audit_todo.md`).

**Where it actually is: this repo, not the mailbox.**

`internal/agent/agent.go` `runTurn` used to do:

```text
userMsg.Content = storeText + "\n\n" + clock
```

`clock` is `[location]` (from `here`) + `[current time]` (week grid,
day-part, tz sermon) + `[hours]`. That is prompt-only — SQLite history
does **not** store it (`TestHandle_PendantGPSLeadsClockFooterOnUserTurn`).
The model still saw it as **the human's words** this turn. **Closed:**
RoleUser is speech; `[harness]` RoleSystem after it holds the clock.

`[memory]` hydration is a **system** message (after history, for the
prompt cache). Different block. Do not fold it into `Text` either.

### Not pendant BE (Worker)

The Durable Object forwards `text` and `context` as separate JSON
fields (`lib/mailbox/frame.ts`). It must **not** merge GPS / battery /
tz into `text`. Do not "fix" this by rewriting bodies in the Worker.

### Mouths (open in those repos)

Happy-path `"near me"` without `[location]` in `text` is **not** the
Cab ticket. Cab still strips a pasted header **on send**. PWA is done.

| Mouth | Repo | Ticket |
| --- | --- | --- |
| PWA | gantry-pendant **FE** | closed (`stripHarnessContext` + geo-only `context`) |
| Cab | gantry-cab | `docs/todo.md` Small |
| Worker | gantry-pendant **BE** | no work |

### This checkout

- [x] Completer **RoleUser** `Content` is `storeText` only (what they
      typed / `[photo]` / steers). No `[location]`, `[current time]`,
      `[hours]` in that string.
- [x] Clock / location / hours stay **prompt-only**, tagged, recency-
      weighted. Do **not** persist them in `session` (already true —
      keep the pin_test history assertion).
- [x] Small-model gate: do **not** put `[current time]` *before* their
      words (calendar/tool fixation). Tagged **`[harness]` RoleSystem
      after** RoleUser (not unlabeled trailing system, not leading
      time). Gate: `internal/agent/pin_test.go`,
      `TestAgent_Handle_MemoryHydration`.
- [x] If inbound `Message.Text` already contains those tags (paste /
      old client), strip before store and before hydrate FTS
      (`stripHarnessContext`).
- [x] `docs/architecture.md` prompt order matches the loop (hydration
      after history; `[harness]` clock after user speech).
- [ ] Phone `context.at` / `tz` / `surface` / `battery` / `net` —
      PWA no longer stamps them (geo only). Old Cab may still send
      them. Unused here except `geo` → `here`. Not this ticket.

### Sibling (mouths — not this tree)

PWA send strip is closed in gantry-pendant (`stripHarnessContext` +
geo-only `context`). Cab still owns its Kotlin strip (`docs/todo.md`
Small). Do not implement Cab from this checkout.
