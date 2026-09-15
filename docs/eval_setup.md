# Eval setup

> Why it exists: [evaluation.md](evaluation.md#1-behavioral-regression-closed) ·
> What it checks: [persona_doc_goals.md](persona_doc_goals.md#scenario-checks) ·
> Fixtures: `internal/agent/testdata/eval/`

Three values, two places. The eval replays each fixture against a live model
with the shipped persona seed; it needs the same three `LLM_*` the binary
needs and nothing else. No MCP servers, no Telegram, no Google — every tool
is canned.

| Value | Local | GitHub |
| --- | --- | --- |
| `LLM_BASE_URL` | `.env` | Repository **variable** |
| `LLM_MODEL` | `.env` | Repository **variable** |
| `LLM_API_KEY` | `.env` | Repository **secret** |

## 1. Local

1. Copy the example and fill the three lines at the top:

   ```sh
   cp .env.example .env
   ```

   ```sh
   LLM_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai
   LLM_API_KEY=your-key
   LLM_MODEL=gemini-3.5-flash
   ```

   `.env` is gitignored. Use the model you ship with — the eval is a reading
   of the seed on *that* model, not a model benchmark.

2. Run it:

   ```sh
   make integration-test
   ```

   The target sources `.env`, then runs `go test -tags integration` on
   `TestEval_Live`. A turn is ~15 s on a Flash-class model: seven fixtures
   × 3 runs is ~5 minutes, × 10 is ~20. The target passes
   `-timeout $(EVAL_TIMEOUT)` (default `120m`) because Go's default 10 m
   kills a long run mid-fixture with `panic: test timed out`.

   Without the three values it does not fail, it skips:

   ```text
   --- SKIP: TestEval_Live (0.00s)
       eval_integration_test.go:36: LLM_BASE_URL, LLM_API_KEY, LLM_MODEL required; ...
   ```

3. Narrow it while you tune:

   ```sh
   make integration-test EVAL_ARGS='-eval.only=scoop_at_2'          # one fixture
   make integration-test EVAL_ARGS='-eval.n=10'                     # nervous
   make integration-test EVAL_ARGS='-eval.n=3 -eval.only=no_aims_one_question,spark_gym_no_workout'
   ```

   `-eval.only` takes fixture `name`s (the JSON field, not the file name),
   comma-separated.

   Windows: the target is POSIX shell. Set the three variables in the
   session and run the `go test` line from the Makefile directly.

## 2. GitHub

The release gate reads the key from a **secret** and the endpoint and model
from **variables** — the endpoint and model are not sensitive, and variables
show up in the run log so a wrong one is visible.

**Settings → Secrets and variables → Actions**

- **Secrets** tab → *New repository secret*
  - Name `LLM_API_KEY`, value: the key.
- **Variables** tab → *New repository variable*
  - Name `LLM_BASE_URL`, value: the endpoint (same as `.env`).
  - Name `LLM_MODEL`, value: the model id (same as `.env`).

Or with the `gh` CLI from the repo:

```sh
gh secret set LLM_API_KEY
gh variable set LLM_BASE_URL --body 'https://generativelanguage.googleapis.com/v1beta/openai'
gh variable set LLM_MODEL --body 'gemini-3.5-flash'
```

That is all the wiring. Two workflows already read them:

- **`eval.yml`** — *Actions → eval → Run workflow*. Inputs: `runs` (default
  3) and `only` (a fixture name, blank = all). By hand:

  ```sh
  gh workflow run eval.yml -f runs=5 -f only=scoop_at_2
  ```

- **`release.yml`** — on a `v*` tag push the `eval` job runs first and
  GoReleaser `needs` it. Eval fails → no release. Fix the fixture or the
  persona, then re-run the failed job from the Actions tab (or push the
  next tag).

Without the secret the eval job skips and stays green, so a fork with no key
still releases. GitHub does not hand secrets to fork pull requests, and
`ci.yml` does not call the eval — the free goldens are the PR gate; this is
the release gate.

## 3. Reading a failure

Every failed run prints what the model did, then the reply:

```text
run 2/3 FAIL: expected tool cron_schedule; waiting_for_reply=false, want true
--- tool calls ---
google__calendar_list_events {}
--- waiting=false silent=false jobs=none ---
--- reply ---
Sprint's at 2:30 and the scoop is at 2 — I'll keep an eye on it.
```

| Failure line | Means |
| --- | --- |
| `expected tool X` | The recorder never saw `X`. A call the agent **blocked** (prefix off, never `mcp_enable`d) also never reaches the recorder — that is a real miss, not a harness gap. |
| `expected tool X with args /re/` | `X` was called, but no call's JSON args matched. |
| `order: X before …` | Both called, wrong order (e.g. the tool before `mcp_enable`). |
| `reply should match /re/` · `should not match` | Shape of the reply — a `?`, a bare “got it”. Never a sentence. |
| `N questions, max M` | Counted `?` in the reply. |
| `waiting_for_reply=false, want true` | The model asked but did not put `[wait]` on its own line — the kernel follow-up rule. |
| `silent=true, want false` | `[silent]` (or an empty reply) where a nudge was owed. |
| `expected memory row kind subject` | No live `memory_store` row with that subject after the turn. |
| `no cron job A–B min out (have …)` | Nothing on the board in the window; `followup`-kind wait pokes do not count, only the reminder. |
| `none of any_of held — alt 1: … \| alt 2: …` | Every alternative failed; each is listed with its own reason. |

A rule that holds two runs in three is a rule the persona is not carrying —
that is why every run must pass.

## 4. Tuning

Two kinds of red:

- **The model is right and the fixture is wrong.** It scheduled the scoop
  at +118 minutes and the window was 120–140; it said “buzz you” and the
  regex only knew “ping”. Fix the JSON. Widen the window, add the word to
  the regex, add an `any_of` alternative. The fixture describes the rule,
  and rules have more than one right shape.
- **The persona lost a rule.** It said “nothing today” and stopped; it
  narrated a reminder and set no cron; it asked three questions. Do not
  loosen the fixture. Put the line back in
  `examples/persona/PERSONA.example.md` (and its two lockstep copies), run
  `-eval.only` on that fixture until it holds, then the whole set.

Regexes are Go `regexp` (RE2): `(?i)` for case-insensitive, no lookaround.
Fixtures are validated in `go test ./...` before any model call:

```sh
go test ./internal/agent/ -run 'TestEvalFixtures_WellFormed|TestEvalHarness'
```

## 5. Adding a fixture

Copy the nearest file under `internal/agent/testdata/eval/` and edit. The
table in [persona_doc_goals.md](persona_doc_goals.md#scenario-checks) is the
list of rows; add the row there when you add the file.

| Field | What |
| --- | --- |
| `name`, `why` | `name` is what `-eval.only` matches; `why` is the rule in one sentence, printed at the top of the run. |
| `inbound` | The human's text. `{{+120m}}` becomes the local clock 120 minutes from now (“2:00PM”) — the eval runs on the real clock because `cron_schedule` does. |
| `spark` | Instead of `inbound`: a substring of one `cron.DefaultSparkPrompt` line (“Gym / fitness aim”). Sends a real wake turn. |
| `history` | Prior `user` / `assistant` turns appended to the session first. |
| `memory` | Seed rows: `kind`, `subject`, `content`. `pref/hours` here gives the model `[hours]`. |
| `self` | `SELF.md` body (`- ` bullets). Empty file when absent — that is the “empty SELF.md” scenario. |
| `tools` | Canned MCP tools: `name` (must be `server__name`), `description`, optional `params` schema, `result` returned verbatim every call. |
| `force` | MCP prefixes published without `mcp_enable`. Leave empty to test the off → enable → call path. |
| `expect` | The shape contract — see the failure table above for each key. |

Then:

```sh
go test ./internal/agent/ -run TestEvalFixtures_WellFormed     # parses, regexes compile
make integration-test EVAL_ARGS='-eval.n=1 -eval.only=<name>'   # first live read
```

The three table rows without a fixture yet (`self_note` on a landed joke,
the weight/dinner spark, the Denver flight) need an `expect` that reads
`SELF.md`, or canned search/flights tools — a small addition to
`eval_harness_test.go` each, and one JSON file.
