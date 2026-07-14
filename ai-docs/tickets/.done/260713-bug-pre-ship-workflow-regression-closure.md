---
title: "Close pre-ship regressions in low-ceremony and proportional implementation routing"
sage-review-design: completed
spec:
  - 260505-proceed-routing-pipeline
  - 260505-implementation-workflow-skills
  - 260625-session-state-tools
  - 260505-ticket-document-system
related-mental-model:
  - workflow-skills
  - mcp-runtime
  - prompt-bundle
  - documentation-system
sage-review-completeness: completed
completed: 2026-07-13
---

# Close pre-ship regressions in low-ceremony and proportional implementation routing

## Background

Pre-merge regression review found that the low-ceremony and proportional-workflow
changes are internally coherent at the resolver level but leave six important
execution gaps across real Git observation, inline fact gathering, ticketless
delegated planning, single-review dispatch, documentation ownership, and the
ticketless decision boundary. A minor Result-authoring mismatch also omits the
behavioral delta required by the canonical ticket convention.

These gaps must be closed before the branch is merged or shipped. The fixes must
preserve the accepted direction: low ceremony remains explicit and safety-gated;
bounded work may remain ticketless regardless of file count or public surface;
delegated implementation and survey planning remain available; MCP-owned todo
instructions remain authoritative; and normal discuss-to-ticket workflows remain
unchanged.

## Decisions

- Current-branch eligibility must reject an absent or Git unborn marker
  (`(initial)`) as `start_commit`; a named branch without a real commit cannot
  receive the no-merge completion path.
- Ticket targets keep scope facts frozen from the ticket before source reading.
  Inline targets may derive scope facts from the caller request, loaded context,
  focused source inspection, and command output before the single
  `enter.implement` call. Unsupported facts remain `unknown` and fall back
  conservatively.
- Reuse the existing `plan-populator-survey` for ticketless delegated work.
  Add an explicit target-kind/inline-contract render path: ticket mode reads the
  ticket and selected phase; inline mode treats the caller-provided accepted
  scope, constraints, non-goals, and verification boundary as authority and must
  not read a placeholder ticket path.
- Ticketless delegated review uses the generated plan plus inline contract as
  authority. Reviewer guidance must support a plan-only inline frame instead of
  requiring a ticket path.
- Automatic `single` review means the existing delegate-grade `reviewer`
  wrapper, backed by the shared `code-reviewer` base, covers correctness, fit,
  and test. The generated todo must name that wrapper and the matching generic
  review frame so no partition selection or reviewer metadata is improvised.
- The generated doc-pre-pass todo exclusively owns the mental-model dispatch
  threshold. The always-rendered lead playbook may invoke `lead-update-spec` but
  must not restate a broader updater condition.
- `needs-ticket=no` covers one bounded reviewable slice whose scope and
  verification can be captured by the eventual implementation commit and any
  relevant existing spec. `needs-ticket=yes` covers multiple independently
  reviewable phases or a need for durable pre-implementation traceability beyond
  those artifacts.
- New Result text records the behavioral delta as well as deviations,
  verification evidence, unresolved findings, and deferred follow-up without
  restating the phase plan or spec.

Rejected alternatives:

- Do not narrow ticketless work back to single-file/direct-edit changes; that
  would undo the accepted proportional routing boundary.
- Do not pass `n/a` as a fake ticket authority and rely on delegate conversation
  context; inline authority must be explicit and self-contained.
- Do not add a second inline-only survey playbook when the existing survey can
  support both authority modes with a small render contract.
- Do not render the flat `code-reviewer` base directly; it intentionally lacks
  delegate role/tier metadata and is reused through reviewer wrappers.
- Do not preserve a one-partition label in `single`; the generic reviewer is the
  intended full-scope single-review contract.

## Constraints

- Preserve all existing low-ceremony raw safety predicates and override
  isolation.
- Preserve standard `create`, `rename`, `continue`, and `stop` branch behavior.
- Preserve explicit review overrides and two-or-more automatic partition output.
- Keep canonical `agents-plugin/rsrc` authoritative; regenerate manifests and the
  byte-identical `agents-plugin-wsflow/rsrc` mirror.
- Update existing specs and mental models only where the corrected executable
  contract differs from their current text.

## Phases

### Phase 1: Close reviewed routing and execution regressions

Implement the decisions above as one pre-ship correction slice. Add an
end-to-end `enter.implement` test using an initialized but uncommitted temporary
repository, plus focused tests for inline/ticket survey rendering, generic single
review dispatch, authoritative doc-pre-pass wording, exhaustive ticketless
judgment text, and behavioral-delta Result guidance.

Verification:

- `go test ./internal/mcp ./internal/wsdoc ./internal/wsrsrc -count=1`
- `go test ./... -count=1`
- `python3 -m unittest discover agents-plugin/tests`
- `python3 -m unittest discover agents-plugin-wsflow/tests`
- `scripts/smoke-ws-mcp.sh ..`
- `claude plugin validate agents-plugin`
- `claude plugin validate agents-plugin-wsflow`
- canonical/wsflow byte identity and `git diff --check`

### Result (f3f9b900) - 2026-07-13

- **Behavioral delta:** current-branch low-ceremony routing now requires a real
  start commit; delegated planners receive explicit ticket or inline authority;
  automatic single review uses the delegate-grade generic reviewer; and the
  generated MCP runbook exclusively owns proportional documentation guidance.
  Ticketless routing and Result guidance now cover the accepted recovery and
  behavioral-delta boundaries exhaustively.
- **Deviation:** review showed that rendering the flat `code-reviewer` base
  directly would omit delegate role, tier, and child-session metadata. The
  implementation therefore reuses the existing `reviewer` wrapper and includes
  the shared base through it.
- **Verification:** `go test ./... -count=1`; both plugin Python unittest
  suites (43 and 9 tests); both `claude plugin validate` calls; canonical/wsflow
  byte identity; and `git diff --check` passed. Partitioned correctness, fit,
  and test reviews are clean after the review-route correction.
- **Unresolved findings:** none.
- **Deferred follow-up:** `260713-bug-tickets-move-error-mutates-frontmatter`
  separately tracks the ticket-move error-path mutation observed while preparing
  this fix.
