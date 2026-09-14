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
  - 260903-pi-bridge-session-key-fill-forward
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: aa6350281b58e25b
sage-review-design-reviewed: aa6350281b58e25b
completed: 2026-09-08
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
   fork stop writing the directive temp file and stop passing that directive
   as a system-prompt append. Preserve the lead's effective append bytes,
   whether discovered or explicitly supplied, under Review closure; the
   choice of transport must not impose a new launch restriction.
   `systemPromptPath` becomes optional on the spawn/resume record for
   fork-family spawns; worker/execute-worker spawns keep their rendered
   playbook file unchanged.
2. **Directive moves into the first message.** `buildForkInitialMessage`
   (and the discussion-fork equivalent) prepends the directive text, the
   fork's own ws session key, and a plain-prose "do not call `ws-fork`,
   `ws-ask` or `ws-resolve`; you are the fork, do the task" line. The
   existing directive constraints (short natural language, no identity
   framing, no ALL-CAPS overrides, both `kind` values named, all
   `REQUIRED_FINAL_REPORT_FIELDS` listed) now govern the merged message.
3. **Lead's ws block verbatim.** The fork does not fetch its own
   `workflow_manual` snapshot for the inherited system prompt. Transfer the
   lead's complete rendered block out-of-band, including manual, guide and
   skills. The guide is locally loaded and skills are live-resolved from
   each process's command list, so independent reconstruction does not
   guarantee equality. Preserve the separate `staticBodySnapshot` mapping
   path (`playbook.read lead-workflow-manual`) without coupling its success
   to inherited-block availability. Consequence: the fork's system prompt
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

### Review closure (owner-approved)

The following refinements supersede narrower or unconditional wording in
Decisions 1–7 above; the first-message directive and fail-loud parent-key
refusal remain settled. These are implementation-completeness corrections,
not a new policy or a reason to widen this fix into the lead-profile work.

- **Full effective prefix.** Transfer the complete rendered ws block actually
  used by the lead (manual, guide and skills), not only `manualSnapshot`.
  Do not rebuild its guide/skills from the child's current files. A failed
  independent static-body mapping fetch must not discard a delivered block.
  Preserve effective lead prompt bytes/inputs, including explicit append
  overrides: dropping the fork-specific append restores discovery but does
  not alone prove equality. Test changed resources and explicit overrides.
  No fork-specific directive belongs in the system prompt. A general
  extension-cloning framework is out of scope; if preservation requires
  rejecting a supported launch or restricting functionality, escalate that
  policy choice rather than silently changing behavior.
- **Key readiness.** Acquire the child's own key before the first new message
  reaches the provider, through deterministic child-side transformation or
  a readiness exchange. The parent cannot assume a key minted only after
  child startup is already available. The exact readiness/transformation
  boundary is an implementation choice, verified before the first provider
  call rather than a pending owner policy decision. Never insert a
  placeholder or parent key; missing own-key readiness must not let a
  parent-key call through.
  After restart establish the current own key without rewriting inherited
  messages; stale historical own keys must not route into another session.
- **Persistent fork metadata.** Retain the original full rendered block,
  effective prompt inputs, parent ws key and cache-affinity id through
  dormant resume, task sidecar serialization/parsing/rehydration and
  discussion-thread capture/rehydration, including repeated restarts.
  Do not substitute the restarting lead's current snapshot or identity.
  `systemPromptPath` optionality covers every fork recovery reader, not only
  spawn types. Snapshot lifetime must survive temporary-file cleanup and
  sidecar transitions for the fork's lifetime.
- **Bounded equality guarantee.** Preserve actual serialized tool definitions
  and order, not just names. Verify child registration and schema equality.
  Cache-prefix equality applies to the same effective provider/model/API
  compatibility configuration. Explicit model overrides remain supported
  but are outside the cross-model cache-reuse guarantee.
- **Body affinity only.** Role-gate the request hook to forks and restrict
  `prompt_cache_key` rewriting to supported cache-enabled API payloads.
  Missing metadata, unsupported providers and disabled caching are no-ops.
  This does not share transport/session identity or eliminate provider-side
  routing uncertainty. Clear fork-only metadata from descendant worker and
  explore environments. Effective prefix comparisons must accommodate
  provider continuation/delta transports rather than assume every wire
  request contains full history.
- **Task versus discussion.** Task forks keep both report kinds and all
  required final fields. Discussion forks keep the existing owner dialogue
  and short decision-summary behavior, not task-final fields or a task
  completion directive. Both receive their own-key correction and role
  refusal reminders without changing their distinct completion semantics.
- **Scope and evidence.** Decision 8 is a consumer interface, not this phase's
  implementation or acceptance gate. The lead-tool-profile ticket already
  owns its loader and post-load live verification and is not a prerequisite
  of this fix. Historical cache measurements are not a fresh reproduction.
  The adapter guarantees prefix preservation under the stated conditions;
  provider retention/routing, cache expiry, compaction and new suffix size
  still affect observed hits and billing.

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
directive `--append-system-prompt`; directive and own-key statement in the first
message; tool array equals the lead's; role-keyed refusals; cache-key
affinity), and add to `{#260905-pi-lead-bootstrap-system-prompt}` that a fork
reuses the lead's full rendered ws block and effective prompt inputs verbatim
and refuses parent-key ws calls. Include restart metadata lifetime, own-key
readiness, provider guards and the same-model boundary from Review closure.
Also update `{#260903-pi-bridge-session-key-fill-forward}`: replace its
silent parent-key rewrite promise with the fork's fail-loud refusal while
preserving ordinary lead fill-or-forward behavior.
Caller-visible change: a fork sees `ws-fork`/`ws-ask`/`ws-resolve` in its tool
list but gets a refusal on use; the preserved inherited prefix is eligible
for cached reads without guaranteeing provider billing.

## Phases

### Phase 1: Byte-identical fork prefix with fail-loud role and key correction

Implement Decisions 1–7 as refined by Review closure for both task and
discussion forks, including dormant and restart recovery. Update tests and
spec passages under Spec Impact. Decision 8 is owned by the lead-profile
consumer ticket and does not block this phase.

Verification:

- Unit: no fork-specific directive append; effective explicit lead append
  configuration is preserved; `computeForkToolSurface` is identity; task
  initial messages contain the own key, directive, both report kinds and
  required final fields without identity framing / ALL-CAPS. Discussion
  messages retain their dialogue/summary semantics. All three forbidden
  side-thread tools refuse in fork handlers; parent-key ws calls refuse and
  name the current own key; lead behavior is unchanged. Key readiness
  precedes the first new provider call, and missing readiness never forwards
  the parent key. Delivered full-block reuse and legacy absent-metadata
  fallback are covered. Affinity rewriting is fork-only, provider-aware,
  cache-enabled and metadata-dependent; other cases are no-ops.
- Offline integration: compare effective provider-prefix bytes and actual
  serialized tool definitions/order for task spawn, discussion spawn,
  dormant resume, task-sidecar restart and discussion restart, including
  repeated recovery. Cover explicit append overrides, changed guide/skills
  resources, current/stale own keys, snapshot lifetime, missing or changed
  tool registration, absent metadata, unsupported providers, disabled
  caching, model overrides and descendant env isolation.
- Live (owner-run, gpt-6-astra): from a lead with at least 20k context,
  spawn a task fork, discussion fork and resume a fork. Identify the first
  NEW assistant usage in each JSONL, excluding inherited entries. Target
  `cacheRead >= 90% of input + cacheRead`; record model/API, timing,
  compaction and uncached suffix conditions. Investigate lower ratios
  without treating them alone as proof of unequal bytes. Confirm no
  task-role bleed on a previously affected task (own task, final report)
  and correct discussion behavior. Record unrun live checks as pending,
  never as passed by offline tests.
- Deferred-load live verification remains exclusively in
  260907-feat-ws-pi-lead-tool-profile-and-orchestrator-role.
- Regression: worker/execute-worker argv and `systemPromptPath` handling
  unchanged; full adapter test suite green.

### Result (58c5a5d8) - 2026-09-08

Phase 1 implementation is reviewable at `58c5a5d8`; live acceptance is **not
complete** and this ticket remains in `ready/`. Source range:
`7e7788b1299cf722e9c2f5955d59cd0c4cfe98a1..58c5a5d8ec3650f1d46c3cc96210d7ed6fc17d24`
on `impl/track/pi-agent/charm-muck-keg`.

Task and discussion forks now reuse the full effective lead prompt and actual
ordered callable definitions, move directives into their distinct first-message
forms, establish a current own key before work, and refuse parent/stale keys
rather than rewriting them. Original captures survive dormant and repeated
recovery; fork-only metadata is cleared from worker/explore descendants. Fork
source-extension loading works without a new global-install requirement. Guarded
Codex body affinity is independent of prompt restoration and preserves child
transport identity. The four linked spec anchors describe this implemented
contract; historical verification is explicitly distinguished from current gates.

The accepted legacy absent-metadata local-snapshot/no-affinity exception remains.
Malformed present context does not enter that fallback. No general fail-closed
paid-prefix policy, historical cache-annotation freezing, provider replacement,
or Decision 8 loader/profile implementation was adopted. Native Anthropic marker
advancement and Codex continuation deltas remain intentional, explicitly tested
representations, not normalized-away differences. Worker/execute-worker prompt
contracts remain unchanged; no shared Go/rsrc or historical done-ticket edits.

Review dispositions at the completed source head:

- C1 — independently clean: first new process input carries the current own key
  and role refusals without changing inherited history or duplicating later frames.
- C2 — independently clean: initial/resume readiness and actual-registration
  checks precede work; invalid input readiness blocks a provider turn.
- C3 — independently clean: child-bound historical own keys and known parent keys
  are refused across recovery, while ordinary explicit-key forwarding remains.
- C4 — independently clean: final effective prompt capture, rendered inputs,
  restarted-lead capture and never-paid initial discussion composition.
- C5 — independently clean: effective configuration/effort checks and supported
  body-only affinity, including disabled-cache and changed-model continuations.
- C6 — independently clean: source-extension spawn/resume loading and discovery
  deduplication without fork directive append.
- I1–I5 — **implementer-reported fixed, not independently re-reviewed**:
  descendant environment isolation; canonical environment bindings; actual
  production adapter/SDK lifecycle coverage; side-effect-free role-handler
  refusal coverage; exact provider-native transition/recovery assertions.

Verification evidence (offline, not a cache-hit or billing verdict):

- Read the complete final relay log
  `a98fa8d2-02-fork-cache-relay-verification.log` under
  `/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d@9f097df0/review-paths/`:
  full `npm test` with a compact reporter, **1260 passed, 0 failed, cancelled,
  skipped or todo; 173 suites**. This is reused source verification, not a fresh
  documentation-delegate test run; adapter source/tests are unchanged at closeout.
- Read sibling `a98fa8d2-01-fork-cache-review-dispositions.md` and
  `a98fa8d2-03-fork-cache-critical-rereview.md`. Independent re-review reports
  **12 passed** across six Critical regressions and six actual-SDK lifecycle
  cases, with no remaining C1–C6 defect or correction-induced Critical regression.
  Important findings were outside that re-review; it is not release acceptance.
- Bounded offline coverage uses SDK 0.84.4 and 0.85.1 with Codex Responses,
  OpenAI Completions and Anthropic Messages. Real adapter/resource/session and
  serializer paths run with substituted local MCP/RPC transport; entire raw
  continuation requests are compared to independent references. The evidence
  records zero provider-send attempts; no paid probes were run for closeout.

**Owner acceptance pending:** first NEW task/discussion/resume usage on at least
20k gpt-6-astra lead context, excluding inherited JSONL entries; the specified
cacheRead target and billing investigation with model/API, timing, compaction
and suffix conditions; previously affected task anti-bleed and final-report
behavior; and discussion completion. Offline client equality cannot certify
provider retention, routing, expiry, billing or a cache hit. Do not close the
ticket on this Result alone. Deferred-load verification remains with the
lead-tool-profile consumer ticket.


## Resolution (2026-09-08)

Owner closed this work track at the verified client-side boundary on 2026-09-08, retaining the implementation and stopping further cache experiments. This is an owner-accepted partial-outcome closure, NOT a claim that the original live cache-hit acceptance passed or that all possible hypotheses were exhausted. It supersedes the prior Result's pending administrative disposition without rewriting its historical evidence. No merge or push is authorized.

Client-side preservation was verified against complete captured outgoing decompressed HTTP JSON, not just reconstructed requests or hashes. For lead -> first fork and first -> second fork, exact serialized non-input field spans, key order and inherited input item spans match; only new input suffixes append. This includes instructions, ordered tools, model, reasoning and prompt_cache_key. Each after-hook capture equals its decoded HTTP body bytes. Fork hooks change only prompt_cache_key to the parent's value. The captured safe request headers differ across parent/child only at session-id and x-client-request-id; they remain identical within the child. Compressed transport bytes, uncaptured headers and backend routing are not proven identical.

Live evidence on installed bundled Pi 0.85.1, openai-codex/gpt-6-astra:
- Direct ws-fork from the ongoing lead conversation: last lead input 253/cacheRead 73600; first NEW child input 74430/cacheRead 0; second child input 277/cacheRead 74240. The child reported successfully. Historical wire requests for this specific run were not captured.
- Full raw pair through programmatic invocation of the production registered handler: first child input 73168/cacheRead 0 despite equal shared body fields/prefix. This was not model-selected tool dispatch.
- Full raw triplet through actual parent-model ws-fork toolCall and the ordinary SDK tool loop: parent input 72855/cacheRead 0; first NEW child input 73444/cacheRead 0; second child input 189/cacheRead 73344 (99.74297% cached). A successful fork tool result, child report result and parent report receipt were recorded; subsequent parent model processing was quota-blocked. This lead was a private continuation copy, not the ongoing interactive lead. Therefore this triplet does not supply a cache-hit parent request as its control.

The original >=90% first-fork target remains unmet. Identical body/cache key does not establish guaranteed backend reuse. Session-header/connection routing and server cache behavior remain unisolated hypotheses; no header-identity workaround or structural impossibility conclusion was established. Resume/discussion live acceptance, broader task anti-bleed acceptance and actual billing remain unverified, not silently passed. The originally authorized six experimental sends plus three additionally authorized sends were consumed; no further experiments are planned in this closed track.

Ephemeral local evidence (raw inherited content is not committed): /tmp/ws-pi-live-acceptance-KrltZbQ3/capture-live/raw/ and triplet-live/raw/, with their sibling summary.json, run-result.json and session/observation files. The independent lead audit compared raw JSON value spans without normalizing field differences away. Direct smoke child session: 2026-09-08T06-41-51-917Z_01a07fc0-6b2c-7056-94b3-e736faf79658.jsonl.

Bounded public community research found analogous reporter observations, not maintainer-confirmed backend semantics or a verified workaround: https://github.com/openai/codex/issues/29377 (fixed-key fresh-thread reuse sometimes high, sometimes low); https://github.com/openai/codex/issues/30425 (stable-key misses, duplicate of 29377); https://github.com/openai/codex/issues/20301#issuecomment-4515349136 (stable request fingerprint/cache key with cache collapse and recovery); https://github.com/openai/codex/issues/33821 (WebSocket reused/fresh connection comparison, not evidence of an SSE-specific cause). Different-key sibling partial reuse in the original Background remains evidence against claiming categorical cross-session impossibility. No separate follow-up implementation or changes to the consumer ticket are authorized by this closure.

## Upstream Lead Review Handoff - 2026-09-08

Owner requested this appendix before the approved merge to `track/pi-agent` so a returning lead can review the implementation separately from its unsuccessful cache-hit outcome. This adds subsequent evidence and approval; it does not rewrite the frozen Phase 1 Result or imply that its unrun gates passed. The owner has now authorized the parent-track merge, but not a push or promotion to `develop`/`main`.

### Implementation to review

- Main implementation and lifecycle repairs: `7e7788b1..58c5a5d8`. Preserve effective lead prompt bytes and actual ordered callable tools; move fork role/key correction into the first new input; enforce role/key refusal without deleting tools; preserve restart metadata and original-parent identity; guard Codex body affinity while retaining distinct child transport identity. Review the final source rather than the initial implementation alone: six Critical and five distinct Important omissions were found after the first test-green version.
- Initial tests reported 1076 passing. After fixes, the relay reported 1260 passing across 173 suites. The independent Critical re-review ran 12 tests with SDK 0.84.4/0.85.1 and cleared C1–C6. I1–I5 remain implementer-reported fixed rather than independently re-reviewed. These are offline client/lifecycle tests, not a provider cache acceptance test.
- Documentation at `6ea890a4` and closure at `a2521f26` explicitly separate prefix eligibility from backend reuse. The closure is owner acceptance of a partial outcome, not proof that the original cache bug is fixed.
- Subsequent owner-only cosmetic notification: `deaf5a86`. `fork-cache-notice.ts` observes only the first new task-fork assistant `message_end` on initial spawn, computes `cacheRead / (input + cacheRead)`, and calls the parent TUI's `ui.notify`. Missing/error/invalid/zero-total usage is unknown, not a miss. No `pushToLead`, custom model message, wakeup or extra inference is introduced. The callback is ephemeral, absent for non-TUI owners, normal resumes and discussion forks; UI exceptions are isolated. Inspect `fork.ts`, `spawner.ts` and `test/fork-cache-notice.test.ts` for integration. Exactly-once persistence across process restarts was deliberately not added.
- Notification verification: the implementer reported 343 targeted and 1271 full-suite tests passing; the lead independently ran the new 11-test file successfully. After reloading, the owner requested a real `ws-fork` smoke (`72714086-4f0e-423c-8a16-84c44f58333e`), received its final report, and visually confirmed the owner-only `cache 0.0%` notice. This confirms display in that run, not improved reuse or general release acceptance.

### Attempts, in chronological order

1. Offline serializer/prefix and production lifecycle testing. Early broad comparisons and zero-network probes established candidate serialization equality but initially missed wiring defects; independent review drove the repairs above. No offline equality result is evidence of provider retention or billing.
2. Zero-paid startup verification established a working production extension/MCP startup on Pi 0.85.1. `--offline` was found not to prohibit model sends; endpoint/attempt guards, not that flag or hook exceptions, were used for paid experiments.
3. Initial paid synthetic pair (two sends): lead input 72691/cacheRead 0; first new child input 73171/cacheRead 0. The real registered `ws-fork.execute` was invoked programmatically after the lead returned `LEAD_READY`; the lead model did not choose the tool. The handler then used production `spawnAgent`/RPC with native Pi `--fork`. This was not a standalone interactive `/fork` test. Only selected parsed-body hashes/prefix counts were retained, not historical raw requests. Lead dispatch-to-child dispatch was 5.295 seconds and lead completion-to-child dispatch 1.613 seconds, so a delay exceeding five minutes did not explain this pair. Full report delivery was not accepted by this first harness.
4. Direct `ws-fork` from the actual ongoing lead (two child sends): the last parent response had input 253/cacheRead 73600; first NEW child 74430/0; next child 277/74240. This independently reproduced a boundary miss while both the parent and subsequent child calls could hit. Report delivery succeeded. No HTTP originals for these calls were saved, so session JSONL cannot prove their transmitted prefix equality.
5. Offline reconstruction of that real session found 253 shared input items and three appended child items, using persisted metadata and the current SDK. This was not historical wire evidence: parent tools were reconstructed, unpersisted hooks/transport were not replayed, auth was not verified, and affinity application was assumed in one variant. The reconstruction also exposed a harness entry-point mismatch (unbundled versus installed bundled Pi), corrected for later captures. Temporary capture work expanded unnecessarily and was stopped before resuming a bounded raw-capture task; reconstruction was not promoted into proof.
6. Raw HTTP pair (two sends): native bundled Pi 0.85.1, SSE, direct invocation of the registered handler, first child 73168/0. Captured full before/after-hook JSON and decompressed outgoing HTTP bodies plus allowlisted headers. Parent/child common body fields and input prefix matched; only appended input and the two session-related request headers differed. No cache hit resulted. Subsequent child sends were blocked by the guard.
7. Raw model-driven triplet (three additionally authorized sends): private continuation copy of the prior lead, real model `ws-fork` toolCall, ordinary SDK tool execution, production child. Parent 72855/0; first child 73444/0; second child 189/73344 (99.74297% cached). Input item counts 3 -> 6 -> 8. Full raw value-span comparison showed no non-input body differences and byte-identical inherited item spans for both transitions; after-hook bytes equal decoded outgoing body bytes. Across parent/child only the captured `session-id` and `x-client-request-id` headers differed; within the child all captured request headers matched. The parent was cold, so this is not a raw three-way comparison beginning with an already-hitting parent. Child report call/result and parent receipt were observed; parent follow-up model requests were blocked. Repeated native `--fork` ancestry is an implementation detail, not a substitute for the verified model toolCall in this run.
8. Bounded community research fetched public issue bodies/comments linked in Resolution. Analogous instability and occasional cross-thread reuse were reported, but no maintainer-confirmed cache-routing contract or reliable workaround was found. The WebSocket report cannot establish an SSE cause. Latest public guide text was not freshly retrieved; installed official SDK descriptions and Pi source were consulted instead.
9. Cosmetic live smoke after owner reload, separately requested after the nine-send investigation allowance was exhausted: owner confirmed the displayed 0.0% ratio. Do not count this later authorization as part of the earlier experimental quota or infer an audited final cumulative send count from it.

### What passed, what did not, and what was not attempted

**Passed within the measured scope:** preservation of transmitted common body fields and inherited input spans in the raw synthetic pair/triplet; actual model-selected task-fork dispatch in the triplet; same-child second-call caching; successful simple task report delivery; owner-only notification display; the stated offline tests and scoped Critical re-review.

**Did not pass:** the first-new-fork >=90% cache-read target. Every newly measured first fork request above was a miss. No implementation change was shown to improve first-call billing or eliminate the boundary miss. The experiment did not establish a causal relationship between either session header and caching. Stable body/key is not a provider reuse guarantee.

**Not attempted or not established:** an isolated change of `session-id`/`x-client-request-id`; a controlled transport-identity or backend-routing experiment; raw capture of a warm ongoing interactive parent followed by its first child; discussion TUI completion and cache measurement; dormant/restart live cache acceptance; a broad adversarial task anti-bleed live test; independent Important re-review; exact billed charges; compressed wire byte equality; backend tokenization, cache residency, expiry or guarantees. The branch-summary workaround remains idea-only (`260908-research-ws-pi-agent-driven-branch-summary-return`). Observed delegate tool-surface failures are captured separately in `260908-bug-ws-pi-delegated-tool-surface-unavailable`; no root cause was established here. Deferred fork-only tool loading remains the separate lead-profile consumer's scope.

### Review evidence locations and interpretation

- Main source range and the exact test/review artifacts are in the frozen Result above. Notification source/test commit is `deaf5a86`.
- `/tmp/ws-pi-live-acceptance-KrltZbQ3/live/`: initial hash-only pair; `driver.mjs`, `run.py`, `guard.mjs`, `preload.mjs` are temporary evolving harness files, not release implementation. Later harness content does not by itself prove earlier execution behavior.
- `/tmp/ws-fork-miss-8Q7zNkFU/comparison.json`: explicitly limited offline reconstruction.
- `/tmp/ws-pi-live-acceptance-KrltZbQ3/capture-live/`: first complete raw pair, including observations, summaries and per-role raw captures.
- `/tmp/ws-pi-live-acceptance-KrltZbQ3/triplet-live/`: model-driven raw triplet; `sessions/lead-continuation.jsonl` contains the actual parent toolCall/toolResult. Child session `2026-09-08T07-26-21-325Z_01a07fe9-2682-71d1-93f7-74a22ccf7626.jsonl` contains the two new responses. `raw/lead-1--fork-1.comparison.json` and `raw/fork-1--fork-2.comparison.json` accompany the full captures; the lead also independently checked raw JSON value spans locally.
- Raw captures are private local files, not committed artifacts, and may expire. Durable numerical results and verification limits are recorded above so loss of `/tmp` does not convert an unrepeatable claim into a passed gate. Captures contain decompressed request bodies and allowlisted response metadata, not full response bodies or all transport headers. `cf-cache-status: DYNAMIC` concerns HTTP/CDN caching and is not a model prompt-cache verdict.

Review the correctness and maintainability of the retained implementation on its own merits. Neither ticket closure nor the owner's merge approval should be read as a cache-fix claim, a release approval, or authorization for another paid investigation.
