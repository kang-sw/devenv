---
title: "ws personal common store: a cross-project, git-tracked store for the user's own TODOs, notes, and search, opened from every dashboard page"
related:
  261107-epic-ws-dashboard-messenger-round2: starts after round 2's remaining children and their dogfood; the panel sits on the round-2 session page
sage-review-design: required
---

# ws personal common store: a cross-project, git-tracked store for the user's own TODOs, notes, and search, opened from every dashboard page

## Scope

The user (2026-11-07) wants one store that spans all their projects, for their
own use, git-tracked and owned by ws-mcp rather than any one project, and
reached from the dashboard that is already open per working folder:

- a git-tracked store for human TODOs and notes, with its git history
  searchable for past data;
- search over the store and, later, over the user's own files;
- when a local LLM is available, indexing the user's files during idle time;
- a panel on every dashboard session page: a button at the top right that
  drops the panel down over the page and folds it back up, so the user never
  navigates away from the session page;
- asking the current session's lead for help with the store from that panel:
  no agent runs in the store; the panel sends a request to the session the
  page belongs to, carrying the store's path, in the sense "I know what you
  are doing, but help me update the common store for a moment".

User wording: "git track되는 형태로, 뭔가 ws mcp의 루트에? 하여간에 모든
프로젝트를 아우르는 저장소를 하나 둬서 제가 사용할 목적의 어떤 todo 리스트
관리"; "각 대시보드 화면에서 '위로 접혀 올라가고, 누르면 내려오는' 형태의 패널
버튼을 우상단에 두고"; "해당 노트 저장소에 에이전트를 띄우는 게 아니라, 그
저장소의 현재 경로를 에이전트한테 알려줘서".

## Non-Scope

- Any mention in ws or wsflow skills, playbooks, or MCP tool surfaces before
  the user explicitly asks for it (see Cross-Child Decisions).
- Syncing one store across machines as the designed default (user, (65) a);
  a user may add a git remote by hand.

## Child Tickets

- Planned (phase 1): the store, the top-right drop-down panel on every
  session page, human TODOs, and the ask-this-session request.
- Planned (phase 2): search over the store and its git history.
- Planned (phase 3): idle-time indexing and search of the user's files when a
  local LLM is available; this phase starts with research (which local LLM
  runtimes, which folders, CPU and power budget).

## Cross-Child Decisions

- **Timing** (user, 2026-11-07, "(64) a"): captured now; design starts after
  round 2's remaining children (link actions, Now markdown, status band) and
  their dogfood land, because the panel sits on that page. Rejected: designing
  in parallel with round 2.
- **One store per machine** (user, "(65) a"): the store stays on the machine
  by default; connecting a git remote is the user's own choice. Rationale: the
  user also uses ws at work, and indexing or search results from a work
  machine must not leave it through a personal store by default. Rejected: one
  store shared across machines through a remote as the designed default.
- **Only human-written content is git-tracked** (user, "(66) ok"): notes and
  TODOs are tracked; the file index and embeddings are a local cache outside
  git. "Past data" search covers the tracked content's git history. Rejected:
  tracking the index - it is derived, rebuildable, and large.
- **Exposure** (user, "(67) ok"): off by default in both ws and wsflow; no
  skill, playbook, or MCP tool mentions it; the store's authoring rules are
  served only by the dashboard daemon's REST API. The ask-this-session request
  goes through the session's existing dashboard inbox and carries the store
  path and that API address; the lead reads the rules there and edits the
  store. The enable switch is a dashboard config key (ws and wsflow manifests
  differ only in default).
- **Phase order** (user, "(68) ok"): store + panel + TODOs + help request,
  then search over the store and its history, then local-LLM file indexing,
  the most uncertain part, last.
- Shipped behavior stays downstream-first (AGENTS.md Architecture Rule 4): the
  store is designed for any ws user's personal use, not for this repository.

## Completion Criteria

- Done: the three phases have landed on `develop`, and the user has used the
  panel from session pages in at least two projects to manage TODOs and to ask
  a session for help with the store.
- Dropped: the user stops using the panel or moves personal TODOs elsewhere.
- Deferred: cross-machine sync as a designed feature; exposing the store to
  ws/wsflow skills or MCP tools.
