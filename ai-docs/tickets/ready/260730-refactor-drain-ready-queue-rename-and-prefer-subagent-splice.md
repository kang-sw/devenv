---
title: "Rename lead-goal-step to lead-drain-ready-queue and splice lead-prefer-subagent at build time"
sage-review-design: completed
spec:
  - 260723-lead-goal-step-rename-reposition
  - 260703-drain-ready-queue-skill
  - 260707-drain-goal-branch-staging
  - 260723-goal-step-ticket-curation-authority
  - 260723-goal-step-blocked-progress-conclusion
  - 260725-goal-step-in-progress-ticket-affinity
related-mental-model:
  - workflow-skills
related:
  260723-feat-goal-step-rename-and-goal-loop-completion: the rename this reverses; its "Mechanism facts (verified)" section records a Stop-hook fact that is wrong, and that fact is what its name choice rested on
  260703-chore-prefer-subagent-verify-discussion-inline-mirror: established both the inline-SKILL.md decision this preserves and the substitution-mirrored generation mechanism this extends
  260730-refactor-retire-goal-fan-out-step-and-session-note: sibling; must land first so the fan-out transclusion hook is gone before this reshapes the same skill surface
  260725-research-goal-loop-restart-starved-by-background-delegation: owns the background-dispatch starvation caveat this ticket deliberately does not encode in skill prose
  260722-feat-goal-run-autonomy-posture: adjacent open work on the same skill body; coordinate sequencing
sage-review-completeness: completed
---

# Rename lead-goal-step to lead-drain-ready-queue and splice lead-prefer-subagent at build time

## Background

### The corrected Stop-hook mechanism

`260723-feat-goal-step-rename-and-goal-loop-completion` recorded this under a
heading reading **"Mechanism facts (verified, load-bearing for the design)"**:

> the **continue-vs-stop decision is AI judgment over the skill body prose** …
> **the skill name does not drive loop behavior** (the hook reads the body, not
> the name).

**This is wrong**, per user observation of live goal runs. The actual mechanism:

- The Stop-hook judges from the **skill name plus the transcript up to the
  stop** — not from the skill body.
- When compaction discards the body, the name is what survives into the
  judgment. The body cannot be relied on as the hook's input at all.
- The judging model is **less capable than the main agent** and does not
  reliably recognize termination conditions.

Every naming conclusion in `260723` was derived from the retracted fact, so the
conclusions do not survive it. Concretely, `260723` chose `step` on the
reasoning that the name is behaviorally inert; under the corrected mechanism
`step` is actively harmful, because a weak judge seeing `/lead-goal-step` and a
transcript in which one step visibly completed has every reason to classify the
run as over. Symmetrically, `260723` rejected `drain` for reading as "a terminal
single action" — an objection that only bites if the name is inert prose. When
the name is the judge's primary input, an intrinsic completion test ("is the
queue empty?") is the property you want most.

### Name and body have different readers

The deeper error in `260723` was treating the name as an expression of the
skill's identity, which conflated two audiences:

- **The body's reader is the main agent.** It needs the full goal-run posture,
  the terminal states, branch staging, curation authority. `260723`'s
  repositioning of the body is correct at this layer and is **not** reverted
  here.
- **The name's reader is a weak judge with no body.** It needs exactly two
  things: a signal that the work repeats, and a termination test it can resolve
  by lookup rather than inference.

Separating these resolves the apparent conflict: the body keeps its goal-run
identity while the name stops carrying it.

### What already landed as prose

The fixed-line terminal contract from the same discussion is **already in the
skill body** (commit on `main`, 2026-07-30): every turn ends with one of two
verbatim lines, with disjoint vocabularies and a prohibition on trailing
wrap-up text. That work is done; this ticket only renames the skill the lines
refer to, and must update the two line templates to the new name in lockstep.

### Why build-time splice, not serve-time transclusion

`lead-prefer-subagent` is invoked alongside this skill on essentially every
goal run, so the standing directive currently has to name it explicitly. Folding
it in should not duplicate its prose. Two mechanisms were considered:

- **Serve-time** (`printPlaybook` transclusion, as the fan-out overlay used).
  Rejected: the hooks live in `printPlaybook`, so the skill would have to become
  a `playbook.print` shim. That reintroduces exactly the failure `260703`
  removed — "structurally prone to being skipped by the model, especially for
  reminder-style content the model may believe it already knows" — on the
  highest-frequency, most reminder-shaped skill in the loop. It also exposes the
  hottest skill path to the open runtime bug
  `260726-bug-playbook-render-serves-stale-prompt-text`, which silently serves
  old text and cannot be caught by a test.
- **Build-time splice** (chosen). The skill keeps its inline `SKILL.md` body, so
  nothing about `{#260703-drain-ready-queue-skill}`'s inline decision is
  reversed — it is reinforced. Generated-artifact staleness is caught
  deterministically by a guard test, following the existing
  `TestWsflowRsrcMirrorUpToDate` / `WS_REGEN_MANIFEST` precedent.

Note that the existing build-time mechanism, `GenerateWsflowSkillBody`
(`agents-plugin-tool/internal/wsrsrc/skills_mirror.go`), is **namespace
substitution, not composition** — it mirrors one skill's ws copy to its wsflow
copy. Composition is new work.

**Eligibility verified.** `agents-plugin/skills/lead-prefer-subagent/SKILL.md`
contains no `disqualifyingTokens` entry — no `mercenary`, no `<!-- ws:full-only:`
or `<!-- ws:wsflow-only:` marker, no `ws.`, and in fact no `ws:`/`ws/` token at
all. The `<!-- ws:override:PreferSubagentInvocationGuidance -->` marker that
`260703` describes was resolved during that ticket and is not in the shipped
body. A spliced result therefore passes `guardSubstitutionEligible`.

## Decisions

**Name: `lead-drain-ready-queue`.** It states the process (`drain`) and the
object whose exhaustion is the termination test (`ready-queue`), so the weak
judge's stop decision reduces to a single lookup. Autocomplete removes typing
cost, but length is not free in the other direction: the terminal **set** is
volatile — the blocked-progress terminal was added only days before this
ticket — and a name is the most expensive place to store a volatile fact (Go
constants, manifest hashes, spec anchors, mental-model, wsflow mirror, tests,
saved user directives, muscle memory). So the name encodes the invariant
process shape only. Names like
`lead-drain-ready-queue-until-empty-or-blocked` are rejected: they become lies
when a terminal is added, and a lying name misleads the judge worse than a
silent one.

**Rejected: `lead-drain-protocol`** (the original proposal). `protocol` is a
category noun that supplies no termination test — a judge asked "is the
protocol done?" has nothing to resolve against.

**Splice `lead-prefer-subagent` at build time, verbatim, with no local
modification.** An earlier draft of this design added a drain-local
"dispatch synchronously, end the turn clean" constraint to mitigate the `260725`
starvation. Dropped: the background-agent/Stop-hook relationship is being
actively resolved at the harness layer, and encoding a workaround for it in a
permanent skill contract ossifies it. Verbatim splice is not a regression — the
body already points at `lead-prefer-subagent` today, so this changes a pointer
into a guaranteed load and leaves the starvation exposure exactly where it is.
The caveat stays in `260725` with a removal trigger.

**Splice before namespace substitution.** The wsflow copy must be generated from
the already-spliced ws source. The reverse order leaks ws-namespace text into
the wsflow package.

**Reuse the existing `<playbook>` boundary as the splice delimiter.** The
spliced region is wrapped exactly as `wrapRenderedPlaybookForConcatenation`
already wraps it at serve time
(`agents-plugin-tool/internal/mcp/playbook_tools.go:863-871`):
`<playbook name="lead-prefer-subagent" title="Prefer Subagent">` … `</playbook>`.
Three reasons: idempotent regeneration needs a paired delimiter to locate and
replace the region, and this is already one; hook #1 delivers this same body
under this same boundary, so a second boundary shape for identical content would
be gratuitous; and the tag carries no `ws:`/`ws/` token, as does the
`lead-prefer-subagent` body itself, so the composed result passes
`guardSubstitutionEligible` and the wsflow copy receives a byte-identical block.

Do **not** use an `<!-- ws:… -->` comment marker for the region.
`disqualifyingTokens` in `skills_mirror.go` hard-fails `<!-- ws:full-only:` and
`<!-- ws:wsflow-only:`; a new `ws:`-prefixed marker token would sit directly
adjacent to that guard.

The wrapper helper currently lives in `internal/mcp`. Move it to
`internal/wsrsrc` and have `mcp` call it there — the dependency already runs
that direction (`mcp` → `wsrsrc.LoadSkillBody`).

**Splice at the end of the target's `## Posture` section, not at the bottom of
the file.** An earlier draft of this ticket said "the very bottom" while
justifying it by protecting the `## Ending the turn` section — self-contradictory,
since that section *is* the last one in
`agents-plugin/skills/lead-goal-step/SKILL.md`. Bottom-append is concretely
wrong, not just cosmetically: `lead-prefer-subagent`'s body ends with "Require
this exact return format: `Outcome: …`; … `Commit: <hash> or none`", so appending
there puts a competing output-format contract immediately after the rule that
says "Write nothing after that line."

The spliced block therefore lands at the end of `## Posture`, anchored before the
`## Select` heading — which is also where it belongs semantically, directly after
the paragraph that already points at `lead-prefer-subagent`. The terminal-line
contract stays the last thing in the file.

Mechanically this is anchored insertion, not append: on first generation the
region is inserted before a named heading; on every later generation the region
is located by its `<playbook>`/`</playbook>` delimiter pair and replaced in
place, so regeneration stays idempotent and the anchor is only consulted when no
region exists yet.

**Splice the frontmatter-stripped body.** "Verbatim, with no local modification"
means the body as `wsrsrc.LoadSkillBody` returns it — that loader strips
frontmatter (`internal/wsrsrc/loader.go:64-72`) and is what the serve-time caller
already feeds the wrapper. Splicing the raw file would embed
`lead-prefer-subagent`'s `---\nname: …\ndescription: …\n---` block mid-markdown.

**Accept the duplicate body; do not suppress hook #1.** Once spliced,
`lead-prefer-subagent` appears twice in any session that also renders
`lead-workflow-manual` with `workflow.prefer_subagent` on. Accepted, on two
measurements: the builtin default for that item is `off`
(`agents-plugin-tool/internal/mcp/server.go:462-464`), so the duplication occurs
only for users who explicitly opted in; and the body is 414 words / ~2.7 KB,
roughly 700 tokens. Suppressing hook #1 for drain callers, or migrating it to
build time, both stay out of scope below.

**Size trajectory, recorded so it is not a surprise at review.** The 2026-07-30
compression pass took the target body from 1378 words to 786; this splice adds
414 back, landing near 1200. The skill ends up close to its pre-compression bulk
— what changed is that the remaining words are rules rather than changelog
residue and design-reviewer rationale, and that the body is now sectioned.

## Phases

### Phase 1: Build-time skill-body splice mechanism

Add composition to `wsrsrc` alongside the existing substitution mirror: a
declarative mapping from a target skill to the skill bodies appended to it, a
generator behind a regen env var, and an up-to-date guard test that fails loudly
when the target's committed `SKILL.md` does not match the freshly composed
output. Insert the region at the end of `## Posture`, wrapped in the `<playbook>`
boundary, per Decisions — source the appended body through
`wsrsrc.LoadSkillBody` so frontmatter is stripped. Run
`guardSubstitutionEligible` on the composed result, and order composition before
namespace substitution so both package mirrors derive from one source.

Move `wrapRenderedPlaybookForConcatenation` from `internal/mcp` to
`internal/wsrsrc` and repoint its existing serve-time caller, so both the
build-time and serve-time paths emit one boundary shape from one implementation.

Deliberately narrow: one mapping only. It registers against the target's name as
it exists in this phase — `lead-prefer-subagent → lead-goal-step` — because the
rename lands in Phase 2; Phase 2 updates the mapping key along with every other
surface. Also update the curated skill list and its prose contract in
`ai-docs/ref/wsflow-mirroring.md`, which states that adding a skill requires
updating both that document and the generator's list in
`internal/wsrsrc/skills_mirror_test.go`.

Verification: guard test fails on a hand-edited target and passes after regen;
regenerating twice is a no-op (idempotent region replacement located by the
delimiter pair, not repeated insertion); `go test ./...` green; the wsflow copy
contains no ws-namespace token and its spliced block is byte-identical to the ws
copy's; `## Ending the turn` is still the target's last section afterward.

### Phase 2: Rename across every surface

Rename the skill directory and `name:` frontmatter in both package mirrors, and
propagate.

**The `step` framing is part of the rename, not incidental.** Three surfaces
carry it without containing the token `lead-goal-step`, so a stem-only sweep
cannot reach them:

- The `description:` frontmatter, currently "Advance a goal-pursuit run by one
  step, picking the next `ready/` ticket … Stop if ready/ is empty." This is the
  surface a harness skill listing pairs with the name. Shipping
  `lead-drain-ready-queue` described as "by one step" reinstalls the exact
  "one step completed, run is over" inference the rename exists to remove, at the
  name layer. Rewrite it to the name-layer contract: repeated draining of
  `ready/`, terminating when nothing advanceable remains.
- The `# Goal Step` H1.
- The opening bold line "**Goal-pursuit step; `ready/` is the sole progress
  gate.**" Keep the sole-progress-gate clause — `260723` mandates it as the
  body's first line — and drop only the step framing. This line is asserted
  verbatim by `TestPlaybookPrintGoalFanOutStepAppendsGoalStepUnconditionally`
  (removed by the sibling ticket) and must stay consistent with whatever guard
  survives.

Remaining surfaces: the two fixed terminal-line templates in the body,
`agents-plugin/skills/manifest.json` (regenerate, never hand-edit), the wsflow
mirror, the Phase 1 splice mapping key, `playbook_tools.go` constants if any
survive the sibling ticket, `ai-docs/spec/workflow-skills.md`,
`ai-docs/mental-model/workflow-skills.md`, `ai-docs/ref/wsflow-mirroring.md`
(four live non-history references), `internal/wsrsrc/skills_mirror_test.go`, and
the python skill-bundle tests.

Sweep for `lead-goal-step`, `Goal Step`, `goal-pursuit step`, and `one step` —
not the stem alone. Every surviving `ai-docs/` hit must be justified as history
rather than assumed to be.

Verification: `go build ./...`, `go test ./...`, both python test files;
`ws/playbook.print` and the harness skill listing both resolve the new name; the
rendered skill listing entry contains no step framing.

### Phase 3: Documentation closeout

Rewrite `{#260723-lead-goal-step-rename-reposition}` so it carries the two-layer
contract explicitly — the name layer (weak-judge reader, termination test
intrinsic to the name) and the body layer (main-agent reader, goal-run posture)
— rather than being renamed in place. The corrected Stop-hook mechanism must
land in the spec prose, not only in this ticket, or the retracted "verified"
claim in `260723` stays discoverable and gets re-litigated a third time. Add
the fixed terminal-line contract to the spec if the prose commit did not already
cover it.

Verification: `grep -rn "does not drive loop behavior\|reads the body, not the
name"` over `ai-docs/spec/` and `ai-docs/mental-model/` returns nothing; the
rewritten anchor names both the name layer (weak-judge reader, termination test
intrinsic to the name) and the body layer (main-agent reader, goal-run posture)
as distinct contracts; the two verbatim terminal lines appear in the spec;
`ws/spec_index.verify` passes. Removing the retracted claim is the phase's
acceptance criterion — an added note that leaves the old claim standing does not
complete this phase.

## Spec Impact

The `spec:` stems above are confirmed existing anchors in
`ai-docs/spec/workflow-skills.md` and cover every phase's caller-visible
behavior except one: the build-time splice mechanism of Phase 1 has no existing
anchor. That needs a new statement in `ai-docs/spec/plugin-runtime.md`, beside
the substitution-mirror generation it extends, describing composition as a
generated-artifact contract — target skill, appended bodies, `<playbook>`
boundary, and the guard test that makes committed `SKILL.md` files
regeneration-verifiable.

Anchor-by-anchor:

Rewrites `{#260723-lead-goal-step-rename-reposition}` in
`ai-docs/spec/workflow-skills.md` to separate the name-layer and body-layer
contracts and to record the corrected Stop-hook mechanism. Renames the skill in
`{#260703-drain-ready-queue-skill}`, `{#260707-drain-goal-branch-staging}`,
`{#260723-goal-step-ticket-curation-authority}`,
`{#260723-goal-step-blocked-progress-conclusion}`, and
`{#260725-goal-step-in-progress-ticket-affinity}` without changing their
substance. Adds a spec statement for the build-time splice mechanism.

## Out of Scope

- The fixed terminal-line prose itself — already committed to `main` on
  2026-07-30, before this ticket. Only the skill name inside those two line
  templates changes here.
- Any drain-local delegation constraint. See the `260725` caveat.
- Migrating hook #1 (`lead-workflow-manual` ← `lead-prefer-subagent`) from
  serve-time to the new build-time mechanism, and suppressing it for drain
  callers. Both are responses to the duplicate body accepted in Decisions;
  capture as `idea/` once Phase 1 proves the mechanism. Revisit if the duplicate
  turns out to cost more than the 700 tokens measured here, or if the
  `workflow.prefer_subagent` builtin default ever moves off `off`.
