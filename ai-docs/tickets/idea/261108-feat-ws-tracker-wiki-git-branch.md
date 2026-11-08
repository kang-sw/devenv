---
title: "ws tracker wiki: a native-git wiki branch in the store repository"
parent: 261107-epic-ws-personal-common-store
related:
  261107-feat-ws-tracker-ledger-protocol: the store repository, auth, and sync cadence the wiki branch shares; the wiki deliberately does not use its event protocol
  261108-feat-ws-tracker-data-structure: unchanged by this ticket (no new event types)
---

# ws tracker wiki: a native-git wiki branch in the store repository

## Background

Side discussion on the board (2026-11-08, topic "트래커: 위키 기능"): the user
asked whether a wiki fits the tracker's protocol model, then proposed keeping
the wiki out of the event protocol entirely: "그냥 git branch 하나를 위키로
쓴다던지 ... 위키용 브랜치는 뭔가 native git 모델을 따라가는". Sequenced
after the tracker core children (the caller child and the support-agent chat)
by board (376) b; this is an idea, not yet planned.

## Decisions

- **Storage: a separate wiki branch in the same store repository, following
  the native git model** (board (377) b). Pages are markdown files on one
  shared wiki ref (for example `refs/heads/tracker/wiki`); history, rename,
  delete, attachments, diff, blame, and search come from git and plain file
  tools. Merging concurrent edits is git's own 3-way merge: non-overlapping
  edits merge automatically, and a person resolves only real text conflicts.
  The store repository's auth and sync are shared.
  Rejected: wiki pages as a tracker event kind (`page` / `page.edit`) in the
  ledger protocol (whole-body register with a concurrent-change marker, or a
  deterministic diff3 over the causal past; more engine surface for what git
  already does); a wiki repository separate from the store (second auth and
  sync path).
- **Edit permission: any store member edits any page** (board (375) a). With
  the native git model, per-page locks or per-commit validation cannot be
  enforced on a shared linear history anyway.
- **Recovery over prevention** (board (375) note): vandalism is rare, but
  recovering from it must be convenient. The user expects to ask an agent to
  do it ("어차피 에이전트한테 시킬 거니까 아예 UI도 필요 없을 수도"), so the
  recovery path (identify the offending commits, restore pages to an earlier
  state through new commits) is an agent-usable tool surface first; a UI is
  optional.

## Constraints

- No change to the tracker event protocol or `wstracker` engine; the wiki
  adds no event types.
- The shared wiki ref is append-only for clients (no history rewrite); a
  rejected push resolves by fetch, merge, and re-push, never by force.

## Open Questions (unsettled; not decisions)

- Conflict handling when the daemon's automatic merge hits a text conflict:
  where the conflict waits and who resolves it (the user in a UI, or an agent).
- Commit signing: whether to verify wiki commit signatures against the roles
  file keys and show unverified commits with a warning.
- Mentions and notices: deriving `@mention` notices from new wiki commits'
  diffs, alongside the tracker's derived notices.
- Links between items and pages (`[[wiki:path]]` in item bodies, item ids in
  pages) and the drawer surface (a Wiki tab).
- The agent tool surface (read, search, edit, revert) and its MCP exposure.

## Phases

### Phase 1: plan the wiki branch

Settle the open questions above with the user before planning
implementation.
