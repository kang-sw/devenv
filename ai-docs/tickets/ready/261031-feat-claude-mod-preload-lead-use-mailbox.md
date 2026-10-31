---
title: Claude mod expands lead-use-mailbox like every other lead skill, with its key as a placeholder
related:
  261031-feat-claude-mod-manual-via-preload-once: extended (epoch manual row; reverses its (65) pass-through for lead-use-mailbox)
  261030-feat-claude-mod-ws-preload-skill-expansion: extended (preload block and key guard)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8461c75a2858ee66
sage-review-completeness-reviewed: 8461c75a2858ee66
---

# Claude mod expands lead-use-mailbox like every other lead skill, with its key as a placeholder

## Background

Since 261031-feat-claude-mod-manual-via-preload-once, the Claude mod delivers
the workflow manual as one appended row per conversation epoch when it expands
a lead skill's `<ws-preload>` block. `lead-use-mailbox` is the one lead skill
that never expands: its shim (ws and wsflow) has no block, and the mod's
`PRELOAD_EXCLUDED` (`agents-plugin/hooks/claude-mod/preload.ts`) skips it,
because its playbook body carries the reading key. The `arm-the-wait`
include renders `{{.MailboxWaitCommand}}`, which ws-mcp resolves to a concrete
`<bin> mailbox wait --session-key <key> [--slug <address>]` command
(`agents-plugin-tool/internal/mcp/playbook_tools.go` `mailboxWaitCommandVar`),
and `skill.prompt` also fires for subagent preloads, so a key-bearing body is
never inlined.

The shim therefore still tells the model to call `playbook.read` and
`workflow_manual` itself. Dogfooding 0.48.8 (2026-10-31) showed the cost: a
fresh session that starts with `/ws:lead-use-mailbox` gets no row, calls the
sentinel, and is re-keyed; a session that already has the row receives the
whole manual and state a second time, because the mod no longer rewrites
`workflow_manual` results. The owner uses the mailbox skill often.

## Decisions

(71) and (72) are the owner's 2026-10-31 discussion item IDs: (71) asked
whether to keep the pass-through, and the owner chose to unify; (72) settled
the placeholder. (65) is the decision ID in
261031-feat-claude-mod-manual-via-preload-once that this ticket partly
reverses.

- supersedes 261031-feat-claude-mod-manual-via-preload-once (65): its
  "a block the mod passes through (lead-use-mailbox, ...) appends no row"
  clause no longer covers lead-use-mailbox, which now has a block the mod
  expands; the no-key and failed-read pass-throughs stand.
- **lead-use-mailbox expands like the other lead skills ((71), (72)).** Both
  `agents-plugin/skills/lead-use-mailbox/SKILL.md` and
  `agents-plugin-wsflow/skills/lead-use-mailbox/SKILL.md` wrap their existing
  read lines in `<ws-preload playbook="lead-use-mailbox">`, the shape
  `lead-discuss` uses (the `workflow_manual` line inside the block). The mod
  drops `lead-use-mailbox` from `PRELOAD_EXCLUDED`, so the expansion inlines
  the playbook body (or the playbook pointer on a repeat) and appends the
  epoch row like any other lead skill. A harness without the mod sees the
  unchanged prose inside the block.
  - Rejected (71 as first raised): keep the pass-through and append only the
    row for this skill; the shim's own `workflow_manual` call would still
    duplicate the manual.
  - Rejected: keep the status quo; the owner uses the mailbox skill often and
    pays a duplicate manual or a re-key each time.
- **The key becomes a placeholder in the mod ((72)).** Before the existing
  key guard, the preload replaces every occurrence of the conversation's key
  in the body read for expansion with `<your key>` (the placeholder the
  playbook's generic wait-command form already uses). The real key reaches
  the lead in the epoch row; a subagent preload never sees it. The existing
  key guard stays as a backstop after the replacement. This applies to every expanded body, not a `lead-use-mailbox` special case.
  - Accepted: an expanded `lead-use-mailbox` body and a direct
    `playbook_read` result of it differ (placeholder vs key), so the
    cross-path playbook dedupe does not match between those two copies for
    this playbook; expansion-to-expansion repeats still dedupe. One
    playbook-keyed hint holds the last body hash for both paths, so a keyed
    direct `playbook_read` between two expansions makes the second expansion
    inline the full body again rather than the pointer (safe: a duplicate,
    never a wrong pointer). Tests must not expect a pointer in that order.
  - Accepted: the `--slug <address>` part of the rendered wait command stays
    inlined. The address is how peers reach the lead, not an authority like
    the key; it matters only when the mod's push is not running (then
    `arm-the-wait` step 3 runs the command), and a stale address after a
    reconnect fails visibly on the wait. The shipped claims that the preload
    inlines "never the session state or the key" are reworded to say it
    never inlines the key or the session state, with the address named as
    possibly present. Rejected: also replacing the address with
    `<your address>` (the mod would need the registered address at
    expansion time for a non-secret value).
  - Rejected: a keyless-render argument on `playbook_read` in ws-mcp (an MCP
    tool contract change for a Claude-only need).
- **Claude mod and shims only.** The Pi adapter keeps its own
  `PRELOAD_EXCLUDED` (`agents-plugin-pi/src/lead-skills.ts`) and gets no
  source edit. Pi's shims are not tracked: `agents-plugin-pi/scripts/copy-skills.mjs`
  copies `agents-plugin/skills/` at pack time, so the Pi `lead-use-mailbox`
  shim carries the new block, Pi's exclusion passes it through unchanged, and
  the model follows the prose as today. Rejected:
  aligning Pi here (Pi keeps the per-turn system-prompt manual, so it lacks
  the duplicate this ticket removes).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (shims and the mod ship downstream)
- Convention: ai-docs/manuals/wsflow-mirroring.md (both shims; the claude-mod tree mirrored byte-identically to agents-plugin-wsflow/hooks/claude-mod/)
- Convention: ai-docs/manuals/skill-authoring.md (shim edits)
- The playbook body, `arm-the-wait.claude.md`, and ws-mcp are unchanged.

## Prior Decisions

- 261031-feat-claude-mod-manual-via-preload-once (2026-10-31, Decisions): "Every expanded lead skill, shims untouched ((65)). The row is appended for every lead skill whose preload block the mod expands; the SKILL.md shims stay unchanged. A block the mod passes through (lead-use-mailbox, no key held, a failed read) appends no row" — bearing: superseded for lead-use-mailbox by this ticket (see Decisions); the rest supports
- 261031-feat-claude-mod-manual-via-preload-once (2026-10-31, Result e525c1d53): "lead-use-mailbox's Claude arm-the-wait include reworded from \"your system prompt\" to \"your context\" (semantics unchanged); Go guard, rsrc manifest, wsflow and pi rsrc mirrors updated." — bearing: constrains
- 261030-feat-claude-mod-ws-preload-skill-expansion (2026-10-31, Result b22697bdd): "a body containing the lead key also passes through (beyond the lead-use-mailbox exclusion); ... the tag keeps its ws- name in wsflow (shared mod contract, matches no forbidden pattern)." — bearing: constrains
- b22697bd (2026-10-31, commit): "Added a guard beyond the ticket: a body that contains the lead key passes through unexpanded, so the never-inline-the-key rule holds even for a future playbook that substitutes it" — bearing: supports
- 261023-bug-claude-mod-sentinel-key-mailbox-push (2026-10-23, Decisions): "The ws, wsflow, and pi lead-use-mailbox skill shims take each package's lead-discuss two-call parallel form: playbook.read together with workflow_manual(...)" — bearing: supports
- 65a18047 (2026-10-23, commit): "the Claude CLI wait fallback is shrunk, not deleted, because a Claude Code without the mod has no other wake path" — bearing: constrains
- 261031-feat-ws-preload-playbook-unit-dedupe (2026-10-31, Result 21e0ea53): "After-pointer rule implemented as \"any pointer candidate of the unit, valid or not, yields the full body\": it covers both pointer kinds and makes forged/stale markers fail safe" — bearing: constrains
- 261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind (2026-10-23, f0558ec9): "lead-use-mailbox moves to EXPECTED_PARALLEL_INIT_SKILLS with its PARALLEL_INIT_TITLES entry" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | both lead-use-mailbox SKILL.md shims, agents-plugin/hooks/claude-mod/preload.ts, manual.ts, the wsflow claude-mod mirror, both hooks.json descriptions, two Python shim tests, wsflow-mirroring.md |
| scope.surface | cross-module | shipped shims in two packages plus the shared mod; exported PRELOAD_EXCLUDED in preload.ts loses its only entry |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; expandPreload keeps its signature |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/preload.test.ts, manual-engine.test.ts, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py |
| complexity.reuse_points | confirmed | expandPreload key guard and the lead-discuss block shape in agents-plugin/skills/lead-discuss/SKILL.md |
| complexity.side_effect_risk | moderate | expansion now fires for a skill whose body carries a key; the placeholder replacement is the only barrier before subagent preloads |
| risk.correctness | moderate | replacing every key occurrence must precede the includes(key) guard and must not alter other body text |
| risk.fit | moderate | agents-plugin/skills/manifest.json hash and the wsflow shim mirror must follow the shim edit; the Pi shim is a pack-time copy and Pi keeps its own exclusion |
| risk.test | moderate | existing tests pin the passthrough: preload.test.ts#L98, manual-engine.test.ts#L365, #L940, #L1032, both Python shim tests |
| risk.security_or_contract | moderate | a key leaking into a subagent preload is the failure mode; shipped shim contract changes in both packages |

## Phases

### Phase 1: Expand lead-use-mailbox with a key placeholder

1. Wrap the read lines of both `lead-use-mailbox` shims in
   `<ws-preload playbook="lead-use-mailbox">`, keeping the prose identical.
   Update the shim-shape tests that pin it as a no-block skill
   (`agents-plugin/tests/test_skill_dispatch_contracts.py`,
   `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`): the ws test's
   `no_block` set keeps only `lead-revive`, and the wsflow
   `EXPECTED_PRELOAD_EXCLUDED` becomes empty (its comment already treats the
   revive and the `lead-proceed` alias separately); reword both tests'
   comments. Regenerate the tracked skills manifest hash with
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run
   TestGenerateRealSkillsManifest`.
2. Remove `lead-use-mailbox` from the claude-mod `PRELOAD_EXCLUDED`; in
   `expandPreload`, replace the key with `<your key>` in the read body before
   the key guard. Rewrite the tests that pin `lead-use-mailbox` as a
   pass-through (`preload.test.ts` `PRELOAD_EXCLUDED` assertions,
   `manual-engine.test.ts` mailbox pass-through cases); keep the existing
   tests of the `body.includes(key)` guard, which stays as the backstop.
   Update the text that describes the exclusion or promises the preload
   never inlines "the session state or the key": the `preload.ts` and
   `manual.ts` header comments, the `description` in both
   `agents-plugin/hooks/hooks.json` and `agents-plugin-wsflow/hooks/hooks.json`
   (no test pins it), and the claude-mod paragraph of
   `ai-docs/manuals/wsflow-mirroring.md`.
3. Mirror the claude-mod tree to `agents-plugin-wsflow/hooks/claude-mod/`.

Verification: `claude plugin test` and `claude plugin validate <path>` in both
packages; `diff -r` of the two `hooks/claude-mod` trees; the package Python
unittests in `agents-plugin/tests` and `agents-plugin-wsflow/tests`;
`go test ./internal/wsrsrc/...`. Tests cover: a `lead-use-mailbox` expansion
whose read body contains the key inlines the body with `<your key>` in place
of every occurrence and never the key; it appends the epoch row; a second
expansion yields the pointer form; other skills' expansions are unchanged.
