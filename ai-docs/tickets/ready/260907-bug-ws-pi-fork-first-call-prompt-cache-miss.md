---
title: "Pi fork's first model call is a full prompt-cache miss over the whole inherited lead context"
related:
  260904-feat-ws-pi-side-thread-fork-question-surface: origin of the three decisions this ticket supersedes (append-only system prompt with "prefix cache irrelevant", allowlist-based `ws-fork` removal, directive-in-system-prompt split); its `.done/` text is not edited
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role: consumer — the fork's tool array equals the lead's at spawn, and the fork authoring profile loads after the prefix through the deferred-tool channel decided there (spike evidence recorded in that ticket's Background)
  260907-feat-ws-pi-persistent-explore-deep-research: sibling adapter work in flight on another impl branch; no shared files expected beyond `spawner.ts` argv helpers
spec:
  - 260905-pi-side-thread-fork-task-thread
  - 260905-pi-side-thread-owner-question-surface
  - 260905-pi-lead-bootstrap-system-prompt
sage-review-design: recommended
---

# Pi fork's first model call is a full prompt-cache miss over the whole inherited lead context

## Background

`ws-fork` (and the discussion fork behind `ws-ask`) spawns `pi --fork <lead session>`
so the child inherits the lead's full context. The point of inheriting is that
the provider has already cached that context; the fork's first call should
cost roughly cached-read price for the inherited part plus the new task
message. Measured on this worktree's Pi session logs (openai-codex
`gpt-6-astra`, input $10/M, cacheRead $1/M), every fork's first own call
instead reads zero tokens from cache:

| fork (session file prefix) | uncached input | cacheRead | cost |
|---|---|---|---|
| `2026-09-07T10-49-57` | 101,728 | 0 | ~$1.02 |
| `2026-09-06T07-56-37` (write-ticket fork) | 89,886 | 0 | $0.90 |
| `2026-09-05T13-47-35` / `14-21-18` | 33,188 / 34,224 | 0 | $0.33 / $0.34 |
| eleven more `gpt-6-astra` forks, 09-05 | 21k–30k | 0 | $0.22–0.31 |
| `2026-09-05T05-59-16` (see below) | 12,334 | 10,112 | $0.135 |

Seventeen forks, first-call total $3.45; at cached-read price the same calls
would have cost about $0.40. The lead itself hits normally (231 calls, 3
misses, steady state `input ~200–500 / cacheRead ~29k`). A fork's second and
later calls also hit normally. Every fork *resume* re-passes the same divergent
system-prompt file and pays the miss again. write-ticket forks spawn late in a
discuss session, at the lead's largest context, so the per-spawn cost tracks
the top of the table, not the bottom.

The one partial hit is the diagnostic: `05-58-23` and `05-59-16` are two forks
of the same parent one minute apart, in two processes with two different
`prompt_cache_key`s. The second read 10,112 tokens from the first's prefix and
then diverged, and 10k is where the system prompt's ws block sits. So (1)
byte-identical prefixes do hit across processes and across cache keys, and (2)
the divergence point is exactly the seams listed below.

### Why the prefix diverges (audited, file references as of `c891ba0e`)

Request body for openai-codex is `{instructions, tools, input, prompt_cache_key,
...}` (`pi-coding-agent` bundle chunk `openai-codex-responses-*.js`,
`buildRequestBody`), full history every call. The fork's first request differs
from the lead's last request in five places:

1. **Directive in the system prompt.** `fork.ts` writes `buildForkDirectiveText()`
   to a temp file passed as `--append-system-prompt` (`spawner.ts` fork argv).
   Pi inserts it as `appendSection` *before* `<project_context>`, skills and the
   cwd line (`core/system-prompt.js`), so everything after it re-hashes.
2. **The lead's own `~/.pi/agent/APPEND_SYSTEM.md` is dropped.** An explicit
   `--append-system-prompt` replaces the discovered file
   (`core/resource-loader.js`, `appendSystemPromptSource`). The file exists on
   the owner's machine, so the fork also *loses* text the lead has.
3. **The ws block's session key.** `lead-bootstrap.ts` appends
   `buildWsBlock(manualSnapshot, guide, skills)` to both lead and fork system
   prompts (`isLeadOrFork`), but `bridge.ts` fetches the fork's
   `manualSnapshot` with the fork's own freshly minted key, so the block ends
   in a different `## Session Key` line and per-session agenda/todo state. The
   block is the tail of `instructions`, and `instructions` precedes `tools` and
   `input`, so this alone invalidates the entire replayed conversation.
4. **Tools array.** `computeForkToolSurface` deletes `ws-fork`, `ws-ask`,
   `ws-resolve` (`FORK_EXCLUDED_TOOL_NAMES`) from the lead's active list. The
   system prompt's tool section is unaffected (no ws tool sets
   `promptSnippet`), and ordering is preserved end to end (`--tools` order seeds
   `activeToolNames`; the fork's bootstrap reshape is an order-preserving no-op
   for role `fork`), so the delta is exactly three in-place deletions.
5. **`prompt_cache_key`** is the Pi session id (`core/sdk.js`), and `--fork`
   always mints a new id. This is a routing hint, not a partition (see the
   partial hit above), so it is the least important of the five.

Model, thinking level (restored from the copied `thinking_level_change` entry),
cwd, context files, skills and the replayed messages (`SessionManager.forkFrom`
copies entries verbatim; both processes run the same `convertToLlm`) are
identical.

### Decisions superseded, without editing `.done/`

`260904-feat-ws-pi-side-thread-fork-question-surface` is done and is not
edited (policy). Three of its recorded decisions are superseded by this
ticket; they are quoted here so the history stays recoverable:

- "System prompt: **append only**. Replacement is rejected by the user; prefix
  cache is explicitly irrelevant to this design." — The measured cost above is
  the price of "irrelevant". Append-only still holds; this ticket moves the
  *fork-specific* append out of the system prompt entirely.
- "Only side-thread recursion is blocked, structurally, by removing `ws.fork`
  from the fork's allowlist ... without a prompt." — Replaced by a handler-side
  refusal keyed on the spawner-set role env marker. That is still mechanical
  (the model cannot unset the env), not a prose gate; the tool merely stays
  in the array so the array matches the lead's.
- §4 re-decision (2026-09-05): directive in the system prompt, anti-bleed frame
  in the first message. — Both now live in the first message. The anti-bleed
  concern it was answering must be re-verified live (Phase 1 verification).

## Decisions

### Prefix-identical fork spawn

The fork's `instructions` and `tools` must be byte-identical to the lead's at
spawn time; the only new bytes are the appended first user message.

1. **No fork-specific `--append-system-prompt`.** `ws-fork` and the discussion
   fork stop writing the directive temp file; the fork argv carries no
   `--append-system-prompt`, which also restores the lead's discovered
   `APPEND_SYSTEM.md`. `systemPromptPath` becomes optional on the spawn/resume
   record for fork-family spawns; worker/execute-worker spawns keep their
   rendered playbook file unchanged.
2. **Directive moves into the first message.** `buildForkInitialMessage`
   (and the discussion-fork equivalent) prepends the directive text, the
   fork's own ws session key, and a plain-prose "do not call `ws-fork`,
   `ws-ask` or `ws-resolve`; you are the fork, do the task" line. The
   existing directive constraints (short natural language, no identity
   framing, no ALL-CAPS overrides, both `kind` values named, all
   `REQUIRED_FINAL_REPORT_FIELDS` listed) now govern the merged message.
3. **Lead's ws block verbatim.** The fork does not fetch its own
   `workflow_manual` snapshot for the system prompt. The lead passes its
   rendered `manualSnapshot` (the exact string in its own `wsBlockBaseRef`)
   to the fork out-of-band — a file whose path travels in a new env var next
   to `WS_PI_PARENT_SESSION_KEY` — and the fork's bootstrap uses it as
   `manualSnapshot`. The guide text and skills block are already computed
   identically. The `staticBodySnapshot` path (`playbook.read
   lead-workflow-manual`) is unaffected. Consequence: the fork's system prompt
   names the *lead's* session key; the correction is item 4.
4. **Fail-loud key correction, own key issued.** The fork still gets its own
   lead-scope ws session key (unchanged). A ws call from a fork that carries
   the parent lead key is **refused** with an error naming the fork's own key
   ("this session is a fork; its session key is `<own>`; retry with it"),
   replacing today's silent rewrite in `normalizeSessionKey`
   (`parentLeadKey` branch). The first message (item 2) states the own key
   up front so the refusal is the backstop, not the normal path.
5. **Tools array equals the lead's.** `FORK_EXCLUDED_TOOL_NAMES` becomes empty;
   `computeForkToolSurface` returns the caller's active list unchanged (the
   `ws-report-to-lead` append was already a no-op on a real lead and is
   dropped). `ws-fork`, `ws-ask` and `ws-resolve` handlers check
   `readSpawnRole(process.env) === "fork"` and return a tool error: "you are a
   fork; do not delegate or open owner questions — do the task and report via
   `ws-report-to-lead`". This keeps the `ws-ask` rationale from 260904 §3
   (no respondent-less threads from a fork) as a mechanical refusal.
6. **Cache-key affinity.** A `before_provider_request` handler in the fork
   rewrites `prompt_cache_key` to the lead's Pi session id, delivered in the
   same env channel as item 3. Cheap, and removes the one remaining
   routing-side uncertainty; the partial-hit evidence shows it is not strictly
   required, so it must not be able to fail the spawn (missing env → no-op).
7. **Resume path.** Fork resumes no longer re-pass a system-prompt file, so a
   resumed fork's prefix is again the lead's prefix plus the fork's own
   history.
8. **Fork-only tools load after the prefix, never at spawn.** Any tool a fork
   needs beyond the lead's active set (the ticket-authoring profile once the
   lead profile ticket lands) is activated by the fork's first tool call —
   an adapter loader whose `execute()` grows the active set — so Pi records
   `addedToolNames` on that result and the provider adapter emits the
   schemas as `additional_tools` in `input` after the cached prefix. Spike
   on gpt-6-astra (2026-09-07): 6,006 uncached on the first call, then 306
   uncached / 5,888 cached on the call right after the load, and the prefix
   kept hitting after the added tools were called. The loader is add-only and
   role-gated to `fork`; nothing in a fork ever removes an active tool.

Rejected: keeping the directive in the system prompt and only fixing items 3–5
(any system-prompt delta before `tools`/`input` re-hashes everything);
computing the fork's tool surface from a fork-specific profile (a different
array is a miss regardless of prompt text); giving the fork the lead's ws
session key outright (todo/agenda would be shared with the lead — not
decided, and not needed for the cache); keeping the silent parent-key rewrite
(owner chose fail-loud: a fork that calls with the lead's key is acting as
the lead, and the refusal re-anchors its identity at the moment it slips —
worth more than the one turn the rewrite would save).

## Constraints

- Adapter-only (`agents-plugin-pi/`); no ws-mcp or shared rsrc edits.
- `.done/260904` is not edited. New spec text supersedes; the ticket's
  quotations above are the recovery pointer.
- Worker/execute-worker spawns (`--session`, rendered playbook via
  `--append-system-prompt`) are out of scope and must not change.
- Anti-bleed: the merged first message must not reintroduce the live role-bleed
  260904 §4 recorded (fork re-running the lead's plan). If dogfood shows bleed,
  strengthen the *message*, never the system prompt.
- The env channel for the lead's snapshot and session id must degrade to
  today's behavior (fork fetches its own snapshot; no cache-key rewrite) when
  unset, so an old lead spawning with a new adapter build cannot break.

## Prior Art

- `src/fork.ts`: `buildForkDirectiveText`, `buildForkInitialMessage`,
  `computeForkToolSurface`, `FORK_EXCLUDED_TOOL_NAMES`, directive temp file at
  spawn.
- `src/ask.ts`: `buildDiscussionForkDirectiveText` and the same temp-file spawn
  for the discussion fork; resume paths re-pass `systemPromptPath`.
- `src/spawner.ts`: fork argv (`--fork`, `--append-system-prompt`, `--tools`),
  `WS_PI_PARENT_SESSION_KEY` delivery, `systemPromptPath` on spawn/resume
  records.
- `src/bridge.ts`: `normalizeSessionKey` (`parentLeadKey` silent rewrite to
  replace with refusal), the `workflow_manual` snapshot fetch gated on
  `isLeadOrFork`.
- `src/lead-bootstrap.ts`: `buildWsBlock`, `wsBlockBaseRef.manualSnapshot`.
- Pi hook `before_provider_request` (`core/extensions/types.d.ts`) — handlers
  may replace the payload; unused by the adapter so far.
- Tests pinning current behavior: `test/spawner.test.ts` fork argv
  `deepEqual`; `test/fork.test.ts` `FORK_EXCLUDED_TOOL_NAMES` membership and
  five `computeForkToolSurface` cases, directive/initial-message shape;
  `test/ask.test.ts` excluded-set assertions and discussion directive shape;
  `test/lead-bootstrap.test.ts` ws-block composition.

## Spec Impact

`ai-docs/spec/pi-adapter-runtime.md`: rewrite the directive/tool-surface
passages of `{#260905-pi-side-thread-fork-task-thread}` and
`{#260905-pi-side-thread-owner-question-surface}` (no fork
`--append-system-prompt`; directive and own-key statement in the first
message; tool array equals the lead's; role-keyed refusals; cache-key
affinity), and add to `{#260905-pi-lead-bootstrap-system-prompt}` that a fork
reuses the lead's rendered manual snapshot verbatim and refuses parent-key ws
calls. Caller-visible change: a fork sees `ws-fork`/`ws-ask`/`ws-resolve` in
its tool list but gets a refusal on use; a fork's first call is billed at
cached-read price for the inherited context.

## Phases

### Phase 1: Byte-identical fork prefix with fail-loud role and key correction

Implement Decisions 1–7 for both `ws-fork` and the discussion fork, update the
tests listed under Prior Art, and update the spec passages under Spec Impact.

Verification:

- Unit: fork argv has no `--append-system-prompt`; `computeForkToolSurface` is
  identity; `buildForkInitialMessage` output contains the directive, the
  fork's own key, both report `kind`s and all required final fields, and no
  identity framing / ALL-CAPS; role-`fork` calls to `ws-fork`/`ws-ask`/
  `ws-resolve` return the refusal; a ws call with the parent key from a fork
  returns the refusal naming the own key, while the same call from a lead is
  unaffected; the fork bootstrap uses the delivered snapshot when the env is
  set and falls back to its own fetch when unset; `before_provider_request`
  rewrites `prompt_cache_key` only when the lead id env is set.
- Live (owner-run, gpt-6-astra): from a lead with ≥20k context, spawn a
  `ws-fork`; the fork's first assistant `usage` in its session jsonl must show
  `cacheRead` ≥ 90% of `input + cacheRead`. Repeat once for the discussion
  fork and once for a fork resume. Confirm no role-bleed on a task that
  previously bled (fork does its own task, reports `final`).
- Live, deferred load: with a loader present, the fork's call right after
  the loader shows `cacheRead` ≥ 90% of `input + cacheRead` and the loaded
  tools are callable; the session file carries `addedToolNames` on the
  loader's tool result.
- Regression: worker/execute-worker argv and `systemPromptPath` handling
  unchanged; full adapter test suite green.
