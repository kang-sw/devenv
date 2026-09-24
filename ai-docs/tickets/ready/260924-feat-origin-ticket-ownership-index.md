---
title: Origin-backed ticket ownership index (MVP coordination overlay)
related:
  260924-research-origin-ticket-ownership-index: source research; its Outcome Ledger is the authority for this ticket
  260917-feat-ticket-assignee-awareness: existing advisory assignee gate this overlay complements
  260806-feat-worktree-ticket-scope: sparse-checkout scope tooling that gains an acquire side effect
  260728-research-duplicate-ticket-stem-silent-resolve: duplicate-stem behavior left unchanged here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: dc5ffd168dee017f
sage-review-completeness-reviewed: dc5ffd168dee017f
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
- **Writes.** Each index version is one commit whose parent is the previous
  tip.
  - The chain preserves the audit trail, such as takeover records, across later
    writes. CAS already needs the expected old oid.
  - Push with `git push --force-with-lease=<ref>:<expected-oid>` and
    `--no-verify`.
  - On rejection, fetch the ref, re-apply the operation to the new tip, and
    retry. The retry count is bounded, and exhaustion returns a clear error.
  - Creation uses an empty expected value (must-not-exist).
  - Local access is git plumbing only (`hash-object`, `mktree`, `commit-tree`,
    `update-ref`). There is no checkout and no worktree.
  - The local cache is a local ref, so every worktree of the clone shares it.
- **Remote git environment.** Every remote git command runs non-interactively
  with a hard timeout:
  - `GIT_TERMINAL_PROMPT=0`
  - `GIT_SSH_COMMAND` with `-o BatchMode=yes`, preserving any configured SSH
    command
  - This applies on the query path and on the piggyback write path.
  - No remote git command may prompt the user or block a ticket tool
    indefinitely.
  - The hard-timeout value for mutating tools is the implementer's choice. The
    A4 and E10 assertions use that chosen bound.
- **Discovery.** One `git ls-remote` against a code-constant candidate list.
  This ticket implements only tier 1 (the custom ref).
  - Shape discovery as a list so a later tier 2 branch fallback
    (`refs/heads/__<ns>_v1`) can be appended.
  - No `AGENTS.md` pointer.
  - The remote is `origin` unless ws already has a configured remote concept to
    reuse. Do not hard-code assumptions beyond that.
  - A repository with no `origin` remote is silently index-absent (local-only
    repositories, per `260503`).
  - Absence is cached per clone with a TTL, so a project that never ran init
    does not pay a remote round trip on every ticket tool call.
- **Enablement.**
  - An explicit init verb creates the ref and bulk-registers every currently
    open ticket (idea/todo/ready) found on the origin review-track.
  - Init resolves the review-track from origin's tree, not the local checkout:
    it reads the `review-track` declaration from the origin default branch's
    `AGENTS.md`, falling back the same way `wsreview.ResolveTrack` does.
  - Nothing auto-creates the ref.
  - When the ref is absent, all ticket tools keep current behavior and output
    byte-for-byte.
  - **Legacy mock.** In an index-absent project, `tickets.acquire` and
    `tickets.release` are a silent mock: they perform no validation and no
    write, and return only a plain `ok` (text `ok`; JSON `{"status":"ok"}`).
    - The response carries no index-absent report, marker, or setup hint.
      A downstream project that never opted in sees nothing new.
    - The override params added to `tickets.move` and `tickets.close` are
      ignored silently in an index-absent project.
    - Index-absent covers: no `origin` remote, no ref found (including the
      cached absence), and an unreachable remote when this clone has never
      seen an index.
  - When the ref is present, it is used automatically.
  - **Setup guidance appears only at bootstrap.** The init verb stays a
    public tool that may be called directly; only the guidance is confined.
    The init verb has a read-only check mode that reports the index state
    without pushing anything: `initialized`, `uninitialized` (origin
    reachable, no ref), `no-origin`, or `unreachable`.
    - The check bypasses the absence cache and runs a live `ls-remote`, so
      the bootstrap prompt reflects the remote's real state. An `initialized`
      result may refresh the local cache ref; the other states leave no local
      trace (A12).
    - Init against an empty or unpushed origin (no default branch, no
      `refs/remotes/origin/HEAD`, no `AGENTS.md` on origin) falls back through
      the review-track resolution chain and creates the ref with zero
      registrations rather than failing. Later piggyback registration and
      acquire cover tickets not yet pushed.

    The bootstrap playbook (`lead-bootstrap`) runs this check and is the one
    place the uninitialized state is loud:
    - `uninitialized`: explain what the index does and that init pushes a ref
      to `origin` with the user's credentials, then ask the user whether to set
      it up now. On yes, run init; if init fails (for example a rejected push),
      report the failure and continue the rest of bootstrap. On no, change
      nothing.
    - `no-origin`: one line saying the index can be set up by re-running
      bootstrap once an `origin` exists.
    - `unreachable`: one line saying the check could not reach `origin`.
    - `initialized`: one line confirming the index is active.
    - Only users who run bootstrap (new projects, or users aware of the
      feature) meet the prompt. No other tool or playbook surfaces setup
      guidance.
  - There is no opt-out setting in this ticket. Declining at bootstrap records
    nothing, so a later bootstrap run asks again.
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
  - **Owner identity is the full triple `(email, clone_id, track)`.** Every
    "caller's own" comparison (view, queue, guard, release, impl record) uses
    the triple. Two collaborators on the same branch name are different owners.
  - Worktree paths are never published. They are derived locally.
  - Stage (idea/todo/ready), project settings, and terminal state are **not**
    stored in this ticket. The schema must leave room to add them in a later v2
    without migration.
- **Registration.**
  - These mutating tools piggyback an idempotent "register stem if absent"
    onto their write:
    - `tickets.create_empty`
    - `tickets.move`
    - `tickets.close`
    - `tickets.sage_stamp`
    - `tickets.acquire`
  - `tickets.query` is read-only and never registers.
    - The read-path sync and piggyback registration are separate paths that
      share the local cache ref.
    - Any write fetches the tip for its CAS, and so refreshes the cache as a
      side effect.
  - A stale view is caught at acquire, which always CASes against the fresh
    remote tip once the index is known to exist. Within the absence TTL,
    acquire returns the legacy mock without contacting the remote; a lease
    missed in the window after another clone's init is the same accepted
    window as A9.
  - The first registrant wins. A cross-branch stem collision is an accepted
    accident and is not detected.
  - No nonce and no ticket template change.
  - Piggyback failure (offline, rejected, timeout) never fails the host
    operation.
- **Ownership.**
  - Ownership exists only through explicit acquire, or through the close rule
    below. Creating a ticket confers none.
  - The unit is the track, so two worktrees of the same person are distinct
    owners.
  - The same model applies to every category: feat, bug, refactor, chore,
    research, and epic. There is no epic-to-child inheritance.
- **Track resolution.** Acquire records `implementMergeRootFor(<current
  branch>)` as the track (`agents-plugin-tool/internal/mcp/implement_resolver.go`).
  - This is one level only: `goal/*` is its own track.
  - A rootless `impl/<stem>` or any `implement/<stem>` is refused unless the
    caller passes `track` explicitly.
  - A detached HEAD is refused.
  - Non-acquire tools (view, queue filter, move/close guard, release) derive the
    caller's track with the same resolution. An unresolvable track (rootless
    impl, `implement/*`, detached HEAD) makes the caller match no lease: it is
    never "the owner".
- **Acquire.**
  - Preconditions: the ticket file exists in the caller's checkout, and the
    stem is registered. For an unregistered stem, acquire registers and leases
    in the same CAS commit.
  - Acquire fetches the origin review-track together with the index. It refuses
    a stem that is absent from the index but already sits under `.done/` or
    `.dropped/` on the origin review-track, and tells the caller to pull.
    - This closes the stale-checkout hole: the ticket was implemented, merged,
      and pruned elsewhere, while it still looks `ready/` locally.
    - The review-track tree is the done list. Keep no done list or tombstones
      in the index.
    - Resolve the review-track origin-first, the same way as init:
      1. the `review-track` declaration in `AGENTS.md` of origin's default
         branch (read from the local `refs/remotes/origin/HEAD` tree when no
         fetch is in flight)
      2. then the fallback chain of `wsreview.ResolveTrack`
         (`agents-plugin-tool/internal/wsreview/track.go`)

      Init, acquire, pruning, and the query hint all use this one resolution.
  - The lead performs acquire from its track.
  - **Worker impl record.** The worker calls `tickets.acquire` from its impl
    branch. When the impl branch's merge root, email, and clone_id match the
    existing lease's `(email, clone_id, track)`, acquire records only
    `impl: { branch }` and changes no ownership. The worker playbooks carry
    this call.
  - Re-acquire by the same owner triple is idempotent and refreshes
    `touched_at`.
  - **Unreachable remote.** Acquire fails loudly only when this clone has seen
    an index (its local cache ref exists).
    - When no index was ever cached, or absence was the last cached result,
      acquire treats the index as absent and returns the legacy mock's plain
      `ok`. This keeps "No ref, no change" for projects that never ran init
      while offline.
    - There is no offline queue in this ticket.
- **Owner-conflict matrix.** One matrix governs `tickets.acquire`,
  `tickets.move`, and `tickets.close` when the ticket's lease is held by an
  owner other than the caller's triple:

  | existing holder vs caller | acquire | move / close |
  |---|---|---|
  | different email | refuse; allowed with `dangerously_override_lease_status: true` + non-empty `reason` | refuse; allowed with the same flag + reason |
  | same email, different clone | proceed with warning | proceed with warning |
  | same clone, different track | refuse; allowed with flag + reason | proceed with warning |

  - An override records the previous holder and the reason in the index
    commit.
  - **Effect on the lease.**
    - An acquire takeover (flag, or the same-email-different-clone row) moves
      the lease to the caller's triple.
    - Move and close never transfer the lease, whether they proceed on a
      warning or under the override. Close sets `phase: closed` on the existing
      holder's lease, and move leaves the lease unchanged.
    - An overridden move or close records the actor and the reason in the
      index commit.
  - Staleness (`touched_at`) is shown as information only; there is no time
    threshold.
  - Shipped playbook text sets the flag only on an explicit user instruction.
- **Release and close.**
  - `tickets.release` removes only a lease held by the caller's own triple.
    Releasing another owner's lease is refused; that path is an acquire
    takeover.
  - `tickets.close` does not delete the lease. It sets `phase: closed` (pending
    landing).
  - `tickets.close` on an unleased ticket creates a `closed` lease owned by the
    caller's triple.
  - Queue and view treat a `closed` lease as owned until pruning removes it.
    This prevents the window between an impl-branch close and its merge.
- **Pruning.**
  - Remove a registration, with its lease, once the ticket's file sits under
    `.done/` or `.dropped/` on the origin review-track.
  - Landed-closure pruning runs on every index write, as part of the same CAS
    commit. It evaluates registrations against the review-track tree:
    - the freshly fetched tree when the operation already fetched it (acquire)
    - otherwise the local `refs/remotes/origin/<review-track>`
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
  - When the index state is unknown (no cache and the fetch failed), ownership
    is shown as unknown, never as unowned.
  - A server-side ownership filter parameter on `tickets.query` restricts
    results to caller-owned plus unowned tickets. It follows the
    `assigned_to_me` precedent (`260917`) and composes with it.
    - Queue selectors (`ticket-selector`, `ticket-batch-selector`) use this
      filter.
    - Tickets whose ownership is unknown (remote unreachable, no cache) are
      included, because acquire is the authoritative gate.
    - A `closed` lease counts as owned.
  - The collapsed default is applied only to compact discovery text: tickets
    owned by the caller plus unowned tickets are listed in full, and the rest
    collapse to a count with one-line entries. JSON output and point-resolve
    are never collapsed.
  - Query and queue apply the origin-closed check as a hint, using the local
    `refs/remotes/origin/<review-track>` tree with no network.
    - A ticket closed there shows as closed on origin (local checkout stale).
    - The ownership filter and the queue exclude it.
    - Acquire stays the authoritative gate.
- **Read path.** `tickets.query` fetches the index ref with a short timeout and
  caches it with a TTL.
  - On fetch failure it serves the cache marked stale, with its age.
  - Query must not block beyond roughly 2 seconds on network trouble.
  - There is no background refresh loop.
  - The TTL value is the implementer's choice.
- **Integration.**
  - The owner is surfaced in `tickets.query`, `git.status`, and queue
    selection. The `git.status` format and the placement of the GC stale flag
    are the implementer's choice.
  - `lead-run` acquires before spawning a worker.
    - On a refusal or a loud failure, `lead-run` stops and reports.
    - It sets the override flag only on explicit user instruction.
    - The legacy mock's plain `ok` proceeds like any successful acquire;
      `lead-run` text needs no index-absent branch.
  - `lead-bootstrap` runs the init check and handles each state as described
    under Enablement. This is the only shipped playbook that mentions setup.
  - The worker's impl-record acquire is informational, because ownership is
    already held by the lead's track. A refusal or network failure there is
    reported and never fails the worker's run.
  - A `lead-scope-worktree` scope assignment also acquires the scoped tickets
    for the worktree's track. It calls acquire unconditionally with no
    index-mode check; in an index-absent project the mock's `ok` makes this
    harmless. A partial refusal (some tickets
    held by other owners) reports each refused stem and keeps the successful
    acquires.
- **Tool surface sketch.** Parameter names follow the existing snake_case MCP
  convention.

  ```text
  tickets.acquire(session_key, ticket_stem, track?, dangerously_override_lease_status?, reason?)
  tickets.release(session_key, ticket_stem)
  tickets.move(..., dangerously_override_lease_status?, reason?)    # added params
  tickets.close(..., dangerously_override_lease_status?, reason?)   # added params
  tickets.query(..., <ownership filter param>?)                     # added param; name chosen by implementer
  <init verb>(session_key, <check param>?)   # names chosen by implementer within tickets.* naming;
                                             # check mode is read-only and returns the index state
  ```

## Integration Test Scenarios

Each scenario is an automated integration test. The harness uses:

- a local bare repository as `origin`
- several clones with distinct `user.email` and `clone_id`
- several worktrees per clone
- controllable remote failure: an unreachable URL, a credential-requiring
  remote, and injected push rejection

Each phase implements the scenarios mapped to it. IDs are referenced from the
phase verification lists.

Phase 1 has no tool surface, so it implements tool-phrased scenarios at library
level (A2, A3, A4, F1, F2). Later phases re-run them at tool level.

**A. Enablement and index absence** (Phases 1–2)

- A1. There is no index ref. Every existing ticket tool's output is
  byte-identical to the pre-change output. `tickets.acquire` and
  `tickets.release` return exactly the plain `ok` (text and JSON), with no
  index-absent wording, even for a stem with no local file. `tickets.move` and
  `tickets.close` with the override params set produce output identical to the
  call without them.
- A2. There is no index ref, and repeated ticket tool calls happen within the
  absence TTL. At most one remote discovery call is made (count remote
  invocations).
- A3. There is no `origin` remote. Every ticket tool behaves as index-absent
  with no error and no remote call.
- A4. The remote is unreachable, and this clone already has a local cache
  ref (it has seen the index; without one, A10 applies).
  - `tickets.query` returns within the bound.
  - Mutating tools succeed without registration within the timeout.
  - `tickets.acquire` fails loudly.
- A5. The remote requires credentials. No interactive prompt occurs, and the
  command fails fast.
- A6. Two clones run init concurrently. Exactly one creates the ref, and the
  other adopts it without error.
- A7. Init bulk-registers the open tickets from the origin review-track. It
  excludes tickets that exist only in a stale or unpushed local branch, and
  those closed on origin.
- A8. The local `AGENTS.md` `review-track` differs from origin's. Init,
  acquire, pruning, and the query hint all use origin's.
- A9. The index is created by another clone after this clone cached absence.
  It is discovered once the absence TTL expires.
- A10. A project that never ran init goes offline after its absence TTL
  expires. `tickets.acquire` returns the plain `ok`, and does not fail.
- A11. A clone that has seen an index (cache ref present) goes offline.
  `tickets.acquire` fails loudly.
- A12. The init check mode reports `uninitialized`, `initialized`,
  `no-origin`, and `unreachable` in the matching setups. In every state it
  pushes nothing. In the three non-initialized states it creates no local
  cache ref, and after an `uninitialized` check the remote still has no ref.

**B. CAS and concurrency** (Phase 1, with the verbs in Phase 2)

- B1. Two clones acquire the same unregistered stem simultaneously. Exactly one
  wins, and the loser is refused with the winner's identity. The index shows no
  partial state.
- B2. Two clones write different stems concurrently. Both writes land after
  retry, with no lost update.
- B3. N clones × M mixed operations run concurrently. The final state equals
  the serial union, and every version's parent is its predecessor.
- B4. Piggyback registration races an acquire of the same stem. There is one
  registration, and the lease belongs to the acquirer.
- B5. GC races an acquire. A leased entry is never pruned, and GC runs at most
  once per period.
- B6. Push rejection persists (injected). Retries are bounded, a clear error is
  returned, and the local cache is not corrupted.
- B7. A takeover audit record survives later writes: it is reachable in the
  index commit chain.

**C. Identity matrix** (Phases 2–3)

- C1. Different email, acquire:
  - without the flag: refused
  - with the flag but no `reason`: refused
  - with the flag and `reason`: succeeds, and the audit record names the
    previous holder and the reason
- C2. Same email, different clone, acquire: proceeds with a warning.
- C3. Same clone, different track (two worktrees of one person), acquire:
  refused without the flag, succeeds with flag + reason.
- C4. Same owner triple re-acquires: idempotent, and `touched_at` is refreshed.
- C5. Two collaborators on the same track name (both on `develop`, with
  different email and clone): never treated as the same owner in view, queue
  filter, guard, release, or impl record.
- C6. Move/close guard:
  - different email: refused without flag + reason, succeeds with them
  - same email, different clone: warning
  - same clone, different track: warning

  In every allowed case the holder's lease is not transferred: close sets the
  holder's lease to `closed`, and an override records the actor and the
  reason.
- C7. Release of one's own lease removes it. Release of another triple's lease
  is refused.
- C8. `user.email` changes between acquire and a later operation on the same
  clone. The matrix applies to the current identity (treated as a different
  email).
- C9. A fresh clone generates `clone_id` once. A worktree added later shares
  it. Two separate clones of one person get distinct ids.

**D. Track resolution** (Phase 2)

- D1. Acquire from `track/my-view` records the track with the slash intact.
- D2. Acquire from `impl/track/my-view/<key>` by the holder's email and clone
  records only `impl`. The same branch under a different identity goes through
  the matrix.
- D3. Acquire from `goal/<x>` records `goal/<x>` as the track.
- D4. Acquire from a rootless `impl/<stem>` or from `implement/<stem>` is
  refused without `track`, and succeeds with an explicit `track`.
- D5. Acquire from a detached HEAD is refused.

**E. Lease lifecycle, landing, and staleness** (Phases 2–4)

- E1. Full cycle:
  1. acquire from a track
  2. worker impl record
  3. close on the impl branch sets `phase: closed`, and the queue still treats
     the ticket as owned
  4. merge into the review-track and push
  5. the next index write prunes the entry
- E2. `tickets.close` on an unleased ticket creates a `closed` lease owned by
  the caller.
- E3. A `closed` lease whose impl branch is never merged stays owned. GC does
  not prune it and may flag it stale.
- E4. Stale-checkout hole: the ticket is closed and pruned on origin, and a
  stale local checkout shows it as `ready/`.
  - With a fresh remote-tracking ref, query shows "closed on origin" and the
    queue excludes it.
  - With a stale remote-tracking ref, query shows it unowned, but acquire
    refuses after its fetch.
- E5. The same as E4 for a ticket dropped on origin.
- E6. GC predicates:
  - older than 30 days, unleased, and open on no origin branch: pruned
  - open on some unmerged origin branch: kept
  - leased: never pruned
  - `last_gc` younger than 30 days: no GC run
- E7. A registration pruned by GC for a local-only ticket is re-registered by
  the next piggyback without error.
- E8. Two branches create the same stem and both register. A single
  registration results, with no error.
- E9. A ticket file is deleted locally without close. Query hides it
  (intersection), and the registration persists until pruning or GC.
- E10. A mutating tool runs offline in index mode. The folder operation
  proceeds, registration is skipped, and the tool does not fail.

**F. Read path and cache** (Phases 1 and 3)

- F1. Within the TTL, query makes no remote fetch (count invocations).
- F2. The TTL has expired and the fetch fails. The stale cache is served with
  its age marker within the bound.
- F3. A write in worktree A is visible to query in worktree B of the same clone
  without a fetch, through the shared local cache ref.
- F4. Another clone's write is visible after TTL expiry.
- F5. There is no cache and the remote is unreachable. Ownership is shown as
  unknown, never as unowned. The ownership filter includes those tickets.

**G. View and queue** (Phase 3)

- G1. Each of the three ownership levels renders correctly. The
  other-local-worktree level shows the local path, and that path never appears
  in the remote index.
- G2. Compact discovery collapses tickets owned by others. JSON output with the
  ownership filter returns only own plus unowned. Point-resolve is never
  collapsed.
- G3. The queue selectors pick only own or unowned tickets. A `closed` lease
  counts as owned, and origin-closed tickets are excluded.
- G4. Indexed stems with no local file never appear (intersection).
- G5. The ownership filter composes with `assigned_to_me`.

**H. Scope integration** (Phase 4)

- H1. Scope-assignment bulk acquire where some tickets are held by a different
  email. Each refused stem is reported, and the rest are acquired.

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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260924-research-origin-ticket-ownership-index (2026-09-24, Confirmed Decisions): "Lease phase. A lease carries a binary phase, active or closed (pending landing)... tickets.close sets closed rather than deleting the lease." — bearing: supports
- 3c3b99fc (2026-09-24, commit): "User settled derivation queue: single feat with four sequential phases; tier-1 custom ref only (discovery shaped as a list); same-email clone takeover in MVP; short-timeout fetch + TTL read path" — bearing: supports
- 754a8e23 (2026-09-24, commit): "MVP limited to registration + ownership overlay because the user needs branch collaboration within a week; status authority deferred as highest-risk cross-cutting refactor, with v1 schema reserving room for it." — bearing: supports
- 260921-feat-ws-impl-branch-identity-resolver (2026-09-21, Decisions): "worktree.acquire is unchanged. It stays ticket-unaware; the lead resolves the canonical branch first and passes it as target_branch verbatim. Branch-name authority lives in the resolver, not in acquire." — bearing: constrains
- 260917-feat-ticket-assignee-awareness (2026-09-17, Decisions): "Selector: hard-skip via a server-side filter. The selector calls tickets.query with an assignee filter so others'-assigned tickets are omitted from its result entirely" — bearing: constrains
- 523054ba (2026-08-10, commit): "Parsing rule is strip "impl/", split on the LAST "/": everything before is <merge-root> (may itself contain "/"), final segment is <stem>." — bearing: supports
- 260806-feat-worktree-ticket-scope (2026-08-06, Result): "git sparse-checkout check-rules needs git >= 2.42. Below that the destination pre-flight fails open and the refusal arrives after the source write" — bearing: constrains
- 260503-feat-agents-plugin-agent-session-runtime (2026-05-03, Decisions): "Do not key project identity by remote origin. Local checkout identity is more stable for this workflow than remote identity because repos may be local-only, forked, mirrored" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/implement_resolver.go, new index library under agents-plugin-tool/internal/, internal/mcp ticket handlers in server.go, agents-plugin-tool/internal/wsdoc/tickets*.go, agents-plugin/rsrc/lead-run, lead-scope-worktree, lead-bootstrap, ticket-selector, ticket-batch-selector plus wsflow and agents-plugin-pi rsrc mirrors |
| scope.surface | public-interface | new MCP tools tickets.acquire, tickets.release, init verb; changed tickets.query, tickets.move, tickets.close, git.status output; cross-module across mcp, wsdoc, wsgit, and shipped playbooks |
| scope.new_public_symbol | yes | tickets.acquire, tickets.release, init verb within tickets.* naming |
| scope.new_type_contract | yes | remote index record schema v1 registration, lease, meta and the acquire signature with dangerously_override_lease_status and reason |
| scope.test_surface | new-files | no index tests exist; existing agents-plugin-tool/internal/mcp/tickets_*_test.go, internal/wsdoc/tickets_*_test.go must pass unchanged; new bare-remote race tests needed |
| complexity.reuse_points | confirmed | implementMergeRootFor agents-plugin-tool/internal/mcp/implement_resolver.go#L882-L890, wsgit.CurrentUserEmail agents-plugin-tool/internal/wsgit/git.go#L67, review-track resolution agents-plugin-tool/internal/wsreview/track.go |
| complexity.side_effect_risk | high | first remote git write in ws-mcp, grep for push, fetch, ls-remote in non-test agents-plugin-tool/internal Go returned nothing, and piggyback writes attach to every ticket tool |
| risk.correctness | high | CAS retry convergence, lease phase semantics, prune and GC predicates, and the 2 second read bound must all hold under races and network failure |
| risk.fit | moderate | overlay coexists with the assignee gate and sparse-checkout scope tooling, and playbook edits must stay in sync across wsflow and agents-plugin-pi mirrors |
| risk.test | high | race and offline behavior need bare-remote integration tests, the no-ref path must stay byte-identical across all ticket tools, and the live GitHub probe is not reproducible in CI |
| risk.security_or_contract | high | pushes to the user's remote with their credentials and --no-verify, publishes user email, adds a takeover override flag and new MCP tool API |

## Phases

### Phase 1: Live probe and index storage core

**Goal.** A tested library for discovering, reading, and CAS-writing the index
ref. No tool surface changes.

**Live probe** against this repository's GitHub origin (`origin`, SSH):

- create, fast-forward, non-fast-forward rejection without force, and delete of
  a throwaway custom ref
- two racing `--force-with-lease` pushes, confirming exactly one wins
- `--no-verify` push behavior
- whether a custom-ref push triggers GitHub Actions or webhooks
  - This repository's workflows filter `push` to `develop` and `v*` tags, so
    "no run observed" proves nothing.
  - Use an unfiltered `on: push` workflow on a throwaway branch, or inspect
    webhook/event delivery.

Use a throwaway probe namespace and delete every probe ref and throwaway branch
afterward. Record the results in this phase's Result. A GitLab CE probe is out
of scope.

If a result breaks a design assumption, stop and report before building the
library. Examples:

- custom-ref pushes rejected
- the CAS race not holding
- custom-ref pushes triggering Actions or webhooks

**Library.** Implement:

- discovery: candidate list (tier 1 only), absence cache, no-origin handling
- the non-interactive remote git environment with hard timeouts
- plumbing read/write with chained commits
- the local cache ref
- the bounded CAS retry loop
- `clone_id` generation and storage
- the record model with schema version
- the TTL read cache with stale fallback

**Verification.** Scenarios A2, A3, A4 (library level), A5, A9, B2, B3, B6,
F1, F2, F3, F4, C9; missing ref reported as absent, not as an error.

### Phase 2: Registration, ownership verbs, and init

**Goal.** Add `tickets.acquire`, `tickets.release`, and the init verb:

- the init verb's read-only check mode
- the legacy mock for acquire/release in index-absent projects
- track resolution
- the owner-conflict matrix for acquire
- the worker `impl` record through acquire
- `tickets.close` setting or creating a `closed` lease
- piggyback registration on the listed mutating tools
- landed-closure pruning on every write
- the monthly GC

**Verification.** Scenarios A1 (acquire/release plain `ok`; the override-param
clause waits for Phase 3), A4 (tool level), A10, A11, A12, A6, A7, A8, B1, B4, B5, B7,
C1, C2, C3, C4, C7, C8, D1–D5, E1 (through close), E2, E3, E4 (acquire
refusal), E5 (acquire refusal), E6, E7, E8, E10. Every existing ticket-tool test must pass
unchanged with no ref present.

### Phase 3: Query view, queue filter, and move/close guards

**Goal.**

- `tickets.query` shows the authority-local intersection with the three
  ownership levels and the unknown state.
- The ownership filter parameter.
- Compact-text collapse, and stale-cache marking.
- The origin-closed hint.
- The owner shown in `git.status`.
- `tickets.move` and `tickets.close` guards per the matrix, with the override
  params.

**Verification.** Scenarios C5, C6, E4 (query hint), E5 (query hint), E9, F5,
G1–G5; tool-level re-runs of A2, A3, F1, F2. The no-ref path stays
byte-identical (A1 re-run, including its override-param clause for
`tickets.move` and `tickets.close`, which Phase 2 cannot yet exercise).

### Phase 4: Playbook integration and dogfood

**Goal.**

- Shipped playbook changes:
  - `lead-run` acquires before worker spawn.
  - The worker playbooks (`ticket-worker`, `ticket-worker-elevated`) record the
    impl through `tickets.acquire` from the impl branch.
  - `lead-scope-worktree` acquires on every scope assignment, with no
    index-mode check.
  - The queue selectors use the ownership filter.
  - `lead-bootstrap` runs the init check after its mode handler (every mode
    except `refuse`) and handles each state per the Enablement decision: ask
    the user on `uninitialized`, one line otherwise.
  - Takeover flag discipline: explicit user instruction only.
- Mirror to wsflow where `wsflow-mirroring.md` requires.
- Keep `agents-plugin-pi/rsrc/` byte-identical for every changed playbook, so
  that `TestPiMirrorUpToDate`
  (`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`) passes.
  - `bump-ws-version.sh` resyncs the Pi mirror on version bumps.
  - Check whether a standalone resync path exists before hand-copying.
- Init the index on this repository's origin and dogfood one full cycle.

**Verification.**

- Scenario H1, and E1 end-to-end.
- Playbook text covers the refusal and failure handling for `lead-run` and for
  the worker impl-record acquire.
- `lead-bootstrap` text covers all four check states, asks before init, and
  changes nothing on decline. No other playbook mentions index setup.
- Playbook and package tests pass, including the wsflow drift tests and
  `TestPiMirrorUpToDate`.
- The dogfood cycle is recorded in the Result: init, acquire from a track,
  worker impl record, close to `phase: closed`, merge, and prune on landing.

## Sage Review Round 1 (2026-09-24)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Owner-conflict matrix incomplete (move/close guard; same-clone cross-track acquire) | important | missing |
| 2 | Index discovery imposes network cost on uninitialized projects | important | autonomous |
| 3 | Landed-closure pruning has no trigger | minor | autonomous |
| 4 | Worker impl sub-record has no caller or path | minor | autonomous |
| 5 | Queue filter mechanism diverges from assignee-filter precedent | minor | autonomous |
| 6 | Caller's track comparison must use full owner identity | minor | autonomous |
| 7 | Init review-track resolution reads local state; research says origin | minor | autonomous |
| 8 | Live probe cannot answer GitHub Actions question as designed | minor | autonomous |
| 9 | Index commit ancestry for audit trail | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | move/close owner guard says 'block or warn' | important |
| 2 | acquire/release behavior with no index ref unstated | important |
| 3 | How the worker writes the impl sub-record is unspecified | important |
| 4 | Landed-closure pruning trigger not stated | minor |
| 5 | Open-ended piggyback tool list | minor |
| 6 | No contingency if Phase 1 live probe finds a problem | minor |
| 7 | Unspecified tunables and display shapes | minor |
