---
title: "Configure model-specific system-prompt supplements in Pi"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 425fc61e81559555
sage-review-completeness-reviewed: 425fc61e81559555
---

# Configure model-specific system-prompt supplements in Pi

## Background

Some models need stable, model-specific operating guidance that should apply whenever the Pi adapter selects them. Pi can append such guidance before each agent run, but ws currently has no model-ID-keyed prompt setting. Add a Pi-owned global configuration and command surface rather than extending or overloading ws `config.tune`.

## Decisions

- Store this feature's configuration globally under the Pi extension's ownership. Do not add a ws-MCP knob, change `config.tune`, or reuse playbook-oriented `prompt.*` keys.
- Address models by a case-sensitive Pi model ID, with an optional explicit provider qualifier. A provider-qualified rule takes precedence over a bare-ID fallback.
- Keep two independent prompt channels per selector:
  - the general channel applies to every Pi agent session;
  - the additive `lead-only` channel applies to the root lead and `ws-fork`, but not `worker` or `explore` processes, even when a worker holds lead authority.
- Resolve the general and lead-only channels independently. For each channel, select an exact provider-qualified rule when present and otherwise the bare-ID fallback. Append the resolved general text first and the resolved lead-only text second.
- Provide the following command grammar. Model IDs are kept separate from provider qualification because a Pi model ID may itself contain `/`.

  ```text
  /ws-model-prompt set <id>
  /ws-model-prompt set --provider <provider> <id>
  /ws-model-prompt set --lead-only <id>
  /ws-model-prompt set --lead-only --provider <provider> <id>
  /ws-model-prompt show ...
  /ws-model-prompt list
  /ws-model-prompt clear ...
  ```

  `show` and `clear` accept the same selector and channel flags as `set`.
- `set` opens Pi's multiline extension editor, seeded with the existing value. Cancelling leaves the configuration unchanged. An empty submission does not silently delete a rule; deletion stays explicit through `clear`.
- Load persisted configuration at Pi session startup and update the active process immediately after a successful command write. Outside the fork-inheritance exception below, select the live model in `before_agent_start` and append the corresponding prompt block on every agent run; `model_select` may invalidate any derived cache but is not the injection hook.
- Allow selectors that are absent from the current Pi model catalog, but show a non-blocking warning when such a rule is written.
- External edits to the configuration become visible when a root or independently launched Pi session starts. Do not add a file watcher or cross-process live reload.
- Preserve a fork's inherited prompt bytes on its first call even when global configuration changed after the parent lead started or the fork starts on a different model. Fork startup must not replace the inherited model-prompt block: cache coherency takes precedence over giving a cross-model fork its selected model's supplement at startup. Keep that inherited block while the fork remains on its startup model; only a later explicit model change or successful `set`/`clear` may rebuild it from current configuration.
- Render the injected material as one identifiable, idempotent block: remove or replace a prior block when rebuilding it. This prevents duplicate general or lead-only text while retaining the inherited fork block when prompt-cache preservation requires it.

## Constraints

- Preserve existing Pi lead/fork/worker/explore role semantics; do not infer roles from prompt text, model tier, or ws session authority.
- Do not rebuild Pi's base system prompt or persist the supplement as a conversation message.
- The configuration write path must not lose unrelated selectors or channels when multiple Pi sessions use the same global store.
- Keep the feature Pi-specific; no downstream project configuration contract is introduced.

## Prior Decisions

- 260907-bug-ws-pi-fork-first-call-prompt-cache-miss (2026-09-08, Decisions): "The fork's `instructions` and `tools` must be byte-identical to the lead's at spawn time; the only new bytes are the appended first user message." — bearing: contradiction-candidate
- 260904-feat-ws-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "`before_agent_start` appends the ws block when the role is absent (lead) or `fork` (lead-caliber peer, needs the same manual and guide). It appends nothing for `worker` / `explore`." — bearing: supports
- 260906-bug-ws-pi-lead-cannot-see-or-load-skills (2026-09-06, commit 5b8d956c): "lead-bootstrap.ts's buildWsBlock grew a third skillsBlock parameter (order: manual snapshot, guide text, skills block)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Pi extension command and persistence in agents-plugin-pi/src/index.ts, existing prompt hook in agents-plugin-pi/src/lead-bootstrap.ts, role gate in agents-plugin-pi/src/process-role.ts, and tests under agents-plugin-pi/test/ |
| scope.surface | public-interface | new user-facing /ws-model-prompt command and globally persisted selector/channel settings |
| scope.new_public_symbol | yes | /ws-model-prompt command is a new Pi user-facing symbol |
| scope.new_type_contract | yes | new persisted provider-qualified/bare-ID selector and general/lead-only channel schema, exact storage representation not yet specified |
| scope.test_surface | existing | agents-plugin-pi/test/lead-bootstrap.test.ts and agents-plugin-pi/test/fork-lifecycle.integration.test.ts cover current injection and inherited fork prompts; new test files may also be needed |
| complexity.reuse_points | confirmed | existing registerLeadBootstrap before_agent_start hook in agents-plugin-pi/src/lead-bootstrap.ts and readSpawnRole in agents-plugin-pi/src/process-role.ts |
| complexity.side_effect_risk | high | global concurrent writes and replacement of inherited fork prompt blocks affect unrelated model sessions |
| risk.correctness | high | independent channel resolution, persistence read-modify-write, and inherited-block idempotence interact |
| risk.fit | high | agents-plugin-pi/src/lead-bootstrap.ts#L306-L320 returns the inherited fork prompt directly; new injection must compose with that path |
| risk.test | high | command editor, process lifecycle, reload/model switching, concurrent writes, and fork inheritance need separate gates |
| risk.security_or_contract | moderate | lead-only instructions must not leak to worker/explore even if worker holds lead authority |

## Phases

### Phase 1: Add persistent model-prompt configuration and runtime injection

Implement the global persistence and `/ws-model-prompt` command surface, then compose the selected general and lead-only supplements into Pi's system prompt through the existing extension lifecycle.

Verification must cover selector parsing with slash-containing IDs, provider-qualified precedence, independent general and lead-only fallback, role applicability, multiline editor cancellation and empty-input behavior, persistence without unrelated-entry loss, immediate command visibility, startup reload, model changes between runs, cross-model fork inheritance without startup replacement, and idempotent replacement after an eligible fork rebuild.

## Sage Review Round 1 (2026-09-26)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Cross-model fork prompt contract is contradictory | important | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
