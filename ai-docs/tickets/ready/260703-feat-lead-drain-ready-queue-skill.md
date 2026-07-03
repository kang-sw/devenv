---
title: "Add lead-drain-ready-queue skill: single-cycle ready/ ticket handoff to lead-proceed"
spec:
  - 260703-drain-ready-queue-skill
sage-review: completed
---

# Add lead-drain-ready-queue skill: single-cycle ready/ ticket handoff to lead-proceed

## Background

The user repeats a standing `/goal` directive prompt of the shape "finish all
`ready/` tickets ONE BY ONE, in delegate heavy mode based on skill
`/lead-prefer-subagent`" across sessions. The prose is verbose to retype and
mixes two distinct concerns: (1) a session delegation posture that already
exists verbatim as `lead-prefer-subagent`, and (2) picking the next `ready/`
ticket and routing it, which has no existing skill.

`lead-proceed`'s own rsrc playbook resolves its `Target` from `user request`
text and has no judge branch for "pick from the ready queue autonomously" —
`judge: actionable` returns `No` when the target does not name a concrete
change. Calling `lead-proceed` with no explicit ticket path therefore risks
an ambiguous or wrong route (for example sliding into ticket-creation/todo
flow) rather than picking up the next ready ticket. See discussion captured
this session (`260703`, parallel to `260703-chore-bootstrap-staleness-alarm`
and `260703-chore-implement-branch-rename-default-allow`).

## Decisions

- **Single-cycle shim, not an internal loop.** The skill resolves at most one
  ready ticket per invocation and stops after handing off. It does not poll
  `tickets.list` in a loop or repeat internally. The existing `/goal`
  Stop-hook mechanism already re-invokes a standing directive each turn; a
  loop embedded in this skill would duplicate that responsibility. Repeated
  draining of the whole `ready/` backlog is the caller's job (e.g. a `/goal`
  prompt that just names this skill).
- **Explicit-target handoff, not implicit routing.** The skill must select a
  specific ready ticket path itself and pass that path as `lead-proceed`'s
  explicit target — it must not call `lead-proceed` bare and expect it to
  infer "pick from ready/" on its own, since no such judge branch exists
  there today.
- **Apply `lead-prefer-subagent` posture before handoff**, by reference (skill
  invocation), not by duplicating its prose into this new skill's body.
- **Ticket selection rule** — evidence-based, not open-ended qualitative
  judgment (chosen for auditability in an unattended, unwatched loop):
  1. If `ready/` is empty: report that and stop with no handoff.
  2. Otherwise, read each ready-candidate ticket's `related:`/`parent:`
     frontmatter annotations for explicit precedence language (e.g.
     "prerequisite", "predecessor", "must land first", "blocks", "depends
     on") naming another ticket that is not yet `done`/`dropped`.
  3. If a candidate's annotation names such an unresolved ticket, and that
     referenced ticket is also in `ready/`, prefer the referenced ticket
     first.
  4. With no precedence signal among current ready candidates, default to
     the oldest date-prefix ticket (FIFO).
  5. If two candidates' precedence annotations conflict or cannot be
     resolved from the stated text, stop and ask the user — do not guess.
  This reuses the existing free-text `related:`/`parent:` annotation
  convention; it does not introduce a new structured dependency field.
  A candidate's precedence annotation naming an unresolved ticket that is in
  `todo/` or `idea/` (not `ready/`, not `done`/`dropped`) has no in-ready
  precedence target to defer to — treat this as no precedence signal and
  fall through to FIFO. Precedence resolution is single-level only (resolve
  one referenced ticket, do not chase transitive chains); a single-cycle
  shim re-evaluates from scratch on its next invocation, so a multi-hop
  chain resolves itself over successive cycles without needing transitive
  logic in one pass.
- **Rejected: full-loop variant.** Considered having the skill loop
  internally until `ready/` is empty. Rejected because the `/goal` Stop-hook
  already owns that repetition; embedding a second loop mechanism is
  duplicated responsibility for the same job.
- **Rejected: unconstrained "pick whatever seems reasonable" judgment.**
  Considered letting the agent freely judge "most impactful next ticket" by
  reading ticket bodies. Rejected for a skill meant to run unattended and
  repeatedly: free-form judgment is not reproducible run-to-run and is hard
  to explain after the fact ("why did it skip ticket X").
- Container tickets (epic/workset) are not specially filtered out at
  selection time — `lead-proceed`'s existing `scope_blocked=container-ticket`
  guard already stops safely if one is picked, so this ticket does not need
  to duplicate that check.

## Out of Scope

- Changing `lead-proceed`'s own routing judges.
- Any new structured ticket dependency/`blocks:` frontmatter field.
- Any change to `lead-prefer-subagent` itself.

## Phases

### Phase 1: Author the lead-drain-ready-queue skill

- Read `agents-plugin/skills/lead-skill-authoring/SKILL.md` and apply its
  invariant checklist before authoring.
- Author `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` as a compact,
  static-prose skill (same inlined-body shape as `lead-verify-discussion` /
  `lead-prefer-subagent` — no rsrc playbook indirection), implementing:
  ticket selection per Decisions, `lead-prefer-subagent` posture application
  by reference, and explicit-target handoff to `lead-proceed`.
- Register the skill as a user-invocable `/ws:lead-drain-ready-queue` entry
  point (update `agents-plugin/skills/manifest.json` and any Codex-facing
  skill registration list alongside the other 14 entry skills).
- Update `ai-docs/mental-model/workflow-skills.md` to describe the new skill
  alongside the other lead-* entries, matching the existing description
  style (see `lead-verify-discussion`'s entry there for shape).
- Decide during implementation whether `agents-plugin-wsflow` needs a mirror
  copy (substitution-mirrored generation per `agents-plugin-tool/internal/wsrsrc/skills_mirror.go`'s
  curated list, or a hand-authored wsflow variant) or whether this skill is
  full-ws-only; record the decision and rationale in this phase's Result.
- Verification: confirm the new skill's static text matches the Decisions
  above (selection rule text, single-cycle framing, no internal loop
  language); confirm `agents-plugin/skills/manifest.json` regenerates clean;
  run existing skill dispatch/manifest drift tests
  (`agents-plugin/tests/test_skill_dispatch_contracts.py`,
  `agents-plugin-tool/internal/wsrsrc` manifest/drift tests) and add a
  dispatch-contract test asserting the skill contains the FIFO-fallback and
  precedence-language selection rule text, mirroring the existing
  `test_verify_discussion_is_inlined_static_body`-style pattern.

### Result

Implemented as planned, no deviations from the Decisions above.

- Authored `agents-plugin/skills/lead-drain-ready-queue/SKILL.md` as a static-
  inline entry skill (no rsrc playbook, no `playbook.print` indirection),
  containing the full 8-step selection rule text verbatim-equivalent to
  Decisions, explicit single-cycle framing, `lead-prefer-subagent`
  posture-by-reference, and explicit-target `lead-proceed` handoff.
- Registered in `agents-plugin/skills/manifest.json` (regenerated via
  `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc -run TestGenerateRealSkillsManifest`,
  not the `TestRegenerateShippedManifest`/`WS_REGEN_MANIFEST=1` command named
  in this phase's verification bullet, which regenerates a different
  parallel manifest under `agents-plugin/rsrc/`).
- Updated `ai-docs/mental-model/workflow-skills.md` with a new entry bullet
  under `{#260703-drain-ready-queue-skill}` and corrected the stale entry-
  skill count (was already 2 skills stale pre-existing this ticket; now 16,
  matching the actual `manifest.json` `SKILL.md` file count post-add).
- Finalized `ai-docs/spec/workflow-skills.md`'s `{#260703-drain-ready-queue-skill}`
  block from its `> [!note] Planned 🚧` pre-implementation callout to landed
  prose, and added a closing sentence recording the inline-body/mirror shape.
- **Wsflow-mirror decision: mirror it.** The skill's body contains no
  `ws:`/`ws/`/`ws.` full-namespace tokens (only skill names), so it is
  substitution-mirror-eligible via the existing curated-list mechanism
  (`agents-plugin-tool/internal/wsrsrc/skills_mirror.go`) — added
  `"lead-drain-ready-queue"` to `substitutionMirroredSkills` and regenerated
  the byte-identical `agents-plugin-wsflow/skills/lead-drain-ready-queue/SKILL.md`
  via `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`,
  matching the `lead-prefer-subagent`/`lead-verify-discussion` precedent; no
  hand-authored wsflow variant was needed.
- Added `test_drain_ready_queue_is_inlined_static_body` to
  `agents-plugin/tests/test_skill_dispatch_contracts.py`, and added the stem
  to `EXPECTED_SKILLS`/`EXPECTED_INLINE_SKILLS` in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`.
- Verification: `agents-plugin` (41 tests) and `agents-plugin-wsflow` (9
  tests) Python suites pass; `agents-plugin-tool` (`go test ./... -count=1`)
  passes; `go build ./...` clean. Partitioned `correctness` and `test`
  reviewers both returned `clean`.
- Commits: `375e8c33` (ticket), `88c8de2e` (spec Planned callout),
  `4bd0b805` (sage-review stamp), `6788c678` (survey plan),
  `a1f50ac3` (implementation).

#### Edition (b98e45c) - 2026-07-03

User review found the landed skill too long and too discussable-workflow-
shaped for what is a purely user/`/goal`-invoked shim. Shrunk it:

- Selection rule condensed from the 8-step list to one sentence: prefer a
  `related:`/`parent:`-named prerequisite when it is also in `ready/`,
  otherwise oldest-first FIFO. The explicit conflict-stop-and-ask,
  todo/idea-fallback, and single-level-only sub-rules were dropped from the
  skill text (they remain recorded as rejected/superseded detail in
  `## Decisions` above); FIFO is now the sole fallback for any case the
  one-sentence rule does not resolve.
- `description` shortened to a plain trigger sentence, since this skill is
  effectively user/`/goal`-invocation-only rather than a general discussable
  workflow entry.
- Added an explicit lead-context-conservation directive: never read/write
  files or run commands directly for this invocation; delegate everything,
  including simple tasks like commits, to an appropriately tiered subagent
  or forked subagent per `lead-prefer-subagent`. This reflects the skill's
  purpose inside a long-running `/goal` drain loop, where lead context is
  the scarce resource.
- Updated `agents-plugin/tests/test_skill_dispatch_contracts.py`'s
  `test_drain_ready_queue_is_inlined_static_body` to match the shortened
  text; regenerated `manifest.json` and the byte-identical wsflow mirror;
  updated `ai-docs/mental-model/workflow-skills.md` and
  `ai-docs/spec/workflow-skills.md` entries to match.
- Verification: `agents-plugin` (41 tests) and `agents-plugin-wsflow` (9
  tests) Python suites pass; `agents-plugin-tool` (`go test ./... -count=1`)
  passes; `go build ./...` clean.
- Commit: `b98e45c`.
