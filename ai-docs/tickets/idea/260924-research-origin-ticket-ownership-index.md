---
title: Origin-backed ticket ownership and registration index
related:
  260917-feat-ticket-assignee-awareness: prior advisory-only answer to "two people start the same ticket"
  260806-feat-worktree-ticket-scope: sparse-checkout board scoping that the index view is meant to supersede
  260728-research-duplicate-ticket-stem-silent-resolve: same stem in two status dirs resolves silently
  260829-research-review-watermark-multi-maintainer-model: canary vs platform rendezvous backends
  260921-feat-ws-impl-branch-identity-resolver: impl branch derived from ticket stem hash
---

# Origin-backed ticket ownership and registration index

## Background

Heavy worktree parallelism plus multi-person collaboration made ticket
inventory management a bottleneck. Ticket status is the ticket's directory
(`idea/`, `todo/`, `ready/`, `.done/`, `.dropped/`), so status lives inside
each branch's history. Worktrees and collaborators cannot see each other's
ownership, the same ticket gets implemented twice, and status moves create
rename conflicts and history clutter.

Goal: keep ticket files and their history coupled with code, a property the
user values. Lift *who owns a ticket* (and later, its status) into one shared
authority on the git remote, so each worktree sees only its own share of the
board. The authority must:

- work regardless of hosting backend
- require only developer (not maintainer) permission
- not break downstream projects that use the directory convention

Scope note: the user needs team collaboration on a branch within about a week.
An MVP is scoped below; the fuller status-authority design is recorded as
deferred direction.

## Current state (as of 2026-09-24)

- **Status is the directory;** there is no frontmatter status. The status list
  is hard-coded in three places:
  - `agents-plugin-tool/internal/wsdoc/tickets_mutate.go` (`statusDirs`)
  - `internal/wsrationale/tickets.go`
  - `internal/wsgit/git.go` (`ticketStatusStem`, which also accepts legacy
    `wip`)
- **Status changes are staged moves.** `tickets.close` and `tickets.move`
  stage a `git mv`. Workers commit the close on their impl branch, and the lead
  merges serially.
- **No claim or lock exists.** The closest mechanisms:
  - advisory `assignee:` frontmatter
  - the impl branch name derived from the stem hash (local only)
  - the `worktree.acquire` pool owner, which is not a ticket claim
- **ws-mcp performs no remote operation at all** (no push, fetch, ls-remote,
  notes, or update-ref).
  - The review ledger (`ai-docs/.review-ledger.md`) is a tracked file. Its
    concurrency "canary" is an ordinary merge conflict.
  - `rendezvous-backend` is parsed but unused.
  - This feature would be ws-mcp's first remote write.

## Host policy research (docs/source, not yet live-probed)

- **Custom refs are pushable.** Refs outside `refs/heads`/`refs/tags` (e.g.
  `refs/<ns>/v1/...`) accept pushes with ordinary write/developer permission on
  GitHub, GitLab CE, and Gitea.
- **Reserved namespaces to avoid:**
  - GitHub: `refs/pull/*`
  - GitLab: `refs/merge-requests/`, `refs/pipelines/`, `refs/environments/`,
    `refs/keep-around/`, `refs/tmp/`, `refs/remotes/`
  - Gitea: `refs/pull/*`, `refs/for/*`
- **Visibility.** Default clone refspecs skip custom refs, so they do not
  clutter teammates' clones. None of the three hosts lists them in the web UI.
- **CI.**
  - A custom-ref push triggers no CI on GitLab or Gitea; both process only
    branch and tag changes. GitHub is unconfirmed.
  - A new branch triggers CI on all three by default.
  - All three honor `[skip ci]` in the commit message. GitLab also supports
    `-o ci.skip`.
- **Branch creation.** GitLab CE has no branch-name push rules (a Premium
  feature). Only a protected-branch wildcard can block a developer from
  creating a branch.
- **Mirroring.** GitLab push mirroring is documented for branches and tags
  only, so custom refs may not be mirrored (unconfirmed).
- **Compare-and-swap.** `git push --force-with-lease=<ref>:<expected>`. An
  empty `<expected>` means "must not exist". The server checks the old oid
  inside the ref transaction.

## MVP design: coordination overlay

Directory position stays the status authority in the MVP. The index adds
registration and ownership on top without changing folder semantics.

- **Storage.** One ref, `refs/<ns>/v1/`, holding a single commit tree.
  - Writes use CAS via `--force-with-lease`. On rejection, the writer fetches,
    re-applies, and retries.
  - Local access is git plumbing only (`hash-object`, `mktree`, `commit-tree`,
    `update-ref`), with no worktree checkout.
  - The local cache is a local ref, so all worktrees of the same clone share
    it.
  - State pushes use `--no-verify`. The index is tool-internal data, not code,
    and server-side hooks still apply.
- **Discovery.** A single `git ls-remote` over code-constant candidates:
  1. `refs/<ns>/v1/...`
  2. branch fallback `refs/heads/__<ns>_v1` (not implemented in the MVP)

  If both exist, tier 1 wins and ws warns loudly.
- **Naming.** The namespace is named after the protocol and carries a version
  segment. It is never named after a distribution identity such as the
  marketplace name. Uniqueness is what matters: a readable prefix plus an
  arbitrary word is fine. The exact name is chosen at implementation.
- **Enablement.**
  - An explicit init creates the ref and bulk-registers currently open tickets.
    Creation is itself a CAS (must-not-exist).
  - When no index exists on the remote, behavior is identical to today.
  - When one exists, every ws user of that project follows it automatically.
- **Record shape.** Each stem maps to a registration plus an optional lease
  `{email, clone_id, track, impl?, touched_at}`.
  - `clone_id` is a random per-clone id stored in `.git/config`. All worktrees
    of the clone share it.
  - `track` is the work-line branch of the owning worktree.
  - Worktree paths are derived locally and never published.
  - The v1 schema reserves room for the deferred status-authority fields
    (stage, project settings, terminal state), so v2 needs no migration.
- **Registration.**
  - Every mutating ticket tool call (never `tickets.query`) piggybacks an idempotent "register stem if
    absent" write.
  - The first registrant wins. A true stem collision across branches is treated
    as an accident and not handled.
  - There is no per-ticket nonce and no template change.
- **Ownership.**
  - Ownership arises only from an explicit acquire. Creating a ticket does not
    confer ownership.
  - The unit is the track (work line), so two worktrees of the same person are
    distinct owners.
  - The lead performs acquire. The worker records only the `impl` sub-record.
  - Acquire requires both that the ticket file exists locally and that the stem
    is registered. For an unregistered stem, acquire piggybacks registration in
    the same CAS commit.
  - The same model applies to every category (epic, research, actionable).
    There is no epic-to-child inheritance in the MVP.
- **Takeover.**
  - A different email takes over only with
    `dangerously-override-lease-status: true` and a required `reason`. The
    previous holder is recorded.
  - The same email on another clone takes over with a warning and needs no
    flag.
  - Staleness is shown as information only; there is no time threshold.
  - Playbooks set the flag only on explicit user instruction.
- **Query and view.** `tickets.query` returns the intersection of authority and
  local: only tickets whose file exists locally, annotated with ownership.
  - Ownership has three levels:
    - this worktree
    - another worktree of this clone (path computed locally)
    - remote (email and track shown)
  - The default view fully lists tickets owned by my track plus the unowned
    pool, and collapses the rest.
  - Queue selection draws only from that range.
- **Integration.**
  - `tickets.move` and `tickets.close` block or warn on a ticket another track
    owns.
  - `tickets.close` sets the lease phase to `closed`, and landed-closure
    pruning clears it.
  - The owner is surfaced in `tickets.query`, `git.status`, and queue
    selection.
  - In index mode, a `lead-scope-worktree` scope assignment is an acquire.
- **Pruning.**
  - A registration is pruned once its closure lands on the origin review-track.
    In the MVP, that means the file sits under `.done/` or `.dropped/`.
  - A monthly GC is piggybacked on writes via a `last_gc` record, itself a CAS
    so only one client wins. It prunes an entry only when all of these hold:
    - registered more than 30 days ago
    - unleased
    - absent as an open ticket from every origin branch (`ls-tree`)
  - GC never prunes leased entries; it only flags them stale.
- **Offline.** In the MVP, acquire fails loudly when the remote is unreachable.

## Deferred direction (v2 and later)

- **Status authority.** The index becomes the sole status authority, and
  directory position becomes a projection. The user accepted that folder
  position need not be truth.
  - Status is two orthogonal axes: stage (idea/todo/ready) and ownership.
  - Terminal state is recorded as file markers (`completed:`/`dropped:`), and
    the registration is pruned after the closure lands.
  - A stem that is neither in the index nor carries a terminal marker is
    surfaced as unregistered, never silently treated as closed.
- **Project settings in the index.** Mode switches and per-layer opt-out
  (coordination overlay vs status authority) live as a record inside the remote
  index, not in `AGENTS.md`. This avoids branch-dependent mode splits when
  someone has not pulled.
  - The uninitialized state renders loud setup guidance, keyed on the detected
    failure reason: rejected push, branch fallback in use, CI exclusion needed.
  - An explicit opt-out renders at most one line.
- **Offline queue.** `tickets.move` degrades implicitly: pending transitions
  queue in a local ref and flush on reconnect.
  - The response reports `synced | queued | legacy`.
  - There is no separate `tickets.relocate` verb.
- **Merge-time folder sync.** At lead `git.merge` into the review-track, sync
  directory positions to the index.
  - Folder moves arriving from branches created before bootstrap are reported
    as legacy-transition candidates for the lead to ingest, not auto-reverted.
- **Branch fallback.** Tier 2 discovery as above. State commits carry
  `[skip ci]`.
- **Deferred indefinitely: an `AGENTS.md` custom-name pointer (tier 3).** It is
  the only branch-dependent tier, and no need for it has been demonstrated.

## Outcome Ledger

### Verified Findings

- Ticket status is directory-based, and the status list is duplicated in three
  source files (see Current state).
- ws-mcp has no remote git operations today. The review ledger is a tracked
  file, and `rendezvous-backend` is parsed but unused.
- No ticket claim or lock mechanism exists. `assignee:` is advisory only.
- The merge root can be recovered from an impl branch name even when it
  contains `/`: `parseImplBranchRoot` splits on the last `/`, and
  `implementMergeRootFor` returns the root.
  - This is one level only: `goal/*` returns itself, and a rootless
    `impl/<stem>` returns `""`.
  - Source: `agents-plugin-tool/internal/mcp/implement_resolver.go`,
    `parseImplBranchRoot` and `implementMergeRootFor`.
- Per host docs and source (not live-probed), on GitHub, GitLab CE, and Gitea:
  - custom refs are pushable with developer permission
  - custom-ref pushes trigger no CI on GitLab or Gitea
  - default clones skip custom refs
  - `[skip ci]` is honored by all three

### Confirmed Decisions

- **MVP scope.** The MVP is the coordination overlay only: registration plus
  ownership, with the integration items. Directory position remains the status
  authority, and status authority is deferred to v2.
- **Storage.**
  - One CAS-updated ref under a protocol-named, versioned namespace, not
    derived from distribution identity.
  - Plumbing-only local access, with no materialized worktree.
  - State pushes use `--no-verify`.
- **Discovery.** Discovery uses code-constant candidates probed in one
  `ls-remote`: tier 1 (custom ref) and tier 2 (branch fallback). The first
  implementation builds tier 1 only, with discovery shaped as a candidate list
  so tier 2 can be added later. The `AGENTS.md` custom-name pointer (tier 3) is
  deferred.
- **Same-email takeover.** The MVP includes same-email takeover across clones,
  with a warning and no flag.
- **MVP read path.** A short-timeout fetch with a TTL cache. On fetch failure,
  the cache is served marked stale. There is no background refresh loop in the
  MVP.
- **Track resolution.** Acquire records `implementMergeRootFor(<current
  branch>)` as the track: one level, no recursion.
  - `goal/*` is its own track.
  - A rootless `impl/<stem>` is refused, and the caller must pass the track
    explicitly.
- **First probe host.** The first live probe and dogfood target this
  repository's GitHub origin. A GitLab CE probe is deferred until on-site
  access.
- **Enablement.**
  - The index is on by default when it exists on the remote.
  - It is created only by an explicit bootstrap, which reads origin's
    `AGENTS.md` and review-track rather than local state.
  - Project-wide settings live in the index, not in `AGENTS.md`.
  - The uninitialized state may be loud.
- **Status model.**
  - Stage and ownership are orthogonal axes.
  - The ownership unit is the track (work line).
  - Acquire is lead-performed and explicit. Creation does not confer
    ownership.
  - The same model applies to epic and research tickets.
- **Takeover.** Taking over another owner's ticket requires
  `dangerously-override-lease-status` plus a reason. There is no time-based
  threshold.
- **Registration.**
  - Stems register via piggyback on every mutating ticket operation. Query
    never registers.
  - The first registrant wins, and collisions are not handled.
  - Acquire requires a local file plus registration, and piggybacks
    registration when needed.
- **Query.** `tickets.query` shows authority intersected with local files.
- **Owner-conflict matrix (acquire / move / close).** The matrix applies when
  the existing holder differs from the caller:

  | existing holder | acquire | move / close |
  |---|---|---|
  | different email | refuse without the dangerous flag + reason | refuse without the dangerous flag + reason |
  | same email, different clone | proceed with a warning | proceed with a warning |
  | same clone, different track | refuse without the dangerous flag + reason | proceed with a warning |

  - Owner identity is the triple `(email, clone_id, track)`.
  - `tickets.release` removes only one's own lease.
- **No-index behavior.** `tickets.acquire` and `tickets.release` are an
  index-absent no-op.
- **Worker impl record.** The worker records impl by calling `tickets.acquire`
  from its impl branch, which requires a matching owner triple.
  - `tickets.close` on an unleased ticket creates a `closed` lease owned by the
    caller.
- **Queue filter.** A server-side `tickets.query` ownership filter, following
  the `assigned_to_me` precedent. Collapse applies only to compact text.
- **Origin-closed check.** Acquire fetches the origin review-track and refuses a
  stem that is absent from the index but closed there. Query and queue use the
  local remote-tracking tree as a hint.
  - The review-track tree serves as the done list, so the index keeps no done
    list or tombstones.
- **Lease phase.** A lease carries a binary phase, `active` or `closed`
  (pending landing), and stage stays out of the index in the MVP.
  - `tickets.close` sets `closed` rather than deleting the lease.
  - Queue and view treat a `closed` lease as owned until the landed-closure
    prune removes it. This avoids an unowned-but-still-`ready/` window between
    an impl-branch close and its merge.
- **Integration items.**
  - owner guard on move and close
  - close sets the lease phase to `closed`; landed-closure pruning clears it
    (see Lease phase)
  - owner surfaced everywhere
  - scope assignment equals acquire
- **Pruning.** Closure-landed pruning, plus a piggybacked monthly GC that never
  prunes leased entries.
- **Offline (v2).** Offline handling queues transitions rather than moving
  folders.
- **Terminal state (v2).** Terminal state is a file marker. Absence from the
  index is never read as closed.

### Proposals

- **Stale-while-revalidate reads.**
  - `tickets.query` reads the local cache ref and shows its age, while
    long-lived ws-mcp refreshes in the background.
  - Writes run a sub-500ms TCP reachability pre-check before the CAS push.
  - SSH ControlMaster may cut round trips.
  - The MVP may instead use a simpler short-timeout fetch with a TTL cache.
- **Derived done.** Done could be derived rather than declared:
  `implemented @ <commit>` becomes landed when that commit is reachable from
  `origin/<review-track>`.
- **Content pinning.** Pin the stage to content: record `ready @ <commit>` so
  workers read the reviewed ticket version.
- **Opt-in physical board hiding.** Hide tickets physically via sparse-checkout
  patterns generated from the index. The default stays tool-level filtering,
  which agents already use well through `tickets.query`.
- **Stale-ownership signals.**
  - The owning clone self-audits and marks a lease `orphaned` when its track
    branch is gone.
  - Each clone records `last_seen`, and each lease records `touched_at`.

### Open Questions

- **GitLab CE live probe** (deferred until on-site access). It should cover:
  - ref create, fast-forward, rejection of non-fast-forward pushes, and delete
  - two racing `--force-with-lease` pushes
  - custom-ref preservation across mirroring and backup
- **Whether goal branches should collapse into their parent track.** Today a
  `goal/*` branch is its own track.
- **Epic ownership.** Epic-to-child ownership inheritance, or batch acquire.
- **Non-ws collaborators.** Whether future collaborators all use ws. A single
  non-ws collaborator forces status authority off for that project.
- **v2 migration inventory.** List every direct-folder assumption to migrate:
  - the three status lists
  - `ticketStatusStem`
  - scope tooling
  - shipped playbook text such as "move with `git mv`"
- **Sparse-checkout safety.** Review the safety of dynamic physical hiding:
  - non-cone mode
  - churn when ownership changes
  - IDE and worktree-wide sparse marking

  The starting point is the rationale in `260806-feat-worktree-ticket-scope`.

### Rejected Alternatives

- **git notes as storage.** Notes attach to commits, not tickets.
- **Distribution identity in the ref name** (e.g. marketplace or repo name).
  The name is a protocol constant that must survive renames and forks.
- **Default-name-first discovery when a custom name is declared.** It risks two
  indexes (split-brain), and the custom-name tier is deferred anyway.
- **Per-ticket refs.** They bloat ref advertisement and need `--atomic` for
  multi-ticket writes. A single ref with CAS retry was chosen instead.
- **Treating "absent from index" as closed.** It conflates closed with
  never-registered and silently hides new tickets.
- **Auto-ownership on ticket creation, and a dual clone/worktree ownership
  model.** An uncommitted ticket file exists only on its creating branch, so
  creation-time ownership is clutter. It would also pin ownership to the wrong
  track, for example a ticket created on master but developed in another
  worktree.
- **Per-ticket nonce for collision detection.** A first-registrant-wins
  accident policy is sufficient.
- **Time-threshold-gated takeover.** Replaced by an explicit dangerous flag.
- **Silent auto-bootstrap** (a first remote write without consent). Pushing new
  refs to a downstream remote on upgrade is an outward-facing,
  hard-to-reverse action.
- **Status authority in the MVP.** It carries the largest cross-cutting
  refactor and the highest risk of tickets vanishing or misreporting during the
  first collaboration week, for small near-term gain. The v1 schema reserves
  room for it instead.
