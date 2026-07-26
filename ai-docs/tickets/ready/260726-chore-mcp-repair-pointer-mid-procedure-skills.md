---
title: mcp-server-repair pointers reach only front-door skills, not the skills a procedure lands in
related:
  260624-epic-pre-release-cleanup: item 8 · repair-pointer coverage gap
  260726-feat-ws-cli-call-stdin-payload: sibling CLI-fallback ergonomics ticket from the same MCP-down dogfooding class
  260726-feat-ws-cli-lenient-tool-name-resolution: sibling CLI-fallback ergonomics ticket from the same MCP-down dogfooding class
sage-review-design: completed
sage-review-completeness: completed
---

# mcp-server-repair pointers reach only front-door skills, not the skills a procedure lands in

## Background

Reported downstream (wsflow 0.36.1) as "no local fallback when the MCP server
drops mid-procedure". **Two of the report's premises are false and must not be
acted on:**

- *"`rsrc/` ships `impl-playbook.md` and `sample-playbook/`, but not the `lead-*`
  playbooks."* False. Every `lead-*` playbook ships in `rsrc/`, verified in
  source (`agents-plugin/rsrc/lead-write-ticket/`), in the wsflow mirror, and in
  an installed plugin cache. The reporter appears to have read the top-level
  listing and mistaken two entries for the whole set. **Do not bundle anything.**
- *"There is no documented degraded path."* False in current source. The
  `mcp-server-repair` skill exists precisely for this: it makes no MCP call, maps
  every `ws/x.y(a: b)` to `ws-cli call x.y '{"a": "b"}'`, covers cold start, and
  carries a verbatim reconnect-steps relay for the user.

The report is explained almost entirely by distribution lag. `mcp-server-repair`
landed in `adbf5ec3` on 2026-07-25; the 0.36.1 release the reporter ran was
`62d9a1a1` on 2026-07-24. It missed the release by one day.

The genuine residue is pointer coverage. `adbf5ec3` added front-door pointers to
`lead-discuss`, `lead-sprint`, `lead-proceed`, and `lead-revive` — entry-point
skills. `lead-write-ticket` and `lead-write-spec` have none; their SKILL.md still
ends at "If the playbook cannot be loaded, stop and report that blocker."
Downstream died at exactly that line, mid-procedure, after routing.

**The two trees are not symmetric, and the direction of the fix differs per
skill.** `agents-plugin/skills/` contains no `lead-write-ticket` or
`lead-write-spec` directory at all — in the ws tree those are playbook-only
surfaces reached through `playbook.print`, per the skill-playbook diet. The shims
carrying the un-pointed tail exist only in `agents-plugin-wsflow/skills/`. So for
the two skills this ticket is named after, wsflow is the *sole* target, not a
mirror destination.

Separately, ten ws-tree skills do call `playbook.print` without naming the repair
route (verified by enumeration): `lead-add-rule`, `lead-bootstrap`,
`lead-forge-mental-model`, `lead-forge-spec`, `lead-goal-fan-out-step`,
`lead-review`, `lead-salvage`, `lead-ship`, `lead-skill-authoring`, `lead-tune`.

## Decisions

- **Scope is a pointer sweep, nothing else.** No bundling, no new fallback
  mechanism, no softening of the stop-and-report default beyond naming the repair
  route.
- **Cover skills reached after routing, not just entry points.** A transient
  transport failure should not read as a hard stop on user-requested work when a
  documented CLI fallback exists one pointer away.
- **Do not close the tree asymmetry by creating new ws-tree front-door skills.**
  The ws tree deliberately has no `lead-write-ticket`/`lead-write-spec` shim.
  Adding one is a distribution decision needing its own approval, not a
  side effect of a pointer sweep.

## Prior Art

- `adbf5ec3` established the pointer wording; reuse it verbatim rather than
  inventing a second phrasing.

## Spec Impact

- Target spec area: none. `ai-docs/spec/workflow-skills.md` already documents the
  `mcp-server-repair` fallback as the MCP-down route; this ticket changes only
  which SKILL.md files point at it.
- Expected caller-visible change: none beyond skill text. No tool contract, no
  routing semantics, no new behavior — a skill that previously dead-ended now
  names an already-documented route.
- Contract-first spec: no. Nothing is planned that is not already specified.

## Phases

### Phase 1: Extend the repair pointer to mid-procedure skills

- **wsflow tree, sole target for the two named skills:** add the existing
  front-door pointer wording to `agents-plugin-wsflow/skills/lead-write-ticket`
  and `.../lead-write-spec`. Do not mirror these from the ws tree — they do not
  exist there.
- **ws tree, enumerated sweep:** add the same wording to the ten skills listed in
  `## Background`. The set is closed and stated; do not treat it as discoverable.
- **Update the shipped test.** `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py::test_skill_files_are_thin_playbook_shims`
  regex-matches the un-pointed tail verbatim across all non-exempt shims, with
  `lead-proceed` / `lead-discuss` / `lead-sprint` carved into separate
  pointer-tail assertions. Move `lead-write-ticket` and `lead-write-spec` into the
  pointer-tail set. This test is the verification probe.
- **Regenerate `agents-plugin/skills/manifest.json`** after the ws-tree edits
  (`WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1`); it carries a sha256
  per SKILL.md.

Verification boundary: `test_wsflow_skill_bundle.py` passes with
`lead-write-ticket` and `lead-write-spec` asserted as carrying the pointer tail;
the ten enumerated ws-tree skills each contain the pointer wording; the manifest
regenerates clean with no diff on re-run.
