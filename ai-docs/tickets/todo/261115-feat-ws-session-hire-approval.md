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

## Open Decision Queue

Last ID: (956)

(938) [open] [policy] Expose the request tool in every lead's tool list and refuse unflagged callers in the server
- Context: a per-session tool list would have to change when the owner flips the flag, so clients need a refresh or reconnect.
- Alternative a: always listed for lead-scope sessions; the server refuses with "hiring is not enabled for this session" (T0, static tool list).
- Alternative b: listed only for flagged roots; a flag change needs a tools/list refresh (T1 reconnect-recoverable).
> Recommend a: the tool list stays static and T0; the server check is the authority either way.

(939) [open] [policy] Roster scope: the caller's hired sessions plus the other top-level leads of the same project
- Context: 782 assigns work to "the least-loaded peer", so the roster must include peers, not only hired children.
- Alternative a: only sessions grouped under the caller (hired children).
- Alternative b: hired children plus top-level leads of the same project (same canonical git root).
- Alternative c: every session on the daemon, across projects.
> Recommend b: it is what 782 needs without exposing other projects' sessions.

(940) [open] [policy] Any lead-scope session may call the roster, not only flagged roots
- Alternative a: any lead-scope session (read-only data, 786 grants no control).
- Alternative b: flagged roots only.
> Recommend a: 782 balancing is useful to every lead, and the rows are already visible in the dashboard.

(941) [open] [policy] Tool identifiers
- Alternative a: the research examples, `daemon.request_hosted_session` and `daemon.list_children_sessions`.
- Alternative b: `session.request_hire` and `session.roster`, next to the existing `session.children`.
- Alternative c: `daemon.request_session` and `daemon.list_sessions`.
> Recommend b: `session.*` already holds the session tools, and "children" would wrongly suggest the roster lists only hired sessions, which conflicts with (939)b.

(942) [open] [policy] Pending requests survive a daemon restart and keep their 30-minute wall-clock expiry
- Alternative a: persist pending proposals in the daemon state directory (a new file, T0); after a restart they reappear, and any that expired meanwhile send `expired` mail.
- Alternative b: in memory only; a restart drops them, and the requester learns nothing.
> Recommend a: under b a request can vanish silently, which breaks the mailbox outcome promise of 750/767.

(943) [open] [policy] Eligibility is checked again at approval; a request whose requester lost the flag cannot be approved and is closed with a `withdrawn` mail, while a closed requester does not block approval
- Context: the research leaves requester closure and eligibility loss open; 768=c says hired sessions are independent of the requester's lifetime.
- Alternative a: as stated (flag off -> auto-withdraw with mail; requester closed -> still approvable, mail waits on its reply-id).
- Alternative b: any requester change (closed or flag off) withdraws the request.
- Alternative c: no recheck; the owner decides regardless.
> Recommend a: the flag is the owner's own revocation signal, while closing a session is routine and 768=c keeps the hire independent of it.

(944) [open] [policy] No limit on active hired sessions
- Context: 767 removed only the pending cap; the research leaves an active limit open.
- Alternative a: no limit; every hire is owner-approved anyway.
- Alternative b: a per-requester cap (for example 3), refused at request time.
> Recommend a: owner approval already gates each creation.

(945) [open] [policy] `recovery_context` is limited to 16 KiB, stored with the proposal, shown collapsed in the modal with a character count, and echoed in full in outcome mail
- Alternative a: 16 KiB.
- Alternative b: 4 KiB.
- Alternative c: no limit.
> Recommend a: enough for a real compaction-recovery brief while keeping mail and proposal files small.

(946) [open] [policy] Drop `tier`: the request carries optional harness, model and effort, and missing values fall back to the new-session popover defaults
- Alternative a: no tier; raw optional harness/model/effort.
- Alternative b: a tier word (medium/large/xlarge) resolved through the configured agent bindings into model and effort before the modal opens.
> Recommend a: the owner adjusts the values in the modal anyway, and a tier mapping adds a second resolution path to explain.

(947) [open] [policy] The hired session's root defaults to the requester's canonical git root and is editable in the modal; the request cannot name a root
- Alternative a: as stated.
- Alternative b: the request may name an absolute root (the owner sees and can edit it).
> Recommend a: smaller request surface, and the owner can still change it.

(948) [open] [policy] A new request does not interrupt: it adds a rail chip and a toast, and the modal opens on click; several pending requests queue
- Alternative a: as stated (mockup shows the chip and the pending ghost row).
- Alternative b: the modal pops up immediately when a request arrives.
> Recommend a: requests are nonblocking, so a modal stealing focus mid-typing is the wrong cost.

(949) [open] [policy] The hired session's turn reports do not go to the requester; only the creation, rejection and expiry outcomes do
- Context: Runner `ParentRef` (`agents-plugin-tool/internal/wsagent/record.go` ParentRef) routes a child's turn reports to its parent as mail. 775 grants only "approval/creation notices", so reusing ParentRef would add per-turn mail.
- Alternative a: outcome mail only; the requester talks to the hired session through normal mailbox.
- Alternative b: route every turn report to the requester, like a spawned child.
> Recommend a: it matches 775's "notices", and b would make the requester a de facto supervisor.

(950) [open] [technical] Display grouping uses a new optional record field (for example `hired_by`, the requester's reply-id) instead of Runner `ParentRef` or MCP `sessionEntry.parent` - those carry report routing and authority lineage, which 775 excludes.

(951) [open] [technical] The approval, rejection and flag-toggle endpoints sit behind the existing trusted gate in `wsagent/http.go` (`opts.Trusted`, local plus own-origin) rather than a new auth path - this is 765's boundary, and one gate keeps one failure mode.

(952) [open] [technical] Outcome mail goes through the existing reply-id system mail (`wsmailbox/replyid.go`) rather than a new notification channel - it already wakes a requester before any child exists.

(953) [open] [technical] Approval calls the existing hosted-lead creation (`Runner.CreateLead`) with the owner's final values, so the result is the same as a rail-created session rather than a separate creation path - this is 768=c. Uncertainty: CreateLead mints a parent-less lead key, so the hired session is itself a top-level lead in MCP terms; 775 needs exactly that, but it must not get the hiring flag (752).

(954) [open] [technical] The CLI-lead flag (769=a) is stored as an optional field next to the dashboard's existing per-session record for that lead rather than in the lead-key registry - the registry is authority state, and a flag change must not touch it. Uncertainty: exact store to be grounded by fact population.

(955) [open] [technical] The phases split server lifecycle and tools (Phase 1), dashboard UI (Phase 2), and shipped guidance (Phase 3) rather than one phase - Phase 2 edits `dashboard.js`/`dashboard.css`, which are under a split freeze, so Phase 1 can land first.

(956) [open] [policy] Accept the approval modal mockup as the visual reference
- Mockup: `ai-docs/ref/brand/mockup/session-hire/approval-modal.png` (source `approval-modal.html`). It shows the requester row with a hire chip, the pending ghost row, the modal (reason, collapsed recovery context, harness/label/model/effort/root, the authority note, the reject reason, Reject, Approve & create) and the root toggle in session settings, including its disabled state for grouped sessions.
- Alternative a: accept as is.
- Alternative b: accept with changes the owner names.
> Recommend a once (947) and (948) are answered, since the mockup assumes their recommendations.

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
  record (954). It is set only through a trusted dashboard endpoint (951). It
  cannot be set while the session is grouped under a requester (775), and
  never for a hired session (752).
- Request tool (name per (941)): takes `reason` and `recovery_context`
  (both required; size per (945)), an optional label, and optional
  harness/model/effort per (946). It refuses callers without the flag
  (750/751), hired sessions (752), and non-lead scopes. It returns at once
  with a request id and a line saying the outcome will arrive by mail (750).
- Proposal lifecycle: pending -> approved | rejected | expired (30 min, 767)
  | withdrawn (943). Persistence per (942). Re-check eligibility at approval
  (943).
- Approval creates the session through hosted-lead creation (953) with the
  owner's final values and root (947), records the display grouping (950),
  and sends the outcome mail (952). Every outcome mail echoes
  `recovery_context` (766/770). Turn reports per (949).
- Roster tool (name per (941)): read-only rows with exactly the 786 fields;
  scope per (939), callers per (940).
- Verification: Go tests for flag gating (unflagged, hired, worker and clerk
  callers refused; the model cannot set the flag), each lifecycle edge with
  its mail (including the recovery_context echo and expiry on a fake clock),
  restart behavior per (942), the recheck per (943), the requester's inability
  to stop or close the hired session, the roster's fields and scope, and
  compatibility (existing tool list and records unchanged apart from the
  additions). Use focused `-run` selections; a full `./internal/mcp` run needs
  `-timeout 20m` or more.

### Phase 2: Dashboard approval modal, rail cues, and the root toggle

- Build to the accepted mockup (956):
  `ai-docs/ref/brand/mockup/session-hire/approval-modal.png`.
- The root toggle goes in hosted-session settings and on CLI lead rail rows
  (769=a). Disabled, with a reason, for a grouped session (775).
- Rail: a hire chip on the requester and a pending row while requests are open.
  The hired session is grouped under the requester (775). Arrival behavior per
  (948).
- Modal: requester identity, reason, collapsed recovery context with a
  character count, harness/model/effort controls reusing the new-session
  popover's controls, label and root (947), expiry countdown, reject reason,
  Reject, and Approve & create. Approve and reject go through the trusted
  endpoints (951).
- Strings in all 11 locales.
- Verification: node harness tests for the modal (prefill, owner edits win,
  reject needs no reason or carries it, countdown, disabled toggle), the
  focused wsdashboard Go tests, wsi18n, and a side-by-side comparison of a
  headless capture against the mockup PNG.

### Phase 3: Shipped lead guidance

- Lead-facing guidance in the workflow manual or playbook: when to hire, that
  the request is nonblocking and the outcome arrives by mail, that
  `recovery_context` must be enough to resume after compaction, the roster,
  782 allocation, and keeping strengths in the agenda (788=d). Downstream-
  neutral wording, mirrored to wsflow and Pi through the existing pipelines.
- Verification: manifest/mirror tests and product-mode playbook checks.
