---
title: Pin the ticket-index playbook contracts and fix two stale directions
blocked-by: 260924-bug-ticket-index-read-timeout-below-ssh-roundtrip
related:
  260924-feat-origin-ticket-ownership-index: its Phase 4 (00c0c0da) added the playbook contracts this ticket pins
  260924-bug-ticket-index-read-timeout-below-ssh-roundtrip: sibling fix ticket from the same review sweep; both edit mcp/ticket_index.go (holderText, indexMockText/indexMockResponse near indexVerbResponse), so blocked-by orders it after that ticket lands to keep the merge clean
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e53bb292ac699034
sage-review-completeness-reviewed: e53bb292ac699034
---

# Pin the ticket-index playbook contracts and fix two stale directions

## Background

The develop review sweep 0a361491..4a2f4721 found that the shipped playbook
contracts added by 00c0c0da (ownership index Phase 4) have no test pins. Its
only test edit renumbered a step. No test mentions `tickets.acquire`,
`unleased_or_mine`, or `index_init` in any playbook, so dropping the selector
filter or the lead-run acquire step would dispatch leased tickets silently with
every test still passing. The ODQ work in the same range shows the expected
pattern: `TestPlaybookPrintGoldenLeadTicket` pins its contract phrases.

The sweep also found two directions made stale or misplaced:

- **F5.** `lead-run.md` step 4 ends with "A success that carries a `warning:`
  line proceeds; relay that line to the user verbatim." That tells the lead
  what to do with post-call tool output, which `ai-docs/manuals/skill-authoring.md`
  (Layer Model, Layer 2) assigns to the tool's own output text.
- **F6.** `AGENTS.md` (Ticket System) says "Move status with `git mv` when
  possible." With the ticket index live on this repository's origin, a `git mv`
  skips the lease guard and index registration that `tickets.move` and
  `tickets.close` perform. `ticket-conventions` already treats `git mv` as the
  fallback only.

## Decisions

- **Pins are phrase pins in rendered playbooks**, following
  `TestPlaybookPrintGoldenLeadTicket` (render through `printPlaybook` for both
  harnesses, assert the contract phrases). Pinned contracts:
  - lead-run: `tickets.acquire` runs before the worker render and spawn
    (order checked); a refusal or error ends the turn with no dispatch;
    `dangerously_override_lease_status` only on the user's explicit
    instruction.
  - ticket-worker and ticket-worker-elevated: record the impl branch through
    `tickets.acquire`.
  - lead-scope-worktree: acquire on scope assignment.
  - ticket-selector and ticket-batch-selector: list `ready/` with
    `unleased_or_mine: true`, and the batch selector's second unfiltered
    listing to tell an empty `ready/` from an all-leased one.
  - lead-bootstrap: `tickets.index_init(check: true)` and its four handled
    states `uninitialized`, `initialized`, `no-origin`, `unreachable`.
  Both harnesses means `claude` and `codex`, as in the existing golden test;
  comparisons are whitespace-normalized like the existing pins (0a1d5b5b).
  Rejected: full-body golden files, which break on every unrelated wording
  edit.
- **F5: the relay direction moves into the tool output.** Delete the sentence
  "A success that carries a `warning:` line proceeds; relay that line to the
  user verbatim." from lead-run step 4 in all three mirrors
  (`agents-plugin/rsrc`, `agents-plugin-wsflow/rsrc`, `agents-plugin-pi/rsrc`)
  and regenerate the manifests. `tickets.acquire`'s text output carries the
  direction itself: on a lease acquire, each `warning:` line ends with the
  agreed suffix ` (tell the user this line verbatim)`. The suffix is limited
  to the lease path of `tickets.acquire`, keyed on the call carrying no impl
  branch (the entry's `ImplBranch` is empty), not on the result status: an
  impl-branch acquire can also return `pending` (offline) or `takeover` with a
  warning. The worker's impl-branch record and `tickets.release` share the
  formatter (`indexVerbResponse`) but keep their output unchanged, since a
  worker has no user to relay to and release carries no such contract. Only
  the call's own `warning:` lines get the suffix; replayed-entry lines from a
  pending-log flush (including a replayed takeover, which the sibling ticket
  prints as a `ticket-index:` report) stay `report:` lines without it. JSON
  output is unchanged. Tests pin the suffix on the lease path, and its absence
  on the impl-record path (online and offline) and on release. The relay itself stays as 633697f1
  confirmed ("lead-run proceeds and relays the warning"); only where the
  instruction lives moves. Rejected: suffixing every verb and caller.
  Keep "A refusal or an error ends the turn" and the override sentence: they
  are the stop list and judgment, not post-call restatement.
- **F6: `AGENTS.md` Ticket System line** becomes: move status with
  `tickets.move` / `tickets.close`; `git mv` only when those tools are
  unavailable or error. This is this repository's own file, not shipped text.

## Constraints

- Shipped text changes follow `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md`; the three rsrc trees stay
  byte-identical and manifests stay consistent (package tests check both).
- No other playbook wording changes; the pins describe current text.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 633697f1 (2026-09-24, commit): "User confirmed all six recommendations: cached-index acquire with offline warning, pending log for every index write, ... lead-run proceeds and relays the warning, ..." — bearing: constrains
- 57c930bb (2026-09-24, commit): "tickets.index_init is lead-only because it pushes to origin with the user's credentials. acquire and release stay open to workers for the impl record." — bearing: constrains
- 0897bb57 (2026-09-24, commit): "User directive: a visible 'index absent' acquire response would confuse downstream projects that never opted in; legacy projects must see nothing new" — bearing: constrains
- 00c0c0da (2026-09-24, commit): "lead-run acquires after the goal-branch checkout (Spawn step 3), not before it. The lease's track must be the branch the worker's impl branch merges into" — bearing: supports
- 00c0c0da (2026-09-24, commit): "The selectors always pass unleased_or_mine, including on not-assigned runs, because not-assigned relaxes only the assignee steer while a lease is hard ownership." — bearing: supports
- 00c0c0da (2026-09-24, commit): "The bootstrap index check sits in On: invoke as step 7, after the mode handler and the _index.md health check, and is skipped only on refuse." — bearing: supports
- 260915-refactor-lead-run-playbook-diet-drop-assignment-note (2026-09-15, commit 0a1d5b5b): "Pins were kept only for structural fragments (section names, terminal lines, tier table, ...) and all use whitespace-normalized comparison" — bearing: constrains
- 260620-feat-ws-ticket-status-transition-tools (2026-06-22, commit afff970f): "Kept `git mv` as a named fallback rather than removing it; non-MCP playbook execution contexts still need an actionable native transition reference." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md plus its agents-plugin-wsflow/rsrc and agents-plugin-pi/rsrc mirrors, the three rsrc/manifest.json files, agents-plugin-tool/internal/mcp/ticket_index.go indexVerbResponse, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-tool/internal/mcp/ticket_index_test.go, AGENTS.md |
| scope.surface | public-interface | caller-visible text output of the tickets.acquire MCP tool and shipped lead-run playbook text in three packages |
| scope.new_public_symbol | no | none; new test functions and a text suffix only |
| scope.new_type_contract | no | none; JSON output unchanged per the F5 decision, text warning lines gain a suffix |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go already holds TestPlaybookPrintGoldenLeadTicket and a lead-bootstrap render at L2655; ticket_index_test.go L511 asserts a warning line; mirror guards in agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go |
| complexity.reuse_points | confirmed | printPlaybook at agents-plugin-tool/internal/mcp/playbook_tools.go#L950 and the TestPlaybookPrintGoldenLeadTicket pattern at playbook_tools_test.go#L2220 were read; indexVerbResponse at ticket_index.go#L164-L186 emits the warning lines |
| complexity.side_effect_risk | moderate | indexVerbResponse is shared by tickets.acquire and tickets.release via handleIndexVerb, and workers call acquire too, so an unscoped suffix changes release output and reaches callers with no user |
| risk.correctness | low | every contract phrase to pin was found in the current rendered sources, and the lead-run step 4 sentence to delete exists byte-identically in all three mirrors |
| risk.fit | moderate | the tell-the-user suffix lands in output also read by ticket-worker and lead-scope-worktree acquire calls, and the sibling ticket 260924-bug-ticket-index-read-timeout-below-ssh-roundtrip edits the same ticket_index.go |
| risk.test | moderate | pins must be non-vacuous including an order check of acquire before render and spawn across two harness renders, plus a new text-form test for the warning suffix |
| risk.security_or_contract | moderate | changes the text contract of a shipped MCP tool and removes a shipped playbook direction whose relay behavior was a confirmed decision in 633697f1 |

## Phases

### Phase 1: Contract pins, acquire warning direction, and AGENTS.md wording

- Add the pins above; each fails when its contract phrase or order is removed.
  Show non-vacuity once per pin by temporarily deleting the phrase (or moving
  the acquire step after the spawn step) and observing the failure; record
  this in the Result.
- F5 playbook deletion plus the acquire text-output change and its test.
- F6 `AGENTS.md` edit.
- Mirror the lead-run edit into `agents-plugin-wsflow/rsrc` and
  `agents-plugin-pi/rsrc`, then regenerate manifests with
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  (from `agents-plugin-tool`), following `wsflow-mirroring.md` for any other
  regeneration it names.
- Verification: `go test ./...` in `agents-plugin-tool` with an isolated
  `HOME` (this includes the wsrsrc mirror and manifest guards, among them
  `pi_mirror_test.go`), `python3 -m unittest discover -s agents-plugin/tests`
  and `-s agents-plugin-wsflow/tests`.
