---
title: "tickets.sage_stamp commits unrelated ticket edits under a stub review message"
related:
  260723-feat-ticket-write-verify-commit-gate: introduced sage_stamp as the lead-only replacement for sage_record, including its canonical-title commit
  260725-idea-ws-git-commit-rename-and-payload-rejections: blocking prerequisite — once sage_stamp stops committing, the caller commits the posture change through git.commit, which today fails whenever a status transition is staged
  260721-bug-lead-write-ticket-sage-ready-ordering: edits the same Sage Review Gate step, and its open question about retrying tickets.move after sage_record commits the posture is invalidated by this ticket's decision
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

`sage_stamp` will write the posture and return — no commit, and no staging
either. The caller's own `ws/git.commit` then carries the posture change together
with any review-response edits under real rationale, which removes the swallow at
its cause instead of labelling it.

**The governing criterion is the swallow, not family symmetry.** An earlier draft
justified this by calling `sage_stamp` "the only member of the ticket mutation
family that commits". That is false: `tickets.sage_gate` also commits, on the
ask-decline path (`internal/mcp/server.go:1363-1373`), and `mcp-tools.md`
documents it. The family picture is mixed rather than one-sided:

| Tool | Documented contract |
|---|---|
| `tickets.create_empty` | does not stage or commit |
| `tickets.move` | stages atomically; does not commit |
| `tickets.close` | stages the change set atomically; does not commit |
| `tickets.sage_gate` | persists posture and **commits** on ask-decline |
| `tickets.sage_stamp` | writes posture and **commits** with a canonical title |

What actually distinguishes `sage_stamp` is *when* it commits: uniquely among
these, it fires at the one moment the flow guarantees uncommitted body edits are
present. `sage_gate` runs before any reviewer output exists, so it swallows
nothing today. Family alignment is therefore supporting evidence, not the
argument — and Phase 1 must still decide `sage_gate` explicitly rather than
leaving one committer behind by omission.

Note also that `create_empty`, not `move`/`close`, is the correct analogue:
those two stage because they perform renames that exist only in the index, while
a frontmatter field write needs no staging at all.

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
`27b3b599`, `025bab2c`, `8a4d81ce` — one swallowed commit per review stage per
ticket, and the swallowed content is specifically the review-response rationale,
which is the most valuable rationale the flow produces.

**Correction to an earlier reading of why it recurs.** This was first written as
"recurs by construction: `lead-write-ticket` runs a reviewer, has the lead fix
findings in-place, then calls `sage_stamp`". Design review checked the playbook
and that middle step does not exist. `lead-write-ticket` section 6 is only
`sage_gate` -> **On: Reviewer Spawn** -> `sage_stamp`, and Reviewer Spawn step 3
is "Parse `verdict:` ... return it to `tickets.sage_stamp`" — nothing instructs
the lead to apply findings to the ticket body at all. (Section 4's "fix confirmed
gaps in-place" belongs to the intent-checklist step, before commit, not to the
sage stage.)

So the four occurrences came from a lead improvising into a documented gap, not
from following the documented flow. The swallow is real either way — the edits
existed and the message did not describe them — but the recurrence mechanism is
"the playbook has no home for review-response edits", which is a second defect
sitting underneath this one.

Compounding it, `260725-idea-ws-git-commit-rename-and-payload-rejections` means
the caller often cannot pre-commit those edits through `ws/git.commit` anyway
when a status transition is also staged.

## Spec Impact

- Target spec areas: `mcp-tools.md` anchor `{#260720-sage-gate-record-tools}`,
  which covers **both** sage tools, for the `tickets.sage_stamp` contract (drops
  the commit and the returned commit ref) and for whatever `sage_gate` decision
  Phase 1 records; and `workflow-skills.md` for `lead-write-ticket`'s Sage Review
  Gate step, which gains an explicit commit after stamping.
- While editing that anchor, correct an existing drift found during design
  review: it claims "A declined `ask` and a **config-fallback resolution** each
  persist the resolved posture and commit", but the config-fallback half is
  already untrue in code — `resolveConcretePosture` (`tickets_sage.go:162-171`)
  writes the field with no commit metadata, and `resolveStage` returns a bare
  `stageOutcome{action: "skip"}` for terminal postures.
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
dispatch (`internal/mcp/server.go:1398`). Do **not** stage the posture write in
its place: `wsgit.Commit` already stages its own `paths`
(`stagingCommandsForCommit`, `internal/wsgit/git.go:528`), so a pre-stage buys
the caller nothing, and it introduces a new failure mode — a staged ticket file
left behind by a lead that does not commit immediately makes the next
`ws/git.commit` on a different path set fail `validateCommitStatus`
(`git.go:615`) with "refusing to commit unrelated staged path", which cannot
happen today. Match `create_empty`: write, return, leave the tree alone.

Drop the commit ref from the tool's response and from its schema description, and
**update `next_instruction` in both verdict branches** (`server.go:2726` and the
`concern` branch above it). That string is the lead's actual control surface —
playbook step 6.2 says to call `sage_stamp` "and follow its returned
next_instruction" — so leaving it reading "recorded and committed ... proceed to
handoff" would have the tool override the edited playbook and silently defeat
this fix.

`SageRecordResult`'s `CommitTitle`/`CommitPaths`/`AIContext` become unused;
remove them rather than leaving an unreferenced commit-shaped struct behind.

Decide `tickets.sage_gate`'s ask-decline commit explicitly in the same change
rather than by omission. It does not swallow today (it runs before any reviewer
output exists), so the swallow criterion does not force it; the recommendation is
to align it anyway so one convention covers both sage tools, but if it is left
committing, record why in the ticket.

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
