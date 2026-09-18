---
title: Lightweight embeddable TICKETS_WORKFLOW.md for no-plugin downstream projects
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d34ee15d96b77940
sage-review-completeness-reviewed: d34ee15d96b77940
---

# Lightweight embeddable TICKETS_WORKFLOW.md for no-plugin downstream projects

## Background

Large downstream projects run by a plain "naive" Claude Code (no ws plugin, no
`ws/*` MCP tools — only native Read/Write/Edit/Bash/Explore) lose and forget
decisions: what the user actually decided, what is still an open question, and
why an approach was chosen all leak out of the project over time. The full ws
workflow (discuss / run / ticket status flow / review / ship) is too much
surface to hand such a project.

Deliver one lightweight, self-contained document, `ai-docs/ref/TICKETS_WORKFLOW.md`,
that distills only the highest-leverage core: a ticket system as durable
decision memory, plus the commit `## AI Context` convention. The canonical
master copy lives in this repo under `ai-docs/ref/`; the maintainer hand-copies
it into a downstream project's root, where it is embedded from `CLAUDE.md` via
`@TICKETS_WORKFLOW.md`. This is copy-once distribution: no automated propagation,
the downstream owns its copy.

Framing to preserve in the doc: the ticket decision-ledger and the commit
`## AI Context` are the **same memory expressed at two moments** — when a
decision forms it lands in the ticket, when it becomes code it lands in the
commit. The doc must read as one coherent memory mechanism, not three loose
rules.

## Decisions

All decisions below are user-confirmed in the originating discuss session.

1. **Structure = strict subset of the full ticket model (not a parallel design).**
   Same `ai-docs/tickets/` root. Folders limited to `todo/` and `.done/`, with
   `.dropped/` allowed for abandoned tickets. Status is directory-based only —
   never duplicated in frontmatter. Filename stem format `YYMMDD-<category>-<slug>`
   where `<category>` is one of the full system's six fixed values —
   `bug`, `feat`, `refactor`, `chore`, `research`, `epic` — so every stem
   validates as a strict subset. In the lightweight flow the category is a
   naming/scan aid only (no per-category process); day-to-day work is mostly
   `feat`/`bug`/`refactor`/`chore`.
   - *Why:* a strict subset gives zero-migration, bidirectional compatibility.
     If wsflow/ws is installed later, the reduced folders are read natively and
     the upgrade is purely additive (idea/ready/.dropped simply start being
     used); a downstream `todo/` ticket is a valid full-system `todo/` ticket.
   - *Rejected:* a parallel/custom layout or a new status folder for open
     questions — that divergence is exactly what breaks round-trip compat.
   - *Rejected:* a free-form category word — it reads lighter (no palette to
     teach), but off-enum stems would fail the full system's category enum once
     its currently-unenforced validation lands, breaking the subset guarantee;
     the six-word enum costs one line and preserves compat.

2. **Embed the ticket template + title/naming convention directly in the doc.**
   This is what buys the compatibility above "for free"; the naive agent has no
   `tickets.template`/`convention.read` to consult, so the shape must be inline.

3. **Autonomous, memory-like operation.** Record without asking; perform ticket
   transitions (`todo` <-> `.done`) autonomously. Capture is **search-first**:
   cheap `grep`/`glob` over `ai-docs/tickets/` first, then append to a matching
   ticket or open a new one. Escalate to the native Explore agent **only when
   the match is ambiguous**.
   - *Rejected:* ask-and-block on open questions (defeats memory-like autonomy);
     spawn Explore on every capture (cost sink in a naive project).

4. **Explore model must be named explicitly** whenever the doc tells the agent
   to use Explore: haiku by default, sonnet for ambiguous/synthesis reads.
   Never leave it on inherit.
   - *Why:* recent native Explore inherits the caller's model, so an Opus session
     would run Explore on Opus for work haiku handles — expensive. Matches the
     project's own Explore guidance.

5. **In-ticket decision ledger, 3 canonical headers** (reuse the full system's
   exact header names so upgrade maps 1:1): `Confirmed Decisions`,
   `Open Questions`, `Rejected Alternatives`.
   - **Invariant:** only user-stated decisions go under `Confirmed Decisions`.
     The agent must never silently promote its own inference or proposal into
     that section.
   - Behavior rule: instead of blocking to ask, log an open question under
     `Open Questions` and proceed, surfacing it as one line in that turn's reply.
   - *Why 3 of the full 5-header Outcome Ledger:* Verified Findings / Proposals
     belong to research tickets; the three kept are the decision-integrity core.

6. **Commit `## AI Context` is unconditional** — every commit gets it. Body is
   proportional to the change: trivial = one line; substantive = rationale /
   rejected alternatives / user directives. Do not restate the diff — capture
   the *why*. Reference the related ticket stem.
   - *Why unconditional:* a "when needed" judgment is the first thing an
     autonomous agent erodes under time pressure, and it goes dark exactly when
     a project is large enough to need the memory. Removing the judgment keeps
     the habit intact; a one-line filler on a trivial commit is an acceptable
     cost.

7. **Pull exactly two behavioral lines from `AGENTS.md`** (`## Response
   Discipline`), reworded host-neutral: "Evidence before claims" (read
   verification output before stating success/done) and "No performative
   agreement" (restate the requirement, verify, then act or push back). These
   are the behavioral pair that directly protects decision integrity.
   - *Rejected pulls:* `## Code Standards` (downstream owns its own code
     standards; including them is scope creep), `## Approval Protocol` (already
     covered by the record-don't-ask autonomy rule and presumes skills/flows),
     Project Orientation / Architecture Rules / Documentation System (devenv-
     specific; shipped-surface boundary forbids even naming them).

8. **Produced artifacts are authored in English by default** — ticket bodies and
   commit `## AI Context`. Human-facing UI strings are exempt. Phrase the rule so
   a downstream team could swap the record language by changing that one line,
   but the default is English.

## Constraints

- **Audience has zero MCP tools.** The doc must be self-contained prose. It must
  not reference any `ws/*` MCP hook (`convention.read`, `tickets.*`, etc.) as a
  live tool, and must instruct only native Read/Write/Edit/Bash/Explore + git.
- **Shipped-surface boundary applies.** Read `ai-docs/manuals/shipped-surface-boundary.md`
  before authoring. The produced doc must not name any devenv-specific artifact:
  no plugin/package names (`agents-plugin*`, `wsflow`, `ws`), no `install.sh`, no
  real ticket stem/epic/hash, no devenv migration vocabulary. When it needs
  project-specific input, it uses a downstream-generic hook (a declared
  `CLAUDE.md`/`AGENTS.md` section), never a devenv value.
- **Behavioral-doc authoring quality.** Read `ai-docs/manuals/skill-authoring.md`
  before authoring — the doc is behavioral instruction for a current-generation
  model; apply its rule-phrasing standard (no over-negation, testable rules).
- **Compatibility invariant.** The on-disk shape (folder names, stem format
  including the six-value category enum, ledger header names) stays identical to
  the full system; only process ceremony (phases, ready-gate, sage review,
  epics) is dropped.

## Prior Art

- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` — the closest existing
  artifact (the "plugin-less maintenance" guide bootstrap emits as
  `ai-docs/WORKFLOW.md`). Same register/voice target (its `## Manual Fallback`
  section), but full 5-state model and assumes bootstrap ran. Use as a voice
  reference; this doc is its no-plugin, reduced-status cousin.
- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md` — canonical
  source for the status model, stem naming, and the 5-header Outcome Ledger the
  3 kept headers are drawn from.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md` — the already
  downstream-safe wording of the `## AI Context` / commit block; prefer its
  phrasing over devenv's own `AGENTS.md` copy as the distillation source.
- `AGENTS.md` `## Response Discipline` — source of the two behavioral lines (item 7).

## Prior Decisions

- d273782d (2026-09-13, commit): "Category is bug: the write-guardrails enforce stem/status-dir/frontmatter but not the documented category enum, so tooling behavior deviates from the convention it is meant to uphold." — bearing: contradiction-candidate
- 260910-bug-route-ticket-absolute-path-symlink-alias (2026-09-10, commit): "Reject only the retired category at creation to preserve unrelated existing unknown-category behavior; reuse the template rejection to keep the accepted list consistent." — bearing: constrains
- 260726-refactor-retire-spec-planned-marker-mechanism (2026-07-28, commit): "These four files are genuinely hand-maintained, unlike agents-plugin-wsflow/rsrc which is generated; no test reads either WORKFLOW.md." — bearing: constrains
- 260825-refactor-ws-wsflow-bootstrap-artifact-convergence (2026-08-25, commit): "Recorded the worktree/clone note-layer bullet (WORKFLOW.md L33-36) as a drop (plugin-local runtime state, no downstream on-disk path) for a downstream artifact." — bearing: constrains
- 260503-epic-agents-plugin-skill-porting (2026-05-03, commit): "Shared skill text must not read paths like claude-plugin/infra/spec-conventions.md because downstream projects will not contain this repository's plugin source tree." — bearing: supports
- 260524-workset-ticket-conventions (2026-05-24, commit): "User requested a durable category whose single-word attention separates from epic while grouping unrelated tickets for goal/session work." — bearing: constrains
- 260510-claude-plugin-retirement-freeze (2026-05-11, commit): "The ws-mcp doctor check no longer requires claude-plugin, matching downstream and post-retirement repository layout." — bearing: supports
- 260506-feat-bootstrap-workflow-guide (2026-05-06, commit): "Captured the agreed hybrid boundary: plugin/runtime semantics remain canonical while downstream projects receive a pinned plugin-less maintenance guide." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | ai-docs/ref/TICKETS_WORKFLOW.md (new file to author) |
| scope.surface | public-interface | copy-once artifact embedded by a downstream `CLAUDE.md` via `@TICKETS_WORKFLOW.md` (ticket Background); shipped-surface-boundary.md governs it as downstream-facing text |
| scope.new_public_symbol | no | none — prose document, no code symbol |
| scope.new_type_contract | no | none — no type or signature introduced |
| scope.test_surface | none | agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py TEXT_TREES (lines 43-51) does not include ai-docs/ref/; Phase 1's verification boundary is manual re-read plus grep, no test added |
| complexity.reuse_points | confirmed | agents-plugin/skills/lead-bootstrap/WORKFLOW.md (voice), agents-plugin/skills/lead-bootstrap/AGENTS.template.md (AI Context wording), agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md (ledger headers, status model), AGENTS.md `## Response Discipline` (the two pulled lines) — all confirmed present with matching content |
| complexity.side_effect_risk | low | one new markdown file under ai-docs/ref/; no code, runtime, or plugin package path touched |
| risk.correctness | moderate | the doc must reproduce ticket-conventions.md's exact header names, status folders, and stem shape for the stated bidirectional-compatibility invariant to hold, with no automated check enforcing that shape |
| risk.fit | moderate | must hold WORKFLOW.md/AGENTS.template.md register while deliberately diverging from the full 5-header ledger and the category enum (see contradiction-candidate above), with no test bounding that divergence |
| risk.test | moderate | no existing or added automated coverage for ai-docs/ref/ content; verification is the worker's own manual re-read and grep |
| risk.security_or_contract | low | advisory documentation only; no code parses or enforces the doc's shape, so drift degrades gracefully rather than breaking a live contract |

## Phases

### Phase 1: Author `ai-docs/ref/TICKETS_WORKFLOW.md`

Write the single self-contained document realizing Decisions 1–8 under the
Constraints. Intended structure (worker may refine ordering): a short "why this
exists" framing (the two-moment memory mechanism); the ticket system as a
reduced subset with an inline ticket template and the `YYMMDD-<category>-<slug>` (six-value enum)
naming convention; the autonomous search-first capture + transition rules
(including the explicit-Explore-model rule); the 3-header in-ticket decision
ledger with the `Confirmed Decisions` user-only invariant and the log-don't-block
rule for `Open Questions`; the unconditional proportional commit `## AI Context`
rule with ticket-stem reference; and the two behavioral lines. English output;
human-facing UI strings exempt.

Deferred scope: no changes to the full ticket system, bootstrap, or any plugin
package; no automated distribution mechanism (copy-once by hand).

Verification boundary: (1) re-read the produced doc as a lead in a project that
has never heard of devenv — it must stand alone; (2) grep the produced doc for
banned tokens (`agents-plugin`, `wsflow`, `install.sh`, `ws/`, `convention.read`,
`tickets.`, real stems) and confirm none appear as live references; (3) confirm
the folder/stem/header shape is a strict subset of `ticket-conventions.md` so a
later plugin install is additive.
