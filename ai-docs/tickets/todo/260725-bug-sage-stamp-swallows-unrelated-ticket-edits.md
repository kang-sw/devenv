---
title: "tickets.sage_stamp commits unrelated ticket edits under a stub review message"
related:
  260723-feat-ticket-write-verify-commit-gate: introduced sage_stamp as the lead-only replacement for sage_record, including its canonical-title commit
sage-review-design: recommended
sage-review-completeness: recommended
---

# tickets.sage_stamp commits unrelated ticket edits under a stub review message

## Background

Observed while applying sage design-review findings to
`260725-feat-ws-cli-mcp-fallback-surface`. The sequence was: run the design
reviewer, apply its findings to the ticket body in the working tree, then call
`tickets.sage_stamp(stage: "design", verdicts: [...])` to record the verdicts.

`sage_stamp` is documented as writing "the resolved frontmatter posture" and
committing "with the canonical title". In practice it committed the **entire
working-tree state** of the ticket file: `b9c72975` is titled `docs(sage): mark
design review completed` with the AI Context body `- design review passed`, but
carries 135 insertions and 44 deletions — the full set of review-driven body
edits, none of which the message describes. `27b3b599` did the same for the
completeness stage.

The caller-visible consequence is lost decision rationale. Those 135 lines
encoded seven review findings and the reasoning for each fix, and the commit that
carries them says only "design review passed". `AGENTS.md` treats commit
`## AI Context` bodies as a project memory tier, so the rationale is not
recoverable from history for exactly the change that most needed it. A later
`git log --grep=<stem>` reader sees a review-posture commit and has no signal
that substantive content landed inside it.

## Why it commits at all

Not an independent design goal — a verbatim port of playbook prose. The commit
boundary comment (`wsdoc/tickets_sage.go:19-23`) states the intent was to return
commit title/paths/ai_context "for the MCP dispatch layer to actually commit via
`wsgit.NewClient().Commit(...)`", keeping output "byte-for-byte identical to
today's `git.commit`-produced commits". `SageGateResult`'s field comment names
the source outright: the **legacy prose** persisted `skipped` and "committed a
small standalone commit". So `260723`'s mutation-tool collapse absorbed the old
lead-facing instruction "record the posture and make a small commit" into the
tool, and the commit came along with it. The goal was a faithful port, not a
decision that this tool should own a commit.

The swallow is likewise not sweeping logic. `CommitPaths` is a single element,
the ticket file path, and `wsgit.Commit` stages by path — while the posture field
and the ticket body live in the same file. Path-granular staging therefore
*cannot* separate them.

That also rules out one otherwise-attractive fix: "commit only the frontmatter
field" is not cheaply implementable, since committing part of a file requires
index surgery.

## Decision: remove the commit

`sage_stamp` is the only member of the ticket mutation family that commits:

| Tool | Documented contract |
|---|---|
| `tickets.create_empty` | does not stage or commit |
| `tickets.move` | stages atomically; **does not commit** |
| `tickets.close` | stages the change set atomically; **does not commit** |
| `tickets.sage_stamp` | writes posture, **commits with the canonical title** |

The family contract is "stage, and let the caller commit". `sage_stamp` will be
brought in line: write the posture, stage, return — no commit. This removes the
swallow at its cause rather than labelling it, since the caller's own
`ws/git.commit` then carries the posture change and the review-response edits
together under real rationale. The cost is that `lead-write-ticket` must commit
explicitly after stamping, which is a step the playbook already has.

Rejected alternatives:

- **Refuse to run when the ticket file has other uncommitted edits.** Makes the
  existing commit honest but leaves `sage_stamp` inconsistent with its siblings,
  and forces an extra commit round-trip mid-review.
- **Keep the sweep, add an optional `ai_context` argument.** Labels the symptom
  while a single commit still mixes a posture transition with unrelated content,
  and keeps the family inconsistency.
- **Commit only the frontmatter field.** Not achievable without index surgery,
  per the section above.

## Frequency

Not a one-off. A single ticketing session hit it four times — `b9c72975`,
`27b3b599`, `025bab2c`, `8a4d81ce` — because the shape recurs by construction:
`lead-write-ticket` runs a reviewer, has the lead fix findings in-place, then
calls `sage_stamp`, so review-driven body edits are *always* uncommitted at stamp
time. The rate is one swallowed commit per review stage per ticket, and the
swallowed content is specifically the review-response rationale, which is the
most valuable rationale the flow produces.

Compounding it, `260725-idea-ws-git-commit-rename-and-payload-rejections` means
the caller often cannot pre-commit those edits through `ws/git.commit` anyway
when a status transition is also staged.

## Spec Impact

- Target spec areas: `mcp-tools.md` for the `tickets.sage_stamp` contract (drops
  the commit, gains the stage-only guarantee and loses the returned commit ref),
  and `workflow-skills.md` for `lead-write-ticket`'s Sage Review Gate step, which
  gains an explicit commit after stamping.
- Expected caller-visible change: `sage_stamp` no longer creates a commit and no
  longer returns a commit hash; the posture write is staged for the caller to
  commit. Callers that relied on the returned hash must read it from their own
  `git.commit` instead.
- Contract-first spec: no. This aligns one tool with the contract its siblings
  already document, so the spec is corrected at closeout rather than needing an
  upfront design.

## Phases

### Phase 1: Make sage_stamp stage-only, and commit explicitly in the playbook

Remove the `wsgit.NewClient().Commit(...)` call from the `tickets.sage_stamp`
dispatch (`internal/mcp/server.go:1398`) and stage the posture write instead,
matching how `tickets.move` and `tickets.close` stage atomically without
committing. Drop the commit ref from the tool's response and from its schema
description. `SageRecordResult`'s `CommitTitle`/`CommitPaths`/`AIContext` become
either unused or repurposed as staging inputs — decide which and remove whatever
is left dead rather than leaving an unreferenced commit-shaped struct behind.

Leave `SageGateResult`'s ask-decline commit path alone unless the same audit
shows it has the identical problem; it writes `skipped` with no reviewer output
in flight, so it does not obviously swallow anything. If it does share the
defect, fold it in and say so; if not, record why it was left.

Then update `lead-write-ticket`'s Sage Review Gate step to commit after stamping,
so the posture change and any review-response edits land in one commit with real
`## AI Context`. The playbook must make clear that this is the commit carrying
review-response rationale, since that content is exactly what was being lost.

**Depends on `260725-idea-ws-git-commit-rename-and-payload-rejections` Phase 1.**
Once `sage_stamp` stops committing, the caller commits the posture change through
`ws/git.commit` — which today fails whenever a status transition is also staged.
Landing this first would replace a bad commit message with an uncommittable
state.

Verification: `sage_stamp` produces no commit and leaves the posture write staged;
a subsequent `ws/git.commit` by the caller carries both the posture change and any
concurrent body edits under caller-supplied `## AI Context`; the tool's response
and schema no longer advertise a commit ref; running the full
`lead-write-ticket` flow end-to-end on a ticket with review-driven edits produces
a commit whose message describes those edits, which is the regression this ticket
exists to prevent.
