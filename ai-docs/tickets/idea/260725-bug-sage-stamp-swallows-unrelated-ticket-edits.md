---
title: "tickets.sage_stamp commits unrelated ticket edits under a stub review message"
related:
  260723-feat-ticket-write-verify-commit-gate: introduced sage_stamp as the lead-only replacement for sage_record, including its canonical-title commit
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

Reasonable caller expectation is that a tool described as stamping a posture
field either (a) commits only the frontmatter field it owns, leaving unrelated
working-tree edits for the caller's own `git.commit`, or (b) accepts caller-
supplied context when it is going to sweep up whatever else is staged.

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

## Open questions

- Should `sage_stamp` restrict its commit to the frontmatter posture change, or
  refuse to run when the ticket file has other uncommitted edits?
- If sweeping is intentional (it does keep the tree clean between stages), should
  the tool take an optional `ai_context` so the caller can describe what else is
  riding along?
- Does `lead-write-ticket`'s step ordering assume review findings are applied
  *after* the stamp? If so the playbook and the tool disagree, since applying
  findings before stamping is the natural reading of "fix confirmed gaps
  in-place".
