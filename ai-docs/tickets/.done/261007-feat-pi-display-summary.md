---
title: "Pi lead TUI display summary: a cheap model replaces collapsed tool and push rows with user-language summaries; workflow.lang becomes tunable"
related:
  261003-feat-pi-execute-readonly-auto-approval: sibling; same Pi-provider-auth seam for a secondary model, distinct mechanism and config keys
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: caae1a0db4777619
sage-review-design-reviewed: caae1a0db4777619
completed: 2026-10-07
---

# Pi lead TUI display summary: a cheap model replaces collapsed tool and push rows with user-language summaries; workflow.lang becomes tunable

## Background

The Pi adapter renders lead tool rows with its own preview renderer
(`createToolPreviewRenderers` in `agents-plugin-pi/src/tool-result-render.ts`):
a near-raw preview with `...[N bytes total]` markers and width-based line
splitting. Push messages (`push-render.ts`) are similarly raw. For the human
reading the lead TUI this is noise: what they want per row is what the agent
meant to do, what came back, and enough context to follow the lead's terse
follow-up remarks.

Asking the lead model to annotate each call with a rationale was rejected: it
spends lead attention, and a much cheaper model is good enough because the
output is cosmetic and need not be exact.

The response language for summaries should come from the user's configured
language. ws already has `workflow.lang` (`agents-plugin-tool/internal/wsconfig/scope.go`,
global default scope, injected into playbooks declaring `WorkflowLang`), but it
is absent from the `config.list` tuning catalog, so `config.tune` rejects it and
`ws:lead-tune` cannot reach it.

## Decisions

### workflow.lang

- **Register the existing `workflow.lang` key in the tuning catalog** so
  `config.tune` writes it and `config.list` surfaces it; value is a free-form
  language name (e.g. `Korean`), default scope stays global, empty means unset.
  - Rejected: a new Pi-specific language key. One key keeps the lead's response
    language and the summary language aligned.
  - Existing semantics (playbook `WorkflowLang` injection, layered resolution
    including repo scope) are unchanged.
- **`ws:lead-tune` routes a response-language preference to `workflow.lang`.**
  Its `judge: tune-target` line currently sends "language" to the
  `UserPreferenceSection` prompt override; after this change a request like
  "answer me in Korean" goes to the `workflow.lang` knob, while style,
  terminology and wording preferences stay with `UserPreferenceSection`.
  - Rejected: unchanged routing, which leaves the knob reachable only when the
    user names it.
- `workflow.skeptical_posture`, also absent from the catalog, stays out of scope.

### Display summary: scope

- **Applies only to the interactive lead TUI session**, the session that talks
  to the user: `ctx.mode === "tui"` and no `WS_PI_SPAWN_ROLE` (lead role). Fork
  sessions are excluded even though push renderers are registered for lead or
  fork today. Off unless `pi.display_summary_model` is set.
- **Summarized rows:**
  - every extension tool registered through `registerWsTool` (ws MCP `ws__*`,
    `do-i-really-have-to-*`, `ws-execute`, agent spawn/send/etc., web tools,
    goal tools, fork, skill);
  - Pi built-in tools active in the lead (`edit`, `write`, and `grep`/`find`/`ls`/`powershell`
    when the user's settings activate them), via a same-name wrapper that
    spreads the native definition and wraps only `renderResult`/`renderCall`,
    delegating to the native renderer when expanded or no summary exists. The
    wrapper is registered only at `session_start` after the lead-TUI gate
    (`ctx.mode` is known only there), so it never collides with the scoped
    `edit`/`write` overrides children register in `src/write-scopes.ts`;
  - all push custom messages (`ws-agent-report`, `-settled`, `-question`,
    `-approval`, `-advisory`, `-orphaned`, `ws-push-batch` including its mailbox
    items);
  - adapter custom messages that today have no renderer (`ws-lead-compact`,
    `ws-lead-context-milestone`, `ws-thread-summary`), which gain one;
  - the `ws-lead-compaction-history` entry.
- **Excluded:** adapter-injected user-role messages (`[system message from
  ws-pi-plugin]` goal reminders, push wake lines, adapter prompts); lead
  assistant text and thinking; `!` bash executions; compaction/branch summary
  bodies; notify/status lines. These have no Pi hook that supports raw restore,
  are already short, or are written for humans.
  - Rejected: summarizing user-role adapter messages through
    `registerMarkdownTransformer` (synchronous, no expanded flag, no raw restore
    under Ctrl+O).
  - Rejected: converting those messages to custom messages (changes how the
    lead model receives them; out of scope).

### Display summary: configuration

- Two adapter keys in `agents-plugin-pi/config-manifest.json`:
  - `pi.display_summary_model` (string, `provider/model-id`, split at the first
    `/`; empty = feature off);
  - `pi.display_summary_effort` (enum
    `off|minimal|low|medium|high|xhigh|max`, default `medium` so summaries are
    not written thoughtlessly). These are Pi's provider-neutral thinking levels
    (`ModelThinkingLevel`, the `/thinking` vocabulary). A value is clamped with
    pi-ai `clampThinkingLevel` to what the model supports; `off`, or a clamp
    result of `off`, sends no `reasoning` option. The key's description says
    that with no `reasoning` most providers disable thinking and some (e.g.
    `openai-codex`) use their server default.
    - Rejected: an empty `""` member (the manifest loader rejects empty enum
      members and a non-member default, `agents-plugin-tool/internal/mcp/config_manifest.go`,
      and a failed load takes every `pi.*` key down). Rejected: a separate
      `default` member, which behaves identically to `off` on this path.
- Model and effort are set directly, not through `agents.tier`: the summarizer
  is a separate mechanism (like Jev in the sibling ticket), not a delegate tier.
- Auth reuses Pi's provider auth through `ctx.modelRegistry`; no key is held or
  named in config.
- **Call path: Pi's provider-neutral adapter.** Pi's own session calls
  `modelRuntime.streamSimple(model, context, { reasoning, ... })`
  (`pi-coding-agent/dist/core/sdk.js`); `ctx.modelRegistry` does not expose
  `streamSimple`/`completeSimple`, but the same adapter is reachable through
  public registry calls:
  - auth from `ctx.modelRegistry.getApiKeyAndHeaders(model)` (apiKey, headers,
    baseUrl, env), applied the way `ModelRuntime.prepareRequest` applies it
    (baseUrl onto the model, the rest into options);
  - `ctx.modelRegistry.getProvider(model.provider).streamSimple(model, context,
    { apiKey, headers, env, reasoning, sessionId, cacheRetention, signal })`,
    awaiting `.result()`;
  - the output tool is not forced: Pi's neutral `ToolChoice` is only
    `"auto" | "none"` (`pi-ai/dist/types.d.ts`). The system prompt requires
    exactly one call to the output tool; a response with no call, or a
    malformed one, leaves the affected rows raw.
    - Rejected: forcing through per-API strings (`required`/`any`), which
      reintroduces the per-API mapping this path exists to avoid. Rejected:
      plain JSON text output without a tool; the tool schema keeps guiding the
      shape and failure handling is the same either way.
  - Pi's header-transform hooks (attribution headers, `before_provider_headers`)
    are not applied on this path; accepted.
  - `getApiKeyAndHeaders` resolves auth through the same `runtime.getAuth` the
    lead session's own requests use (`pi-coding-agent/dist/core/model-registry.js`),
    so OAuth token refresh (e.g. `openai-codex`) behaves as for the lead.
  - Rejected: `ctx.modelRegistry.complete` with API-specific options (the
    `goal-loop.ts` precedent); it would make the adapter own a per-API mapping
    of effort (`reasoningEffort` vs Anthropic `effort`, `none` invalid on some
    APIs) that Pi's `streamSimple` adapters already own.
  - Rejected: reaching the TypeScript-private `runtime` field of the registry.
- The model key's description states that enabling it sends the lead
  conversation and tool/message content to the configured provider.
- Settings are read per use through the existing adapter config reader (no
  restart). The summary language is read from `workflow.lang` (unprefixed; the
  reader currently prefixes `pi.`); when empty, the prompt tells the model to
  use the language the user writes in.

### Display summary: request shape

- **Batching:** pending items are queued and flushed at `turn_end` and on idle
  entry (Pi's `agent_end`, for pushes arriving while the lead is idle). One request in flight at
  a time; items arriving during a request wait for the next flush.
  - Rejected: one request per tool call (parallel calls race the append-only
    log). Rejected: waiting for the lead's next assistant message to include its
    reaction (the lead's follow-up remarks are context for the human reader,
    not input the summarizer needs).
- **Append-only summarizer conversation per lead session**, for prefix cache
  hits:
  - fixed instructions in `systemPrompt`;
  - each flush appends the new lead conversation since the previous flush,
    serialized to text (`convertToLlm` + `serializeConversation`), plus the
    request for the queued item ids, as a user message; the model's answer is
    kept in the log;
  - a stable `sessionId` per lead session and `cacheRetention` not `"none"`
    (unlike the compaction fallback in `goal-loop.ts`, which uses `"none"` and a
    random id);
  - each logged answer (an assistant output-tool call) is followed by a short
    explicit tool result, so the log never relies on pi-ai's repair of orphaned
    tool calls;
  - reset on lead compaction.
- **Output schema**, returned through a single output tool the system prompt
  requires (not forced; see the call path):

  ```text
  items: [{ id, optionalContext?, toolIntention, toolResult }]
  ```

  - `id`: a short per-request label (`t1`, `t2`, ...) the adapter assigns to
    each queued item and lists, with the item, at the end of the flush request
    (the serialized conversation does not carry tool call ids). The adapter
    keeps the label-to-id map: an item's id is its tool call id, or the UUID the
    adapter puts in a custom message's `details`. An unknown label is ignored
    and an item whose label is missing stays raw.
    - Rejected: echoing the real ids (long random strings a small model
      mis-copies). Rejected: positional 1:1 mapping, where one dropped or
      reordered item shifts every later summary onto the wrong row.
    - Custom messages carry no id today (`agent_id` repeats across an
    agent's messages), so every summarized send site adds a fresh UUID to
    `details`, and each `ws-push-batch` item (including folded mailbox and
    thread-summary items) gets its own id; a batch row shows each item card's
    own summary in place of its raw body. The `ws-lead-compaction-history`
    entry uses its existing entry id; `pi.appendEntry` returns nothing, so the
    adapter reads that id after appending (e.g. `ctx.sessionManager.getLeafId()`).
  - `optionalContext`: context not recoverable from neighbouring rows (e.g.
    "this follows up the earlier auth check"), omitted when empty.
  - `toolIntention`: what the call tried to do, from its arguments.
  - `toolResult`: what came back, from its output.
  - For push/custom messages the same fields mean "who reported what" and "the
    key content".
  - Each field is in the user's language, soft-capped at about 200 words or the
    equivalent, with "one or two sentences by default; up to the cap only when
    the content warrants it". No code-side length enforcement.
- **Asynchrony:** event handlers never await the summary request (Pi awaits
  extension handlers and would block the agent loop). Requests use their own
  `AbortController`, not `ctx.signal`, so Esc does not cancel summaries.

### Display summary: display

- Collapsed: raw preview (today's rendering) until the summary arrives, then the
  summary replaces it. A summarized row is a bold header with the tool name or
  message kind, then the indented fields; the raw argument preview is hidden
  once a summary exists. `optionalContext` renders dim (`theme.fg("dim", ...)`),
  `toolIntention` and `toolResult` in the default color.
- Expanded (Pi's global Ctrl+O tool expansion, `options.expanded`): raw output,
  as today.
- Tool rows re-render through `ToolRenderContext.invalidate()`; message
  renderers have no invalidate handle, so their component reads summaries from
  an adapter-owned map at render time and the adapter calls the TUI's
  `requestRender()`. `ctx.ui` does not expose `requestRender`; call the
  existing `AgentFooterController.refresh()` (`src/agent-footer.ts`), which
  wraps the TUI handle captured in the one `setFooter`, and never call
  `setFooter` a second time (that would replace the agent footer). Summaries
  live in an adapter-owned map keyed by id (not in
  Pi's per-row `rendererState`, which resets when a row is rebuilt).
- Summaries are not persisted: a resumed session shows raw rows.

### Display summary: failure

- Any missing model, missing auth, provider error, timeout (a fixed adapter
  constant; not configurable), or unparsable or partial answer leaves the
  affected rows raw. Never surfaces as a lead turn.
- When the summarizer log would exceed the summary model's `contextWindow`
  (estimate) or the provider reports overflow, summaries stop until the next
  lead compaction resets the log.

## Constraints

- Shipped strings covered by the shipped-surface boundary include the
  `config-manifest.json` descriptions, the `workflow.lang` catalog description
  and the `lead-tune` playbook text.
- Child, worker, explore and fork processes keep today's rendering.
- New TUI components resolve pi-tui through the host (`loadHostPiTui` in
  `agents-plugin-pi/src/pi-tui.ts`), never by a static runtime import: the
  host nests its own pi-tui copy, so `instanceof` against a second copy fails.
- Automated tests inject the completion function; no test makes a live model
  call.
- No ws-mcp outbound network: the summarizer call lives entirely in the Pi
  adapter.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261007-bug-pi-adapter-messages-read-as-user-text (2026-10-07, commit 030f07d5): "adapter user-role and custom-message injections open with ADAPTER_MESSAGE_LABEL (src/adapter-label.ts); new injection sites must label at the send so the dialog filter recognizes them." — bearing: constrains
- 261007-bug-pi-adapter-messages-read-as-user-text (2026-10-07, commit 030f07d5): "push-render's single-push head parser skips the label line, otherwise the label would render as the push head." — bearing: constrains
- 261002-feat-pi-lead-ws-owned-compaction (2026-10-02, commit f065de0d): "The fallback calls ctx.modelRegistry.complete with Pi's convertToLlm/serializeConversation (generateSummary would impose Pi's own prompt); maxTokens follows Pi's 0.8 x reserveTokens capped by the model limit" — bearing: supports
- 261001-feat-config-repo-scope-and-tune-weight-guidance (2026-10-01, commit c561d332): "\"delegate less\" without a value now goes to the explanation path; that is Decision 5's no-inference rule, while proactive-propose still names a concrete change for confirmation." — bearing: constrains
- 260906-feat-ws-pi-tool-result-yaml-tui-rendering (2026-09-09, Result): "New src/tool-row-render.ts: five per-tool call-summary builders (explore, ws-execute, ws-agent-spawn, ws-agent-send, ws-fork), formatResolvedLine, a shared head-truncator, and createDispatchToolPreview — all pure, no pi-tui static import" — bearing: constrains
- 260625-feat-lead-tune-schema-backed-knob-catalog (2026-06-25, commit f171bd22): "Kept wsflow behavior mode-aware by advertising config.tuning but omitting full-ws-only knobs from the no-agent catalog." — bearing: constrains
- 260625-feat-lead-tune-schema-backed-knob-catalog (2026-06-25, commit f171bd22): "User approved the schema-backed knob registry design: new config entries register a semantic knob while enum/property metadata stays sourced from the writer tool schema." — bearing: supports
- 260624-feat-workflow-lead-language-config (2026-06-24, commit eed36d84): "Phase 1 only: config key registration + render injection + template update" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_registry.go, agents-plugin-tool/internal/wsconfig/scope.go, agents-plugin/rsrc/lead-tune/lead-tune.md, agents-plugin-wsflow/rsrc/lead-tune/lead-tune.md, agents-plugin-pi/rsrc/lead-tune/lead-tune.md, agents-plugin-pi/config-manifest.json, agents-plugin-pi/src/tool-result-render.ts, agents-plugin-pi/src/push-render.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/adapter-config.ts |
| scope.surface | cross-module | ws-mcp config.list/config.tune catalog, lead-tune playbook and wsflow mirror, Pi adapter tool and message renderers |
| scope.new_public_symbol | yes | config keys pi.display_summary_model and pi.display_summary_effort; workflow.lang catalog knob |
| scope.new_type_contract | yes | summarizer output tool schema items id optionalContext toolIntention toolResult |
| scope.test_surface | new-files | existing agents-plugin-pi/test/tool-result-render.test.ts, push-render.test.ts, adapter-config.test.ts and agents-plugin-tool/internal/mcp config tests; summarizer module needs its own test file |
| complexity.reuse_points | confirmed | registerWsTool tool-result-render.ts#L658, ctx.modelRegistry.getProvider/getApiKeyAndHeaders (Pi provider-neutral streamSimple), serialization precedent goal-loop.ts#L1575, createWsConfigReader adapter-config.ts#L71, loadHostPiTui pi-tui.ts#L112, configRegistry config_registry.go#L122 |
| complexity.side_effect_risk | moderate | wraps every lead tool and push row renderer and adds outbound provider calls from the lead process |
| risk.correctness | moderate | async batching with one request in flight, append-only log, id-keyed summary map and re-render invalidation must never block or inject lead turns |
| risk.fit | moderate | same-name wrappers over Pi built-in tools and host-resolved pi-tui components must fit Pi renderer contracts |
| risk.test | moderate | injected completion fake must cover batching, prefix stability, compaction reset and failure fallbacks |
| risk.security_or_contract | moderate | sends lead conversation content to a configured provider and adds a shipped catalog knob plus lead-tune routing change |

## Phases

### Phase 1: Make workflow.lang tunable

- Add `workflow.lang` to the `config.list` tuning catalog with a writer through
  `config.tune` (free-form string, default scope global, reset supported), and a
  description saying it sets the user's conversation language for lead
  responses and for adapter-produced user-facing summaries.
- `config.tune`'s default scalar branch lowercases values; exempt free-form
  string knobs so `Korean` is stored as written. Register both the
  `configRegistry` row and the `buildTuningCatalog` knob, visible in the
  no-agent (wsflow) catalog too, since the wsflow `lead-tune` mirror routes to
  it.
- Update `lead-tune`'s `judge: tune-target` routing per the decision above, in
  `agents-plugin/rsrc/lead-tune/lead-tune.md` and its byte-identical mirrors
  `agents-plugin-wsflow/rsrc/lead-tune/lead-tune.md` and
  `agents-plugin-pi/rsrc/lead-tune/lead-tune.md`, regenerating the rsrc
  `manifest.json` hashes. Existing tests pin the substring
  ``prompt override (`UserPreferenceSection`)``; the narrowed line keeps it.
- Verification: `config.tune(key: "workflow.lang", value: "Korean")` succeeds
  and `config.get` / `config.list` report it; reset returns it to unset;
  existing `WorkflowLang` injection tests and the mirror/bundle drift tests
  pass; a test pins that the `lead-tune` routing line sends a response-language
  preference to `workflow.lang`.

### Result (c44b61644) - 2026-10-07

- `workflow.lang` registered in `configRegistry` and the `config.list` tuning
  catalog (`agents-plugin-tool/internal/mcp/config_registry.go`, `server.go`):
  free-form value, scope selector with global default, reset through
  `config.tune`, visible in the no-agent (wsflow) catalog.
- `config.tune` keeps the case of a free-form knob (value field with no Enum);
  a blank value is rejected, reset is the one unset path.
- `lead-tune` `judge: tune-target` sends a response-language preference to
  `workflow.lang`; style, terminology and wording stay on the
  ``prompt override (`UserPreferenceSection`)`` line. Mirrors in wsflow and Pi
  rsrc are byte-identical; the three `manifest.json` hashes regenerated.
- Verification: `go test ./internal/mcp/... ./internal/wsconfig/...` ok
  (includes `config_workflow_lang_test.go`: tune Korean, get/list, reset,
  blank rejection, no-agent catalog, routing line pin);
  `go test ./internal/wsrsrc/...` ok; wsflow package unittests (14) and
  shipped-surface/dispatch-contract unittests (24) ok.

### Phase 2: Pi display summary

Implements every `Display summary` decision above in `agents-plugin-pi`.

- Verification: automated tests with an injected fake completion function
  cover: feature off when the model key is empty; collapsed rows show raw before
  and summary after a response, expanded rows always raw; dim vs default field
  styling; batch flush at `turn_end` and idle with one request in flight;
  append-only log (each request's message prefix equals the previous request's
  messages) with stable `sessionId` and non-`none` retention; reset on
  compaction; failure and overflow fall back to raw and never inject a lead
  turn; built-in wrapper delegates to the native renderer when expanded;
  inactive outside the lead TUI role; the summary language read from
  unprefixed `workflow.lang` and the empty-value prompt fallback; message rows
  re-rendering through `AgentFooterController.refresh()`; the effort mapping
  (`off` or a clamp to `off` sends no `reasoning`, other levels clamped, default
  `medium`); a response with no output-tool call leaves its rows raw; custom
  message ids are unique per send and each `ws-push-batch` item card gets its
  own summary; no built-in wrapper is registered in child or fork sessions;
  label mapping (labels resolve to the right rows, an unknown label is ignored,
  a missing label leaves its row raw); every summarized row kind is exercised
  at least once: a `registerWsTool` extension tool, a wrapped built-in, a push
  message, a `ws-push-batch` item, the newly rendered `ws-lead-compact`,
  `ws-lead-context-milestone` and `ws-thread-summary` messages, and the
  `ws-lead-compaction-history` entry.
- The verification gate is the automated tests only. Summary quality is judged
  by the user dogfooding after merge; no live-model TUI smoke is required.
  Rejected: a worker-run live smoke in an isolated tmux TUI, which adds provider
  auth and a TUI harness to the run for little gating value on cosmetic output.

### Result (0ba80243d) - 2026-10-07

- Summarizer core `agents-plugin-pi/src/display-summary.ts`: id-keyed store,
  batched append-only log flushed at `turn_end`/`agent_end` with one request in
  flight, `t1..` label mapping, provider-neutral `streamSimple` call with
  `getApiKeyAndHeaders` auth, effort clamp (`off`/clamp-to-`off` sends no
  `reasoning`, default `medium`), stable `sessionId` and `cacheRetention:
  "short"`, explicit tool results after each logged answer, reset on
  compaction, estimate and provider overflow stops, 90 s fixed timeout on its
  own `AbortController`.
- Session wiring `src/display-summary-session.ts`: lead-TUI gate
  (`mode === "tui"`, no spawn role) at `session_start`, where the built-in
  wrappers (edit/write/grep/find/ls/powershell when active) and the
  `ws-lead-compact`/`ws-lead-context-milestone`/`ws-thread-summary` renderers
  register once; handlers never await a request.
- Rendering `src/display-summary-render.ts` plus `tool-result-render.ts`
  (`registerWsTool`), `push-render.ts` (each `ws-push-batch` item card on its
  own id) and `compaction-history.ts` (entry id from `getLeafId()` after
  `appendEntry`). Message rows repaint through
  `AgentFooterController.refresh()` (`bindDisplaySummaryRender` in
  `index.ts`). Row ids: `src/summary-id.ts` stamps a fresh `ws_summary_id` in
  `details` at every summarized send site.
- Config keys `pi.display_summary_model` / `pi.display_summary_effort` in
  `config-manifest.json`; `workflow.lang` read unprefixed through the new
  full-key `createWsConfigKeyReader`.
- Decisions: `UseNativeResultFallback` moved to a leaf (`native-fallback.ts`)
  to avoid an import cycle; a failed request carries its conversation text to
  the next request so the log never skips context; language goes in each
  request rather than the system prompt so a retune keeps the cached prefix;
  a provider overflow from a request that started before a compaction does
  not re-stop the fresh log (review fix).
- Verification: `npm test` in `agents-plugin-pi` 2134 pass / 0 fail / 3
  skipped; the TestShippedAdapterManifestsLoad Go test loads the new keys.
  Lite review: no Critical or Important; one Minor fixed (0ba80243d), one
  Minor left (rows queued while a request is in flight wait for the next
  `turn_end`/`agent_end`; per the ticket's flush wording, and re-flushing on
  completion could summarize a tool row before its result is observed).
