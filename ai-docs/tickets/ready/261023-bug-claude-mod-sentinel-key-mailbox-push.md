---
title: Claude mod mailbox push stays on a key the lead never saw when the manual section does not reach the model
related:
  261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind: reverses its "sentinel is never adopted" decision
  261018-feat-claude-mod-ws-manual-injection: origin of the minted key and the system prompt section
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b9321f2ac8c89a85
sage-review-completeness-reviewed: b9321f2ac8c89a85
---

# Claude mod mailbox push stays on a key the lead never saw when the manual section does not reach the model

## Background

In a ws project the Claude mod mints a lead session key at `session.start`,
binds the mailbox push to it through `keyChanged`, and carries the key in the
`ws:workflow-manual` system prompt section (`manual.ts` `setup`,
`register.ts` `prompt.compose`). The bind happens whether or not the section
reaches the model.

On an organization Claude account the security policy sometimes keeps the
section from reaching the model. The lead then sees no key, calls
`workflow_manual(session_key: "obsidian-latch", root)`, and ws-mcp mints a
second key and returns it under `## Session Key`
(`agents-plugin-tool/internal/mcp/workflow_manual.go`, search
`freshBootstrapKey`). `revivedKeyToAdopt` skips the sentinel, so the push
stays bound to the first key, which the lead never saw; only a later
`workflow_manual` call with the second key adopts it. In a ws project the
mod registers no mailbox watch tool, so the lead has no way to bind the push
itself.

The lead also often starts a session with `ws:lead-use-mailbox`, whose skill
shim loads only its playbook and never bootstraps a key.

## Decisions

- **Adopt the sentinel's minted key.** A main-thread `workflow_manual` call
  with `obsidian-latch` whose result carries a `## Session Key` line adopts
  that key as the conversation's key, and the push rebinds to it. This
  reverses `261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind`'s
  "The sentinel is never adopted" on the user's direction (2026-10-23).
  A sentinel result without a `## Session Key` line adopts nothing.
- **Through the existing adoption flow.** The minted key goes through the
  same re-login, key switch, key entry rewrite, and cache rebuild a revive's
  key does. Rejected: rewriting sentinel calls to the conversation key as pi
  does (a second code path, and it has no key to rewrite to when the mod's
  own setup failed).
- **`ferrule` is not adopted.** It starts an independent track by contract,
  so a main-thread `ferrule` key leaves the push where it is. Rejected:
  adopting a `ferrule` key while the model has not yet used the current key
  (an extra heuristic for a case the sentinel already covers).
- **The adoption notice names the push.** When the push runs here, every
  adoption notice, a revive's included, gains a sentence that the mailbox
  push is bound to the new key and no mailbox wait is needed. Without it, a
  lead whose system prompt lacks the mailbox line falls through
  `arm-the-wait.claude.md` to a redundant background wait. Rejected: the
  sentence on sentinel adoptions only (two notice shapes for one fact).
- **`lead-use-mailbox` bootstraps like `lead-discuss`.** The ws, wsflow, and
  pi `lead-use-mailbox` skill shims take each package's `lead-discuss`
  two-call parallel form: `playbook.read` together with
  `workflow_manual(session_key: <your key or "obsidian-latch" if fresh>,
  root: <absolute worktree path if fresh>)`, then the returned procedure. The
  wsflow shim keeps its `wsflow/` prefix and drops its blocker sentence, as
  the other wsflow parallel-init shims carry none; on pi the call is harmless,
  since pi rewrites the sentinel to the conversation key.
- **The CLI wait is a one-line fallback.** `arm-the-wait.claude.md` step 3
  shrinks to one line for a Claude Code without the mod (the only Claude host
  where neither the push nor the watch tool exists). Steps 1, 2 and 4 stay.
  The line applies only when neither the system prompt nor a key-change
  notice says the push is bound: the notice arrives in a tool result, so the
  pinned step 1 never fires on it. Rejected: amending step 1 to accept the
  notice (it would change a step this ticket keeps).
  Rejected: deleting step 3 (a Claude Code without the mod would have no
  wake path).

## Constraints

- Main thread only, in a ws project, through the existing `rewriteRead`
  filter: a subagent's sentinel call adopts nothing.
- Python-less environments are not a target; the push probe's `python3`
  requirement stays as it is, and the fallback line does not mention it.
- Every copy stays identical: `agents-plugin/hooks/claude-mod/` with its
  wsflow copy, and `arm-the-wait.claude.md` under `agents-plugin/rsrc/`,
  `agents-plugin-wsflow/rsrc/`, and `agents-plugin-pi/rsrc/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind (2026-10-18, Decisions): "The sentinel is never adopted, as on pi: a workflow_manual call with obsidian-latch mints a separate key in ws-mcp and the mod does not follow it." (reversal acknowledged in this ticket's Decisions) — bearing: constrains
- 261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind (2026-10-18, Decisions): "Adopt a revived key, as pi does. A main-thread workflow_manual call (no agentId) whose session_key is an explicit key, not the fresh-bootstrap sentinel ... triggers adoption after the call returns" — bearing: supports
- 261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind (2026-10-18, Decisions): "Mailbox push binds automatically where the mod owns the key. Under the manual feature's gate, every key the conversation gets ... or adopted - is bound to the mailbox push through the existing bind path" — bearing: supports
- 88187aff (2026-10-18, commit): "Adoption bumps the manual generation; setup re-checks it before its store write and before the listener, so an in-flight mint or re-login never overwrites the adopted key or rebinds the push to the old one." — bearing: constrains
- 1ffd64e1 (2026-10-18, commit): "Minting uses ferrule (format json) rather than the workflow_manual sentinel: a JSON answer, ws-mcp's key-record prune piggybacks on ferrule, and the keyed workflow_manual fetch is needed either way." — bearing: constrains
- 260904-feat-ws-pi-lead-bootstrap-system-prompt (2026-09-04, Decisions): "the fresh-bootstrap sentinel obsidian-latch | own key | the skill text's FRESH path must never double-mint on Pi; the bridge already bootstrapped" (pi rewrites the sentinel; rejected here) — bearing: constrains
- 260630-feat-lead-skill-parallel-init (2026-06-30, cac80883): "The obsidian-latch sentinel is exposed in SKILL.md per the plan; the prior per-file no-expose rule is stale and not applied." — bearing: supports
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Result): "lead-use-mailbox/arm-the-wait.claude.md variant; rsrc manifest plus the wsflow and pi rsrc mirrors regenerated; composition test extended" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/manual.ts, manual.test.ts, manual-engine.test.ts and the agents-plugin-wsflow/hooks/claude-mod/ copies; agents-plugin/skills/lead-use-mailbox/SKILL.md, agents-plugin-wsflow/skills/lead-use-mailbox/SKILL.md; arm-the-wait.claude.md under agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/, agents-plugin-pi/rsrc/ with agents-plugin/rsrc/manifest.json and agents-plugin-pi/rsrc/manifest.json hashes |
| scope.surface | public-interface | model-facing: sentinel workflow_manual results gain the adoption notice, every adoption notice gains a push sentence, two shipped skill shims and a shipped playbook branch change |
| scope.new_public_symbol | no | none; revivedKeyToAdopt and adoptionNotice in manual.ts change behavior or signature only |
| scope.new_type_contract | no | none; adoptionNotice may take a push flag as sectionText already does, internal to manual.ts |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/manual.test.ts, manual-engine.test.ts; agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py pins the lead-use-mailbox shim as a single-call SESSION_KEY_SHIM_TITLES form and the parallel-init form without a blocker sentence; agents-plugin-tool/internal/wsrsrc/lead_use_mailbox_arm_wait_test.go pins the claude step 3 text "Your harness re-invokes you when that background task" |
| complexity.reuse_points | confirmed | adopt in manual.ts (re-login, switchKey, key entry rewrite, begin rebuild) read; rewriteRead main-thread filter in register.ts read |
| complexity.side_effect_risk | moderate | adoption re-logs in, rewrites the stored key entry, and rebinds the live mailbox push on a result-parsed key |
| risk.correctness | moderate | the minted key is parsed from result text rather than taken from the call argument, and generation races with an in-flight setup must hold |
| risk.fit | moderate | the shim change moves lead-use-mailbox between two pinned shim forms in the wsflow bundle test, and the agents-plugin-pi lead-use-mailbox shim is not named |
| risk.test | moderate | the organization-account policy cannot be reproduced live; coverage rests on unit tests, and two pinned text tests must be updated in step |
| risk.security_or_contract | moderate | reverses a recorded sentinel-never-adopted contract and changes the key the mailbox push binds to |

## Phases

### Phase 1: Adopt the sentinel key and bootstrap from lead-use-mailbox

Implement the Decisions in `agents-plugin/hooks/claude-mod/` (`manual.ts`,
its tests) with the byte-identical wsflow copy, the three `lead-use-mailbox`
skill shims, and every `arm-the-wait.claude.md` copy. Update the pinned tests
with the change: `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`
moves `lead-use-mailbox` from `SESSION_KEY_SHIM_TITLES` to
`EXPECTED_PARALLEL_INIT_SKILLS`, and
`agents-plugin-tool/internal/wsrsrc/lead_use_mailbox_arm_wait_test.go`
`claudeFallback` pins the new step 3 line; regenerate the
`rsrc/manifest.json` hashes.

Verification, by unit tests rather than a live organization-account probe
(the policy cannot be reproduced on this machine), in `manual.test.ts` /
`manual-engine.test.ts`: a main-thread sentinel result is adopted and the key
listener is told the new key; a `ferrule` result, a subagent's sentinel call,
and a sentinel result without `## Session Key` adopt nothing; the push
sentence appears only when the push runs; a main-thread sentinel result
reaches the model whole, not cut to the state-only text (if `stateOnly` cuts
it, fix that here: a cut leaves the lead in this scenario without the
manual). The claude-mod suites and the
skill-shim drift tests pass in every copy.

Implementation notes from review:
- The minted key is parsed from the result text's `## Session Key` section
  (the line after the heading), not from the call argument.
- "The push runs" means `pushAvailable()` resolves true (its host-command
  probe passed), the same condition that gives `sectionText` its mailbox
  line.
- The pi shim is `agents-plugin-pi/skills/lead-use-mailbox/SKILL.md`.
- Step 3's new line, in substance: "Without the mod, and when neither the
  system prompt nor a key-change notice says the push is bound, arm the CLI
  wait in the background and stop the turn."
- The `manual.test.ts` test "no adoption for the sentinel, the current key,
  or a key ws-mcp refuses" (around L822-L826) is inverted for the sentinel
  case, not merely extended; the reaches-the-model-whole check sits beside it.
- Regenerate `agents-plugin/skills/manifest.json` (WSRSRC_REGEN_SKILLS) and
  `agents-plugin-pi/skills/manifest.json` with the shim edits.
- The wsflow test move adds `PARALLEL_INIT_TITLES["lead-use-mailbox"]` and
  retires or rewrites `test_session_key_shims_pin_their_clause` if
  `SESSION_KEY_SHIM_TITLES` empties. This partly reverses `b1648732`'s
  single-call form for that shim, by this ticket's Decisions.
- Parallel with `261023-feat-ws-shared-question-classes-and-ids`: on a
  manifest or mirror conflict at merge, regenerate, never hand-merge.
