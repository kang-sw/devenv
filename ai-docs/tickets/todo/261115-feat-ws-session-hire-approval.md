---
title: "Owner-approved nonblocking hosted-session hiring"
related:
  261114-research-ws-session-hire-approval: "source research; its Confirmed Decisions are this ticket's contract"
  261114-research-ws-clerk-session-role: "clerk decisions 720-722 stay unchanged"
---

# Owner-approved nonblocking hosted-session hiring

## Background

A lead should be able to ask for another daemon-hosted session without the
owner creating it by hand. The lead sends a nonblocking request. The
dashboard shows the owner an approval modal prefilled from that request; the
owner adjusts the launch settings and approves or rejects. The outcome
reaches the requester by mailbox. An approved session is treated exactly as
if the owner had created it from the rail. It is shown under the requester,
and the requester receives its notices, but the requester gains no control
over it.

The research ticket `261114-research-ws-session-hire-approval` settled the
contract. The owner moved it to implementation on 2026-11-15 ("구현으로
올리죠", relayed by MAIN). Only that research's Outcome Ledger
`Confirmed Decisions` are carried here as contract. Its Proposals and Open
Questions are not contract; each one this ticket needs is in the Open
Decision Queue below.

## Decisions

Carried from the research's Confirmed Decisions (IDs are the owner's):

- **750 / 751 - hiring root flag.** Add a root-session option to the session
  settings UI. Only the owner turns it on, through the UI, and the server
  verifies the call. The model can never set it. Only a session with the flag
  on may request hiring; the server refuses every other caller. A flagged root
  cannot become another session's child.
- **750 - nonblocking.** A request returns at once. Accepting a request does
  not mean a session exists. The creation, rejection or expiry outcome arrives
  later by mail.
- **750 - approval modal.** The modal names the requester. The owner can
  adjust harness, model and effort with the existing new-session controls,
  then approve, or reject with a reason.
- **752 - no chaining.** A hired session is never a root, holds no hiring
  entitlement, and cannot hire.
- **753 - compatibility.** Target **T0 additive**: new MCP tools and optional
  fields only. No clerk encoding, no change to existing identity or gate
  semantics, no shared-store rewrite, and no T2.
- **765 - trust boundary.** Approval, rejection and flag changes use the
  existing trusted dashboard control gate (loopback plus exact Origin). It does
  not prove human provenance; that stays deferred.
- **766 / 770 - request text.** `reason` is the owner-facing explanation.
  `recovery_context` is longer text that lets the requester recover, after
  compaction, why it asked. Approve, reject and expiry mail echo
  `recovery_context`. Requested harness, model and effort only prefill the
  modal; the owner's adjusted values are final.
- **767 - expiry.** No cap on pending requests. A pending request expires
  after 30 minutes, and the requester gets an `expired` mail.
- **768=c / 775=a - authority.** An approved session has exactly the
  authority of an owner-UI-created session. The relationship is display
  grouping under the requester in the rail plus routing of
  approval/creation notices to the requester. It is not MCP authority
  lineage. The requester cannot stop or close it. Its root flag cannot be
  turned on while it is grouped under a requester. It does not stop when the
  requester closes. Rejected: authority lineage (775 option b), requester
  stop/close, requester-bounded authority.
- **769=a - CLI leads.** The owner can also enroll a non-hosted CLI lead,
  through the same toggle on that session's rail row.
- **786=ok - roster.** Add a read-only roster tool. Each row holds exactly:
  mailbox address, harness, model, label, idle/busy state, running subagent
  count and last activity. T0 additive. It grants no control or write
  authority.
- **788=d - strengths.** No strengths field anywhere. The owner tells the
  lead verbally, and the lead keeps it in its own agenda.
- **782=a - allocation.** The lead gives new work to the least-loaded peer and
  rebalances when a peer has more than three unstarted items. This is lead
  behavior guidance; it confers no stop/close powers.

Settled 2026-11-15 by MAIN (board IDs; MAIN relay 05:43Z):

- **(952) technical, MAIN - tool exposure.** The request tool is listed in
  every lead-scope session's tool list. The server refuses callers without the
  hiring flag ("hiring is not enabled for this session"). This keeps the tool
  list static and T0. Rejected: listing it only for flagged roots, which needs
  a tools/list refresh on every flag change (T1).
- **(952) technical, MAIN - roster callers.** Any lead-scope session may call
  the roster. Its rows are read-only and already visible in the dashboard.
  Rejected: flagged roots only.
- **(952) technical, MAIN - names.** `session.request_hire` and
  `session.roster`, next to the existing `session.children`. Rejected:
  `daemon.request_hosted_session` / `daemon.list_children_sessions` (the
  research examples; "children" misdescribes a roster that includes peers),
  `daemon.request_session` / `daemon.list_sessions`.
- **(952) technical, MAIN - restart.** Pending proposals persist in a new
  file in the daemon state directory (T0). After a restart they reappear and
  keep their 30-minute wall-clock expiry. Any that expired while the daemon
  was down send their `expired` mail. Rejected: memory only, which drops
  requests silently and breaks the 750/767 mail promise.
- **(952) technical, MAIN - recheck.** Eligibility is checked again at
  approval. If the requester has lost the hiring flag, the request cannot be
  approved; it is closed with a `withdrawn` mail. A closed requester does not
  block approval: the outcome mail waits on its reply-id (768=c). Rejected:
  withdrawing on any requester change; no recheck.
- **(952) technical, MAIN - recovery_context size.** At most 16 KiB. It is
  stored with the proposal, shown collapsed in the modal with a character
  count, and echoed in full in every outcome mail. Rejected: 4 KiB;
  unlimited.
- **(952) technical, MAIN - no tier.** The request carries optional
  `harness`, `model` and `effort`. Missing values fall back to the
  new-session popover's defaults. Rejected: a tier word resolved through
  agent bindings, which would be a second resolution path to explain.
- **(952) technical, MAIN - grouping field.** Display grouping uses a new
  optional record field holding the requester's reply-id (for example
  `hired_by`). Runner `ParentRef` (`agents-plugin-tool/internal/wsagent/record.go`)
  and MCP `sessionEntry.parent` are not reused: they carry turn-report routing
  and authority lineage, which 775 excludes.
- **(952) technical, MAIN - trusted gate.** The approve, reject and
  flag-toggle endpoints use the existing trusted gate in
  `agents-plugin-tool/internal/wsagent/http.go` (`opts.Trusted`: local plus
  own-origin), which is 765's boundary. Rejected: a new auth path.
- **(952) technical, MAIN - outcome mail.** Outcomes use the existing reply-id
  system mail (`agents-plugin-tool/internal/wsmailbox/replyid.go`), which
  already wakes a requester before any child exists. Rejected: a new
  notification channel.
- **(952) technical, MAIN - creation path.** Approval calls the existing
  hosted-lead creation (`Runner.CreateLead`) with the owner's final values,
  so the result matches a rail-created session (768=c). CreateLead mints a
  parent-less lead key, so the hired session is a top-level lead in MCP
  terms. 775 wants exactly that; the session still never gets the hiring
  flag (752). Rejected: a separate creation path.
- **(952) technical, MAIN - CLI flag storage.** The CLI-lead flag (769=a) is
  an optional field next to the dashboard's existing per-session record for
  that lead, not in the lead-key registry. The registry is authority state,
  and a flag change must not touch it. The exact store is grounded by fact
  population.
- **(952) technical, MAIN - phase split.** Phase 1 covers the server
  lifecycle and tools, Phase 2 the dashboard UI, Phase 3 shipped guidance.
  Phase 2 edits files under the split freeze, so Phase 1 can land first.
  Rejected: one phase.
- **(953) announced - no active limit.** There is no limit on active hired
  sessions; owner approval already gates each creation (767 removed only the
  pending cap). Rejected: a per-requester cap.
- **(953) announced - no turn reports.** The hired session's turn reports do
  not go to the requester. The requester gets only creation, rejection,
  expiry and withdrawal mail; otherwise the two talk through normal mailbox.
  This follows 775's "approval/creation notices". Rejected: routing every turn
  report to the requester like a spawned child, which would make it a
  supervisor.

## Open Decision Queue

Last ID: (953)

Local IDs from the first draft were re-minted on the board: (939) -> (948), (947) -> (949), (948) -> (950), (956) -> (951).

(948) [open] [policy] Roster scope: the caller's hired sessions plus the other top-level leads of the same project
- Context: 782 assigns work to "the least-loaded peer", so the roster must include peers, not only hired children.
- Alternative a: only sessions grouped under the caller (hired children).
- Alternative b: hired children plus top-level leads of the same project (same canonical git root).
- Alternative c: every session on the daemon, across projects.
> Recommend b: it is what 782 needs without exposing other projects' sessions.

(949) [open] [policy] The hired session's root defaults to the requester's canonical git root and is editable in the modal; the request cannot name a root
- Alternative a: as stated.
- Alternative b: the request may name an absolute root (the owner sees and can edit it).
> Recommend a: smaller request surface, and the owner can still change it.

(950) [open] [policy] A new request does not interrupt: it adds a rail chip and a toast, and the modal opens on click; several pending requests queue
- Alternative a: as stated (the mockup shows the chip and the pending ghost row).
- Alternative b: the modal pops up immediately when a request arrives.
> Recommend a: requests are nonblocking, so a modal stealing focus mid-typing is the wrong cost.

(951) [open] [policy] Accept the approval modal mockup as the visual reference
- Mockup: `ai-docs/ref/brand/mockup/session-hire/approval-modal.png` (source `approval-modal.html`). It shows the requester row with a hire chip, the pending ghost row, the modal (reason, collapsed recovery context, harness/label/model/effort/root, the authority note, the reject reason, Reject, Approve & create) and the root toggle in session settings, including its disabled state for grouped sessions.
- Alternative a: accept as is.
- Alternative b: accept with changes the owner names.
> Recommend a once (949) and (950) are answered, since the mockup assumes their recommendations.

## Constraints

- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for
  `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`). Tool
  descriptions, refusal text and playbook text must make sense in a
  downstream project.
- Convention: `ai-docs/manuals/ws-mcp.md` (declared for
  `agents-plugin-tool/internal/mcp/`).
- Convention: `ai-docs/manuals/release-compatibility.md` (declared for
  `agents-plugin-tool/internal/`). Every new tool, argument, record field
  and state file is T0 additive. Anything that turns out not to be T0 goes
  back to the lead before it lands.
- Convention: `ai-docs/manuals/skill-authoring.md` and
  `ai-docs/manuals/wsflow-mirroring.md` for any playbook or skill text
  (Phase 3).
- Clerk decisions 720-722 and existing worker delegation are unchanged.
- Dashboard strings go through the i18n tables for all 11 locales.
- `dashboard.js` and `dashboard.css` are under a split freeze until the split
  ticket lands. Phase 2 targets the post-split files.

## Phases

### Phase 1: Daemon proposal lifecycle, hiring flag, and the request and roster tools

- Hiring flag: an optional field on hosted session records and on the CLI-lead
  record (CLI flag storage). It is set only through a trusted dashboard
  endpoint (trusted gate). It cannot be set while the session is grouped under
  a requester (775), and never for a hired session (752).
- `session.request_hire`: takes `reason` and `recovery_context` (both
  required; `recovery_context` at most 16 KiB), an optional label, and
  optional `harness`/`model`/`effort` (no tier). The tool is listed for
  every lead-scope session. It refuses callers without the flag (750/751),
  hired sessions (752), and non-lead scopes. It returns at once with a request
  id and a line saying the outcome will arrive by mail (750).
- Proposal lifecycle: pending -> approved | rejected | expired (30 min,
  767) | withdrawn (recheck). Pending proposals persist across a daemon
  restart (restart). Eligibility is checked again at approval (recheck).
- Approval creates the session through `Runner.CreateLead` (creation path)
  with the owner's final values and root ((949)). It records the display
  grouping (grouping field) and sends the outcome by reply-id system mail
  (outcome mail). Every outcome mail echoes `recovery_context` (766/770). No
  turn reports go to the requester (no turn reports).
- `session.roster`: read-only rows with exactly the 786 fields. Scope per
  (948); any lead-scope caller (roster callers).
- Verification: Go tests for:
  - flag gating: unflagged, hired, worker and clerk callers are refused, and
    the model cannot set the flag;
  - each lifecycle edge with its mail, including the `recovery_context` echo
    and expiry on a fake clock;
  - restart persistence and expiry-while-down mail;
  - the approval recheck (flag lost -> `withdrawn`; requester closed ->
    still approvable);
  - the requester's inability to stop or close the hired session;
  - the roster's fields and scope;
  - compatibility: the existing tool list and records are unchanged apart
    from the additions.

  Use focused `-run` selections. A full `./internal/mcp` run needs
  `-timeout 30m`.

### Phase 2: Dashboard approval modal, rail cues, and the root toggle

- Build to the accepted mockup ((951)):
  `ai-docs/ref/brand/mockup/session-hire/approval-modal.png`.
- The root toggle goes in hosted-session settings and on CLI lead rail rows
  (769=a). It is disabled, with a reason, for a grouped session (775).
- Rail: a hire chip on the requester and a pending row while requests are
  open. The hired session is grouped under the requester (775). Arrival
  behavior per (950).
- Modal contents:
  - requester identity and reason;
  - recovery context, collapsed, with a character count;
  - harness/model/effort controls reusing the new-session popover's controls;
  - label and root ((949));
  - expiry countdown;
  - reject reason, Reject, and Approve & create.

  Approve and reject go through the trusted endpoints (trusted gate).
- Strings in all 11 locales.
- Verification:
  - node harness tests for the modal: prefill, owner edits win, reject with
    or without a reason, countdown, disabled toggle;
  - the focused wsdashboard Go tests and wsi18n;
  - a side-by-side comparison of a headless capture against the mockup PNG.

### Phase 3: Shipped lead guidance

- Lead-facing guidance in the workflow manual or playbook covers:
  - when to hire;
  - that the request is nonblocking and the outcome arrives by mail;
  - that `recovery_context` must be enough to resume after compaction;
  - the roster and 782 allocation;
  - keeping strengths in the agenda (788=d).

  Wording stays downstream-neutral and is mirrored to wsflow and Pi through
  the existing pipelines.
- Verification: manifest/mirror tests and product-mode playbook checks.
