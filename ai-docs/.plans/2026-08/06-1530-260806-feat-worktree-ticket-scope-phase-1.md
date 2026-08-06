# Plan: 260806-feat-worktree-ticket-scope — Phase 1: index-aware board resolution

All paths below are relative to `agents-plugin-tool/` unless they start with
`ai-docs/`.

## Relevant Ticket Contract

- Entry set to change (the ticket's own list): `loadTicketGraph`,
  `references.trace`, the `ticket_create` collision check, the explicit-stem
  forms of `tickets.find`/`tickets.status` (report hidden-but-found, not
  absent), and `tickets.move`/`tickets.close` (fail with a message naming the
  scope and the widen-then-retry remedy instead of relaying a raw git error).
- Boundary table: discovery surfaces (`tickets.list`, `tickets.find(query:)`,
  `project_tree`) stay filesystem-only and gain a hidden-count annotation from a
  **separate path-only index enumeration** — never routed through
  `loadTicketGraph` (that would pull `.done/`/`.dropped/` body reads onto every
  discovery call).
- Resolution-vs-discovery is a property of **the call**, threaded as a scan
  option per call site, not inferred from which function is running.
  `references.trace` shares `TicketsStatus`/`TicketsFind` with discovery callers
  and must be able to ask for the whole board through them.
- Enumeration under sparse is the **union** of index and working tree; do not
  replace the filesystem walk with an index walk.
- On the graph path only, derive `Stem`/`Status` for hidden entries from the
  path and read content only for `Parent`. `TicketsFind`'s resolution-mode query
  must supply hidden **bodies** from the index, not skip them, or
  `references.trace`'s spec branch stops matching hidden tickets.
- Every index-aware path is gated on `core.sparseCheckout`; with it unset,
  behavior and cost must be byte-identical to today.
- `TicketVerify`/`loadTicketGraph` failure must not block a commit; the existing
  silent-degrade path (`internal/wsdoc/tickets_verify.go:94-96`) must not get
  worse.
- Verification must include a scoped-worktree fixture proving: (1) a `related:`
  pointing at a hidden stem produces no `FIX:` advisory, (2) an epic with hidden
  open children does not emit the all-children-closed advisory, (3) a blocked
  `idea/` → hidden `todo/` move reports the scope, not a raw git error.
- Spec impact for this phase lands in `ai-docs/spec/mcp-tools.md` under
  `{#260505-ticket-discovery-tools}` and the mutation entries
  `{#260620-ticket-move-tool}` / `{#260620-ticket-close-tool}`.

## Out of Scope

- Phase 2 (`ws:lead-scope-worktree` skill, `ai-docs/ref/worktree-ticket-scope.md`
  manual, `workflow_manual` scope rendering). Phase 1 must not add a scope
  renderer to `workflow_manual`.
- The graph-load cost question owned by
  `260728-research-ticket-graph-load-cost-commit-path`.
- Worktree-local workflow context propagation
  (`260523-bug-worktree-local-index-missing`).
- Automatic pattern derivation from an epic stem or graph closure.
- **Widening the `ticket_create` collision check.** Today it is a
  destination-path-only `os.Stat`; a same-stem ticket sitting in a *different*
  status directory does not block creation today either. Make the existing check
  index-aware; do not turn it into a whole-board stem search.
- **Making `project_tree`'s `parent:`/`related:` title resolution index-aware.**
  `ticketTitle` (`internal/wsdoc/project_tree.go:275-284`) is a presentation
  nicety on a discovery surface; a hidden target simply renders without its
  title suffix. Only the hidden count is added to `project_tree`.

## Codebase Findings

### The decisive precedent: `wsdoc` already runs git directly

`internal/wsdoc/project_tree.go:99-118` (`gitIgnoreMatcher`) shells out with
`os/exec` — `exec.Command("git", "-C", repoRoot, "rev-parse",
"--is-inside-work-tree")`, then a memoized `git check-ignore` per path — with
**no** `GitRunner` parameter and no `wsgit` import. It degrades to a
`func(string) bool { return false }` no-op closure when the repo probe fails.

The import boundary `{#260720-wsdoc-commit-boundary}`
(`ai-docs/mental-model/mcp-runtime.md:104`, echoed in
`internal/wsgit/git.go:36-51`) forbids `wsdoc` **importing `wsgit`** because
`wsgit.Client.Verifier` calls into `wsdoc.TicketVerify` and the reverse import
would invert the dependency direction. It does not forbid `wsdoc` from running
git. `gitIgnoreMatcher` is the standing proof that this repository already
accepts a wsdoc-local, exec-based, per-call, degrade-to-inert git accessor.

`internal/wsdoc/tickets_mutate.go:12-17`'s `GitRunner` exists so the *mutation*
helpers can be handed `wsgit.ExecRunner{}` by the MCP/CLI dispatch layer — it is
a caller-injection convenience for two functions, not a rule that every wsdoc
git access must be a parameter.

### Verified git behavior (git 2.43.0, Linux, 2026-08-06)

Re-run in throwaway repos for this plan; all of it is load-bearing below.

- A **plain `git init` repo needs no linked worktree** to reproduce the feature.
  `git sparse-checkout set --no-cone '/*' '!/ai-docs/tickets/todo/*'
  '/ai-docs/tickets/todo/<kept>.md'` in a plain repo yields
  `core.sparseCheckout=true`, sets `extensions.worktreeConfig=true`, writes the
  pattern file to `.git/info/sparse-checkout`, and removes exactly the excluded
  file from disk. In a linked worktree the same state lives at
  `<common>/.git/worktrees/<name>/info/sparse-checkout`, which is exactly
  `$(git rev-parse --absolute-git-dir)/info/sparse-checkout` in both cases.
- `git config --type=bool --get core.sparseCheckout` prints `true` from inside
  the scoped tree and **exits 1 with no output** when never enabled.
- `git ls-files -z -- ai-docs/tickets` lists every index entry including the
  hidden ones. `git ls-files -v` tags hidden entries `S` (skip-worktree).
- `git ls-files --others --exclude-standard -- ai-docs/tickets` still reports
  untracked new tickets, so the index ∪ worktree union is buildable.
- `git show :<path>` reads a hidden ticket's body. **`git cat-file --batch` fed
  `:<path>` lines on stdin returns every hidden body in one process**, as
  `<oid> blob <size>\n<content>\n` records in request order, and prints
  `<input> missing` for an unknown path (exit 0). This is the batching primitive
  that keeps the graph path from spawning one process per hidden ticket.
- **`git sparse-checkout check-rules` exists at 2.43 and is exact.** Fed paths on
  stdin it echoes back only the ones **inside** the current sparsity rules —
  including paths that do not exist yet (a brand-new
  `ai-docs/tickets/todo/260199-feat-new.md` came back empty while the `idea/`
  sibling came back). This is the destination pre-flight primitive. It was added
  in git 2.42, so it must fail open on older git.
- Cross-scope `git mv` exits **1**, is an atomic no-op (`git status` stays
  clean), and prints git's `advice.updateSparsePath` text: *"The following paths
  and/or pathspecs matched paths that exist outside of your sparse-checkout
  definition, so will not be updated in the index: …"*. That text is
  gettext-localized, so it must not be parsed.
- **A hidden path also fails at `git add`, not only at `git mv`** — same message,
  exit 1. This corrects the ticket's "git add warns and declines to stage": under
  `atomicGitMove` (`internal/wsdoc/tickets_mutate.go:648-662`) the *first* git
  call is `git add oldPath`, so a hidden **source** fails one step earlier than a
  hidden destination. Both surface identically to the caller today.

### Call-site inventory (why the blast radius differs per option)

- `TicketVerify(root, paths)` (`internal/wsdoc/tickets_verify.go:70`) has **26**
  call sites across `internal/wsdoc`, `internal/mcp`, and `cmd/ws-mcp`, most of
  them tests. (The survey's "31" counted the `FormatTicketVerify` /
  `formatTicketVerify` formatter names as well; those are unaffected.) It
  reaches the graph three calls deep:
  `tickets_verify.go:94` → `tickets_graph.go:147` → `tickets_graph.go:79`
  (`loadTicketGraph`) → `tickets_graph.go:80` (`scanTickets`).
- `TicketsList` production callers: `internal/mcp/server.go:1235`,
  `cmd/ws-mcp/main.go:553`, `internal/wsdoc/legacy_marker.go:264`.
- `TicketsFind` production callers: `internal/mcp/server.go:1255`,
  `cmd/ws-mcp/main.go:578`, `internal/wsdoc/references.go:63`.
- `TicketsStatus` production callers: `internal/mcp/server.go:1276`,
  `cmd/ws-mcp/main.go:605`, `internal/wsdoc/references.go:34` and `:90`.
- `scanTickets` call sites: `tickets.go:60` (list), `tickets.go:68` (find),
  `tickets.go:122` (status), `tickets_graph.go:80` (graph).

### Shape facts the executor should not re-derive

- `frontmatter(path)` (`internal/wsdoc/frontmatter.go:8`) is read-file +
  parse-text with no bytes-based variant. `readTicket`
  (`internal/wsdoc/tickets.go:243-275`) reads the file at `:244` and then calls
  `frontmatter(path)` at `:252`, i.e. it already reads every ticket twice.
- `scanTickets` (`internal/wsdoc/tickets.go:144-179`) errors outright when
  `ai-docs/tickets` is missing (`:146-149`) and silently skips a status
  directory that is not on disk (`:157`). Both are reachable under a scope: git
  does not track empty directories, so a fully-excluded status directory — and,
  in the limit, `ai-docs/tickets/` itself — vanishes from disk while the index
  still holds every ticket.
- `toolJSONResponse` (`internal/mcp/server.go:3367-3376`) marshals the value and
  hands it to `toolTextResponse` as the *sole* text content block. Every MCP
  response in this server is single-block (`server.go:3378-3398`); nothing
  exercises multi-block content.
- `CommitResult.Advisories` (`internal/wsgit/git.go:446-448`) carries
  `json:"-"`, the settled precedent (`{#260626-git-commit-todo-reinjection}`,
  `{#260727-git-commit-verify-advisories}`) that a non-blocking annotation on a
  tool result is text-mode only and never changes the JSON contract.
- `formatTickets` (`internal/mcp/server.go:2994-3025`) renders `[]TicketInfo`
  into `[status] stem - title (path) [flags]` lines with a bracketed flag list —
  the natural place for a per-ticket `hidden` flag.
- Test harness already present: `runGit(t, root, args...)` at
  `internal/wsdoc/project_tree_test.go:181-189` is a package-level helper, and
  `TestProjectTreeSkipsGitIgnoredEntries` already `t.Fatalf`s when git is
  missing — a real-git dependency in `internal/wsdoc` tests is pre-existing and
  accepted. `mustWrite` and `graphFixture`
  (`internal/wsdoc/tickets_graph_test.go:17-70`) are the board-construction
  helpers; `newGraphFixture` deliberately creates `ai-docs/spec/demo.md` because
  `scanSpecs` errors on a missing spec directory and the degrade-to-silence path
  would swallow every advisory.
- `mockGitRunner` (`internal/wsdoc/tickets_mutate_test.go:15-37`) fakes `git mv`
  with `os.Rename`. It can never reproduce a sparse refusal; the move tests in
  this phase need a real exec runner.

## Implementation Plan

### Contract changes (lead with these)

Public `wsdoc` surface — all additive, no signature breaks:

- `TicketInfo` gains `Hidden bool \`json:"hidden,omitempty"\`` — set only on
  entries sourced from the index with no file on disk.
- `TicketFindOptions` gains `Resolve bool`.
- `TicketStatusOptions` gains `Resolve bool`.
- New `func TicketScope(root string, statuses []string) (TicketScopeInfo, error)`
  returning `TicketScopeInfo{ Active bool; Hidden int; HiddenStems []string }`.
  This is the path-only index enumeration the boundary table demands for the
  discovery surfaces. It never reads a blob.

`TicketsList`, `TicketsFind`, `TicketsStatus`, `TicketVerify`, `TicketCreate`,
`ReferencesTrace` keep their exact signatures and return types. `TicketsClose`
and `TicketsMove` keep their `runner GitRunner` parameter unchanged.

### Step 1 — `internal/wsdoc/tickets_scope.go` (new): the scope accessor

Governing symbol: unexported `ticketScope`, constructed by
`newTicketScope(root string) *ticketScope`. **A nil return means "no scope
active", and every downstream branch on a nil scope is exactly today's code
path.** This is the single gate; nothing else in the change may test
`core.sparseCheckout`.

Gate order, cheapest first, so an unscoped repository spawns **zero** git
processes (this is what makes the ticket's byte-identical-cost constraint
literally true rather than approximately true):

1. Resolve `GIT_DIR` with the filesystem only: `<root>/.git` as a directory is
   `GIT_DIR`; as a file, read its single `gitdir: <path>` line. Neither ⇒ nil.
2. `os.Stat(<GIT_DIR>/info/sparse-checkout)` — absent ⇒ nil. Verified to be the
   pattern-file location for both a plain repo and a linked worktree.
3. `git -C <root> config --type=bool --get core.sparseCheckout` — non-zero exit
   or output ≠ `true` ⇒ nil.

Methods, each memoized on the struct for the life of one call:

- `indexPaths()` — `git ls-files -z -- ai-docs/tickets`, parsed with the same
  NUL-splitting shape as `wsgit`'s `parseNULTerminatedPaths`
  (`internal/wsgit/git.go:315-328`); reimplemented locally, not imported.
- `bodies(paths []string)` — one `git cat-file --batch` with `:<path>` lines on
  stdin; parse `<oid> blob <size>\n` headers and read exactly `size` bytes.
  `<input> missing` lines are skipped. One process for the whole hidden set.
- `includes(path string) bool` — `git sparse-checkout check-rules` with the path
  on stdin; the path is in scope iff it comes back. **Fails open** (returns
  `true`) on any error, which is how git < 2.42 is handled; the post-hoc wrap in
  Step 6 is the backstop.

Follow `gitIgnoreMatcher`'s posture throughout: exec directly, memoize, and let
a probe failure degrade rather than propagate.

### Step 2 — `internal/wsdoc/tickets.go`: union enumeration in `scanTickets`

Governing symbol: `ticketScanOptions` gains
`Resolve ticketResolveMode` with three values:

- `resolveOff` (zero value) — today's behavior exactly; no scope is even
  constructed.
- `resolveGraph` — hidden entries are materialized with `Stem` and `Status`
  derived from the path and only `Parent` parsed from the index body. This is
  the ticket's `loadTicketGraph`-specific bound.
- `resolveFull` — hidden entries are materialized with the full body, so
  `TicketsFind`'s query form can text-match them.

Behavioral changes inside `scanTickets`, all under `Resolve != resolveOff` **and**
a non-nil scope:

1. The missing-`ai-docs/tickets` error (`:146-149`) and the per-status
   `isDir` skip (`:157`) must no longer be terminal: a fully-excluded status
   directory, or a fully-excluded board, has no directory on disk while the index
   still holds every ticket. Treat an absent directory as "the walk contributes
   nothing" and let the index supply the rest. In `resolveOff` mode both keep
   their current behavior verbatim.
2. After the filesystem walk, take the index paths under the requested statuses,
   subtract the paths already collected from disk, and materialize the remainder
   as `TicketInfo{Hidden: true}`. Hidden-ness is defined as *in the index and not
   on disk* — derived from the two sets already in hand — rather than from
   `git ls-files -v`'s `S` tag, so a manually applied `--skip-worktree` cannot
   change the answer.
3. The append happens before the existing `sort.Slice` at `:172-177`, so
   `loadTicketGraph`'s first-wins `byStem` rule (`tickets_graph.go:98-106`) keeps
   working unchanged.

Enabling refactor (mechanical, no behavior change): split
`frontmatter(path)` into a path wrapper plus `frontmatterFromText(string)`, and
split `readTicket` into a bytes-taking `readTicketFromBytes(root, relPath,
status, raw []byte)` used by both the disk and index paths. This also removes
`readTicket`'s existing double read of every ticket file.

Error posture, which resolves the ticket's "do not make the silent-failure mode
worse" constraint: a failure in the **gate** (steps 1–3 of `newTicketScope`)
degrades to a nil scope, i.e. today's behavior, which is correct in the
overwhelmingly common unscoped case. A failure **after** the gate has confirmed
a scope is active means the board is known-partial, so `scanTickets` returns the
error; `loadTicketGraph` propagates it and `TicketVerify` swallows it
(`tickets_verify.go:94-96`), yielding *no* advisories. Emitting no advisories is
strictly better than emitting false `FIX:` advisories over a partial board, and
it is not a new silent-failure mode — it is the existing one, reached one branch
earlier.

### Step 3 — route the resolution surfaces

- `loadTicketGraph` (`internal/wsdoc/tickets_graph.go:80`) passes
  `Resolve: resolveGraph`. `TicketVerify`'s signature and its 31 call sites are
  untouched.
- `TicketsFind` (`internal/wsdoc/tickets.go:67-112`) maps
  `TicketFindOptions.Resolve` to `resolveFull`. Its `os.ReadFile` at `:88` —
  which today returns an error for the whole call on a failed read — must read
  hidden entries from the scope's memoized bodies instead of the filesystem.
  This is exactly the constraint the ticket calls out: skipping hidden entries
  here would break `references.trace`'s spec branch.
- `TicketsStatus` (`internal/wsdoc/tickets.go:114-136`) maps
  `TicketStatusOptions.Resolve` to `resolveFull`; a hidden match returns
  normally with `Hidden: true` rather than `ticket not found`.
- `internal/wsdoc/references.go` sets `Resolve: true` at all three call sites:
  `:34` (`traceTicketReferences`), `:63` (`traceSpecReferences`), `:90`
  (`ticketsFromSpecRefs`).
- `internal/mcp/server.go`: `tickets.status` (`:1276`) sets `Resolve: true`
  unconditionally; `tickets.find` (`:1255`) sets `Resolve: ticketStem != ""`, so
  the explicit-stem form resolves and the query form stays discovery.
  `tickets.list` (`:1235`) leaves it false. Mirror both in
  `cmd/ws-mcp/main.go:578,605`.
- `formatTickets` (`internal/mcp/server.go:2994`) appends `hidden` to its
  existing bracketed flag list when `ticket.Hidden` is set.

### Step 4 — `TicketCreate` collision check

`internal/wsdoc/ticket_create.go:45`: keep the `os.Stat(destAbs)` fast path; when
it reports absent **and** a scope is active, also reject when the scope's index
paths contain `relPath`. The rejection message must say the colliding ticket is
outside this worktree's scope, since the caller cannot see the file. Semantics
stay destination-path-only (see Out of Scope).

### Step 5 — `findTicketPath` becomes index-aware

`internal/wsdoc/tickets_mutate.go:582-593` gains the scope and a third piece of
information: `(path, status string, hidden bool, err error)`. A stem present in
the scope's index but with no file on disk resolves to `hidden = true`. A stem
in neither resolves to today's `ticket not found: %s`. **This is what makes the
two cases distinguishable at all**, and it is the same lookup `tickets.status`
now performs, so the two surfaces cannot disagree about whether a stem exists.
Both internal callers (`TicketsClose:85`, `TicketsMove:133`) are updated.

### Step 6 — scope-aware failure for `tickets.move` / `tickets.close`

Three layers, in order. **No git stderr is parsed at any layer** — git's
sparse-path advice is gettext-localized (verified), so a substring match would
be locale-dependent, and the index knowledge needed to classify correctly is
already in hand.

1. **Source pre-flight** (exact, free): `findTicketPath` reported `hidden`.
   Fail before any git call with a message naming the active scope, the source
   path, and the widen-then-retry remedy. This is the case that only becomes
   reachable because `tickets.status` now resolves hidden stems.
2. **Destination pre-flight** (exact where available): before `atomicGitMove`,
   `scope.includes(newPath)`. False ⇒ fail with the same message shape naming
   the destination. Fails open on git < 2.42.
3. **Post-hoc wrap** (backstop, non-claiming): if `atomicGitMove` still returns
   an error while a scope is active, wrap — do not replace — the raw git error
   with a suffix that states a scope is active and names the widen-then-retry
   remedy *conditionally* ("if the destination is outside it"). Wrapping rather
   than replacing keeps an unrelated git failure legible and keeps the message
   honest without classifying it.

Both blocked paths are safe to report as no-ops: the cross-scope `git mv`
failure is atomic (verified — index, worktree, and HEAD unchanged, `git status`
clean). `TicketMutateResult.PartialMutationNotice`
(`internal/wsdoc/tickets_mutate.go:42-54`) still applies unchanged on the
separate sage-posture write-then-reject path and must not be conflated with
this one.

### Step 7 — hidden-count annotation on the discovery surfaces

`TicketScope(root, statuses)` (contract above) runs the gate, and when active
returns the count of index paths under `statuses` that are absent from disk. It
reads no blobs and never touches `loadTicketGraph`, satisfying the ticket's
explicit prohibition. When the gate says inactive it returns
`{Active: false, Hidden: 0}` after at most two `os.Stat` calls.

`internal/mcp/server.go` calls it from `tickets.list` (`:1243`),
`tickets.find`'s query form (`:1266`), and `project_tree`, appending one
trailing line to the **text** rendering when `Active && Hidden > 0`, e.g.:

```
scope: 3 ticket(s) hidden by this worktree's sparse-checkout scope (core.sparseCheckout); they remain in the index and resolvable by stem.
```

JSON mode (`wantsJSON`) is left byte-identical: the marshalled `[]TicketInfo`
array is the entire content block. `project_tree` has no JSON mode and its
`renderTickets` (`internal/wsdoc/project_tree.go:218-265`) walks the filesystem
directly, so the annotation is appended by the `project_tree` MCP case rather
than threaded into `ProjectTree`.

### Step 8 — spec

Update `ai-docs/spec/mcp-tools.md` at `{#260505-ticket-discovery-tools}` and the
`{#260620-ticket-move-tool}` / `{#260620-ticket-close-tool}` entries with the
five caller-visible changes the ticket's `## Spec Impact` enumerates, including
the explicit statement that with `core.sparseCheckout` unset every one of them is
unchanged. Record `spec: mcp-tools` in the commit's `## Spec` section.

### Settled decisions and the alternatives rejected

**How the affected `wsdoc` entry points obtain git access.**
*Chosen:* a wsdoc-package-internal, exec-based, per-call `ticketScope` (Step 1),
plus a per-call `Resolve` option on the existing options structs. Evidence:
`gitIgnoreMatcher` (`internal/wsdoc/project_tree.go:99-118`) is the same shape
already in the tree, and `{#260720-wsdoc-commit-boundary}` bans the *import*,
not git execution.
*Rejected — threading `runner GitRunner` the way `TicketsClose`/`TicketsMove` do:*
it touches ~50+ call sites including all 26 `TicketVerify` sites, and it makes
every test that never uses git construct a runner. Worse, it makes the
sparse-off path structurally different from today (a mandatory parameter on
`TicketVerify`), which is the one thing the ticket says must not change.
*Rejected — an optional `Runner GitRunner` field defaulted to a package exec
runner* (the `wsgit.Client.runner()` nil-default at `internal/wsgit/git.go:60-65`
is the precedent): it advertises an injection point every real caller would
leave nil, and duplicates `wsgit.ExecRunner` for no gain over `gitIgnoreMatcher`.
*Rejected — replacing the filesystem walk with an index walk:* forbidden by the
ticket's union constraint, and it would drop uncommitted new tickets.

**What shape the hidden-count annotation takes.**
*Chosen:* two separate mechanisms for two separate needs. The **per-ticket** mark
is an additive `TicketInfo.Hidden bool` with `json:"hidden,omitempty"` — required
anyway by the boundary table's "marked as hidden" row for
`tickets.find(ticket_stem:)`/`tickets.status(ticket_stem:)`, JSON-safe, and
rendered as a flag by `formatTickets`. The **aggregate count** on discovery
surfaces is a text-mode-only trailing line from the standalone `TicketScope`
query, following the `CommitResult.Advisories` `json:"-"` precedent
(`internal/wsgit/git.go:446-448`, `{#260626-git-commit-todo-reinjection}`) that a
non-blocking annotation never changes a tool's JSON contract.
*Rejected — wrapping the return in a `TicketBoard{Tickets, Hidden}` struct:* it
turns `tickets.list`/`tickets.find`'s JSON from an array into an object, a
breaking caller-visible change the ticket's Spec Impact does not authorize, and
it cascades into `legacy_marker.go:264`, `references.go:63`, and ~20 tests.
*Rejected — a second return value `([]TicketInfo, TicketScopeInfo, error)`:*
smaller cascade than the wrapper but still an arity break at every call site,
and it forces every caller that does not care about scope to discard a value.
*Rejected — emitting the count as a second MCP content block:* it would give JSON
callers parity without touching the JSON document, but every response this server
produces today is single-block (`internal/mcp/server.go:3378-3398`) and no host
behavior for multi-block content is exercised anywhere in the tree.
*Accepted limitation:* a `tickets.list(json: true)` caller sees a short list with
no count in Phase 1. Phase 2's `workflow_manual` scope rendering is the
always-available, mode-independent scope surface that closes this.

**Where the `tickets.move`/`tickets.close` failure message comes from.**
*Chosen:* index-driven pre-flight (Step 6 layers 1 and 2) with a non-classifying
post-hoc wrap as backstop. The hidden-source case is distinguished from
genuinely-not-found by `findTicketPath` consulting the index: present in the
index with no file on disk ⇒ hidden; absent from both ⇒ not found. The
destination case uses `git sparse-checkout check-rules`, which is exact even for
a path that does not exist yet (verified).
*Rejected — parsing `atomicGitMove`'s stderr:* git's sparse-path advice is
gettext-localized, so the substring match would silently stop working under a
non-English locale, and `ExecRunner` (`internal/wsgit/git.go:26-34`) does not pin
`LC_ALL`. It would also still leave the hidden-source case undiagnosed, because
that one fails at `git add`, not `git mv` (verified) — one step earlier than the
error shape a `git mv` parser would look for.
*Rejected — pre-flighting the destination by asking whether the destination
status directory contains any hidden sibling:* a heuristic, and wrong whenever
the pattern re-includes by full path.

## Verification Plan

### Harness shape (settled)

Real git in `t.TempDir()`, **plain repo, no linked worktree**. Verified above
that a plain `git init` repo reproduces every property this phase depends on,
and the production gate reads `GIT_DIR` generically, so a linked worktree would
add setup cost while asserting nothing extra. Build on the package-level
`runGit` helper (`internal/wsdoc/project_tree_test.go:181-189`) and `mustWrite`;
extend `graphFixture` (`internal/wsdoc/tickets_graph_test.go:17-70`) with a
`scoped(...)` step rather than writing a parallel fixture type.

*Rejected — a fake `GitRunner`/stub board:* it would encode our belief about
git's sparse behavior rather than test it, and the feature's whole residual risk
is the ticket's unreproduced "hides too much" hazard, which only real git can
exhibit. It is also incompatible with the chosen Step 1 mechanism, which has no
injection point by design.

Fixture sequence: `git init` → `git config user.email/user.name` (required for
the commit) → write the board with `f.ticket(...)` plus the mandatory
`ai-docs/spec/demo.md` → `git add -A` → `git commit -m board` →
`git sparse-checkout set --no-cone '/*' '!/ai-docs/tickets/<status>/*'
'/ai-docs/tickets/<status>/<kept>.md'`.

**The fixture helper must assert the hide actually happened** — the intended-hidden
files absent from disk *and* the intended-kept files present — before the test
body runs. This is the ticket's mandatory listing step expressed as code: the
unreproduced hazard's failure direction is "hides too much", so without this
check F1 and F2 would pass vacuously on a fixture that hid the whole board.

Real-exec runner: `TicketsMove`/`TicketsClose` take a `GitRunner`, and
`mockGitRunner` (`internal/wsdoc/tickets_mutate_test.go:15-37`) fakes `git mv`
with `os.Rename`, which can never refuse. Add a small `execGitRunner` test
helper in the wsdoc test package mirroring `wsgit.ExecRunner`'s
`CombinedOutput` shape for the move tests only.

### Required scenarios (post-impl unless noted)

- **F1 — hidden `related:` target produces no FIX.** `todo/260101-feat-visible.md`
  carries `related: {260102-feat-hidden: note}`; `todo/260102-feat-hidden.md` is
  hidden by the pattern. `TicketVerify(root, ["ai-docs/tickets/todo/260101-feat-visible.md"])`
  returns no `AdvisoryKindFix`. **Paired live control in the same test:** a second
  `related:` entry pointing at a stem in neither index nor disk *does* produce a
  `FIX:` — otherwise the assertion cannot distinguish "correctly resolved" from
  "integrity check silently disabled".
- **F2 — epic with hidden open children.** Epic `ready/260100-epic-x.md`; child
  `.done/260101-feat-a.md` visible with `parent:` set; child
  `todo/260102-feat-b.md` with `parent:` set, hidden. Verifying the `.done` child
  must produce a board block *without* `actionAllChildrenClosed`. **Paired
  inversion control:** the same board with the second child genuinely removed
  from index and disk *does* emit the action. Together these pin the exact
  inversion `260728-research-ticket-graph-load-cost-commit-path` established.
- **F3 — blocked `idea/` → hidden `todo/` move names the scope.**
  `TicketsMove(root, execGitRunner{}, {TicketStem: "…", To: "todo"})` with `todo/`
  scoped out returns an error mentioning the scope and the widen-then-retry
  remedy, not git's raw text, and `git status --porcelain` reports no new change.
  Two further cases in the same table: a **hidden source** (`todo/` hidden ticket
  → `ready/`) blocked with the scope-naming message from the source pre-flight,
  and a **genuinely absent stem** still returning `ticket not found`.
- **Gate no-op.** With `core.sparseCheckout` unset — both in a git repo and in a
  bare `t.TempDir()` that is not a repo at all — `TicketsList`, `TicketsFind`,
  `TicketsStatus`, and `TicketVerify` produce output identical to the pre-change
  behavior, and `TicketScope` reports `Active: false, Hidden: 0`. Assert through
  observable output, not process counting.
- **Resolution surfaces.** `TicketsStatus(Resolve: true)` on a hidden stem
  returns the ticket with `Hidden: true` instead of `ticket not found`;
  `TicketsFind(Resolve: true, Query: <text only in a hidden body>)` matches it,
  which is the `references.trace` spec-branch guarantee; `TicketsFind` with
  `Resolve: false` does not.
- **`TicketCreate` collision.** Creating a stem whose destination path is hidden
  is rejected with a scope-naming message rather than silently overwriting the
  index entry.
- **Fully-excluded status directory.** A pattern that removes an entire status
  directory from disk (git does not track empty directories) must not make
  `scanTickets` error or drop the status; assert both the `resolveOff` and
  `resolveGraph` paths.
- **MCP annotation** (`internal/mcp/server_test.go`, which already has git-init
  helpers at `:700` and `:968`): text mode carries the hidden-count line; JSON
  mode's content block parses as a bare array and is byte-identical to the
  unscoped run.
- **`check-rules` version skew.** On git < 2.42, `includes` fails open and the
  move still fails with the post-hoc wrapped message. Gate this case on a parsed
  `git --version` and skip where the subcommand exists, rather than asserting a
  version-dependent message.

## Escalations

None blocking. Three notes for lead awareness, all settled above rather than
deferred:

- The ticket states "`git add` warns and declines to stage" for a hidden path;
  measured behavior at git 2.43 is **exit 1**, same as `git mv`. This does not
  change any decision — the `idea/` dogfood-capture path stays visible, which is
  what the ticket's reasoning actually depends on — but it does mean a hidden
  source fails at `atomicGitMove`'s first git call, which is why Step 6 pre-flights
  the source rather than classifying a `git mv` failure.
- `tickets.list(json: true)` gets no hidden count in Phase 1 (accepted
  limitation, rationale and closure path recorded under the Q2 decision above).
- `git sparse-checkout check-rules` requires git ≥ 2.42. The ticket's constraint
  "the supported git range should be checked before shipping" now has a concrete
  floor to check against; the fail-open design means older git degrades to the
  post-hoc wrapped message rather than breaking.
