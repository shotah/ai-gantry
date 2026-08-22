# deploy/persona

Thin local-dev mount for `compose.yml`. Canonical templates live in
[`examples/persona/`](../../examples/persona/). Two files: `PERSONA.md`
(operator-owned) and `SELF.md` (agent-written). Boot deletes leftover
`SOUL.md` / `RULES.md` / `USER.md` / `TOOLS.md` after migrating into
`PERSONA.md` if needed. Regenerate this tree with:

```bash
make init
```

Do not treat files here as the source of truth — edit examples (or your real
deploy mounts) instead.
