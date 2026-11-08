---
title: "ws team tracker: agent-first shared stores (team and personal) on git, opened from the dashboard"
related:
  261107-epic-ws-dashboard-messenger-round2: the dashboard and its pull-down drawer the tracker opens from
  261107-feat-ws-dashboard-pull-down-drawer: the drawer frame tracker stores register into
sage-review-design: required
---

# ws team tracker: agent-first shared stores (team and personal) on git, opened from the dashboard

## Scope

The user (2026-11-07) redirected this epic from a personal, one-machine store
into a team platform: "wsflow랑 통합된 jira 같은 거고, 다만 어쨌든 사람보다는
에이전트를 통한 소통에 좀 더 방점을 둔다는 거죠" (board (110) c). One person
holds several stores ("a팀 저장소, b팀 저장소, 개인 저장소"), each a git
repository sized for 50-60 people, reached from the dashboard.

- Stores hold feature-level work items ("특정 기능 누가 완성"), notes, and
  simple TODOs (board (111) c), with assignment down a lead/follower
  hierarchy.
- Agents are the primary writers and readers: a lead session records
  progress on tasks assigned to its user and answers the user's questions
  about the stores.
- Git is the storage and transport layer only, hidden behind the data model,
  and works without a remote.

## Non-Scope

- Binding tracker items to ws tickets: tickets are per-project, AI-only
  artifacts; a tracker item carries at most an optional free-text project
  reference (board (118) c, (121)).
- Users who do not run ws or wsflow, and any UI outside the dashboard
  (board (119) a).
- Per-item confidentiality inside one store: confidentiality is the
  repository boundary (board (124)).

## Child Tickets

- Planned (first): an unstyled, fast-iteration mockup that settles the data
  structures (user, board (139) ok, 2026-11-07: "스타일을 제외한 목업(가능한
  빠르게 이터레이션 할 수 있게)을 보면서 구체적으로 어떤 데이터 구조를
  가져갈지 제대로 정하는 걸로"). Later children are planned only after it.
- `261107-feat-ws-tracker-ledger-protocol`: the payload-agnostic protocol
  layer (per-clone refs, signed commits, identity, causality, sync, rewind
  refusal), planned ahead of the data structures by board (235) a, since the
  data structures do not affect it. The data-structure layer stays behind the
  mockup.
- Mockup v3 (`ai-docs/ref/tracker-mockup/`, merged 2026-11-08) settled the
  data structures: README Settled rows through (293) and a nine-scenario walk
  in `SCENARIOS.md`.
- `261108-feat-ws-tracker-data-structure`: the Go domain layer over
  `wsledger` (event types, validation, fold, view state JSON in the mockup's
  View state shape), ported from the mockup engine with golden parity tests
  (board (283) a). No caller surface.
- Next (unplanned): the caller child, covering MCP and skill exposure
  ((115)), the dashboard drawer, sync triggers, and config.
- `261108-feat-ws-tracker-support-agent-chat` (draft): the support-agent
  chat card in the drawer, on the standalone agent runner
  `261108-feat-ws-daemon-agent-runner` (board (300)-(312)). The mockup's
  per-card view modes and chat placement merged as `1d7fbce0a`.
- `261108-feat-ws-tracker-ledger-attachments` (todo): attachment content as
  content-addressed blobs under `attachments/<sha256>` in the member's ledger
  tree, fetched on demand through a partial clone (board (337) a). The mockup
  and `wstracker` already model the metadata and the `attachments.max_bytes`
  limit (merge `0a80fdbfb`, board (338), (339), (345) b).

## Cross-Child Decisions

- **Storage model** (user, board (116) ok): each member appends events to
  their own ref only. Every view is computed client-side by merging all refs.
  This removes write contention and merge conflicts, and a lead's assignment
  is an event in the lead's own ref. Rejected: per-item files on one shared
  branch with rebase retry ((112), superseded); one CAS-guarded custom ref,
  the ticket-lease pattern.
- **Repository boundary** (user, board (124)): one repository per team or
  confidentiality boundary. Everyone in a repository is assumed to share the
  same NDA, and access control is delegated to the hosting repository's
  permissions.
- **Integrity** (user, board (117) c, (120) ok): commits are SSH-signed
  (`gpg.format=ssh`) with a local key file. Push stays on whatever transport
  the remote uses (HTTP at the user's company). Every client verifies
  signatures against the member public keys in a roles file, and ignores and
  flags events that break the rules. The agent generates the key and
  configures git. Membership is a self-join by anyone with write access to
  the repository, with admins arranging and removing members afterward
  (board (231) b, (232) b; superseding the earlier admin-approved join
  request).
  Host-side enforcement (branch protection, rulesets, pre-receive hooks) is
  optional hardening where the host supports it. Rejected: relying on the
  host's key registration or signature verification.
- **Roles** (user, board (124) b, (127) a, (130) a):
  - Admins are a separate layer that edits the roles file. Several admins
    may be listed, and a signature from any one of them is valid.
  - Below the admins, lead/follower relations nest: a follower may itself
    be a lead. Relations are one-directional and cycle-free.
  - Within one repository each person has at most one lead, a tree. Leads
    are stored as a list validated to length 1, so multiple leads can be
    enabled later without a migration. A matrix organisation is expressed by
    separate repositories.
  - Only admins edit relations.
  - Rejected: multiple leads per repository now; lead-invites-follower-accepts.
- **Assignment** (user, board (128) c, (129) a, (125) c):
  - An assigner targets either everyone within N levels below them, or
    chosen people within that range.
  - A mid-level lead may split a received task into subtasks for their own
    followers, and subtask progress rolls up to the parent.
  - Completion is chosen per task at assignment time: either the assignee
    closes it, or the assigner must confirm. The default is assignee closes.
  - Amended 2026-11-08 (board (273) b, (279) a, (280) a): assignment is
    unified. Chosen people may be anyone in the store; a target within the
    author's lead chain is forced and live, anyone else gets a pending
    assignment until accept or decline, and on accept the accepter's direct
    lead is notified and may return it. An assignee may hand off its
    assignment. "Everyone within N" stays forced-only. Detail: the mockup
    README.
- **Agent recording** (user, board (123) b): an agent automatically records
  progress and completion only for tasks assigned to its user; anything else
  is recorded on request. Rejected: recording only on request; automatic
  session logs.
- **Sync** (user, board (126) ok):
  - A background poll of the remotes every 3 minutes.
  - An immediate check when the drawer opens and when a lead session starts.
  - The member's own writes are pushed after a 5-minute idle delay, at an
    explicit apply (a button that drops down while writes are pending), or
    when the tracker panel is collapsed (board (241), amending (126)'s
    immediate push, which with batched commits (227) a would commit per
    write).
  - New assignments show as a badge on the drawer handle, like the mailbox.
  - Rejected: short-interval polling; webhook receivers, which need an
    inbound server a personal machine does not have.
- **Search and local AI** (user, board (114) ok, (122) ok): search indexes
  are built per store and stay local, outside git. A local AI may search
  across the user's stores read-only, only to answer the user's own query.
  It shows the answer only to the user and writes nothing into another store.
- **Exposure** (user, board (115) ok): the tracker becomes an MCP and skill
  surface, active only where a store is configured. It is off by default in
  both ws and wsflow. This supersedes the earlier "no skill, playbook, or MCP
  tool mentions it" decision.
- **Local UI state** (user, board (332) a, (340)): per-viewer UI state such
  as notification read state, the handle badge's seen marks, and each card's
  chosen view mode lives per clone on the local filesystem outside git, read
  and written by the daemon; it is never an event and never browser storage.
  The mockup simulates it in memory.
- **Only human- or agent-authored content is tracked** (user, (66) ok,
  carried over): indexes and embeddings are a local cache outside git.
- **Personal store** (board (136) ok): a personal store is a repository
  with one member, the same model as a team store.
- **Dashboard placement** (board (137) ok, from (119) a, (126), (131) a):
  each store is its own drawer handle; handles stack leftward from the
  top-right corner.
- **Config** (board (138) ok): the ws and wsflow manifests differ only in the
  enable key's default.
- **Mockup form** (board (140) ok, (141) ok): a standalone HTML+JS page under
  `ai-docs/ref/`, with JSON fixtures of per-member event logs and a roles
  file, every view computed in the browser. Its final state is committed to
  git (user: "mock도 최종 완성 시점엔 git track 되어야해요"). It starts now:
  the earlier timing gate (64) existed because the panel sat on the session
  page, and the mockup touches no shipped code. Rejected: a Go or daemon
  prototype, which iterates more slowly.
- **Superseded** (board (135) ok): the personal-store decisions one store per
  machine ((65) a), no exposure ((67)), and the old phase order ((68)) no
  longer apply. The stem is kept and only the title and body were rewritten
  (board (134) ok), because existing tickets and board items cite it.
- Shipped behavior stays downstream-first (AGENTS.md Architecture Rule 4):
  the tracker is designed for any team using ws, not for this repository.

## Completion Criteria

- Done: the tracker children have landed on `develop`, and the user has used
  the tracker from the dashboard with one team store over a remote and one
  personal store (board (142) ok).
- Dropped: the user stops using the tracker or moves team assignment
  elsewhere.
- Deferred: multiple leads within one repository; host-side enforcement as a
  default; users without ws.
