---
title: Origin-backed ticket ownership index (MVP coordination overlay)
related:
  260924-research-origin-ticket-ownership-index: source research; its Outcome Ledger is the authority for this ticket
  260917-feat-ticket-assignee-awareness: existing advisory assignee gate this overlay complements
  260806-feat-worktree-ticket-scope: sparse-checkout scope tooling that gains an acquire side effect
  260728-research-duplicate-ticket-stem-silent-resolve: duplicate-stem behavior left unchanged here
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 73073ae7da15060c
sage-review-completeness-reviewed: 73073ae7da15060c
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
- **Offline pending log.** When the clone has seen an index (its local cache
  ref exists) and the remote is unreachable, index writes are recorded in a
  local pending log instead of being dropped. This persists the operations the
  CAS retry loop already re-applies to a new tip.
  - Scope: every index write — piggyback registration, acquire (including the
    worker impl record), release, and close's `phase: closed`.
  - Each entry carries its recording owner triple `(email, clone_id, track)`.
    An override entry also carries the triple of the holder it overrode in the
    cached view, plus the reason.
  - **Offline decisions use the overlaid view.** While offline, acquire and the
    move/close guards evaluate the owner-conflict matrix against the cached
    index with this clone's pending entries overlaid. Two worktrees of one
    clone therefore conflict locally (same clone, different track) exactly as
    they would online.
  - The log is a local ref, shared by every worktree of the clone. Appends and
    clears use local compare-and-swap (`update-ref` with the old value), so
    concurrent worktrees never lose an entry and a clear never removes an entry
    appended after the flush read the log.
  - **Flush.** The next mutating ticket tool that reaches the remote replays
    the pending entries in recorded order onto the fresh tip, before its own
    operation (they may share one CAS commit). `tickets.query` never flushes;
    it stays read-only.
  - Replay re-evaluates each entry against the fresh tip: the owner-conflict
    matrix, and for acquire entries only, the origin-closed check (the flush
    fetches the review-track). **The remote wins.** A conflicting entry is
    dropped from the log and reported loudly in the flushing tool's output,
    naming the stem, the recording track (and, locally, its worktree path), the
    current holder, and when it was taken. Non-conflicting entries still apply.
    - An override entry applies only when the remote holder at replay is the
      holder it recorded; any other holder makes it a conflict.
    - Close, registration, and release entries for a stem that has since
      landed under `.done/` or `.dropped/` on origin resolve silently through
      pruning in the same commit; they are the normal lifecycle, not
      conflicts.
    - A release entry whose lease no longer belongs to its recording triple
      (taken over meanwhile) is dropped with a one-line note.
  - Every entry is idempotent on replay, so a crash between the remote push and
    the local clear re-flushes without duplicate records or errors.
  - **Index discontinuity discards.** When a clone with pending entries or a
    local cache ref reaches the remote and finds either condition below, it
    discards the pending log and the local cache ref:
    - a successful remote query shows no index ref, or
    - the remote tip and the local cache tip are unrelated: neither is an
      ancestor of the other (`merge-base --is-ancestor` both ways), as after a
      delete and re-init.

    A remote tip that is an ancestor of the cache tip is a stale read (a
    sibling worktree's push landed in between), not a discontinuity: keep the
    cache and discard nothing. Every index version is a CAS commit whose
    parent is the previous tip, so only a re-init produces unrelated history.
    This relies on the cache ref holding only commits observed on the remote:
    it is advanced only after a push is accepted or a fetch returns, and never
    materializes pending entries.
    - Absence must be told apart from failure. A transport error, timeout, or
      credential failure is unreachable, never absence; only a successful
      remote answer with no matching ref is absence. A read-path fetch that
      fails because the ref is missing is resolved through that distinction
      before falling back to the stale cache.
    - The discard uses the pending log's local CAS, keyed on the log value it
      read; a CAS miss re-reads and discards the current log. Its report
      counts exactly the entries removed.
    - Report once, one line, only when N > 0 entries were discarded: "N
      offline entries were discarded because the remote index disappeared or
      was recreated". Discarding only a cache ref is silent. The report is
      acceptable because this clone had used the index.
    - Any call path that reaches the remote may perform the discard, including
      `tickets.query`: "read-only" means no remote write and no flush, and the
      discard is a local-only cleanup.

    An unreachable remote never counts as absence; offline alone never
    discards. After a discard the clone follows the normal rules for its new
    state (index-absent mock, or adopting the new index).
  - The log never touches code branches or history; reconnect adds no clutter.
  - Offline sync is impossible, so the log does not prevent duplicate work
    while offline; it guarantees the conflict surfaces at reconnect.
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
    remote tip once the index is known to exist and the remote is reachable.
    Offline, the flush's replay is the catch. Within the absence TTL,
    acquire returns the legacy mock without contacting the remote; a lease
    missed in the window after another clone's init is the same accepted
    window as A9.
  - The first registrant wins. A cross-branch stem collision is an accepted
    accident and is not detected.
  - No nonce and no ticket template change.
  - Piggyback failure (offline, rejected, timeout) never fails the host
    operation. In index mode an offline piggyback is recorded in the pending
    log.
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
    a stem that already sits under `.done/` or `.dropped/` on the origin
    review-track, whether or not it is still registered, and tells the caller
    to pull. Online acquire, offline acquire, and flush replay share this one
    unqualified check.
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
  - **Unreachable remote, index seen.** When this clone's local cache ref
    exists, acquire evaluates the owner-conflict matrix against the cached
    index (the last seen state) with this clone's pending entries overlaid.
    - A cached conflict refuses offline exactly as online; an override with
      flag and reason is recorded in the pending log with its reason and the
      overridden holder's triple.
    - Before recording, offline acquire applies the network-free origin-closed
      hint (the local `refs/remotes/origin/<review-track>` tree, as query
      does) and refuses a stem already closed there, so an obviously landed
      ticket is refused at acquire time rather than only at flush.
    - Otherwise acquire succeeds with an explicit offline warning (remote
      unverified), records the acquire in the pending log, and shows it as a
      provisional lease in this clone's view until the flush.
  - **Unreachable remote, index never seen.** When no index was ever cached,
    or absence was the last cached result, acquire treats the index as absent
    and returns the legacy mock's plain `ok`. This keeps "No ref, no change"
    for projects that never ran init while offline.
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
  - Pending offline entries are shown: a pending-count marker, and this
    clone's pending acquires as provisional leases until the flush settles
    them. A provisional lease is resolved by its recorded triple like any
    lease: it renders at the matching ownership level (this worktree, or
    another worktree of this clone) and counts as owned by that triple for the
    view, queue, and ownership filter.
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
    - An acquire that succeeds with the offline warning proceeds, and
      `lead-run` relays the warning to the user verbatim.
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
  - Mutating tools succeed within the timeout.
  - Mutating tools record their registration in the pending log.
  - `tickets.acquire` succeeds with the offline warning (covered in detail by
    group I).
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
  `tickets.acquire` does not fail: it succeeds with the offline warning and a
  pending entry, or refuses on a cached conflict (I1, I2).
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
  proceeds, the registration is recorded in the pending log, and the tool does
  not fail.

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

**I. Offline pending log and reconnect** (Phases 1–3)

- I1. Clone X (index seen) goes offline and acquires a ticket with no cached
  conflict. Acquire succeeds with the offline warning and the entry is
  pending. After reconnect, X's next mutating tool flushes: the lease appears
  on the remote and the log is empty. View clause (Phase 3): before the flush,
  X's view shows a provisional lease.
- I2. Offline acquire where the cached index shows a lease held by a different
  email. Acquire refuses offline. With the flag and a reason it records a
  pending override carrying the overridden holder, which replays as an
  override with its reason when that holder still holds the lease.
- I3. X holds a pending offline acquire while clone Y (different email)
  acquires the same ticket online. X reconnects and flushes: X's entry is
  dropped, and the flushing tool's output names the stem, the recording track,
  Y, and the time. View clause (Phase 3): X's view then shows Y as owner.
- I4. While X is offline, the ticket is closed and lands under `.done/` on the
  origin review-track. X's flush drops the pending acquire with an
  origin-closed report.
- I5. Two worktrees of the same clone record pending entries concurrently
  while offline. Both entries survive and flush in recorded order.
- I6. The process is killed after the remote push but before the local clear.
  The next flush is idempotent: no duplicate records and no error.
- I7. Worktree B appends an entry while worktree A is flushing. B's entry is
  not lost and flushes on the next mutating call.
- I8. Query with pending entries shows a pending-count marker, offline and
  online. An online query with pending entries pushes nothing (count remote
  push invocations).
- I9. Offline close and an offline worker impl record both become pending and
  reach the remote on flush (`phase: closed`, `impl.branch`).
- I10. A flush where some entries conflict and others do not: the
  non-conflicting entries apply and persist, and only the conflicting ones are
  dropped and reported.
- I11. Worktrees A (track a) and B (track b) of one clone are offline. A
  acquires T. B's acquire of T is refused locally (same clone, different
  track) without the flag. View clause (Phase 3): B's view shows T as held by
  another worktree of this clone, not as B's own.
- I12. X records an offline override against holder Y. Before X reconnects, Y
  releases and Z acquires. X's flush drops the override as a conflict and
  reports it; Z keeps the lease.
- I13. An offline release becomes pending and removes the lease on flush. A
  pending release whose lease was taken over meanwhile is dropped with a
  one-line note and does not touch the new holder's lease.
- I14. Offline `tickets.move` and `tickets.close` against a cached
  different-email lease are refused by the guard; with the flag and a reason
  they proceed and record a pending override entry for close.
- I15. A provisional lease counts as owned by its recording triple in the
  ownership filter and queue selection: included for the recording worktree,
  excluded as another worktree's for a sibling worktree.
- I16. An offline close becomes pending, and the ticket then lands under
  `.done/` on origin before the flush. The flush prunes the registration with
  no conflict report.
- I17. The local `refs/remotes/origin/<review-track>` tree already has the
  ticket under `.done/` (and, in a variant, under `.dropped/`). An offline
  acquire is refused with the origin-closed message and records no pending
  entry, even when the cached index still registers the stem.
- I18. X has pending entries, and the index ref is deleted on the remote. On
  X's next successful discovery, the pending log and cache ref are discarded
  with one report; ticket tools then behave as index-absent.
- I19. X is offline with pending entries while the index is deleted and
  re-initialized on the remote. On reconnect, the remote tip and X's cache tip
  are unrelated: X discards its pending log with one report and adopts the new
  index. None of X's discarded pending entries appear in it; the reconnecting
  tool's own fresh write (for example its piggyback registration) may.
- I21. Worktree B reads remote tip T5; worktree A's push then lands T6 and
  advances the shared cache ref before B's continuity check. B treats T5 as a
  stale read: nothing is discarded, no report is shown, and the cache stays at
  T6.
- I22. A clone with a cache ref and no pending entries sees the ref deleted on
  the remote. The cache ref is discarded silently, with no report.
- I23. The read path fetches a deleted ref with a cache ref present. Query
  resolves it as absence (not a stale-cache fallback) and performs the
  discard; a transport failure in the same setup serves the stale cache.
- I20. An unreachable remote never triggers a discard: pending entries survive
  repeated offline calls.

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
- the pending log: local-ref append and clear with local CAS, ordered replay
  onto a fresh tip, idempotent entries

**Verification.** Scenarios A2, A3, A4 (library level), A5, A9, B2, B3, B6,
F1, F2, F3, F4, C9, I5, I6, I7 (library level); missing ref reported as
absent, not as an error.

### Result (24898ba4) - 2026-09-24

**Live probe** (GitHub `origin`, SSH, git 2.50.1, from a scratch repo with
`GIT_TERMINAL_PROMPT=0` and `ssh -o BatchMode=yes`). No design assumption
broke.

- Create with a must-not-exist lease (`--force-with-lease=<ref>:`) was
  accepted; a second must-not-exist create was rejected client-side as
  `[rejected] (stale info)`.
- A plain fast-forward push was accepted; a non-fast-forward push without
  force was rejected `[rejected] (non-fast-forward)`.
- Racing CAS: five rounds of two concurrent `--force-with-lease=<ref>:<tip>`
  pushes from one expected tip. Every round exactly one won; the loser got a
  server-side `[remote rejected] (cannot lock ref '<ref>': is at <winner> but
  expected <tip>)`, so the CAS is enforced by the server, not only by the
  client's advertisement check.
- `--no-verify`: with a failing `pre-push` hook (via `-c core.hooksPath`),
  the push without `--no-verify` was blocked; with it, the push landed.
- Delete of a custom ref (`:<ref>`) works.
- Actions/webhooks: a throwaway branch carrying an unfiltered `on: push`
  workflow ran once as the control (run 35974212358, `event=push`). Two pushes
  of workflow-bearing commits to a custom ref (create `2bb9e15`, fast-forward
  `1e597e2`) produced no run and no check suite after three minutes (`gh run
  list`, `commits/<sha>/check-suites` total 0). The repository has no
  webhooks configured (`repos/.../hooks` is `[]`), so delivery could not be
  inspected; the public events feed listed only `refs/heads/*` pushes.
- Pushed and deleted refs, all under the throwaway namespace
  `refs/wsprobe-260924/` plus one throwaway branch:
  - `refs/wsprobe-260924/v1/index`: create `4c82464`, fast-forward `1b8ecf0`,
    five race winners, `--no-verify` push `c43f194`; deleted.
  - `refs/wsprobe-260924/v1/actions`: `2bb9e15`, `1e597e2`; deleted.
  - `refs/heads/wsprobe-260924-actions`: `7c1e4c3`; deleted.
  - After deletion `git ls-remote origin | grep wsprobe` matched nothing.
  - The control workflow run record remains in the Actions history; deleting
    it was outside the push authorization.

**Library** `agents-plugin-tool/internal/wsindex/`:

- Namespace constant `ticket-index-larkspur`: remote ref
  `refs/ticket-index-larkspur/v1/index`; local cache, pending log, and fetch
  scratch refs under `refs/ticket-index-larkspur-local/v1/`, shared by every
  worktree of the clone.
- Discovery walks a candidate list (tier 1 only) with one `ls-remote` while
  the clone has never seen an index; the absence result (and an unreachable
  result on a never-seen clone) is cached for 10 minutes in a state file in
  the git common dir. No `origin` remote means index-absent with no remote
  call.
- Remote commands run with `GIT_TERMINAL_PROMPT=0`, empty `GIT_ASKPASS` and
  `SSH_ASKPASS`, `GCM_INTERACTIVE=never`, `-c credential.interactive=false`,
  and the configured ssh command (git's precedence) plus
  `-o BatchMode=yes -o ConnectTimeout=5` (`-batch` for plink). Hard timeouts:
  1.5 s for the read path, 10 s per remote command for writes; timeouts kill
  git's whole process group.
- Index versions are plumbing-only commits of one indented `index.json`, each
  parented on the previous tip; audit lines go in the commit message.
- The CAS loop fetches the tip into a scratch ref, re-applies pending entries
  then the caller's mutation, and pushes with `--force-with-lease` and
  `--no-verify`, at most 5 attempts before a clear error. A rejection counts as
  a lost race only for stale-info, non-fast-forward, and ref-lock failures;
  any other refusal fails at once.
- Cache continuity: forward moves advance the cache under local CAS; a remote
  tip that is an ancestor of the cache is a stale read; unrelated histories or
  a remote answer with no ref discard the pending log (one report line when
  N > 0) and the cache ref. A failed fetch is classified through `ls-remote`,
  so a deleted ref reads as absence and a transport failure stays unreachable.
- Read path: within a 60 s TTL no remote call; otherwise one bounded fetch,
  then the stale cache with its age, or `unknown` when the index exists but no
  cache does.
- `clone_id` is 16 hex characters in `.git/config` (`ticketindex.cloneid`),
  first generation serialized by a lock file in the common dir.
- Pending log: a parentless commit holding `pending.json`, replaced under
  `update-ref --stdin` CAS; clears remove exactly the flushed entry ids.
- `Create` (init primitive, adopts a concurrent creation) and `Check` (live
  state without pushing) are in place for Phase 2.

**Verification.** `go test -race -count=3 ./internal/wsindex/` passed
(88 s). Scenario tests: `TestMissingRefIsAbsentNotError`, A2, A3, A4 (hanging
ssh: read under 2.5 s stale, write pending within the timeout), A5 (HTTP 401
remote fails in under 3 s with the non-interactive env), A9, B2, B3 (4 clones x
4 writes, linear chain of init + 16 versions), B6, F1, F2, F3, F4, C9, I5, I6,
I7, plus concurrent `Create` adoption.

**Decisions.** A local file remote reports concurrent lock contention as
`[remote rejected] (failed to update ref)`, so that phrase is treated as a
lost race; the bounded retry absorbs a misclassified permanent refusal. Timing
state lives in a common-dir JSON file rather than git config so per-read TTL
bookkeeping never contends with config writes.

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
- offline acquire against the cached index (warning, provisional lease,
  pending entry) and pending entries for every offline index write
- flush on the next mutating tool that reaches the remote, with replay
  re-evaluation and loud conflict reports
- the offline origin-closed hint at acquire, and the index-discontinuity
  discard

**Verification.** Scenarios A1 (acquire/release plain `ok`; the override-param
clause waits for Phase 3), A4 (tool level), A10, A11, A12, A6, A7, A8, B1, B4, B5, B7,
C1, C2, C3, C4, C7, C8, D1–D5, E1 (through close), E2, E3, E4 (acquire
refusal), E5 (acquire refusal), E6, E7, E8, E10, I1, I2, I3, I4, I5–I7 (tool
level), I9, I10, I11 (acquire refusal), I12, I13, I16, I17, I18, I19, I20,
I21, I22; the view clauses of I1,
I3, and I11 wait for Phase 3. Every existing ticket-tool test must pass
unchanged with no ref present.

### Result (57c930bb) - 2026-09-24

**Tool surface.**

- `tickets.acquire(ticket_stem, track?, dangerously_override_lease_status?, reason?, format?)`
- `tickets.release(ticket_stem, format?)`
- `tickets.index_init(check?, format?)`: lead-only, because it pushes to origin with the user's credentials.

The success text is `status: <effect>`, plus `owner:`, `impl_branch:`, `warning:`, and `report:` lines. The effects are:

- `acquired`
- `takeover`
- `refreshed`
- `impl_recorded`
- `released`
- `not_leased`
- `pending` (offline)

Refusals are `isError` responses that name the holder and the flag. The legacy mock returns exactly `ok` as text, or `{"status":"ok"}` as JSON. The check mode prints `state: initialized|uninitialized|no-origin|unreachable`.

All four tools were added to the three `runtime.json` files. `LIVE_TOOL_NAMES` in the Pi bridge test was updated, and its contract count went from 58 to 61.

**Library (`internal/wsindex`).**

- `apply.go`:
  - `Applier.Live` and `Applier.Replay` implement the matrix, the impl record, the closed lease, the origin-closed refusal, and the replay rules (remote wins, override bound to its recorded holder, landed stems resolved silently).
  - `Maintain` runs pruning and GC.
  - `NeedsOverride` covers each operation's override rows.
- `client.go`:
  - `Submission` carries a `Prepare` hook that runs only once the index is known to be in use, so validation never reaches an index-absent project.
  - Offline evaluation runs through `Overlay`, the cached index with this clone's pending entries replayed.
- `origin.go`:
  - Origin-first review-track: the `refs/remotes/origin/HEAD:AGENTS.md` declaration, then `wsreview.ResolveTrackFallback`.
  - Best-effort track fetch and origin inventory from `ls-tree`.
  - `LoadContext`: fetches the track for acquire, or when a pending acquire will replay; otherwise reads the local tracking ref.
  - The lazy GC predicate and `InitSource`.
- The MCP layer (`internal/mcp/ticket_index.go`) adds the piggyback on `create_empty`, `move`, `close` (close op), and `sage_stamp`. It prints extra lines only in index mode: reports, the offline note, or a one-line failure. It never fails the host operation.

**Verification.**

- `go test ./internal/wsindex/` covers:
  - the matrix: C1–C5, C8
  - D2 at library level
  - C7, I13, E2, I2, I12, I4, I16, E3, E6
  - `NeedsOverride`
  - continuity: I18, I19, I20, I21, I22
- `go test ./internal/mcp/ -run 'TestIndexAbsent|TestA10|TestA12|TestInit|TestAcquire|TestCloseLease|TestOffline|TestFlush|TestB1|TestB4|TestB5'` covers A1 (verbs, plus move/create_empty output identical to a no-origin repo), A3, A10, A12, A6, A7, A8, C1–C4, C8, B7, D1–D5, E4/E5 acquire refusal, I17, E1 through close and prune, E2, A4/A11/E10/I1/I5/I9/I11, I3/I10, I2/I12, I4/I13, B1, B4/E8, B5/E3/E6/E7.
- `go test -race -count=2 ./internal/wsindex/` passes.
- The Pi `bridge`, `native-tool-registration`, and `version-check` node tests pass.
- Existing ticket-tool tests pass unchanged.
- Environment-dependent failures remain, all unrelated to this change:
  - `TestServeStdioConfigResolveAgentFallsBackToDefault` and wsconfig's `TestResolveAgentTierForHarnessFallsBackToDefault` read the developer's home config and pass with a clean HOME.
  - `test_skill_dispatch_contracts.test_delegate_and_sibling_exact_prose` asserts a shim description that already differs on the base.

**Decisions.**

- Init commits carry a random `init-nonce:` line. Without it, two identical inits in the same second produce the same oid, and a delete-and-reinit reads as continuous history (found by I19).
- Index commits use `commit-tree --no-gpg-sign`, so a user's signing config cannot prompt or fail.
- A pending override is kept only when the overlaid view needs it. It records the overridden holder's triple so replay can compare against it.
- GC fetches every origin head (`+refs/heads/*:refs/remotes/origin/*`) only when GC is due. If that fetch fails, GC prunes nothing.

### Phase 3: Query view, queue filter, and move/close guards

**Goal.**

- `tickets.query` shows the authority-local intersection with the three
  ownership levels and the unknown state.
- The ownership filter parameter.
- Compact-text collapse, and stale-cache marking.
- The pending-count marker and provisional-lease display.
- The origin-closed hint.
- The owner shown in `git.status`.
- `tickets.move` and `tickets.close` guards per the matrix, with the override
  params, evaluated offline against the cached index with pending entries
  overlaid.

**Verification.** Scenarios C5, C6, E4 (query hint), E5 (query hint), E9, F5,
G1–G5, I8, I14, I15, I23, and the view clauses of I1, I3, and I11; tool-level
re-runs of A2, A3, F1, F2. The no-ref path stays
byte-identical (A1 re-run, including its override-param clause for
`tickets.move` and `tickets.close`, which Phase 2 cannot yet exercise).

### Result (a27aba50) - 2026-09-24

**Surface.**

- `tickets.query`:
  - Every projection carries `ownership {level, email, track, worktree, phase, touched_at, impl_branch, provisional, origin_closed, index_state, cache_age_seconds}`.
  - The levels are `self` (the caller's triple), `local` (another track of this clone), `remote`, `unowned`, and `unknown`. For `local`, `worktree` is computed from `git worktree list` and never stored.
  - Compact discovery lists own and unowned tickets in full. It collapses the rest under `held elsewhere or closed on origin (N):`. Point-resolve and JSON are never collapsed.
  - Trailing `ticket-index:` lines carry discard reports, the stale-cache age, and the pending-count marker.
- `unleased_or_mine` keeps self, unowned, and unknown tickets, and drops tickets closed on origin. It runs through a new generic `wsdoc.TicketFindOptions.Exclude` hook before pagination, so it composes with `assigned_to_me`.
- `tickets.move` and `tickets.close` take `dangerously_override_lease_status` and `reason`. Their guard evaluates the overlaid view, online or offline:
  - A different email is refused without the flag and reason.
  - Another clone or track gets a `ticket-index: ... is held by ...` warning line.
  - The lease never moves.
  - An override rides the piggyback write as an audited entry.
- `git.status` adds `ticket owner:` for the active impl ticket and `leased to this track:`. JSON gains `impl_ticket.owner` and `leases`. It reads the cache only and makes no remote call.

**Library changes.**

- `Client.Read` reports the unknown state when discovery saw the ref but the fetch failed with no cache (F5). A never-seen, unreachable remote stays index-absent (A10).
- A write whose index is unchanged but carries audit lines still commits. C6 found that an override move on a leased ticket otherwise left no audit.

**Verification.**

- `go test ./internal/mcp/ -run 'TestNoRefPath|TestQuery|TestOwnership|TestOfflineView|TestMoveCloseGuard|TestGitStatusShowsOwner'` covers:
  - A1 with the override params on move and close, A2 and A3 (counted remote calls)
  - F1, F2, I8 (no push on an online query), F5, I23
  - G1, G2, C5
  - G3, G4, G5, E4/E5 query hint, E9
  - the view clauses of I1, I3, and I11, plus I14 and I15
  - C6
  - `git.status` owner
- The full Go suite passes with a clean HOME. `go test -race -count=2 ./internal/wsindex/` passes.
- The `agents-plugin` unittest suite fails only on the known `test_delegate_and_sibling_exact_prose`. The `wsflow` tests pass.

**Decision.** The ticket leaves the GC stale flag optional ("may"), and it is not rendered. `phase closed (pending landing)` together with `since <touched_at>` gives the reader the same signal.

### Phase 4: Playbook integration and dogfood

**Goal.**

- Shipped playbook changes:
  - `lead-run` acquires before worker spawn, and relays an offline acquire
    warning to the user verbatim while proceeding.
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
  the worker impl-record acquire, and `lead-run`'s relay of the offline
  warning.
- `lead-bootstrap` text covers all four check states, asks before init, and
  changes nothing on decline. No other playbook mentions index setup.
- Playbook and package tests pass, including the wsflow drift tests and
  `TestPiMirrorUpToDate`.
- The dogfood cycle is recorded in the Result: init, acquire from a track,
  worker impl record, close to `phase: closed`, merge, and prune on landing.

### Result (00c0c0da) - 2026-09-24

Phase 4 is complete up to E1's close. Close is a push that the user will run.
The merge into develop, the develop push, and prune-on-landing are left to the
lead and the user. Review round-1 fixes landed in df16de9e.

**Playbooks.** 00c0c0da changed these playbooks in `agents-plugin/rsrc`, with
byte-identical mirrors in `agents-plugin-wsflow/rsrc` and `agents-plugin-pi/rsrc`
and regenerated manifests.

- `lead-run`:
  - Spawn step 4 acquires from the checkout the worker branches off.
  - A refusal or error ends the turn.
  - A `warning:` line is relayed verbatim.
  - The override is used only on the user's explicit takeover instruction.
- `ticket-worker` and `ticket-worker-elevated`:
  - An informational impl-record acquire runs after the branch action.
  - A failure goes to `unresolved:` and never stops the run.
  - They never set the override.
- `lead-scope-worktree` step 6 acquires every visible ticket and reports each
  refused stem with its holder.
- `ticket-selector` and `ticket-batch-selector` query with
  `unleased_or_mine: true`. The batch selector lists unfiltered once to tell
  "every remaining ticket blocked" apart from `ready/ empty`.
- `lead-bootstrap` invoke step 7 runs `tickets.index_init(check: true)` in every
  mode except `refuse`.
  - Its new section handles `uninitialized`, `initialized`, `no-origin`, and
    `unreachable`.
  - On `uninitialized` it asks before init and changes nothing on decline.
- The Pi mirror was resynced with
  `sh agents-plugin-tool/scripts/bump-ws-version.sh 0.46.17`, the standalone
  path, which is idempotent at the current version.

**Dogfood.**

Who ran what:

- The auto-mode permission classifier denied the origin index push for the
  worker and for the lead, even after the user's explicit approval.
- The user therefore ran every push-producing step personally through `!`
  commands. The binary was the lead-built branch binary `ws-mcp-ownership`,
  built from ee6cb039.
- The worker ran only read-only checks: `ls-remote`, the cache ref log, and
  `index.json`.

Init, run from the develop root checkout:

- `tickets.index_init(check: true)` reported `state: uninitialized` before
  init.
- The init output was
  `status: created / review_track: develop / registered: 155 open tickets`.
- The init commit is `3ed11a3c`, with `init-nonce: 856812ea97aec36d`. It was
  the remote ref, and the local cache
  `refs/ticket-index-larkspur-local/v1/cache` was the same commit.

E1, run on this ticket itself. A scratch stem never lands on develop, so it
could not reach prune.

1. Acquire from the develop root:
   `status: acquired / owner: ki6080@gmail.com (track develop, clone 6b8c3f740994ed3b)`.
   Index commit `fb897dcb`.
2. Worker impl record, an acquire from this worktree on
   `impl/develop/irate-growl-half`:
   `status: impl_recorded / owner: ki6080@gmail.com (track develop, clone 6b8c3f740994ed3b) / impl_branch: impl/develop/irate-growl-half`.
   Index commit `0c98be62`.
3. `git.status` in this worktree, from the cache only:
   `ticket owner: yours (track develop); impl impl/develop/irate-growl-half; since 2026-09-24T10:40:13Z`
   and `leased to this track: 260924-feat-origin-ticket-ownership-index`.

H1, run on the untracked scratch stems `260924-idea-ownership-dogfood-scratch-a`
and `-b`. The scratch files are deleted. The different-email holder was a
throwaway clone with `user.email h1-dogfood@example.invalid` on develop.

1. The clone acquires scratch-a:
   `status: acquired / owner: h1-dogfood@example.invalid (track develop, clone fada5a56c9701de7)`.
   Index commit `cf2d4f82`.
2. The worktree acquires scratch-a and is refused:
   `tickets.acquire refused: 260924-idea-ownership-dogfood-scratch-a is held by h1-dogfood@example.invalid (track develop, clone fada5a56c9701de7) since 2026-09-24T10:41:27Z; acquiring it needs dangerously_override_lease_status: true with a non-empty reason, set only on the user's explicit instruction`.
3. The worktree acquires scratch-b:
   `status: acquired / owner: ki6080@gmail.com (track develop, clone 6b8c3f740994ed3b) / impl_branch: impl/develop/irate-growl-half`.
   Index commit `47599030`.
4. Both leases are released with `status: released`: scratch-a in `a6c5f050`
   and scratch-b in `b7ecc518`.

Every ref written:

- `refs/ticket-index-larkspur/v1/index` on origin, at
  `3ed11a3c -> fb897dcb -> 0c98be62 -> cf2d4f82 -> 47599030 -> a6c5f050 -> b7ecc518`.
  It stays on origin by design.
- Locally, `refs/ticket-index-larkspur-local/v1/cache` in this clone and in the
  throwaway clone.
- No branch or tag was pushed.

State after H1, verified read-only by the worker at `b7ecc518`:

- There are 158 registrations: 155 from init, this ticket, and the two scratch
  stems.
- The scratch stems stay registered but unleased until monthly GC prunes
  them. They are open on no origin branch.
- This ticket's lease is `phase: active` with
  `impl.branch impl/develop/irate-growl-half`.
- `meta.last_gc` is `2026-09-24T10:38:33Z`.

Pending E1 steps:

- **E1 step 3, close to `phase: closed`.** Close pushes the index, and the
  classifier denies that push to agents, so the user runs `tickets.close` from
  this worktree.
- **E1 step 4, merge into develop and push.** The merge is the lead's job.
  The develop push needs a separate user decision, because standing policy does
  not push develop.
- **E1 step 5, prune on landing.** The first index write after this ticket's
  `.done/` file reaches `origin/develop` prunes the registration. Any acquire or
  close counts as a write.

**Findings.**

- **Bug.** A fresh acquire from an impl branch records that branch as
  `impl_branch` even when the branch belongs to a different ticket, as
  scratch-b showed. This was not intended by the Worker impl record decision,
  which covers only a matching lease. Captured as
  `260924-bug-acquire-impl-record-ignores-branch-stem`.
- **Environment, not a defect.** `workflow_manual` from the throwaway clone
  first failed with `rsrc manifest missing at <scratchpad>/rsrc/manifest.json`.
  The cause was a branch binary running outside the plugin tree, and setting
  `WS_RSRC_ROOT` resolved it.
- The worker's own init attempts were denied by the permission classifier.
  They left no local or remote state: no refs and no clone id at that time.

**Verification.**

- Playbook and package tests pass, including the wsflow drift tests and
  `TestPiMirrorUpToDate`.
- `go test ./internal/wsrsrc/ ./internal/wsindex/ -count=1` passes.
- The agents-plugin unittest `test_delegate_and_sibling_exact_prose` fails on
  this branch. The failure is pre-existing and was fixed on develop by
  `69b18630`, which this branch does not include.
