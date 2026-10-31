---
title: Claude mod delivers the workflow manual once per conversation epoch at lead skill expansion
related:
  261018-feat-claude-mod-ws-manual-injection: superseded mechanism (system-prompt manual section)
  261030-feat-claude-mod-ws-preload-skill-expansion: extended (preload block)
  261031-feat-ws-preload-playbook-unit-dedupe: extended (conversation-scan dedupe)
  261021-feat-claude-mod-manual-static-body-dedupe: superseded (its state-only cut and snapshot are removed here)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 799b55c538368ba4
sage-review-completeness-reviewed: 799b55c538368ba4
---

# Claude mod delivers the workflow manual once per conversation epoch at lead skill expansion

## Background

The Claude mod (`agents-plugin/hooks/claude-mod`, mirrored in
`agents-plugin-wsflow/hooks/claude-mod`) appends the workflow manual to the
system prompt at the first `prompt.compose` after a reset, then cuts the manual
out of later `workflow_manual` results (`stateOnly`, span guard,
`workflow_state` fallback) and tracks snapshot / reused / landing / generation
state to decide when the section is current. The owner judged these rules
more complex than needed, and the system-prompt append is unreliable at
session start in practice.

Five lead skill shims (`lead-delegate`, `lead-discuss`, `lead-run`,
`lead-ticket`, `lead-use-mailbox`) already open with a `workflow_manual` call
(inside the `<ws-preload>` block or as a bootstrap line; the other six, among
them `lead-review`, `lead-ship`, and `lead-tune`, carry only a `playbook.read`
fallback: `agents-plugin/skills/*/SKILL.md`), `lead-revive` forces one after
compaction, and the mod's key store rebinds the mailbox on resume without the
manual. So the system-prompt section buys little, and a lead skill expansion,
which the preload already intercepts, is a better delivery point.

Today a repeated `workflow_manual` result is deduped only when byte-identical;
any agenda, todo, or note change re-inlines the whole state on the next
skill. The agent that authored those changes already knows them, and
`workflow_state`, `agenda_list`, `todo_list`, and `note_query` read them on
demand.

## Decisions

- **Delivery moves from the system prompt to lead skill expansion ((60)).**
  The mod no longer appends a manual section through `prompt.compose`; a lead
  skill expansion is the delivery point. Supersedes the "manual prefix
  injection ... state-only `workflow_manual`" scope of
  `261018-research-claude-mod-ws-manual-injection` and its feature: the
  system-prompt append is unreliable (a managed-settings security mod can
  continue `prompt.compose` past the user tier, `261021`), and its currency
  tracking is most of the mod's manual complexity.
- **One appended row per epoch carries manual, key, and state ((61), (70)).**
  When a lead skill expands and no epoch copy is visible (next decision), the
  mod calls `workflow_manual` with the conversation's key and appends its
  whole response, preceded by the key line and the mailbox line when the push
  runs here, as one marked model-only `user` row through `$.session.append`
  (the mailbox steer path). The row is not re-appended when the state changes
  later in the epoch: the lead authored its agenda, todos, and notes and reads
  them on demand with `workflow_state`, `agenda_list`, `todo_list`, and
  `note_query`. The skill text keeps carrying the playbook body (or its
  pointer) exactly as today and never the manual, the key, or state.
  - Why a row and not the skill text: Claude Code re-attaches invoked skills'
    expanded text after compaction (observed 2026-10-31: the post-compaction
    "skills invoked earlier" reminder carried the preload-inlined
    `lead-delegate` playbook body, some entries truncated), so anything inlined
    there crosses compaction; an appended row is an ordinary message that
    compaction summarizes away. `skill.prompt` also fires for a subagent
    preload and cannot tell it from a lead load, while `$.session.append`
    reaches the main conversation only.
  - Why the whole response: the part before `## Session Key` is not a static
    manual; ws-mcp injects the key line and per-call banners (staleness,
    occupied root, mailbox address, notes cleanup, manuals list, checkpoint and
    one-shot review-track nudges) into it (`workflow_manual.go`
    `injectSessionKeyLine`, `injectBootstrapStalenessWarning`), and the row is
    their only Claude delivery path.
  - Rejected: a static manual inlined in the skill text plus a separate state
    row (the settled-then-revised (61) wording; the Sage design review found
    the "static manual" carries the key and per-call banners, and a subagent
    preload could receive it or a pointer to a copy it cannot see).
    Rejected: manual + key + state inlined in the skill text (crosses
    compaction stale). Rejected: asking the model for a `workflow_state` call
    (a fresh lead holds no key to pass).
  - (70) is the lead's revision under the owner's 2026-10-31 delegation to
    settle remaining calls autonomously; it keeps the owner's confirmed
    outcome (manual and state once per epoch, no re-insert on a delta, no stale
    state across compaction).
- **Epoch ((62), (63)).** An epoch copy is visible when the conversation
  (`$.session.messages({ as: 'api' })`) holds the mod's marked row or a
  non-error `workflow_manual` tool result that delivers the manual, whatever
  key: a result counts when it carries a `## Session Key` heading line, which
  ws-mcp's fail-loud reply for an unresolvable key (`workflow_manual.go`
  "no restorable state" notice, a non-error result) lacks. Compaction,
  `/clear`, and a rewind remove both, so no reset event is wired. A failed
  conversation read appends (a duplicate, never a missing manual); so do two
  expansions whose scans run before either row lands, accepted without an
  in-flight guard. The mod no longer rewrites
  `workflow_manual` or `workflow_state` results (no state-only cut, span
  guard, `workflow_state` fallback, or repeat pointer); the sentinel adoption
  notice is the one addition it keeps. Rejected: `repeatOf`'s playbook rule
  (exactly one copy, no pointer, whole-result equality, same key), which
  re-inlines after every pointer, forever once two copies exist, and once
  more after adoption changes the key. Rejected: a per-session flag reset on
  compaction.
- **Typed slash commands ((63), (70)).** The model-invoked Skill path appends at the
  tool boundary the way the mailbox steer does. For a typed `/name`, Phase 1
  first probes whether a row appended from `skill.prompt` reaches the same
  request as the expansion (`submitOwn` in `captions.tsx` records that a row
  appended after a submit is missed for that reply). If it does not, the row
  is appended anyway and accepted to reach the model at the next request
  boundary (its first tool result); no key-bearing line is put in skill text.
  In that late case a fresh lead may first bootstrap through the sentinel, so
  the late row can name the pre-adoption key next to the adoption notice
  naming the new one; this is part of the accepted degradation. The owner dogfoods this path after implementation.
- **Key handling stays ((64)).** Key mint, relogin, the
  `ws-lead-key:<sessionId>` store, sentinel (`obsidian-latch`) adoption, and
  mailbox binding (`keyChanged` -> `bindAuto` + band) are unchanged. Session
  setup keeps minting or relogging the key through `ferrule` but no longer
  reads `workflow_manual` or `lead-workflow-manual` at start, so one-shot
  banners are consumed by the first row that delivers them. A session that
  never expands a lead skill gets no key in context until it bootstraps
  through the sentinel (adoption then switches the push); this degradation is
  accepted.
- **Every expanded lead skill, shims untouched ((65)).** The row is appended
  for every lead skill whose preload block the mod expands; the `SKILL.md`
  shims stay unchanged. A block the mod passes through (`lead-use-mailbox`,
  no key held, a failed read) appends no row, because its fallback prose makes
  its own `workflow_manual` call where it has one. Rejected: adding a
  `workflow_manual` line to the shims whose fallback lacks one (a
  shipped-text change for a Claude-only concern).
- **The config key keeps its name ((68)).** `claude.manual_injection_enabled`
  keeps its name, defaults (ws `true`, wsflow `false`), and on/off meaning: it
  switches the whole lead-key feature set (key handling, preload, the row,
  mailbox auto-bind, band key rows). Only its descriptions are rewritten.
  Rejected: renaming it with a fallback read of the old key (a config-contract
  change with migration); removing it and always enabling (deletes wsflow's
  conservative default).
- **Claude mod only ((66)).** The Pi adapter keeps its per-turn system-prompt
  manual (`lead-bootstrap.ts` `buildWsBlock`) and its existing state dedupe
  (`261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe`). Rejected:
  aligning Pi in this ticket (Pi rebuilds its prompt every turn, so it lacks
  the Claude instability).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)
- Mirror every changed file under `agents-plugin/hooks/claude-mod/`
  byte-identically to `agents-plugin-wsflow/hooks/claude-mod/`, and update both
  packages' `config-manifest.json` and `hooks/hooks.json` descriptions.
- Playbook preload and its `repeatOf` dedupe are unchanged in behavior; the
  consecutive-only escape hatch ((56)) and manual slimming ((58)) are out of
  scope.
- The mailbox push, watchdog, compaction, captions, and band features are
  unchanged apart from reading the key the same way.

## Prior Decisions

- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-30, Decisions): "Subagent preloads see only the playbook body. `SkillPromptInput` carries no `agentId`, and the type doc says `skill.prompt` also fires for a skill 'preloaded into a subagent'" — bearing: constrains
- 261030-feat-claude-mod-plugin-row-captions 6a2e905f (2026-10-30, commit): "The hidden row goes before the submit: probe 2 showed a row appended after the submit is missed for that whole reply." — bearing: constrains
- 261031-feat-ws-preload-playbook-unit-dedupe (2026-10-31, Decisions): "The skill-side pointer stays self-sufficient. When the preload path dedupes, the block is replaced by a pointer line followed by the block's original fallback prose" — bearing: constrains
- 261002-feat-agenda-size-nudge-and-pi-manual-state-dedupe (2026-10-02, D2): "the system prompt is never compacted and its state tail went stale after the first mutation, while live workflow_manual calls returned the same state again" — bearing: supports
- 261018-research-claude-mod-ws-manual-injection (2026-10-18, Confirmed Decisions): "Scope: manual prefix injection gated on `ai-docs/WORKFLOW.md`, state-only `workflow_manual`, playbook re-read dedupe until compaction; session-key omission dropped (user, 2026-10-18)." — bearing: contradiction-candidate
- 261021-feat-claude-mod-manual-static-body-dedupe (todo): extends the state-only cut this ticket removes ("extends its state-only cut and repeat-read dedupe") — bearing: contradiction-candidate

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/manual.ts, preload.ts, register.ts and tests, mirrored in agents-plugin-wsflow/hooks/claude-mod/; both config-manifest.json and hooks/hooks.json |
| scope.surface | cross-module | claude-mod manual, preload, and register modules plus the shipped config description text; claude.manual_injection_enabled keeps its name |
| scope.new_public_symbol | unknown | the epoch-row marker and any new manual/preload exports are the worker's design; the removed exports (SECTION_ID, stateOnlyText) are not new |
| scope.new_type_contract | unknown | manual.ts edge and preload.ts expandPreload dependency types change but no signature is fixed in the ticket |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/manual-engine.test.ts, manual.test.ts, preload.test.ts, engine.test.ts |
| complexity.reuse_points | confirmed | $.session.append mailbox steer path, $.session.messages({ as: 'api' }) reads in register.ts, expandPreload, key store and bindAuto |
| complexity.side_effect_risk | high | removes the prompt.compose delivery and rewrites workflow_manual handling for every lead session on Claude |
| risk.correctness | high | typed-slash-command row ordering is unverified (captions.tsx#L385-L396 notes a row appended after a submit is missed) and the epoch scan must survive compaction |
| risk.fit | moderate | follows the existing skill.prompt preload and mailbox steer patterns; deletes much existing manual state |
| risk.test | moderate | existing engine tests cover the removed paths and need rewriting; the typed-command ordering needs a probe |
| risk.security_or_contract | moderate | shipped config description and hooks.json text change for downstream; the subagent preload must never receive the key or state |

## Phases

### Phase 1: One appended row per epoch, system-prompt section removed

1. Probe first: check whether a `user` row appended from the `skill.prompt`
   hook of a typed `/name` reaches the same model request as the expansion
   (a live session, or the engine test harness if it models ordering). Record
   the outcome in the commit and in a comment at the append site.
2. Remove the system-prompt delivery: the `prompt.compose` hook, the section
   text and its wait, the setup reads of `workflow_manual` and
   `lead-workflow-manual`, and the snapshot / awaiting / composedEarly /
   landingRefreshes / generation / reused / span state that only served it;
   remove the `workflow_manual` state-only cut, its `workflow_state` fallback,
   and the repeat-pointer dedupe of `workflow_manual` results. Keep the
   playbook dedupe and the sentinel adoption notice.
3. Row: in the preload path, after a lead skill's block is expanded, if no
   epoch copy is visible, call `workflow_manual` with the conversation's key
   and append the marked row (key line, mailbox line when the push runs here,
   the whole response). The marker is the implementer's choice, matched
   exactly by the epoch scan.
4. Rewrite every string and comment that describes the removed section:
   the `manual.ts`, `preload.ts`, and `register.ts` headers, `adoptionNotice`
   ("the system prompt names it after the next /compact"),
   `STATE_POINTER_LINE`, `STATE_ONLY_LINE`, and both packages'
   `config-manifest.json` and `hooks/hooks.json` descriptions.

Verification: `claude plugin test` and `claude plugin validate <path>` in both
`agents-plugin` and `agents-plugin-wsflow`; `diff -r` of the two
`hooks/claude-mod` trees. Tests cover: the first expansion appends one row
carrying the key line and the whole `workflow_manual` response, and the skill
text carries the playbook only; a second expansion after a state change
appends nothing; after the conversation loses the row (simulated compaction)
the next expansion appends again; a visible `workflow_manual` tool result
carrying `## Session Key` suppresses the row, while a visible fail-loud
"no restorable state" result does not; a failed conversation read appends; a passed-through
block appends nothing; no preload replacement text ever contains the key,
the mailbox line, or session state; `workflow_manual` results reach the
model unrewritten apart from the adoption notice; sentinel adoption still
rebinds the key and the mailbox.
