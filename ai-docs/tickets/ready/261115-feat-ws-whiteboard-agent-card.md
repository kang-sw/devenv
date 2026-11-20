---
title: "whiteboard 에이전트 카드: 종속 hosted 세션과 공유 대화 셸"
blocked-by: 261115-feat-ws-worker-profile-session-spawn#phase1
related:
  261115-feat-ws-permission-profile: "prerequisite: Phase 2 supplies reusable permission controls and Claude/Pi launch admission"
  261115-feat-ws-worker-profile-session-spawn: "prerequisite: Phase 1 follows permission Phase 2 and supplies worker controls/restrictions; cards use the specific child-lead amendment below"
  261114-refactor-ws-common-session-interface: "owner 1072 permits the bounded T0 card authority exception now; Phase 2 later absorbs the controls without changing identity/input ownership"
  261114-epic-ws-common-session-foundation: "context only; no whole-epic dependency"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0695680de36ec29e
sage-review-completeness-reviewed: 0695680de36ec29e
---

# Whiteboard agent cards

## Background

Open a real subordinate hosted agent from the whiteboard toolbar and work with
it in a card, reusing the existing conversation and composer. The host lead
remains the authority and owns the whiteboard. Permission Phase 2 and worker
Phase 1 are runtime prerequisites, not satisfied merely by their ready documents.

Authority: owner (1023)=a, (1024)=c, permission (999)=b, MAIN's
2026-11-15 07:51:12 card-specific amendment, and owner (1072)=a / (1073)=a
relayed by MAIN at 08:34:49. The four technical settlements below were made
autonomously by the lead after fact population; they are not new owner-board IDs.

## Decisions

1. **Card-specific authority amendment, MAIN 07:51:12.** The card agent is
   strictly subordinate to the host lead. Spawn with a **child lead key whose
   actual parent is the host lead key**, regardless of the selected worker or
   lead posture. The initial append system prompt carries the **host whiteboard
   id** so the agent reads and updates the **host board**, with **no own board**.
   It is **not a peer**: absent from `lookup_peers`, the lead roster and session
   roster. A prompt id conveys context, not the host credential or authorization.
   Reject independent keys, ordinary delegate-key substitution and unrestricted
   peer authority.
2. **Owner (1072)=a — narrow authority exception now.** An active card-bound
   child lead may read/update only its host board through additive card-scoped
   operations. Preserve ordinary old calls and every unrelated parented caller's
   existing denial. No global child permission or new key role is introduced.
   This explicitly authorizes the bounded **T0-only** card exception before
   common-session-interface Phase 2. Owner (1050)=a alone authorized only
   permission/worker/lead controls, not this board exception. The foundation's
   Phase 2 later absorbs these controls with identity, lineage and single
   input/mail ownership preserved; do not wait for the whole foundation epic.
3. **Toolbar and reuse.** Add the agent-card button immediately right of Files.
   Reuse the existing new-session form's harness/model/effort controls and the
   same conversation/composer shell. Reject another renderer/protocol or a
   cosmetic conversation with no hosted session behind it. Phase 1 opens the
   real child's existing page as an intermediate slice; Phase 2 embeds that
   shell in the card and completes the requested experience.
4. **(1023)=a — real session and host navigation.** The child has a reachable
   mailbox address and revive support. The owning session's left-rail card gets
   a card icon/count badge beside its subagent badge. Clicking opens a list
   popup styled like the existing subagent popup. Row click navigates to the
   host session and scrolls/focuses the card. Row delete asks confirmation;
   deletion is also available inside the card. A real session is not an
   independent roster entry. Its full child view remains available for
   recovery/history without making it a peer or creating its own board.
5. **(1024)=c — posture and permission.** Choose `profile: worker | lead` on
   creation; default to a read-only worker. Remember the last role, permission,
   harness and model in namespaced localStorage shared across session pages.
   Posture, common permission intent and key authority are independent: both
   postures keep the child lead key in decision 1. Use the restricted worker
   surface in Technical settlement 2, not ordinary worker bootstrap identity.
6. **Permission truthfulness and admission.** Consume
   `permission_level: read-only | workdir-write | full` and
   `permission_enforcement: enforced | partial | advisory` from the permission
   producer, keeping native `permission_mode` separate. Pi is not blocked
   solely because read-only intent is unenforceable. Use the producer nudge:
   `Permission <level> is <quality> on <harness>: <limit>.`
   Recompute actual-parent current permission on every launch/revive/restart;
   missing required parent state yields visible admission-pending/error, not
   a guessed ceiling. Preserve durable authorized lowering and user-only
   raises, with the producer warning and `permission_full_ack` for explicit
   full. Saved preferences are not reusable acknowledgement and full never
   expands capabilities. Existing trusted control/approval semantics remain.
7. **Owner (1073)=a — deletion and retention.** Delete closes the hosted child
   and removes its active card binding, retaining history/recovery without
   purge. Closing the host does **not** cascade to its card children. Opening
   deleted-card history does not restore active host-board authority. Recovery
   of an active card preserves the exact key and actual parent. Keep bounded
   holder replay gaps and recovery semantics; do not promise complete output.

### Technical settlement 1: Child identity, storage and old-daemon isolation

Reuse the current Runner conversation, backend, holder and mail owners. The
lead auto-confirms the design review's retention repair: **one shared storage
selector** chooses canonical **`<dashboard>/card-sessions/<child-reply-id>/`**
for card-owned artifacts, outside the old `<dashboard>/sessions/` scan. Ordinary
sessions keep their existing paths. Expose this one selector across existing
owners and to common-session-interface Phase 1; do not introduce a parallel
runtime/store service, duplicate canonical copies or byte-backup recovery.

Keep authoritative **`card-agent.json`** and private **`card.json`** there,
including the deleting/deleted tombstone in the record. Select the same
canonical root for session-owned permission/posture data, claimed/held input
and mail ownership state, attachments, history and native session files.
Integrate the permission/worker producers with this selection rather than
leaving their session-owned sidecars under the prunable public directory.
Shared key, mailbox queue/reply-registry and host-board stores remain with
their existing owners; this does not relocate every mailbox artifact into a
session directory or create another mail consumer.

Propagate the selector through **all** lifecycle writes and readers: startup,
send/close, handoff/reopen/recovery, closed snapshots, transcript, MCP/dashboard
session-data access, inbox/history/attachment readers and producer integration.
Direct seams include `agents-plugin-tool/internal/wsagent/runner.go:882,1191,1682`,
`agents-plugin-tool/internal/wsagent/handoff.go:275–288`,
`agents-plugin-tool/internal/wsagent/spawn.go:423`, and
`agents-plugin-tool/internal/wsdashboard/pages.go:983,1102`.
No card path may accidentally create ordinary `agent.json` or read a stale
canonical copy under the public sessions tree. Keep existing store services
and their data formats behind the selected paths.

The active binding's literal fields remain:

```json
{"version":1,"card_id":"<child reply-id>","host_reply_id":"<host reply-id>","host_session_key":"<host key>","child_reply_id":"<child reply-id>","child_session_key":"<child key>"}
```

Validate this retained identity against the actual key parent and conversation;
credentials remain server-private. Never expose the host key to the child,
browser, URL, public State or append prompt. Card id equals child reply-id.
Restore a missing child key only from validated retained exact identity and
actual parent; an absent/invalid binding cannot authorize fresh minting.

Use holder kind **`ws-card`**. Public
`<dashboard>/sessions/<child-reply-id>/registration.json` may remain an existing-
shape **reconstructible projection and ownership lock**, with `Agent:true`,
`Unlisted:true`, `Hosted:true`, `HostPID:<actual child process>`. It is never the
sole authority, history or recovery store. Do not use ordinary lead projection,
which resets Agent/Unlisted
(`agents-plugin-tool/internal/wsagent/record.go:479–500`). Exclude cards from
independent list/rail/peer projections while retaining the host's card list and
private lifecycle ownership; do not hide unrelated key descendants.

Old pruning may delete this public projection directory when registration is
missing/unreadable and expired; the canonical card directory must remain
intact. `agents-plugin-tool/internal/wsdashboard/registry.go:347–385,422–436`
and `agents-plugin-tool/internal/wsdashboard/index_test.go:131–169,253–284`
explicitly establish orphan deletion and the narrower readable-Agent exemption.
Do not ask old pruning to preserve an orphan public directory. New reopen
reads canonical identity/state and safely re-projects the row before exposure,
using the existing hosted-row ownership lock and one-process checks. Row loss
alone grants no independent authority, starts no duplicate process and never
reactivates a deleting/deleted binding.

The old Runner scans only the public sessions tree and ordinary `agent.json`
(`agents-plugin-tool/internal/wsagent/runner.go:244–276`); its orphan sweep
recognizes only lead/Pi-worker holder kinds (`:349–357`). The selected new
canonical root/private files are a **T0 target**, not runtime proof: pin old
scans, shutdown and pruning, then new exact-identity reopen, restricted exposure,
current-parent admission and at-most-one-process recovery. Reject keeping all
card artifacts under ordinary SessionDir, a new-pruner-only fix, directory-mtime
heartbeats and a fake ordinary agent record: none protects canonical card state
from the already installed old pruner without changing identity/ownership.
No handoff/recovery may fall back to an independent/top-level session.

### Technical settlement 2: Bound bootstrap, board operations and posture

Add **`card.bootstrap`** to adopt only the fixed hosted card child. It is a
bounded bootstrap/manual entry, not ferrule, arbitrary key selection or
ordinary `EnsureLead` fallback. Make it a thin wrapper over the **same adoption
core as `worker.bootstrap`**, passing the validated key/parent restoration and
storage selector; do not fork a parallel mint/adoption body. The shared core
retains each entry's different authority contract. Validate the active binding, exact child
key/reply-id and actual host parent. Only validated retained identity may
repair a missing key with the same parent; unknown parent permission remains
admission-pending/error. Install the posture restriction before tool exposure
at startup and reconnect. Keyless calls and rebind attempts cannot acquire
card authority or use another session's binding.

Add **`card.whiteboard.query`** and **`card.whiteboard.update`** only. Resolve
the host privately from the binding, validating active-card state, exact child
key/reply-id and actual parent on each call. Take **no caller-supplied host or
board selector**. Reuse existing query/update payload semantics and the host's
board owner; do not create a child board or change ordinary whiteboard tools.
Current ordinary parented denial remains at
`agents-plugin-tool/internal/mcp/server.go:756–758`; existing own-board selection
is `agents-plugin-tool/internal/mcp/whiteboard_tools.go:228–257,328–332`.
The card operations are the explicit exception, not a weakening of those gates.

The card worker surface is the sibling worker allowlist **minus
`worker.bootstrap`**, plus `card.bootstrap` and the two card board operations:

```text
mailbox.send, mailbox.recv, mailbox.lookup_peers, mailbox.history,
git.status, git.diff, git.log, git.merge_base, git.commit,
card.bootstrap, card.whiteboard.query, card.whiteboard.update
```

Retain the sibling's native fixed worker loadouts and subagent restrictions;
its short worker manual gains only this bounded host-board exception. No
session creation, ticket/run/review orchestration or configuration-write powers
are added. Lead posture uses the existing **child-lead** surface plus these two
card board operations, through the fixed card bootstrap/manual; it gains no
top-level powers. Full acknowledgement never widens either surface. Preserve
non-peer mail stamps and reply-id reachability without named Presence or an
additional browser mailbox consumer. The host is the owning-lead recipient for
the default worker's introduction.

### Technical settlement 3: Trusted API, support and isolated shared view

Add these exact trusted local/own-Origin creation paths:

- **`POST /agents/cards`**: default/legacy permission path; reject explicit
  permission intent instead of silently ignoring it.
- **`POST /agents/cards/permission`**: explicit `permission_level` and producer
  `permission_full_ack` for full selection.

Both use `{host_reply_id, harness, profile: worker|lead, model?, effort?}`;
the explicit variant adds the permission fields. Root and actual authority
parent derive from the validated host, not an arbitrary caller host key/root.
Reuse the same Runner creation owner and producer admission/permission service.
Preserve existing `/agents/leads`, worker and settings behavior. Card lifecycle
controls retain trusted control gates and the card identity/storage checks;
an existing control URL is never a route around tombstone/bootstrap restrictions.

Use **one `cardSupported(backend)` admission/support source** for creation and
the dashboard offer. Later optional backend-descriptor flags fold into this
source rather than scattered allowlists. Each `GET /support/choices` harness entry advertises
**`card_profile_supported:true`** only when the complete default card path is
available, and **`card_permission_supported:true`** only when its explicit
variant is ready. Explicit requests additionally require the producer's
`permission_levels` support. Validate remembered choices against current
support and current parent, including after reconnect/replacement. Pin old and
intermediate exact-path rejection with zero creations/control writes; never
retry by dropping card/posture/permission intent or using a legacy lead/worker
route. Controls unsupported by the card holder must explicitly fail through
existing control paths where possible, not report silent success or fabricated
zero/default state. Preserve that distinction for interrupt/steer, native
permission mode, subagent count and idle state; this is accurate unsupported
behavior, not a requirement to implement new controls. Current typed decode
ignores unknown body members, so adding card fields
only to legacy URLs is unsafe
(`agents-plugin-tool/internal/wsagent/http.go:148–164,268–286`).

Choose an isolated **same-origin embedded view of the existing conversation
shell**, not a rewrite of its global ids/state or another renderer/protocol.
The source-derived existing page is **`/s/<child-reply-id>?frame=1`**:
`handleSessionPage` renders that framed page rather than the outer navigation
shell (`agents-plugin-tool/internal/wsdashboard/pages.go:1274–1316,1348–1349`).
Adapt this existing view for card presentation: suppress independent board,
roster and navigation chrome; preserve the same message/approval/history/input
owners. The iframe is a view boundary, not an agent or process owner. Bind any
embed-control messages to the exact origin, source window and child session;
two cards and the host must not share draft/stream/control state. The existing
full child view remains recovery/history access without a roster entry or own
board; viewing it does not activate a deleted binding.

### Technical settlement 4: Crash-safe deletion and non-cascading recovery

Leave conversation **`ParentRef` unset**: actual key parent remains the host,
but `Runner.CloseConversation` traverses conversation descendants
(`agents-plugin-tool/internal/wsagent/runner.go:952–969`). Key authority lineage
must not accidentally introduce host-close cascading or turn-report parentage.
Add a comment at the ParentRef assignment sites explaining that cards leave it
unset intentionally to avoid CloseConversation cascading; common-session-interface
Phase 1 gets lineage from the actual key parent, not this conversation field.

Persist a **deleting/deleted tombstone in `card-agent.json` before closing the
child/removing `card.json`**. Authorization denies as soon as deleting begins,
even while the active binding file remains. Retry crash-interrupted close and
binding removal idempotently without resurrecting active authority. Retain the
tombstone and history/recovery; opening the old page cannot reactivate the card.
Active-card recovery uses exact same key/parent, current-parent admission and
one existing input/mail owner. The canonical card root retains the tombstone,
claimed/held state and history even if old pruning removes the public row;
re-projecting that row must not reactivate a deleted card. Handoff must not
produce independent/top-level fallback. Holder replay is bounded; preserve gaps, record sequence/recovery
holds and no-duplicate delivery instead of asserting complete output
(`agents-plugin-tool/internal/wsholder/manager.go:395–496`).

## Approved Visual Reference

[Approved HTML baseline](../assets/261115-feat-ws-whiteboard-agent-card/mock.html)
is retained byte-identical to MAIN's supplied HTML. Its toolbar, create and
running sections are the reference. The original human-facing Korean UI strings
are reference content, not ticket prose.

The baseline predates role selection, host badge/list popup and non-peer
amendments; their absence is not an approved omission. Its example roster claim
and absolute read-only wording are superseded by Decisions 1, 4–6. Do not copy
hard-coded project paths, models, example messages or tool calls into shipped
instructions.

When visual execution is authorized, compare implementation captures side by
side with the tracked baseline and separately verify the later textual
amendments. Final acceptance is the owner's live-screen check after MAIN's
authorized merge/runtime swap. Documentation capture authorizes no screenshots,
provider launches or theme/width matrix.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/skill-authoring.md for authored bootstrap/manual/prompt text; ai-docs/manuals/wsflow-mirroring.md for mirrored package surfaces.
- Scope is current Claude/Pi and current Runner/backend/holder/mail owners.
  No new provider, universal runtime, peer session, own board, hire-approval
  flow, global permission configuration or retroactive session migration.
- Preserve one message/mail delivery owner, approval correlation, holder
  attachment, historical reads and recovery gaps. Never expose the host key,
  create ordinary `agent.json` for a card or silently rekey it on recovery.
  Select canonical card-owned storage outside the public sessions scan across
  every reader/writer and producer integration. The public row is a disposable
  projection/ownership lock, never the sole history or authority store.
- Keep posture and permission independent. Browser preferences are local UX
  state shared between session pages, not a cross-machine authority store.
  Preserve native `permission_mode` and existing non-card omission/defaults.
- Permission Phase 2 then worker Phase 1 Results precede card Phase 1.
  Owner 1072 supplies the specific current-Runner card exception now, T0 only;
  neither common-session-interface Phase 2 nor the whole epic is a prerequisite.
  Forward absorption preserves these card-specific identity and input owners.
- No new key role, protocol/schema bump, top-level power or general child-board
  access. T0 is required, not already proved. Any failed directional fixture
  returns to the lead rather than being relabeled T2/T3 or silently downgraded.
- Project-only citations stay in this ticket, not shipped bootstrap/manual text.

## Prior Art and Source Evidence

Source baseline `74f573faf`; the worker contract is read from merged develop
`cf6d129b9`. Paths below are repository-relative. Source establishes the traps
and reuse owners, not completed card behavior or compatibility tests.

| Existing seam | Verified observation and contract consequence |
|---|---|
| `agents-plugin-tool/internal/mcp/session_auth.go:25–31,205–216`; `agents-plugin-tool/internal/mcp/playbook_tools.go:574–581,1009–1015` | Existing encoding and render path already mint lead-scope children with a parent. Reuse this identity rather than inventing a role; parented keys are not top-level leads. |
| `agents-plugin-tool/internal/mcp/agent_keys.go:28–43,99–118`; `agents-plugin-tool/internal/wsagent/runner.go:593–619,2213–2215`; `agents-plugin-tool/internal/mcp/server.go:2380–2386,2454–2470` | Ordinary hosted MintLead/EnsureLead and top-lead re-login are parent-less; missing ordinary ensure recreates a parent-less record. Cards require fixed child adoption and exact retained-parent recovery, never those fallbacks. |
| `agents-plugin-tool/internal/mcp/server.go:714–758`; `agents-plugin-tool/internal/mcp/whiteboard_tools.go:228–257,328–332`; `agents-plugin-tool/internal/mcp/workflow_manual.go:324–337,400–408` | Transport allowlists precede keyed gates; ordinary whiteboard refuses every parented caller and handlers address the supplied caller key's own board. Hosted parented manual handling disables board guidance. Card bootstrap and two card board ops must supply the narrow exception without altering ordinary gates/manual powers. |
| `agents-plugin-tool/internal/mcp/mailbox_tools.go:68–90,389–465`; `agents-plugin-tool/internal/mcp/lead_mail.go:79–170` | Reply-id reachability is independent of named Presence and top-level peer stamping. Reuse Watch/Drain ownership; no browser drain or independent peer registration. |
| `agents-plugin-tool/internal/mcp/dashboard_sessions.go:883–918`; `agents-plugin-tool/internal/mcp/agent_keys.go:65–87`; `agents-plugin-tool/internal/wsagent/runner.go:780–798`; `agents-plugin-tool/internal/mcp/session_auth.go:418–483` | Dashboard lookup/LeadSession reject parented identities, whereas Runner.List and session.children have separate enumerations. Implement explicit card visibility projections rather than weakening lookup or filtering all descendants. |
| `agents-plugin-tool/internal/wsagent/record.go:479–500`; `agents-plugin-tool/internal/wsagent/runner.go:244–276,349–357,1634–1646,2202–2267`; `agents-plugin-tool/internal/wsagent/handoff.go:275–288` | Registration, startup scan, orphan sweep, recurring updates and reopen are separate lifecycle paths. Ordinary startLead clears allowed-tool restrictions at `runner.go:2262–2267`; cards must install their surface before exposure and retain it across reconnect, not inherit this reset. |
| `agents-plugin-tool/internal/wsagent/runner.go:882,1191,1682`; `agents-plugin-tool/internal/wsagent/handoff.go:275–288`; `agents-plugin-tool/internal/wsagent/spawn.go:423`; `agents-plugin-tool/internal/wsdashboard/pages.go:983,1102` | Send/update persistence, closed snapshots, reopen, transcript and dashboard live/history reads separately select session paths. One selector must reach readers and producer sidecars, not just record writes. Mail queue/registry ownership remains separate from session-owned held/claimed data. |
| `agents-plugin-tool/internal/wsdashboard/registry.go:347–385,422–436`; `agents-plugin-tool/internal/wsdashboard/index_test.go:131–169,253–284` | Old pruning deliberately removes expired orphan public directories. Canonical card artifacts must live outside that scan; readable Agent registration is only a projection exemption, not durable retention. |
| `agents-plugin-tool/internal/wsholder/holder.go:89–116`; `agents-plugin-tool/internal/wsholder/manager.go:395–496` | Holder session metadata is consumer-owned JSON; attach/replay and gap accounting remain manager-owned. An unknown kind alone does not establish shutdown/replay safety. |
| `agents-plugin-tool/internal/wsdashboard/assets/dashboard-leads.js:43–59,75–110,120–184`; `agents-plugin-tool/internal/wsdashboard/assets/rail.js:609–619` | `leadRows`, `rpLeadMake` and `WsRail.leadBody` supply existing form composition, native mode and creation payload patterns. Reuse producer common controls without confusing native mode with common intent. |
| `agents-plugin-tool/internal/wsdashboard/assets/dashboard-rail.css:316–323`; `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js:1259`; `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl:24,256,263` | Fixed `.rpop` geometry needs an in-flow form variant. Existing subagent badge/popup appearance and conversation/status templates are reuse points, not authorization semantics. |
| `agents-plugin-tool/internal/wsdashboard/assets/dashboard-composer.js:1–17`; `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:317`; `agents-plugin-tool/internal/wsdashboard/pages.go:1274–1316,1348–1349` | Global ids/chips/document selectors make duplicate in-document shells unsafe. The existing `?frame=1` session page provides the selected isolated same-origin view boundary. |

### Directional compatibility contract

All rows are **T0 requirements pending focused implementation proof**, not
completed fixture results. Pin the inspected old baseline and the landed
permission/worker producer baselines; test an intermediate server where only
default card creation exists. Old code must remain usable without acquiring or
rewriting card authority. Reopening new code must retain the exact card state;
no whole-file-loss/T2 reconstruction claim is made.

| Surface / direction | Old/new read, write and reopen requirements |
|---|---|
| Ordinary calls and new card tools | Old ordinary calls to new code keep behavior and every unrelated parented board denial. New callers discover `card.bootstrap` / `card.whiteboard.query` / `card.whiteboard.update` before use; old unknown-tool dispatch and undeclared-argument precheck refuse before writes (`agents-plugin-tool/internal/mcp/server.go:2315–2316`; `agents-plugin-tool/internal/mcp/tool_arguments.go:65–128`). Pin actual old dispatch coordinates before fixtures. Zero key/binding/board mutations on refusal; no ferrule/host-key fallback. Reopen does not replay refused calls. |
| `/agents/cards`, `/agents/cards/permission`; new client to old/intermediate server | On the inspected old router default card POST has nonempty id and empty action and returns 405; the permission action reaches unknown-action 404 (`agents-plugin-tool/internal/wsagent/http.go:107–117,175–180,229–230`). Require zero creations/control writes, not just status. An intermediate default-only server refuses the explicit exact path without accepting a prefix or dropping fields. Positive support followed by swap never retries legacy/default URLs. Old lead/worker calls to new handlers remain unchanged. |
| Additive `card_profile_supported` / `card_permission_supported`, optional display projections and browser preferences | Old UI reads named choice members (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-leads.js:43–59`) and must ignore additive members without data changes. New clients re-read current support and validate namespaced preferences; old page writes must not clear those bytes. No support flag in fixed key/registration/Profile storage. New reopen recomputes supported display/admission, not a latent grant. |
| Canonical `<dashboard>/card-sessions/<id>/`, `card-agent.json`, private `card.json`, `ws-card`; new writer to old scan/write/shutdown to new reopen | The shared selector places authoritative card bytes outside the old public sessions scan (`agents-plugin-tool/internal/wsagent/runner.go:244–276`; `agents-plugin-tool/internal/wsdashboard/registry.go:347–385`). Old ordinary record/registration writers touch their existing public paths (`agents-plugin-tool/internal/wsagent/record.go:264–303`; `agents-plugin-tool/internal/wsdashboard/registry.go:282–328`); the orphan-holder sweep ignores unknown kind (`agents-plugin-tool/internal/wsagent/runner.go:349–357`). Pin old reads/writes/scans/shutdown leaving canonical record, private binding, tombstone, permission/posture, session-owned held/claimed data, attachments/history/native files intact. New reopen reads identical settings/key/actual parent and rejoins at most one process through current owners. Test every selector consumer, including MCP/dashboard/transcript and producer sidecars; no duplicate canonical copy or ordinary record. T0 target from old code ignoring the new root, not a filename or unknown-kind survival claim alone. |
| Public registration projection; old prune deletion to new reprojection | The new row uses existing Agent/Unlisted/Hosted/HostPID fields. Pin old pruning of expired public directories with missing/unreadable registration, plus expired readable-Agent and ordinary-expired controls (`agents-plugin-tool/internal/wsdashboard/registry.go:347–385,422–436`; `agents-plugin-tool/internal/wsdashboard/index_test.go:131–169,253–284`). Require public deletion where the old contract deletes it, while canonical bytes/settings/key/actual parent/tombstone/history remain unchanged. New code reads canonical identity, safely re-projects the existing-shape row under hosted ownership/one-process checks before restricted exposure, and recomputes actual-parent permission. A missing projection is not independent authority; deletion recovery cannot reactivate a tombstone. No reconstruction from byte backups, mtime heartbeat, fake agent record or new-pruner-only protection. |
| Key recovery and restricted startup/reconnect | Existing `scope`/`parent` readers preserve the known encoding (`agents-plugin-tool/internal/mcp/session_auth.go:205–216,248–259`), but ordinary EnsureLead rejects a parent or restores a missing parent-less key (`agent_keys.go:99–118`). Isolate cards from old launch owners; new restore requires validated retained exact identity/actual parent, never ordinary ensure. Before exposing tools, install worker/child-lead restrictions and bound bootstrap; test keyless/rebind attempts, missing/changed parent and every current-parent launch check. No host credential leaves the server. |
| Delete crash, host close, retained history and replay | New writer stores the deleting/deleted tombstone in canonical card-agent.json before close/binding removal; old pruning may remove only the public projection, not this authority/history. New reopen retries interrupted deletion without active-binding resurrection. Host close cannot traverse cards because conversation ParentRef is unset (`agents-plugin-tool/internal/wsagent/runner.go:952–969`). Historical view never grants board authority. Preserve manager gap accounting (`agents-plugin-tool/internal/wsholder/manager.go:395–496`), holder sequence ownership and one input/mail delivery across old/new shutdown/recovery; do not promise complete replay or duplicate accepted requests. |

## Prior Decisions

- Owner 1072 explicitly settles the card-only host-board exception now; 1050
  alone did not. Foundation minimum-interface/parity ordering otherwise remains
  unchanged, with no whole-epic prerequisite or broad child authorization.
- Permission-profile Decisions 1–3, 10–14 constrain current-parent admission,
  truthful enforcement, user-only raises and positive support/exact selection
  paths. Permission Phase 2 supplies the reusable runtime service.
- Worker-profile Decisions 1, 5–6 and Technical contract constrain worker
  tools/manuals and supply Phase 1 controls after permission Phase 2. Its
  delegate bootstrap is deliberately replaced only for cards by the fixed
  child lead identity and bounded card bootstrap. Host introduction recipient,
  key parent and conversation ParentRef remain distinct dimensions.
- EXEC4's bounded advisory found no blocking conflict with the settled
  common-session-interface Phase 1 decisions; its Phase 1 design is not yet
  drafted. Reuse the worker adoption core, one storage selector and one card
  support/admission source, preserve actual-key-parent lineage, and explicitly
  fail unsupported controls. This is a technical consistency refinement, not
  a new foundation dependency or authority grant.
- Design-review retention repair, auto-confirmed by the lead: canonical
  `<dashboard>/card-sessions/<child-reply-id>/` replaces the unsafe all-artifacts-
  under-SessionDir premise. Reuse one selector across existing data owners;
  public registration is reconstructible and may be pruned. No duplicate store,
  fake ordinary record, mtime heartbeat or new-pruner-only fix supplies T0.
  This changes retention mechanics, not owner 1072/1073 behavior, the typed
  worker Phase 1 prerequisite or the permission producer chain.
- Owner 1073 closes the child/removes active binding without purge and forbids
  host-close cascade. This overrides any baseline mock implication that close
  merely hides an active session or that a deleted card can resume authority.
- 261115-feat-ws-composer-inline-skill-and-topic-capsules: the textarea remains
  each field's owner for existing form/image/draft/send code — preserve inside
  each reused frame rather than rewrite global state.
- 261114-refactor-ws-dashboard-whiteboard-asset-split: concatenated JavaScript
  fragments share the original IIFE — the isolated view avoids cross-card
  selectors without introducing another conversation implementation.
- 8d14fee7: Mint must agree with Ensure — exact child/parent restoration applies
  to create, bootstrap, missing-key recovery and all lifecycle writes.
- 261110-feat-ws-dashboard-headless-lead-sessions and
  261110-feat-ws-hosted-session-mode-flag use an ordinary listed parent-less
  hosted lead and fixed-key bootstrap. Cards use explicit separate projection
  and bound adoption, not a weakening of those ordinary contracts.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/{mcp,wsagent,wsdashboard,wsholder}; bootstrap/manual and mirrored package surfaces as touched. |
| scope.surface | cross-module | Fixed hosted child identity, private binding/tombstone, MCP operations, trusted HTTP, registration/holder recovery and shared embedded view cross existing owners. |
| scope.new_public_symbol | yes | card.bootstrap, card.whiteboard.query, card.whiteboard.update, /agents/cards, /agents/cards/permission and the two additive support members are settled additions. |
| scope.new_type_contract | yes | Canonical card-sessions/<child-reply-id>/ selected across owners, version:1 private card.json binding with exact fields above, card-agent.json lifecycle/tombstone and session-owned permission/posture/input/history; existing-shape public registration is reconstructible. ws-card holder metadata, creation inputs and scoped authority remain. No new role or ordinary key/registration/Profile member. |
| scope.test_surface | existing | Extend internal/wsdashboard/lead_isolation_test.go, composer/page/rail fixtures, internal/wsagent/{lead_recovery_test,lead_http_test,record_test}.go, mcp key/board/mail fixtures and wsholder manager fixtures. Card-specific cases are requirements, not claimed existing coverage. |
| complexity.reuse_points | confirmed | sessionStore.mint, AgentKeyStore traps, shared worker/card adoption core, one storage selector and cardSupported(backend) source, Runner lifecycle, LeadMailbox Watch/Drain, registration/prune owners, holder attach/replay, leadRows/rpLeadMake and pages.go handleSessionPage ?frame=1. |
| complexity.side_effect_risk | high | Every lifecycle/MCP/dashboard/inbox/history/transcript reader/writer and permission/worker producer integration must use one canonical selector, keep exact parent, re-project public ownership safely and preserve one input/process owner. |
| risk.correctness | high | Ordinary ensure loses missing-key parent; ordinary registration lists leads; old pruning removes public orphan projections, so no canonical authority/history may remain there; conversation ParentRef would cascade close. |
| risk.fit | high | Explicit seams reuse existing owners, with same-origin frame isolation instead of duplicate global shell state. No independent/top-level fallback is permitted. |
| risk.test | high | Require old scan/shutdown/prune/reopen, two-card-plus-host isolation, exact unsupported paths, reconnect and delete-crash proof. No live/provider inference. |
| risk.security_or_contract | high | Only active validated card children get host-board operations. Startup/reconnect keyless/rebind restrictions, credential privacy and unchanged unrelated-child denials are mandatory. |

## Phases

### Phase 1: Create real subordinate sessions and host card navigation

Depends on `261115-feat-ws-permission-profile#phase2`, then
`261115-feat-ws-worker-profile-session-spawn#phase1` Results. Owner 1072 permits
this bounded T0 card-authority slice now without common-session-interface
Phase 2 or whole-foundation completion. Implement all four technical settlements
for identity, storage, admission, scoped tools, trusted creation, lifecycle and
compatibility through current owners.

Reuse producer controls in an in-flow creation form opened by the toolbar
button right of Files. Include role selection, default read-only worker,
remembered role/permission/harness/model and truthful permission quality.
Create an actual hosted Claude/Pi child with fixed actual parent, private
binding, host-board prompt context, reachable mailbox/introduction and revive.
Add the host rail icon/count and subagent-style list popup, host navigation and
card focus, confirmed row deletion and in-card deletion. Keep no independent
roster, named peer presence or child board.

This intermediate slice opens the existing real child conversation page;
embedded conversation is deferred to Phase 2, not dropped or represented by a
fake running placeholder. Preserve full history/recovery access without
reactivating deleted bindings or adding an independent roster entry.

Verification boundary: focused fake/isolated Runner, MCP, mail, HTTP, holder
and browser fixtures cover:

- Exact child lead key/parent for both postures on create/bootstrap, active
  revive/restart and missing-key recovery; no independent mint, ordinary
  record or handoff fallback. Invalid bindings and unknown parent permission
  produce explicit denial/pending errors, not fabricated ceilings.
- Shared worker/card adoption core with distinct authority, one callable
  storage selector, consistent creation/dashboard support, intentional unset
  ParentRef comment and actual-key-parent lineage; explicit unsupported-control
  results rather than silent success or invented zero/default telemetry.
- Restricted startup/reconnect before exposure, keyless/rebind attempts,
  host-private credentials, worker allowlist/full acknowledgement independence,
  host-only card query/update and unchanged unrelated-parented ordinary denial.
- Reachable reply-id, default-worker introduction to host, non-peer/list
  projection, single mail/input owner, current-parent admission, durable
  lowering, full warning, saved-choice validation and Pi nudge-and-launch.
- Every directional matrix row: pin old/intermediate exact tool/path refusal
  with zero writes and no fallback, old callers unchanged, old scans/shutdown,
  canonical-byte retention/new reopen and no ordinary agent.json creation from
  any lifecycle update. Pin old pruning deleting expired public orphan
  projections with missing/unreadable registration while canonical settings,
  key/actual parent, tombstone and history stay intact; include expired
  readable-Agent controls. New reopen safely re-projects the row before
  restricted exposure/current-parent admission with at most one process.
  Exercise the selector through all lifecycle, MCP/dashboard, input/mail,
  history/transcript and permission/posture producer consumers; shared mailbox
  queues/key/host-board owners remain unchanged.
- Host badge/count/list navigation/focus, delete confirm/cancel, tombstone
  written before close/removal, crash retries at each boundary, no authority
  resurrection from history, no host-close cascade and bounded replay gaps
  without duplicate input or processes.

When visual execution is authorized, compare toolbar/create captures to the
tracked HTML and check the later badge/popup amendments separately. These
fixtures do not claim live-provider enforcement or final embedded acceptance.

### Phase 2: Embed the existing conversation/composer shell inside the card

Depends on this ticket's landed Phase 1 Result. Mount the selected same-origin
existing framed session view inside each card, suppressing independent board,
roster and navigation chrome. Reuse the exact child session and all existing
message/approval/history/stream/status/input owners. Do not rewrite global ids
into a second multi-instance renderer, introduce another protocol/process or
route child messages to the host.

Verify two cards plus the host have isolated drafts, image/command state,
message/interrupt destinations, status/context/cost, history pagination,
stream cleanup and approval correlation through focus/fold/reopen, replacement
and recovery. Verify embed controls validate origin, source window and child
session, including wrong-card/stale-window messages. History/full-view access
and deletion retain Phase 1's canonical-storage, tombstone and authority rules,
including public-row reprojection without authority resurrection; a view is not
a process owner. Preserve bounded replay/gap and single-input behavior.

When authorized, compare running captures side by side with the approved
baseline; explicitly check role/permission truthfulness and badge/popup behavior
missing from that baseline. Final acceptance remains the owner's live-screen
check after MAIN merge/runtime swap.

## Verification

Extend focused existing fixtures; do not infer behavior from live providers.
One existing browser entry point is
`go test ./internal/wsdashboard -count=1 -run '^TestLeadPopoverLaunchBehavior$'`
from `agents-plugin-tool/` (`internal/wsdashboard/lead_isolation_test.go:10–19`).
Add card-specific cases for the matrix and phases in the named existing package
harnesses. New case names and selectors are implementation work, not claimed
existing tests. Read complete output and reject empty selectors when executing.

This documentation revision runs only whitespace checks, ticket guardrails and
approved-asset byte comparison. Capture/intent checks cover the settled owner
and four lead technical decisions, literal contracts, dependency order,
compatibility proof obligations and visual acceptance. No runtime/provider,
network, auth/install, screenshot or broad test execution is authorized here.

## Owner Decisions (2026-11-16)

- (1178)=ok, with a follow-up from the owner, verbatim: "마우스 올리면 호버까지
  반영해주세요." The badge popover merged at `ad9c2c79f`/`7585c1620` is
  accepted. Next change: the popover also opens on mouse hover, not only on
  click. Keep the click/pin behavior.
