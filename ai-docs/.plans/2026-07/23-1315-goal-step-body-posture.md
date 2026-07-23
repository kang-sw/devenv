# Plan: 260723-feat-goal-step-rename-and-goal-loop-completion — Phase 2: Blocked-progress completion term + autonomous in-scope bug capture

## Relevant Ticket Contract

- Phase 2 adds three things to `agents-plugin/skills/lead-goal-step/SKILL.md`
  (canonical, Phase 1 already renamed/repositioned it):
  (a) lead ticket-curation authority (edit existing tickets, create + link new
  ones, via the normal ticket-write path, within goal-run autonomy bounds — no
  new ticket-system state field);
  (b) blocked-progress conclusion, distinct from hard-gate pause, with the
  "all remaining `ready/` blocked" scoping guard, the non-skippable "record the
  blocker before yielding" step, the selector's advanceable-now read
  (replacing body-blind FIFO), and the discriminator "is there any work I
  could do without the human?";
  (c) bounded autonomous in-scope bug capture: route via the ticket-write
  path; goal-relevant/blocking → `ready/`; unrelated/incidental → `idea/`;
  explicit deferral → capture only; routing is skill-intrinsic, not anchored
  to any downstream project's own AGENTS.md.
- Hard constraints: wording must NOT nullify hard gates, must NOT let a
  hard-gate pause be reclassified as "goal complete," and must NOT introduce
  any new ticket-system state field (no `blocked:` frontmatter, no
  `.blocked/` status — mechanism is ordinary ticket edits + the selector's
  advanceable-now read).
- Must stay consistent with `260722` Phase 1's already-shipped hard-gate
  wording (the "Goal-run posture" paragraph already in the body) — do not
  scatter completion semantics elsewhere; single source of truth stays this
  skill body.
- Spec addressing: two `🚧 Planned` blocks in `ai-docs/spec/workflow-skills.md`
  (stems `260723-goal-step-ticket-curation-authority`,
  `260723-goal-step-blocked-progress-conclusion`) already carry near-final
  contract-first prose. Implementation must drop the `🚧`/blockquote wrapper
  and settle them — **keep the date-keyed anchors, do not delete them** (this
  exact mistake was caught and corrected during Phase 1 review).
- Mental-model bullets at `ai-docs/mental-model/workflow-skills.md` (ending
  `{#260703-drain-ready-queue-skill}` and `{#260707-drain-goal-branch-staging}`)
  need matching extensions with the new stacked anchors.
- Apply the skill-authoring invariant checklist to every changed line
  (`agents-plugin/skills/lead-skill-authoring/SKILL.md` →
  `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`).
- wsflow mirror (`agents-plugin-wsflow/skills/lead-goal-step/SKILL.md`) is
  substitution-generated, not hand-edited; must regenerate and stay
  namespace-clean (no `mercenary`, no `<!-- ws:full-only:` /
  `<!-- ws:wsflow-only:` markers, no literal `lead-write-code` /
  `lead-write-skeleton` / `lead-salvage` / `lead-skill-authoring`).
- Version bump on dev-merge is deferred to branch-merge time (per Phase 1
  Result note); not an action for this phase's file edit.

## Out of Scope

- The non-goal defensive branches ("when the current branch is not `goal/*`,
  stop") — ticket Constraints explicitly forbid touching these; deferred to a
  separate ticket.
- Phase 1 content (rename/reposition) — already landed, commit `dc29d4aa`.
  Do not re-touch `{#260703-drain-ready-queue-skill}` /
  `{#260707-drain-goal-branch-staging}` prose beyond removing the immediately
  adjacent blockquote wrappers.
- `260722` Phase 2 (Go reinforcement tips + spec) — separate ticket; only the
  already-shipped `260722` Phase 1 hard-gate wording in the "Goal-run posture"
  paragraph is a reference point, not an edit target.
- Any new ticket-system state field, frontmatter key, or status directory.
- Mandating `tickets.move`/directory re-triage as part of "record the
  blocker" — the ticket's own wording is "a recorded blocker note and/or a
  status re-triage" (optional second part); the minimum required mechanism is
  the on-ticket blocker note, matching the sage `## Blocked (YYYY-MM-DD)`
  precedent (blocked tickets stay visible in `ready/`, per the ticket's own
  "same-run fixing" resolution).
- Calling `tickets.sage_record` for this — that tool is sage-gate-specific
  (writes `sage-review-*` frontmatter); the new "record the blocker" step is a
  plain ordinary body edit, not that tool.
- CHANGELOG entry — out of scope for this phase edit (release-time concern).

## Codebase Findings

- `agents-plugin/skills/lead-goal-step/SKILL.md#L1-84` — canonical body to
  edit. Current shape is flowing prose paragraphs, no H2 headers (Invariants/
  Judgments/Templates) — matches the "Choreography skill... inline-body"
  precedent (`lead-verify-discussion`, `lead-prefer-subagent`); new content
  should stay in this dense-paragraph style, not introduce new headers.
  - `#L17-25` — "Goal-run posture" paragraph: the shipped `260722` Phase 1
    hard-gate list ("irreversible or destructive actions, scope expanding
    into public API or cross-module patterns, unresolved binding decisions,
    or any AGENTS.md 'Always ask' item... the hard gates never dissolve").
    New blocked-progress prose must cross-reference this list, not restate or
    weaken it.
  - `#L27-32` — ticket-selection subagent instructions: currently blind
    FIFO/prerequisite pick, no blocked-state read. This is the paragraph the
    "advanceable-now read" replaces.
  - `#L34-49` — "If the subagent reports `ready/` is empty" branch: contains
    both the untouched non-goal stop (`#L35`) and the goal-branch
    empty-queue merge-approval flow (`#L36-49`). The new "all remaining
    blocked" branch is a new sibling inside the `goal/<parent>/<slug>` case
    only — do not touch the non-goal branch, and do not reuse the
    merge-approval flow for the blocked-progress case (merging would
    misrepresent blocked/incomplete work as a finished goal — contradicts the
    ticket's "rather than silently marking the goal done" anti-abuse clause).
  - `#L72-78` — hand-off to `lead-proceed` paragraph: the natural anchor point
    for "record the blocker before yielding" (block is typically discovered
    from this turn's downstream work outcome, before the turn ends).
  - Zero `ws/`/`ws:`/`ws.`-prefixed tokens anywhere in the current body —
    preserve this: reference `lead-write-ticket` by bare name only (as the
    body already does for `lead-proceed`), not `ws/tickets.*` MCP-call
    syntax. Per the skill-authoring Layer model
    (`agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md#L15-21`),
    MCP schema/call-syntax restatement is Layer 1 and must be deleted from
    playbook/skill prose, not added.
- No `agents-plugin/skills/lead-write-ticket/SKILL.md` exists — it is an
  internal rsrc playbook (`agents-plugin/rsrc/lead-write-ticket/
  lead-write-ticket.md`) other skills invoke internally, not a directly
  user-callable top-level skill. Reference it by name as "the normal
  ticket-write path (`lead-write-ticket`)" — matches how the ticket's own
  Decisions text phrases it.
- `agents-plugin-tool/internal/wsdoc/tickets_sage.go#L527` —
  `## Blocked (%s)` (date) is the existing precedent format for an on-ticket
  blocked note with no new frontmatter field; reuse this shape/verbiage for
  the "record the blocker" instruction rather than inventing new structure.
- `ai-docs/spec/mcp-tools.md#L936-943` — confirms a sage `block` verdict
  appends `## Blocked (YYYY-MM-DD)` to the ticket body and the ticket stays
  visible where it is (does not move directories) — direct precedent that
  "advanceable-now" skip-in-place, not directory re-triage, is the reuse
  pattern.
- `ai-docs/spec/workflow-skills.md#L462-476` — ticket-curation-authority `🚧`
  block (inside the `{#260703-drain-ready-queue-skill}` paragraph run):
  content is already near-final (authority clause + bug-capture routing +
  "no new ticket-system state field"); settling is mostly deblockquoting plus
  a light check that wording matches whatever the shipped body says.
- `ai-docs/spec/workflow-skills.md#L513-529` — blocked-progress-conclusion `🚧`
  block: content covers the terminal, discriminator, and advanceable-now read
  already, but is missing two nuances the ticket explicitly calls for: (i)
  "record before yielding" framed as non-skippable, and (ii) that this
  conclusion explicitly skips the merge-approval flow (currently silent on
  merge behavior) — add both when settling.
- `ai-docs/mental-model/workflow-skills.md#L68` — bullet ending
  `{#260703-drain-ready-queue-skill}`; extend with ticket-curation-authority +
  bug-capture summary, append stacked anchor
  `{#260723-goal-step-ticket-curation-authority}`.
- `ai-docs/mental-model/workflow-skills.md#L69` — bullet ending
  `{#260707-drain-goal-branch-staging}`; extend with blocked-progress
  conclusion summary, append stacked anchor
  `{#260723-goal-step-blocked-progress-conclusion}`. Stacked-anchor-on-one-
  bullet precedent: `ai-docs/mental-model/mcp-runtime.md#L31` (two anchors
  space-separated at line end).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror.go#L20-32` — mirror
  eligibility guard forbidden tokens: `mercenary`,
  `<!-- ws:full-only:`, `<!-- ws:wsflow-only:`, `lead-write-code`,
  `lead-write-skeleton`, `lead-salvage`, `lead-skill-authoring`. New prose
  must avoid all of these literally.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-19` —
  `lead-goal-step` already in `substitutionMirroredSkills`; no list change
  needed this phase.
- `agents-plugin/skills/manifest.json#L12` — content hash for
  `lead-goal-step/SKILL.md`; needs regeneration after the body edit.
- `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L44-57` — regen
  command: `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run
  TestGenerateRealSkillsManifest -v` (run from `agents-plugin-tool`).
- `ai-docs/ref/wsflow-mirroring.md#L152-163` — wsflow skill-mirror regen:
  `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run
  TestRegenerateWsflowSkillsMirror` (run from `agents-plugin-tool`); `-count=1`
  is mandatory (stale test-cache risk, same as the `260722` v0.34.4 incident
  referenced in this ticket's own Phase 1 verification note).
- `agents-plugin/tests/test_skill_dispatch_contracts.py#L68-73` and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L20,44` — only
  assert `lead-goal-step` is present/inline/not-a-playbook-print-shim; no
  fragile exact-body substring assertions, so body wording changes are safe
  here.

## Implementation Plan

1. Edit `agents-plugin/skills/lead-goal-step/SKILL.md`:
   - After the "Goal-run posture" paragraph (`#L17-25`), add a ticket-curation
     authority paragraph: the lead may autonomously edit existing tickets
     (record findings, restructure, re-triage status) and create + link new
     tickets, via the normal ticket-write path (`lead-write-ticket`), within
     the goal-run autonomy posture above; state explicitly this adds no new
     ticket-system state — blocked state and captured bugs live as ordinary
     ticket edits.
   - In the same paragraph or an immediately adjacent one, add the bounded
     bug-capture posture: a bug discovered mid-run that blocks or is directly
     relevant to the current goal is promoted to `ready/` via the
     ticket-write path (still subject to the sage ready-landing gate); an
     incidental/unrelated bug is captured at `idea/`; an explicitly deferred
     bug is captured only, not queued to `ready/`. State this routing is
     skill-intrinsic — judged independently of any downstream project's own
     AGENTS.md dogfood-capture convention.
   - Rewrite the ticket-selection paragraph (`#L27-32`): the subagent now
     also reads each `ready/` candidate's body/state for a recorded blocker
     (e.g. a `## Blocked (...)`-style note) and skips blocked tickets;
     preference order (prerequisite via `related:`/`parent:`, else oldest
     FIFO) now applies only among advanceable (non-blocked) candidates. The
     subagent's report becomes: one advanceable ticket path, OR `ready/` is
     literally empty, OR every remaining `ready/` ticket is blocked
     (all-blocked) — carry this third outcome forward into the next step.
   - In the "if the subagent reports `ready/` is empty" branch (`#L34-49`):
     leave the non-goal stop and the goal-branch empty-queue merge-approval
     flow untouched; add a new sibling case, reached only when the subagent's
     report is "all remaining `ready/` tickets are blocked" (not literal
     empty) while on a `goal/<parent>/<slug>` branch — this is the
     blocked-progress conclusion: report the recorded blocker(s) to the user
     explicitly and end the run; do not loop; do NOT run the merge-approval
     flow (that flow is for the completed/empty-queue case only — running it
     here would misrepresent blocked work as a finished goal). State the
     scoping guard inline ("all remaining `ready/` blocked" only — one
     blocked ticket among otherwise-workable ones is not a conclusion, the
     selector just skips it) and the discriminator ("is there any work I
     could still do without the human?") with an explicit cross-reference to
     the Goal-run posture hard-gate list — a hard-gate pause (work remains,
     only a sign-off is needed) must never collapse into this conclusion.
   - Near the hand-off paragraph (`#L72-78`), add the "record the blocker
     before yielding" step as an explicit, non-skippable imperative: when
     this turn's downstream work concludes the dispatched ticket cannot
     advance without a human decision, record that blocker onto the ticket
     itself (ordinary body edit, e.g. a dated blocked note mirroring the sage
     `## Blocked (YYYY-MM-DD)` precedent) before the turn ends — an
     unrecorded block causes the next turn's selector to re-pick the same
     stuck ticket.
   - Keep every new/changed line to the invariant checklist (Falsifiable,
     Actionable, one line, context-free, non-redundant, doctrine-aligned) per
     `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md`.
   - Do not introduce any `ws/`/`ws:`/`ws.` token, `mercenary`, marker
     comments, or the four excluded-skill names (see Codebase Findings).

2. Regenerate the wsflow skill mirror (from `agents-plugin-tool`):
   `WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run
   TestRegenerateWsflowSkillsMirror`. Confirm
   `agents-plugin-wsflow/skills/lead-goal-step/SKILL.md` updated
   byte-appropriately (namespace-substituted copy of the canonical body).

3. Regenerate the skills manifest (from `agents-plugin-tool`):
   `WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run
   TestGenerateRealSkillsManifest -v`. Confirm the `lead-goal-step/SKILL.md`
   hash in `agents-plugin/skills/manifest.json` changed.

4. Settle `ai-docs/spec/workflow-skills.md`:
   - `#L462-476`: remove the `> [!note] Planned 🚧` blockquote wrapper,
     convert to a plain settled paragraph, keep
     `{#260723-goal-step-ticket-curation-authority}` attached at the end.
     Reconcile wording against the shipped SKILL.md body from step 1.
   - `#L513-529`: same treatment for the blocked-progress-conclusion block;
     while settling, add the two nuances not yet present in the draft — the
     non-skippable record-before-yielding framing, and the explicit no-merge
     distinction from the empty-queue completion. Keep
     `{#260723-goal-step-blocked-progress-conclusion}` attached.
   - Leave `{#260703-drain-ready-queue-skill}` and
     `{#260707-drain-goal-branch-staging}` prose otherwise untouched.

5. Update `ai-docs/mental-model/workflow-skills.md`:
   - Extend the `#L68` bullet with a clause on ticket-curation authority +
     bounded bug-capture routing; append stacked anchor
     `{#260723-goal-step-ticket-curation-authority}` after the existing
     anchor.
   - Extend the `#L69` bullet with a clause on the blocked-progress
     conclusion (scoping guard, advanceable-now read, discriminator, no-merge
     distinction); append stacked anchor
     `{#260723-goal-step-blocked-progress-conclusion}`.
   - Keep additions focused on non-obvious modification coupling, not a full
     restatement of the spec prose (per `ai-docs/spec/documentation-system.md`
     mental-model scope rule).

6. Run the skill-authoring "On: Fresh-Reader Audit" and "On: Downstream
   Consistency Sweep" (`agents-plugin/rsrc/lead-skill-authoring/
   lead-skill-authoring.md#L111-138`) against the edited SKILL.md, since this
   touches doctrine/routing/terminology (blocked vs. paused) and a mirrored
   surface.

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1`
- `cd agents-plugin-tool && WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`, then `go test ./internal/wsrsrc -count=1` to confirm the drift guard is green.
- `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`, then confirm the manifest guard passes on a normal `go test ./internal/wsrsrc/... -count=1`.
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `python3 -m unittest discover agents-plugin/tests`
- Manual grep of the new `SKILL.md` body for forbidden mirror-eligibility
  tokens (`mercenary`, `<!-- ws:full-only:`, `<!-- ws:wsflow-only:`,
  `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
  `lead-skill-authoring`) before running the mirror regen, to avoid a loud
  eligibility-guard failure.
- Manual re-read of the blocked-progress paragraph against the Goal-run
  posture hard-gate list (`#L17-25`) to confirm the pause/conclusion
  distinction is unambiguous and no hard gate is weakened or reclassified.
- `ws/spec_index.verify` after the spec edit, to confirm no duplicate anchors
  and a clean index.

## Escalations

- None.
