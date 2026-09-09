# WORKFLOW.md Sections

Two new sections for the bootstrap workflow guide (`WORKFLOW.md` in both
packages, and this repository's `ai-docs/WORKFLOW.md`). They replace
`## Specs` and `## Mental Models`, whose bullets in `## ai-docs/ Layout`
(`spec/`, `mental-model.md`, `mental-model/`) are dropped in the same edit;
`## Index Health` and the `_index.md` dissolution guidance stay until their
own retirement. Insert after `## Tickets`.

```markdown
## Behavioral Contract

Tests are the behavioral contract. A behavior that matters has a test; a
change that alters behavior changes a test in the same change, and review
treats a behavior change without a test change as a finding. There is no
separate specification document to keep in step with the code, and nothing
checks whether a project's tests are strong enough to carry this role: that
is the project's own property, and the workflow assumes it rather than
enforcing it.

Prescriptive knowledge — preferred libraries, patterns, boundaries, domain
constraints — is a human decision that code cannot reconstruct, so it is
written down: one-line universal rules inline in `AGENTS.md`; longer or
path-scoped rules as one manual each under `ai-docs/manuals/`, declared in
`AGENTS.md` under `## Workflow` -> `### Implementation Conventions` with the
paths they cover. A rule a test can check becomes a test; a trap tied to one
site becomes a code comment at that site; a fact about an external system
goes in `ai-docs/ref/`. Descriptive knowledge — what the code does and why —
is reconstructed from the code, the tests, and commit `## AI Context` bodies
when needed, and is not maintained as a document. Manuals carry no
per-commit update obligation; drift is fixed on contact and by review.

## Execution Model

The ticket is the plan. Its decisions are settled when it is written: facts
are checked by a cheap-tier populator that writes them into the ticket, and
design is reviewed by a heavy-tier reviewer before the ticket enters
`ready/`. Execution consumes those decisions instead of re-making them.

One worker executes one whole ticket: it routes, edits, verifies, runs
independent review, commits, records the phase result, and closes the
ticket. It reads the ticket, `AGENTS.md`, the declared conventions and cited
manuals, the tests, the code, and git history; it receives no summary of any
of them. The lead converses with the user, manages the ticket inventory,
spawns workers, and handles what they escalate; it edits no source.

The worker stops only for: a merge into a parent branch (user approval; the
veto point for everything the worker decided alone); an unresolved decision
the ticket does not settle; a ticket decision contradicted by code reality;
an irreversible action in the Approval Protocol's always-ask category; a
Critical review finding surviving three review rounds. Every other decision
is recorded in the commit's `## AI Context` and the ticket's `### Result`
and listed in the worker's terminal report for veto. The lead resolves a
contradicted decision itself when it can (design review over the worker's
proposed resolution), and elevates a surviving Critical finding to a
higher-tier worker; the user sees low-reversibility decisions and exhausted
lead attempts.

Without the workflow tooling, the same model holds: read the ticket and the
declared manuals, work on a branch, keep the stop list, and record decisions
in the commit body and the ticket result.
```
