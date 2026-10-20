---
title: Retire the mcp-server-repair skill in favor of the reconnect tool
related:
  261018-research-claude-mod-ws-mcp-reconnect: origin of the reconnect tool that replaces the skill on Claude
  261020-feat-claude-mod-lever-continue-flag: sibling; also edits reconnect.ts (continuation becomes a flag)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6a4c0b64e5292e00
sage-review-completeness-reviewed: 6a4c0b64e5292e00
completed: 2026-10-20
---

# Retire the mcp-server-repair skill in favor of the reconnect tool

## Background

`mcp-server-repair` is the skill an agent invokes when the ws MCP server is
down. Every lead skill ends with the pointer "If this call fails to connect,
run `/ws:mcp-server-repair`" (`/wsflow:...` in the wsflow tree), across the
ws, wsflow and Pi skill trees. The skill body routes to the Claude mod's
`request_mcp_reconnect` tool when listed, otherwise to a `ws-cli` /
`ws-mcp-launcher.py` CLI fallback and manual `/mcp` reconnect steps for the
user.

The reconnect tool (`agents-plugin/hooks/claude-mod/reconnect.ts`, registered
in `register.ts` `reconnectSetup`) now restores the server on Claude reliably.
The repair need has only ever arisen on Claude: Pi and other harnesses do not
lose the server this way. The user decided to retire the skill and let the
reconnect tool's own description carry the trigger.

## Decisions

- **Retire the skill.** Delete the `mcp-server-repair` skill and every pointer
  to it. The reconnect tool's description becomes the only trigger text, so it
  is strengthened. This deliberately reverses
  `260725-feat-ws-cli-mcp-fallback-surface` (the skill as the CLI fallback's
  discovery path, kept inline so it reads with the server down) and
  `260805-chore-mcp-repair-skill-trigger-attention` (the skill's tuned
  trigger); the reconnect tool now covers the only harness that needs repair.
- **Keep the CLI call surface.** `ws-cli` and `ws-mcp-launcher.py`'s
  `tools` / `call` subcommands stay as they are; only the skill prose that
  taught them goes. The user expects a later use for them. They are
  intentionally left with no agent-facing pointer until that use exists;
  `failureText` does not name them.
- **Scope across trees.** Remove the skill from `agents-plugin/skills/` and
  `agents-plugin-wsflow/skills/`, its entry in the tracked
  `agents-plugin/skills/manifest.json` (regenerated with the repo's existing
  manifest tooling, guarded by the `agents-plugin-tool/internal/wsrsrc`
  manifest tests; `agents-plugin-wsflow/skills/` has no manifest), the
  pointer line in every lead SKILL.md of both trees, and the skill lists in
  `agents-plugin/tests/test_skill_dispatch_contracts.py#L25`,
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L36` / `#L52` and
  `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-L17`.
  `agents-plugin-pi/skills/` is gitignored output regenerated from
  `agents-plugin/skills/` (`.gitignore#L24`), so there is no tracked Pi skill
  to delete. `agents-plugin-pi/test/skills-dir.test.ts#L329-L332` uses the
  name only as a synthetic fixture directory; rename that fixture to a
  neutral name so the verification grep stays empty.
- **Shim tails.** In the ws tree the pointer is its own line and is deleted.
  The wsflow single-call shims in `POINTER_TAIL_TITLES` revert to the generic
  "If the playbook cannot be loaded, stop and report that blocker." tail, and
  the bundle test drops the pointer-tail check so those shims fall under the
  generic shim assertion; `lead-use-mailbox` keeps its pinned `session_key`
  clause (keep a dedicated exact-tail check for it). Rejected: dropping the
  sentence with no tail (leaves the shim without a failure instruction).
- **Substitution mirror stays.** `substitutionMirroredSkills`
  (`skills_mirror_test.go`) and the "Substitution-Mirrored Skill Generation"
  section of `wsflow-mirroring.md` lose their only member; keep the mechanism
  with a "no current members" note. Rejected: deleting the mechanism
  (functionality removal outside this ticket's approval).
- **Docs.** Update `ai-docs/manuals/wsflow-mirroring.md`, which names the
  skill as the inline-body exception. `CHANGELOG.md` history and `.done/`
  tickets stay untouched.
- **Failure text inlines manual recovery.** `failureText` tells the agent to
  ask the user to re-enable the server from `/mcp`, run `/reload-plugins`, or
  restart the session. Rejected: pointing at another document (after
  retirement nothing else carries those steps). This replaces
  `261018-feat-claude-mod-mcp-reconnect-lever`'s failure pointer at the
  skill's manual steps and CLI fallback. It also carries forward the skill's
  no-retry rule: do not call the reconnect tool again for this drop, since a
  reconnect that just failed fails the same way (an unattended retry loops
  through failure prompts, fast in headless mode).
- **Trigger-first description.** `reconnectDescription` opens with its
  trigger ("Call this first when the <plugin> MCP server's tools have left
  your tool list or a call reports it not connected"). Rejected: today's
  mechanism-first order (the description is now the only trigger text an
  agent sees).
- **No replacement on Codex or Pi.** Harnesses other than Claude never need
  repair (user decision).
- **Drop the spec ticket.** `260728-chore-spec-mcp-server-repair-unspecified`
  is dropped in this ticket's closing commit; its subject is retired. So is
  `260728-chore-ws-tree-skill-pointer-guard`, whose subject (a guard on the
  repair pointer tail in ws-tree SKILL.md files) disappears with the pointer.
- **wsflow mirror.** Every `agents-plugin/hooks/claude-mod/` edit is mirrored
  into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/skills/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Runs concurrently with `261020-feat-claude-mod-lever-continue-flag`
  (both rewrite `reconnectDescription` / `failureText` and the pinned strings
  in `reconnect.test.ts`) and `261020-feat-worktree-profiles` (it deletes
  `lead-scope-worktree`, whose SKILL.md and test listings this ticket also
  edits). Implement against this ticket alone; the lead merges this ticket
  before the continue-flag ticket and composes the combined texts at merge.

## Prior Decisions

- 260725-feat-ws-cli-mcp-fallback-surface (2026-07-25, Decisions): "Discovery goes through a dedicated skill, `/ws:mcp-server-repair` (mirrored as `/wsflow:mcp-server-repair`), not through the workflow manual." — bearing: contradiction-candidate
- adbf5ec3 (2026-07-25, commit): "Skill is deliberately NOT a lead-*/playbook.print shim: it must be readable with the MCP server down, so a playbook.print body would die exactly when the skill is needed." — bearing: contradiction-candidate
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "Failure and headless share one path. ... the continuation reports the failure and points the lead at `mcp-server-repair`'s manual reconnect steps and its CLI fallback." — bearing: contradiction-candidate
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "`mcp-server-repair` gains a Claude branch. In both packages' copies of the skill: when the harness lists the reconnect request tool, the lead calls it first" — bearing: supports
- 260805-chore-mcp-repair-skill-trigger-attention (2026-08-05, Decisions): "Scope is the trigger surface only. No change to the skill's handlers, invariants, doctrine, or reconnect template" — bearing: supports
- 261020-feat-claude-mod-lever-continue-flag (2026-10-20, Decisions): "Every `agents-plugin/hooks/claude-mod/` edit is mirrored into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`" — bearing: constrains
- 27b15f09 (2026-10-20, commit): "User decided: retire the repair skill but keep the CLI call capability; flag both levers (reconnect too); Pi out of scope" — bearing: supports
- 260728-chore-ws-tree-skill-pointer-guard (2026-07-28, commit): "260728-chore-ws-tree-skill-pointer-guard: created" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/skills/ (12 lead SKILL.md plus mcp-server-repair and manifest.json), agents-plugin-wsflow/skills/ (15 lead SKILL.md plus mcp-server-repair), agents-plugin/hooks/claude-mod/ and agents-plugin-wsflow/hooks/claude-mod/ (reconnect.ts, reconnect.test.ts), four test files, ai-docs/manuals/wsflow-mirroring.md |
| scope.surface | cross-module | removes the user-invocable /ws:mcp-server-repair and /wsflow:mcp-server-repair skills and rewrites the model-facing reconnect tool description across skills, hooks, tests and docs |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; failureText and reconnectDescription keep their signatures |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/reconnect.test.ts, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go |
| complexity.reuse_points | not-applicable | deletion plus prose rewrite; existing failureText and reconnectDescription are edited in place |
| complexity.side_effect_risk | moderate | manifest hashes regenerate, wsflow bodies are generated or byte-identical mirrors, and the pointer-tail regexes in the wsflow bundle test are removed |
| risk.correctness | moderate | a missed pointer or pinned string leaves a dangling reference; the stated grep also matches files the ticket does not list |
| risk.fit | moderate | reverses recorded decisions that made the skill the CLI fallback's discovery path and an inline-body exception in wsflow-mirroring.md |
| risk.test | moderate | four suites across Python, Go and TypeScript plus mirror and manifest drift guards must all change consistently |
| risk.security_or_contract | moderate | pinned model-facing tool description and failure prompt change; removing the skill drops the only discovery path for ws-cli |

## Phases

### Phase 1: Remove the skill and strengthen the reconnect tool

Delete the skill and its pointers per Decisions, rewrite `failureText` and
`reconnectDescription` in `reconnect.ts`, and update the tests that assert
them (`reconnect.test.ts`, `reconnect-engine.test.ts`) in both claude-mod
copies. (Fact check: the skill name and the pinned `failureText` prose appear
only in `agents-plugin/hooks/claude-mod/reconnect.test.ts#L141-L144` and
`reconnect.ts#L54`; `reconnect-engine.test.ts` builds expected text through the
imported `failureText` / `reconnectDescription` and contains no
`mcp-server-repair` string, so it needs no edit unless those signatures
change. The pinned `reconnectDescription` string is at
`reconnect.test.ts#L125-L127`.)

Verification: no tracked live reference to `mcp-server-repair` remains
outside historical records (`git grep -n mcp-server-repair -- ':!CHANGELOG.md'
':!ai-docs/tickets' ':!ai-docs/.plans' ':!ai-docs/.old'` is empty; tickets and
plans are records and stay untouched, and the gitignored Pi skill copy
regenerates from `agents-plugin/skills/`); the Python plugin tests, the
wsflow bundle tests, `npm test` in `agents-plugin-pi`, `go test ./...` in
`agents-plugin-tool`, and the claude-mod tests pass.

### Result (b1648732a) - 2026-10-20

Landed: the `mcp-server-repair` skill is deleted from `agents-plugin/skills/`
and `agents-plugin-wsflow/skills/`, with its manifest entry (regenerated) and
every pointer line in the lead SKILL.md files of both trees. wsflow single-call
shims revert to the generic "If the playbook cannot be loaded, stop and report
that blocker." tail; parallel-init and `lead-revive` shims just lose the line.
`reconnectDescription` opens with its trigger and `failureText` inlines the
`/mcp` re-enable, `/reload-plugins` and restart steps plus the no-retry rule
(names the tool through `TOOL_NAME`); both claude-mod copies are byte-identical.
Tests updated: `reconnect.test.ts`, the wsflow bundle test (generic shim
assertion plus a dedicated `lead-use-mailbox` session_key tail check),
`test_skill_dispatch_contracts.py`, `skills_mirror_test.go`
(`substitutionMirroredSkills` is now empty), and the Pi fixture name.
`wsflow-mirroring.md` updated; the substitution mechanism stays with no members.

Verification: `git grep mcp-server-repair` outside the excluded historical
paths is empty; `python3 -m unittest discover agents-plugin-wsflow/tests` 15 OK;
`discover agents-plugin/tests` 78 OK; `go test ./...` in `agents-plugin-tool`
all ok; `claude plugin test agents-plugin` and `agents-plugin-wsflow` 279 pass
each; Pi `test/skills-dir.test.ts` 15 pass. Full `npm test` in
`agents-plugin-pi` fails in this worktree on environment (no node_modules,
then sandbox socket and web-search credential failures unrelated to this diff).
Lite review: clean.

Decisions: the wsflow generic shim test excludes `lead-use-mailbox` and checks
it separately (renamed `POINTER_TAIL_TITLES` to `SESSION_KEY_SHIM_TITLES`);
the parallel-init and alias regexes simply drop the pointer line.
