---
title: Origin-backed ticket ownership index (MVP coordination overlay)
related:
  260924-research-origin-ticket-ownership-index: source research; its Outcome Ledger is the authority for this ticket
  260917-feat-ticket-assignee-awareness: existing advisory assignee gate this overlay complements
  260806-feat-worktree-ticket-scope: sparse-checkout scope tooling that gains an acquire side effect
  260728-research-duplicate-ticket-stem-silent-resolve: duplicate-stem behavior left unchanged here
---

# Origin-backed ticket ownership index (MVP coordination overlay)

## Background

Status is the ticket's directory, so status lives inside each branch's history.
Parallel worktrees and collaborators cannot see who owns a ticket, and the same
ticket gets implemented twice. This ticket builds the MVP from
`260924-research-origin-ticket-ownership-index`: a registration and ownership
index kept in one custom ref on the git remote. It is written with
compare-and-swap pushes using the user's own git credentials, so it works on
any backend and needs only developer permission.

Directory position remains the status authority. The index only overlays
registration and ownership. When no index exists on the remote, every tool
behaves exactly as it does today.

## Decisions

- **Storage.** One ref under a protocol-named, versioned namespace:
  `refs/<ns>/v1/...`.
  - The name is not derived from any distribution identity such as the
    marketplace, repo, or user name.
  - Uniqueness is what matters: a readable prefix plus an arbitrary word is
    fine. The implementer chooses it and records it as a single code constant.
  - Reserved namespaces must not be used:
    - GitHub `refs/pull/`
    - GitLab `refs/merge-requests/`, `refs/pipelines/`, `refs/environments/`,
      `refs/keep-around/`, `refs/tmp/`, `refs/remotes/`
    - Gitea `refs/pull/`, `refs/for/`
- **Writes.** A single commit tree per index version.
  - Push with `git push --force-with-lease=<ref>:<expected-oid>` and
    `--no-verify`.
  - On rejection, fetch the ref, re-apply the operation to the new tip, and
    retry.
  - Creation uses an empty expected value (must-not-exist).
  - Local access is git plumbing only (`hash-object`, `mktree`, `commit-tree`,
    `update-ref`). There is no checkout and no worktree.
  - The local cache is a local ref, so every worktree of the clone shares it.
- **Discovery.** One `git ls-remote` against a code-constant candidate list.
  This ticket implements only tier 1 (the custom ref).
  - Shape discovery as a list so a later tier 2 branch fallback
    (`refs/heads/__<ns>_v1`) can be appended.
  - No `AGENTS.md` pointer.
  - The remote is `origin` unless ws already has a configured remote concept to
    reuse. Do not hard-code assumptions beyond that.
- **Enablement.**
  - An explicit init verb creates the ref and bulk-registers every currently
    open ticket (idea/todo/ready) found on the origin review-track.
  - Nothing auto-creates the ref.
  - When the ref is absent, all ticket tools keep current behavior and output
    byte-for-byte.
  - When the ref is present, it is used automatically.
  - There is no opt-out setting in this ticket.
- **Record model.** A stem maps to a registration, which carries an optional
  lease:

  ```text
  registration: { stem, registered_at }
  lease:        { email, clone_id, track, phase: active | closed, touched_at,
                  impl?: { branch } }
  meta:         { schema: 1, last_gc }
  ```

  - `email` comes from `git config user.email`.
  - `clone_id` is a random id generated once per clone and stored in
    `.git/config`. All worktrees of the clone share it.
  - `track` is the owning work line.
  - Worktree paths are never published. They are derived locally.
  - Stage (idea/todo/ready), project settings, and terminal state are **not**
    stored in this ticket. The schema must leave room to add them in a later v2
    without migration.
- **Registration.**
  - Every ticket-touching ws tool call piggybacks an idempotent "register stem
    if absent": query, move, close, sage stamp, acquire, and the rest.
  - The first registrant wins. A cross-branch stem collision is an accepted
    accident and is not detected.
  - No nonce and no ticket template change.
  - Piggyback failure (offline, rejected) never fails the host operation.
- **Ownership.**
  - Ownership exists only through explicit acquire. Creating a ticket confers
    none.
  - The unit is the track, so two worktrees of the same person are distinct
    owners.
  - The same model applies to every category: feat, bug, refactor, chore,
    research, and epic. There is no epic-to-child inheritance.
- **Track resolution.** Acquire records `implementMergeRootFor(<current
  branch>)` as the track (`agents-plugin-tool/internal/mcp/implement_resolver.go`).
  - This is one level only: `goal/*` is its own track.
  - A rootless `impl/<stem>` is refused unless the caller passes `track`
    explicitly.
- **Acquire.**
  - Preconditions: the ticket file exists in the caller's checkout, and the
    stem is registered. For an unregistered stem, acquire registers and leases
    in the same CAS commit.
  - The lead performs acquire.
  - The worker, from its impl branch, records only the `impl` sub-record on a
    lease its own track already holds.
  - Acquire fails loudly when the remote is unreachable. There is no offline
    queue in this ticket.
- **Takeover.**
  - When the lease belongs to a different email, acquire refuses unless
    `dangerously_override_lease_status: true` and a non-empty `reason` are
    given. The previous holder and the reason are recorded in the index commit.
  - The same email on a different clone takes over with a warning and no flag.
  - Staleness (`touched_at`) is shown as information only; there is no time
    threshold.
  - Shipped playbook text sets the flag only on an explicit user instruction.
- **Release and close.**
  - `tickets.release` removes the caller track's lease.
  - `tickets.close` does not delete the lease. It sets `phase: closed` (pending
    landing).
  - Queue and view treat a `closed` lease as owned until pruning removes it.
    This prevents the window between an impl-branch close and its merge.
- **Pruning.**
  - Remove a registration, with its lease, once the ticket's file sits under
    `.done/` or `.dropped/` on the origin review-track.
  - Monthly GC is piggybacked on any write when `meta.last_gc` is older than 30
    days, and is itself a CAS. It prunes an entry only when all of these hold:
    - registered more than 30 days ago
    - unleased
    - not present as an open ticket (outside `.done/` and `.dropped/`) on any
      origin branch
  - GC never removes a leased entry; it may flag it stale.
- **Query and view.**
  - `tickets.query` shows the intersection of authority and local: only
    tickets whose file exists in the caller's checkout, annotated with
    ownership.
  - Ownership has three levels:
    - this worktree
    - another worktree of this clone (path computed locally)
    - remote (email and track)
  - The default discovery view lists tickets owned by the caller's track plus
    unowned tickets in full, and collapses the rest to a count with one-line
    entries.
  - Queue selection (`ticket-selector`, `ticket-batch-selector`) picks only
    from caller-owned or unowned tickets.
- **Read path.** `tickets.query` fetches the index ref with a short timeout and
  caches it with a TTL.
  - On fetch failure it serves the cache marked stale, with its age.
  - Query must not block beyond roughly 2 seconds on network trouble.
  - There is no background refresh loop.
- **Integration.**
  - `tickets.move` and `tickets.close` block or warn when another track owns
    the ticket.
  - The owner is surfaced in `tickets.query`, `git.status`, and queue
    selection.
  - `lead-run` acquires before spawning a worker.
  - In index mode, a `lead-scope-worktree` scope assignment also acquires the
    scoped tickets for the worktree's track.
- **Tool surface sketch.** Parameter names follow the existing snake_case MCP
  convention.

  ```text
  tickets.acquire(session_key, ticket_stem, track?, dangerously_override_lease_status?, reason?)
  tickets.release(session_key, ticket_stem)
  <init verb>(session_key)          # name chosen by implementer within tickets.* naming
  ```

## Constraints

- **Manuals.** Read these before editing the paths they cover, per the
  `AGENTS.md` Implementation Conventions table:
  - `ai-docs/manuals/shipped-surface-boundary.md`: everything under
    `agents-plugin/`, `agents-plugin-wsflow/`, and `agents-plugin-tool/`
  - `ai-docs/manuals/ws-mcp.md`: `agents-plugin-tool/internal/mcp/`
  - `ai-docs/manuals/skill-authoring.md`: `agents-plugin/rsrc/`,
    `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`,
    `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`
  - `ai-docs/manuals/wsflow-mirroring.md`: `agents-plugin/rsrc/`,
    `agents-plugin/skills/`, `agents-plugin-wsflow/`
- **Shipped text** must not name this repository, its namespace history, or its
  hosts (Architecture Rule 4).
- **No ref, no change.** With no index ref on the remote, existing tests and
  tool outputs stay unchanged. This is the downstream compatibility contract.
- **No publishing.** Do not store worktree paths or any data beyond the record
  model in the remote index.

## Phases

### Phase 1: Live probe and index storage core

**Goal.** A tested library for discovering, reading, and CAS-writing the index
ref. No tool surface changes.

**Live probe** against this repository's GitHub origin (`origin`, SSH):

- create, fast-forward, non-fast-forward rejection without force, and delete of
  a throwaway custom ref
- two racing `--force-with-lease` pushes, confirming exactly one wins
- whether a custom-ref push triggers GitHub Actions or webhooks
- `--no-verify` push behavior

Use a throwaway probe namespace, delete every probe ref afterward, and record
the results in this phase's Result. A GitLab CE probe is out of scope.

**Library.** Implement discovery (candidate list, tier 1 only), plumbing
read/write, the local cache ref, the CAS retry loop, clone_id generation and
storage, the record model with schema version, and the TTL read cache with
stale fallback.

**Verification.**

- Unit and integration tests against a local bare repository as the remote.
- Two clones racing CAS writes must converge without lost updates.
- A missing ref must be reported as absent, not as an error.

### Phase 2: Registration, ownership verbs, and init

**Goal.** Add `tickets.acquire`, `tickets.release`, and the init verb:

- track resolution
- takeover rules (the dangerous flag with reason; same-email clone takeover
  with a warning)
- worker `impl` sub-record
- `tickets.close` setting `phase: closed`
- piggyback registration on every ticket-touching tool
- pruning on landed closure, and the monthly GC

**Verification.**

- Tests cover each acquire precondition and refusal.
- Tests cover the flag gate and that the audit record lands in the index commit.
- Rootless impl refusal is tested.
- Piggyback failure must not fail the host tool.
- Prune and GC predicates are tested, including never pruning a leased entry.
- Every existing ticket-tool test must pass unchanged with no ref present.

### Phase 3: Query view, queue filter, and move/close guards

**Goal.**

- `tickets.query` shows the authority-local intersection with the three
  ownership levels.
- Its default view shows own-track plus unowned tickets and collapses the rest.
- Stale-cache marking.
- Owner shown in `git.status`.
- Queue selectors restricted to own or unowned tickets.
- `tickets.move` and `tickets.close` owner guards.

**Verification.**

- Tests for each ownership level, for the collapsed view, and for a `closed`
  lease counting as owned.
- Tests for the guard behavior.
- The no-ref path must stay byte-identical.

### Phase 4: Playbook integration and dogfood

**Goal.**

- Shipped playbook changes:
  - `lead-run` acquires before worker spawn.
  - `lead-scope-worktree` acquires on scope assignment in index mode.
  - Takeover flag discipline (explicit user instruction only).
  - Any ticket-selector text touched by the queue filter.
- Mirror the changes to wsflow where `wsflow-mirroring.md` requires.
- Init the index on this repository's origin and dogfood one acquire, close,
  and prune cycle.

**Verification.**

- Playbook and package tests pass, including wsflow drift tests.
- The dogfood cycle is recorded in the Result: init, acquire from a track,
  worker impl record, close to `phase: closed`, merge, and prune on landing.
