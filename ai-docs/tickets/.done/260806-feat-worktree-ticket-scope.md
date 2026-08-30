---
title: Per-worktree ticket scope via sparse-checkout, with index-aware board resolution
sage-review-design: completed
related:
  260726-feat-verify-ticket-graph-advisories: introduced loadTicketGraph and the cross-reference integrity advisories that a scoped worktree would make fire falsely
  260728-research-ticket-graph-load-cost-commit-path: same graph load; its ruled-out "exclude the archive" analysis establishes why a partial board inverts advisories, and its per-ticket field measurement bounds what the index path must read
  260710-bug-project-index-ticket-focus-stale-status: retired the cached Ticket Focus section and settled that attention discovery is tickets.list/project_tree over the status directories - this ticket scopes exactly that surface
  260523-bug-worktree-local-index-missing: adjacent worktree-local context propagation gap; scope state is deliberately kept out of _index.local.md for the reason recorded there
  260619-feat-ws-layered-config-scope-substrate: layered config was considered as the scope store and rejected (see Decisions)
  260806-bug-drain-select-primitive-unbound: the ready-queue selector's unbound primitive; independent of this ticket because the filesystem filter reaches the selector whichever tool it picks
sage-review-completeness: completed
completed: 2026-08-07
---

# Per-worktree ticket scope via sparse-checkout, with index-aware board resolution

## Background

`ai-docs/tickets/` partitions by **status** (`idea`/`todo`/`ready`), never by
topic. A worktree dedicated to one work line therefore sees every active line's
tickets in its queue. The concrete symptom is that "what should I work on next?"
routes through `ws/tickets.list` / `ws/project_tree`, and those return the whole
board regardless of what the worktree is for.

`260710-bug-project-index-ticket-focus-stale-status` already settled that active
attention is discovered from the status directories through
`tickets.list`/`project_tree` rather than a cached index section. That decision
is not in question here; this ticket makes that same discovery surface
worktree-aware.

Git sparse-checkout expresses the filter directly: patterns are worktree-local,
new matching tickets appear on merge with no pattern maintenance, and hidden
files stay in the index rather than being lost. What it breaks is every ws
surface that resolves the board by walking the filesystem.

## Decisions

### Mechanism: sparse-checkout, not a ws-side presentation filter

The alternative considered first was a worktree-scoped declaration that ws tools
read to fold out-of-scope rows out of their output, leaving git untouched. It
was rejected on cost and completeness:

- It requires a new `ScopeWorktree` config layer. `wsconfig` today resolves
  `session > project > global > builtin`, and none fits: `project` keys on
  `shortHash(commonRoot)` (`internal/wsstate/paths.go:108-113`), so it is shared
  across worktrees, and `session` is per-key and does not survive a new session.
- It requires splitting `scanTickets` - the most widely shared wsdoc entry point
  - into resolution and presentation halves by hand.
- It leaks. A filter attached to tool output does not reach an Explore-style
  subagent that globs the ticket directory instead.

Under sparse-checkout the filesystem *is* the filter, so the presentation half
needs no code at all and no surface can bypass it. Of `scanTickets`'s four call
sites (`internal/wsdoc/tickets.go:60,68,122`,
`internal/wsdoc/tickets_graph.go:80`), only `loadTicketGraph` needs the whole
board unconditionally; the other three keep their filesystem walk and change
only where a caller asks for resolution (see the boundary table and Phase 1).

Cone mode cannot express this scope: it selects directories, not files
(`git sparse-checkout set <file>` fails with `is not a directory`). `--no-cone`
is required.

### Scope covers `ready/` and `todo/`; `idea/` stays visible

`ready/` + `todo/` is the actionable horizon - the statuses a session can pick
work from. `idea/` sits outside that horizon, so leaving it visible costs no
attention.

Hiding `idea/` was rejected because it collides with the repository's mandatory
dogfood-capture rule ("create a short `idea/` ticket immediately when the
surprise implies a bug, feature, or research follow-up"). A captured surprise
is by nature off-topic for the worktree that captured it, so under a hidden
`idea/` that capture would hit the sparse refusal path on every occurrence:
`git add -A -- <path>` - which is how `ws/git.commit` stages - prints git's
sparse-path advice, declines to stage, and exits 1, so the capture fails at
staging before a commit is attempted.

### Boundary: explicit lookup resolves, discovery filters

The rule that decides every surface:

| Surface | Behavior | Reason |
|---|---|---|
| `tickets.list`, `tickets.find(query:)`, `project_tree` | filesystem (filtered), plus a hidden-count annotation | discovery - this is where the filter must bite |
| `loadTicketGraph` -> `tickets.verify` / `git.commit` | index-aware (whole board) | false integrity advisories, inverted epic closure |
| `references.trace` | index-aware | link resolution |
| `ticket_create` stem-collision check | index-aware | a hidden ticket must still block a colliding stem |
| `tickets.find(ticket_stem:)`, `tickets.status(ticket_stem:)` | index-aware, marked as hidden | naming a stem is a lookup, not a discovery |
| `tickets.move`, `tickets.close` | index-aware source lookup; scope-aware failure message | the declared hot path fails here, and a raw git error does not name the cause |

Making the discovery surfaces index-aware would defeat the feature outright, so
the split is load-bearing rather than stylistic.

The split cannot be implemented as a per-tool switch. `references.trace` owns no
enumeration of its own: `traceTicketReferences` reaches the board through
`TicketsStatus` and `traceSpecReferences` through `TicketsFind`
(`internal/wsdoc/references.go:34,63`) - the same two entry points that also
serve discovery callers. Resolution-vs-discovery is therefore a property of the
**call**, and must be threaded as a scan option set per call site rather than
inferred from which function is running.

The hidden count on discovery surfaces is required, not cosmetic: a silently
short list reports "nothing there" when the truth is "filtered". It must come
from a path-only index enumeration of the status directories, which needs no
blob reads. It must **not** be obtained by routing a discovery call through
`loadTicketGraph`: the discovery entry points call `scanTickets` directly
(`internal/wsdoc/tickets.go:60,68,122`) and build no graph, so doing that would
pull `.done/`/`.dropped/` body reads onto every discovery call - the cost
regression this ticket promises not to introduce.

Mutation is the third role, and it fails differently from both. `tickets.move`
and `tickets.close` reach git through `atomicGitMove`, which runs `git add`
then `git mv --force` and propagates the raw git error
(`internal/wsdoc/tickets_mutate.go:648-660`). Under an active scope the
declared hot path - `idea/` -> hidden `todo/` triage of a mandatory dogfood
capture - therefore surfaces as an opaque git failure that names neither the
scope nor the remedy. Two cases need handling: a destination outside the scope
(the exit-1 no-op documented below), and a source that resolves by stem but has
no file on disk, which becomes reachable as soon as `tickets.status`
resolves hidden stems.

### Why a partial board is not an acceptable degradation

`260728-research-ticket-graph-load-cost-commit-path` already established this
for a different partial-board proposal (excluding `.done/`/`.dropped/` from the
scan), and the reasoning transfers unchanged: `graph.byStem` resolves
`parent:`/`related:` targets, so a missing ticket turns a correct reference into
an unresolved-target `FIX:` advisory, and `graph.children` built from a partial
set makes the all-children-closed advisory fire on an epic whose open children
are merely hidden. In this repository `related:` density is high enough that
almost every scoped commit would produce at least one false `FIX:` telling the
agent to delete correct frontmatter.

That ticket also measured what the graph actually consumes per archived ticket -
`Stem`, `Status`, and `Parent` only - which bounds this ticket's index path:
`Stem` and `Status` are derivable from the path with no blob read, so only
`parent:` requires reading content for hidden entries.

### Scope state is discovered through `workflow_manual`, not a file in the repo

Sparse configuration is invisible in the repository, which is how "where did my
tickets go" becomes a recurring re-investigation. `ws/workflow_manual` is loaded
at every session start and already renders environment warnings, so rendering
the active scope there makes the state self-announcing.

Recording it in `ai-docs/_index.local.md` was considered and rejected: that file
is ignored and is not propagated into worktrees at all
(`260523-bug-worktree-local-index-missing`), and a hand-maintained record can go
stale against the live pattern file.

## Constraints

- Every index-aware path is gated on `core.sparseCheckout`. With the filter off,
  behavior and cost must be byte-identical to today.
- Board enumeration under sparse must be the **union** of the index and the
  working tree. `git ls-files` alone misses an uncommitted new ticket; untracked
  entries come from `--others --exclude-standard`.
- Ticket stems are immutable absolute references, so a hidden ticket must remain
  a resolvable `related:`/`parent:` target - hiding is a view concern only.
- `--no-cone` is a legacy mode. No deprecation warning was observed on the
  versions exercised here, but the supported git range should be checked before
  shipping.

## Verified behavior (2026-08-06, git 2.43.0, Linux)

Established by direct experiment in throwaway repositories:

- Patterns are worktree-local: `core.sparseCheckout` lands in worktree-local
  config and the pattern file is `$GIT_DIR/worktrees/<name>/info/sparse-checkout`.
  Enabling it flips `extensions.worktreeConfig=true` in shared config.
- Hiding is per file, not per directory. With
  `!/ai-docs/tickets/todo/*` plus topic re-includes, `todo/` keeps its matching
  tickets on disk while the index retains all of them. A status directory
  disappears from disk only when every file in it is excluded, because git does
  not track empty directories.
- `git ls-files -v` marks excluded entries `S` (skip-worktree); `git show :<path>`
  reads a hidden ticket's body from the index.
- Staging an out-of-scope path exits **1** whether the path is tracked-but-hidden
  or a brand-new untracked file, in both `git add <path>` and
  `git add -A -- <path>` form. The path stays `??` in `git status`.
- Cross-scope `git mv` (e.g. triaging `idea/` -> hidden `todo/`) exits **1** and
  is an atomic no-op: index, working tree, and HEAD are all unchanged and
  `git status` stays clean because nothing happened. Widening the pattern first
  makes the same `git mv` exit 0 and produce a clean `R` rename.
- `git sparse-checkout disable` fully restores the worktree.

### Unreproduced hazard

A prior session (Windows, git 2.48.1) reported that a re-include pattern could
make the whole status directory vanish, taking the explicitly re-included file
with it. A 2x2 isolating the two suspected variables - exclusion at
`tickets/*/*` vs `tickets/<status>/*`, re-include by glob vs by full path - did
**not** reproduce it: all four cells behaved correctly. Two further hypotheses,
cone mode left enabled and a CRLF pattern file, also failed to reproduce it.

The cause is therefore unknown, not isolated. No "avoid this pattern shape" rule
can be written. What follows is that pattern application must be verified by
listing the affected directories every time, and that the failure direction is
"hides too much", which is silent without that check.

## Spec Impact

No existing anchor covers scope-aware board resolution, so neither phase can
point at a confirmed `spec:` stem; what follows is what each phase's
caller-visible change will need documented. Neither phase removes a spec entry,
so no `spec-remove:` applies.

### `mcp-tools`, for phase 1

Target area: `## Ticket Discovery Tools {#260505-ticket-discovery-tools}` and
the mutation entries under it (`#260620-ticket-move-tool`,
`#260620-ticket-close-tool`).

The discovery-vs-resolution split becomes a documented contract rather than an
implementation detail. Expected caller-visible changes, all gated on
`core.sparseCheckout` being set:

- `tickets.list`, `tickets.find(query:)`, and `project_tree` report the
  worktree-visible board and carry a hidden count, so a filtered result is
  distinguishable from an empty one.
- `tickets.find(ticket_stem:)` and `tickets.status(ticket_stem:)` resolve a
  hidden stem and mark it hidden instead of reporting it absent.
- `tickets.verify` / `ws/git.commit` advisories are computed over the whole
  board, so an active scope never changes which advisories fire.
- `references.trace` resolves hidden tickets on both its ticket and spec
  branches.
- `tickets.move` / `tickets.close` failures caused by an out-of-scope source or
  destination name the scope and the widen-then-retry remedy instead of relaying
  a raw git error.

With `core.sparseCheckout` unset, every one of these is unchanged - that
no-change guarantee is itself part of what the spec must state.

### `workflow-skills` and `mcp-tools`, for phase 2

- `workflow-skills`: `ws:lead-scope-worktree` joins the entry-skill inventory
  the spec enumerates, carrying its discuss-first contract and the
  `git sparse-checkout disable` restore path.
- `mcp-tools`: `workflow_manual`
  (`#260626-workflow-manual-restoration-entry`) renders the active scope when
  `core.sparseCheckout` is set, alongside the environment warnings it already
  carries.

## Phases

### Phase 1: index-aware board resolution

Introduce a board resolution path that sees the whole board when
`core.sparseCheckout` is set, and route the resolution surfaces named in the
boundary table onto it. Discovery surfaces keep walking the filesystem and gain
a hidden-count annotation from a separate path-only index enumeration.

Entry set to change: `loadTicketGraph`, `references.trace`, the
`ticket_create` collision check (its `os.Stat` guard cannot see a hidden ticket),
the explicit-stem forms of `tickets.find`/`tickets.status`, which should
report a hidden ticket as found-but-hidden rather than absent, and
`tickets.move`/`tickets.close`, which must fail with a message naming the scope
and the widen-then-retry remedy instead of relaying a raw git error.

Constraints for this phase:

- The resolution/discovery choice is a scan option carried per call site, not a
  per-tool behavior; `references.trace` shares `TicketsStatus`/`TicketsFind`
  with discovery callers and must be able to ask for the whole board through
  them.
- Enumeration is the union described above; do not replace the filesystem walk
  with an index walk.
- On the graph path, derive `Stem`/`Status` for hidden entries from the path and
  read content only for what the graph consumes (`Parent`). This bound is
  specific to `loadTicketGraph` and must not be generalized: `TicketsFind`'s
  query form text-matches every ticket body and returns an error for the whole
  call on a failed read (`internal/wsdoc/tickets.go:88-90`). A resolution-mode
  query must therefore supply hidden bodies from the index rather than skip
  them, or `references.trace`'s spec branch (`internal/wsdoc/references.go:63`)
  stops matching hidden tickets - the exact miss that routing it index-aware
  exists to prevent.
- Failure to resolve the index must not block a commit. `TicketVerify` already
  swallows `loadTicketGraph` errors; do not make that silent-failure mode worse
  (noted as an open gap in `260728-research-ticket-graph-load-cost-commit-path`).

Verification must include a scoped-worktree fixture proving that a ticket whose
`related:` points at a hidden stem produces **no** `FIX:` advisory, that an
epic with hidden open children does not emit the all-children-closed advisory,
and that a blocked `idea/` -> hidden `todo/` move reports the scope rather than
a raw git error.

Caller-visible MCP behavior changes here; `## Spec Impact` above states what the
`mcp-tools` area must document.

### Result (0daa9b74) - 2026-08-06

Board resolution is index-aware behind a filesystem-first gate. `wsdoc` gained an
unexported per-call `ticketScope` (`tickets_scope.go`) that execs git directly -
no `wsgit` import, no `GitRunner` threading, following the `gitIgnoreMatcher`
precedent already in the tree. Resolution/discovery is a `Resolve` scan option
threaded per call site as the constraint required, so `references.trace` reaches
the whole board through the same `TicketsStatus`/`TicketsFind` the discovery
callers use.

Behavioral delta beyond the phase text:

- Discovery enumerates the union too, rather than staying filesystem-only with a
  bare count. Each hidden entry carries `hidden` (text flag list and
  `json:"hidden,omitempty"`); `tickets.list` text mode additionally carries the
  aggregate `scope:` line. The phase intro's "discovery surfaces keep walking the
  filesystem" reads narrower than the constraint block's union rule; the union
  rule won.
- `tickets.list(json: true)` gets the per-ticket mark but no aggregate count. A
  `TicketBoard` wrapper was rejected because it turns the response from an array
  into an object. Accepted limitation, stated in the spec.
- Hidden bodies are supplied from one `git cat-file --batch` so `TicketsFind`'s
  query form still text-matches them, per the constraint.

Deviations:

- `git sparse-checkout check-rules` needs git >= 2.42. Below that the destination
  pre-flight fails open and the refusal arrives after the source write, so that
  path returns a `PartialMutationNotice` naming what was written and what a retry
  would duplicate. Delivered on all four surfaces (MCP + CLI, move + close).
- Plan Step 8 (the `mcp-tools` update) was excluded from the implementer
  delegation and executed in the lead's documentation pre-pass. All three
  reviewers filed it and accepted the deferral across three cycles.
- The mental-model entry was written by the lead inline rather than dispatched to
  `mental-model-updater`.

Verification: the phase's three required fixtures all landed (hidden `related:`
produces no `FIX:` advisory; an epic with hidden open children emits no
all-children-closed advisory; `idea/` -> hidden `todo/` reports the scope). From
`agents-plugin-tool/`: `go build ./...`, `go vet ./...`, and
`go test ./... -count=1` clean across all 12 packages, verified independently by
the lead at each cycle. Three review cycles; cycle 3 returned correctness
`clean`, test `clean`, fit `clean with 1 minor remaining`. Cycle-1 and cycle-2
findings and their mutation evidence are recorded in the dispositions files
referenced from the review-paths cache.

Unresolved, carried out of this phase:

- The CLI `tickets move` partial-mutation rendering omits one clause the MCP
  variant carries. No information is lost; accepted as cosmetic.
- No test exercises a real `git mv` sparse refusal's exact error text; that
  trigger is unreachable on any host whose git has `check-rules`. The delivery
  and content-gating properties are covered by an occupied-destination
  substitute, mutation-confirmed non-vacuous.
- `references.go`'s first-wins `byStem` lookup remains a local reimplementation
  of `TicketsStatus`'s resolution order rather than a shared helper; confirmed
  behaviorally equivalent and kept as a justified perf fix.

Spec: `mcp-tools` `{#260806-worktree-sparse-checkout-ticket-scope}`.
Mental model: `mcp-runtime` `{#260806-wsdoc-git-read-access}`.

### Phase 2: `ws:lead-scope-worktree` skill and reference manual

Depends on Phase 1: shipping the skill first would hand users a configuration
that makes the commit gate emit false `FIX:` advisories.

Deliverables:

- `ws:lead-scope-worktree` - a thin skill. It always opens by discussing with
  the user what this worktree actually targets before touching anything, derives
  the pattern set from that conversation, applies it, verifies by listing the
  affected directories, and offers the restore path. Restore is
  `git sparse-checkout disable`. Naming follows the verb-first convention shared
  by `lead-add-rule`, `lead-drain-ready-queue`, `lead-forge-spec`, and
  `lead-write-ticket`.
- `ai-docs/ref/worktree-ticket-scope.md` - the manual body: verified properties,
  failure modes, the cross-scope `git mv` behavior and its widen-then-retry
  remedy, the unreproduced hazard and the mandatory verification step it forces.
  The skill carries procedure only.
- `ws/workflow_manual` renders the active scope when `core.sparseCheckout` is
  set.

The skill must state that promoting an out-of-scope ticket into a hidden status
directory requires widening the pattern first, since `idea/` -> `todo/` triage of
a captured dogfood ticket is exactly that case and is the expected hot path.

Verification must cover all three deliverables, not just their existence:

- `workflow_manual` renders the active scope only when `core.sparseCheckout` is
  set, and its output is unchanged when it is not. This is the one deliverable
  with a code-level gate, so it takes a test rather than an inspection.
- The skill is exercised end to end in a real worktree of this repository:
  discuss-first before any pattern is written, apply, confirm by listing the
  affected status directories that exactly the intended tickets remain, promote
  an out-of-scope ticket to observe the widen-then-retry path, and restore with
  `git sparse-checkout disable`. The applied-then-restored worktree must be
  byte-identical to its pre-scope state.
- With a scope active, `ws/git.commit` on an unrelated ticket edit emits no
  `FIX:` advisory - the Phase 1 guarantee checked from the skill's side, since
  that is the failure the phase ordering exists to prevent.
- The skill text passes the invariant checklist in
  `ai-docs/ref/skill-authoring.md`, which is where the discuss-first ordering and
  the mandatory listing step are enforced as skill invariants rather than prose.

### Result (abf0859a) - 2026-08-06

All three deliverables shipped and reside in the grouped migration layout:
`agents-plugin/skills/lead-scope-worktree/` (thin shim),
`agents-plugin/rsrc/lead-scope-worktree/` (procedure body), the wsflow mirror,
`ai-docs/ref/worktree-ticket-scope.md` (manual), and
`agents-plugin-tool/internal/mcp/scope_announcement.go` (the render gate).

Verification against the phase's four bars:

- **workflow_manual gate (code-level, tested).** `scope_announcement_test.go`
  proves the render in both root-known branches (`TestScopeAnnouncementFiresOnWorkflowManual`),
  the true no-op when `core.sparseCheckout` is unset
  (`TestWorkflowManualScopeAnnouncementByteUnchangedWhenUnscoped`), and the
  scope-active-but-nothing-hidden branch (`TestScopeAnnouncementFiresWithNoTicketHidden`).
  The gate reuses Phase 1's `wsdoc.TicketScope` and is injected through the
  existing no-op-when-empty `injectBootstrapStalenessWarning`, so no new
  injector was added.
- **End-to-end real-worktree run.** Exercised in a throwaway linked worktree of
  this repo: discuss-first, apply, confirm-by-listing, `git sparse-checkout
  disable`. The applied-then-restored tree was byte-identical to its pre-scope
  state (SHA over 490 tracked files matched exactly).
- **Widen-then-retry path — surfaced and fixed a real remedy defect.** The E2E
  run proved the originally shipped remedy wrong: when every file in a status
  directory is excluded the directory vanishes from disk, and `git
  sparse-checkout add <dest-file>` does not recreate it, so a raw `git mv` into
  it fails with exit 128 `No such file or directory` even after widening. The
  correct promotion tool is `ws/tickets.move`, whose `atomicGitMove` does
  `os.MkdirAll(dir(newPath))` between stage and rename
  (`tickets_mutate.go:832-846`, source-verified). Both the skill (invariant +
  step 5) and the manual (Verified Behavior bullet + Widen-Then-Retry Remedy
  section) now state this; raw `git mv` is documented only with a preceding
  `mkdir -p`. Fixed in the result commit.
- **Skill-authoring checklist.** The fit reviewer confirmed the shim/rsrc shape
  and Choreography layout; a Fresh-Reader Audit caught one context-free
  violation (the idea-visible invariant named an undefined external rule) and it
  was self-contained in `78a3184e`.

Verification limitation recorded honestly: the `ws/git.commit`-emits-no-`FIX`
check and the MCP-side scope-aware refusals **could not be dogfood-verified**.
`ws/runtime.info` shows the running ws-mcp server is `0.38.0-dev` (pre-Phase-1);
the plugin cache serves a stale binary, and the scope behavior activates only
after a version bump plus plugin reinstall at >= 0.38.1. That behavior is
covered by Phase 1's `wsdoc` unit tests (including `requireGateSpawnsNoGit` and
the scope-refusal tests); the skill-side no-`FIX` guarantee follows from Phase
1 being on `main`, not from a live dogfood observation this session could make.

## Non-Scope

- No automatic derivation of patterns from an epic stem or graph closure. A
  wrong scope is cheap to correct here, so the machinery is not justified; the
  skill's conversation plus explicit add/remove is enough.
- Does not address the graph-load cost question owned by
  `260728-research-ticket-graph-load-cost-commit-path`, though Phase 1 touches
  the same load path and should not make that cost worse.
- Does not propagate worktree-local workflow context
  (`260523-bug-worktree-local-index-missing`); only ticket visibility is in
  scope.


## Resolution (2026-08-07)

Both phases shipped. Phase 1 landed index-aware ticket board resolution for scoped worktrees (0daa9b74). Phase 2 added the ws:lead-scope-worktree skill, the ai-docs/ref/worktree-ticket-scope.md manual, and the workflow_manual sparse-checkout scope announcement gate, with an E2E run that surfaced and fixed a vanished-directory promotion-remedy defect (ws/tickets.move recreates the emptied status dir via MkdirAll; raw git mv needs mkdir -p first). The ws/git.commit no-FIX and MCP-side scope refusals are covered by wsdoc unit tests; they could not be dogfood-verified this session because the running ws-mcp server was a stale pre-Phase-1 0.38.0-dev build. Version bumped to 0.38.2 for cache invalidation on merge.
