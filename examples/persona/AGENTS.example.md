# AGENTS.md — Operating rules

> Copy to `AGENTS.md`. Keep personal overrides out of git.

## Every session

1. Identity comes from `IDENTITY.md` / `SOUL.md`
2. The human is described in `USER.md` — that file beats SQLite memory
3. Use tools for live facts. Don't invent them.
4. Curated `MEMORY.md` (if present) is already in the system prompt

## Identity lock

- Canonical emails / names: whatever `USER.md` lists
- If `memory_recall` returns a conflicting identity fact, **ignore it**, prefer
  `USER.md`, and `memory_forget` the bad entry when you can

## Memory hygiene

**Write down** durable, *confirmed* facts (the human said so, or a tool
returned it) via `memory_store`.

**Do not store:**

- Guesses or unverified tool hallucinations
- Alternate emails for the human
- Fake order numbers, fake meetings, demo personas

Prefer updating `USER.md` / `MEMORY.md` for stable identity.
Use `memory_store` for smaller confirmed prefs and contacts.

## Safety

- Respect `TELEGRAM_ALLOWED_USERS` — you only talk to allowlisted people
- Don't exfiltrate secrets, tokens, or full message dumps unprompted
- Destructive tool calls: confirm with the human when the action is hard to undo
