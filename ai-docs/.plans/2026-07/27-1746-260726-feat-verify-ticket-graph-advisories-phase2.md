# Plan: 260726-feat-verify-ticket-graph-advisories — Phase 2: Ticket-graph advisories

## Relevant Ticket Contract

- Host is `wsdoc.TicketVerify`; all new checks are **non-blocking** (no new
  `Findings`, no existing severity changed). Emission rides Phase 1's advisory
  channel.
- **One graph load serves both halves**: the ancestor walk needs each ancestor's
  frontmatter + child set, which is the same input the integrity checks resolve
  against.
- **Graph-load failure degrades to silence, never to a veto.** A whole-board load
  or spec-anchor scan failure drops advisories and lets the commit proceed; it
  never becomes a Go error return.
- Frontmatter only. No check reads a ticket body.
- Two-namespace resolution: `related:` resolves against ticket stems UNION spec
  anchor stems (`{#YYMMDD-slug}` under `ai-docs/spec/`); `parent:` resolves
  against ticket stems only.
- Check set (all non-blocking): `parent:` unresolvable → `FIX:`; `related:` key
  resolving to neither ticket nor spec anchor → `FIX:`; `parent:` cycle →
  `CHECK:` **and no `## Parent Board` block for that ticket**; `parent:` target
  category not `epic` → `CHECK:`.
- Subject sets: integrity checks take the **verified ticket's own frontmatter**
  only and never inspect ancestors. The board block takes walked ancestors.
- Three-way emission split: integrity checks always; ACTION lines (both tiers)
  and the ancestor-already-closed NOTE always; the sibling listing (`N of M
  open` header plus its rows) **only when the verified path sits under `.done/`
  or `.dropped/`**. Gating reads the path's status directory — no staged-rename
  detection.
- Output Format is **settled**: ASCII only, status-first column padded to 8, `|`,
  stem, optional parenthetical; row sort `ready`→`todo`→`idea` then
  `.done`→`.dropped`; 5-row sibling cap with per-status overflow line; 5-advisory
  integrity cap with `... +N more`; ancestors labelled `Parent [N]:` by depth;
  ancestors deduplicated by stem per verify call; no `parent:` → **no section at
  all**.
- **The amend recipe (`Then git commit --amend --no-edit.`) is appended by the
  `git.commit` response layer only.** Check text stays commit-neutral so
  standalone `ws/tickets.verify` omits it. The standalone tool still renders the
  board block and the integrity advisories — identical verdict, identical
  advisories, minus the recipe sentence.
- **Known limitation to resolve:** `internal/wsdoc/tickets.go:263` asserts
  `fm["related"].(map[string]string)`; list-form `related:` parses to `[]string`
  and silently resolves to nil. Normalise the parse or state the gap; do not
  advertise an uncovered floor.
- Verification is **synthetic** (`t.TempDir()` fixtures), following
  `internal/wsdoc/tickets_verify_test.go`'s 19 existing tests. Board numbers in
  Output Format are illustrative, not assertions.

## Out of Scope

- Everything under the ticket's **Out of Scope**: any body-reading check, epic
  convention prose, `workset` retirement, `related:` symmetry, `plans:` path
  existence, `related:`-to-`.dropped` acknowledgement, a board-health surface
  outside verify, amend support in `ws/git.commit`, staged-rename detection.
- Re-deriving any measurement in **Decisions** / **No epic-body checks**. Those
  numbers are settled; do not re-scan the board to confirm them.
- Reshaping the `wsgit` boundary. Confirmed unnecessary (see Codebase Findings).
- Changing any existing `Finding`/`Warning` severity, or the existing
  `formatTicketVerify` `next_instruction` switch.

## Codebase Findings

**Phase 1 channel — confirmed sufficient, no `wsgit` reshape needed.**

- `agents-plugin-tool/internal/wsgit/git.go#L51` — `type Verifier func(root
  string, paths []string) ([]string, error)`. Advisories are pre-formatted,
  unattributed, order-preserving `[]string`; exactly what a multi-line per-call
  block needs.
- `agents-plugin-tool/internal/wsgit/git.go#L441-L449` —
  `CommitResult.Advisories []string` `json:"-"` (text-mode only).
- `agents-plugin-tool/internal/wsgit/git.go#L498-L508` — verifier call site; on
  veto advisories are deliberately discarded.
- `agents-plugin-tool/internal/mcp/server.go#L2495-L2507` — `formatGitCommit`
  splits each advisory on `\n` and indents **every** line by 2, so a multi-line
  `## Parent Board` block renders correctly with no renderer change.
- Conclusion: Phase 2 touches `wsdoc` + the two `internal/mcp` renderers only.
  No `wsgit` change, no new entry-point wiring
  (`agents-plugin-tool/internal/mcp/format.go#L53-L60` `VerifyAdapter` and
  `agents-plugin-tool/internal/mcp/server.go#L1076` both already route through
  `verifyAdapter`).

**Host and existing shapes.**

- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L19-L26` —
  `VerifyResult{OK, Findings, Warnings}`. Add the advisory carrier here.
- `agents-plugin-tool/internal/wsdoc/tickets_verify.go#L42-L56` — `TicketVerify`
  loops paths through `ticketVerifyPathShape` (returns `status, stem, ok`), then
  sets `result.OK = len(result.Findings) == 0`. The graph pass hooks in **after**
  the loop, before the `OK` assignment. Its only current error return is the
  empty-paths caller check at `#L43-L45`.
- `agents-plugin-tool/internal/mcp/server.go#L2699-L2729` — `verifyAdapter`:
  errors on `!result.OK`, else formats `Warnings` into `WARN [%s] %s: %s`
  advisories; short-circuits `return nil, nil` when `len(result.Warnings) == 0`
  (**must also test Advisories**). This is the commit-path-only layer → the amend
  recipe belongs here.
- `agents-plugin-tool/internal/mcp/server.go#L2731-L2755` — `formatTicketVerify`:
  standalone renderer. Its `next_instruction` switch fires "should be addressed or
  explicitly accepted" on `len(result.Warnings) > 0` — advisories must **not** feed
  that switch (the ancestor NOTE is explicitly no-action-needed).

**Graph inputs — all already exist; build nothing from scratch.**

- `agents-plugin-tool/internal/wsdoc/tickets.go#L144-L179` — `scanTickets(root,
  ticketScanOptions{IncludeDone: true, IncludeDropped: true})` returns the whole
  board (`ready/todo/idea/.done/.dropped`) as `[]TicketInfo` with `Stem`,
  `Status`, `Parent`, `Related`, `Path`, already sorted by `ticketStatusRank` then
  stem. Returns a non-nil error if `ai-docs/tickets` is missing or any ticket is
  unreadable — **this is the load-failure path to swallow**.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L224-L241` — `ticketStatusRank`
  already yields `ready`=0, `todo`=1, `idea`=2, `.done`=4, `.dropped`=5 —
  **exactly the settled row sort order**. Reuse it; do not write a second ranking.
- `agents-plugin-tool/internal/wsdoc/spec_discovery.go#L155-L184` —
  `scanSpecs(root)` walks `ai-docs/spec/` and returns `[]SpecInfo` whose
  `Anchors[].SpecStem` is the spec anchor stem set
  (`agents-plugin-tool/internal/wsdoc/spec_tools.go#L13` `specAnchorRE =
  \{#([0-9]{6}-[a-z0-9-]+)\}`). Errors if `ai-docs/spec` is missing — the second
  load-failure path to swallow.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L175-L177` —
  `ticketCategoryRE = ^\d{6}-([a-z]+)-` already extracts the category token. Reuse
  for the "parent is not an `epic`" check.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L48-L55` — `statusDirs` is
  the canonical five-status set already used by the status-dir guardrail.

**The `related:` parse gap — confirmed, and cheap to close.**

- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L36-L64` — the hand-rolled
  parser produces `map[string]string` for the nested `key: note` form,
  `map[string]string{}` for an empty/`null` value, and **`[]string` for the
  `- item` list form**. List items do **not** pass through `cleanScalar`, so a
  trailing ` # comment` stays glued to the item.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L263` — `info.Related, _ =
  fm["related"].(map[string]string)` silently yields nil for the list form.
- Live list-form instances (frontmatter-scoped, verified): exactly 3, all closed —
  `ai-docs/tickets/.done/260407-research-delegation-model-consolidation.md`,
  `ai-docs/tickets/.done/260626-bug-wsflow-lead-revive-skill-inventory-drift.md`,
  `ai-docs/tickets/.dropped/260405-research-marathon-delegation-hardening.md`. Two
  of the three carry a trailing `  # ...` comment on the item.
- **`TicketInfo.Related` has zero non-test consumers** (`grep '\.Related'` over the
  Go tree returns only the assignment at `tickets.go:263`), so normalising is
  near-zero-risk: it only widens what `tickets.list --format json` already
  advertises. **Recommendation: normalise the parse** — see Implementation Plan
  step 1. Stating the gap instead would leave the `FIX:` check silently missing a
  legal frontmatter shape, which is exactly the "floor the check does not cover"
  the ticket forbids.

**Test pattern.**

- `agents-plugin-tool/internal/wsdoc/tickets_verify_test.go#L1-L60` — `t.TempDir()`
  + `mustWrite` (helper lives in
  `agents-plugin-tool/internal/wsdoc/project_tree_test.go#L170-L179`, same package)
  + direct `TicketVerify(root, []string{...})` assertions. 19 tests, no git, no
  on-disk fixtures.
- `agents-plugin-tool/internal/mcp/server_test.go#L1781`
  `TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories` is the
  end-to-end MCP-dispatch template for the commit-path assertions.

## Implementation Plan

### 1. Normalise `related:` parsing (`internal/wsdoc/tickets.go`)

Replace `agents-plugin-tool/internal/wsdoc/tickets.go:263` with a call to a new
package-level helper (put it next to `scalarList`, `tickets.go#L299-L317`):

```go
// relatedEntries normalises every frontmatter shape the hand-rolled parser
// can produce for `related:` into the stem -> note map TicketInfo advertises.
func relatedEntries(value any) map[string]string
```

Handle, in order: `map[string]string` (pass through), `[]string` (each item →
`cleanScalar`, then split on the first `:` into stem/note; no `:` means stem with
empty note), `string` (single key, empty note), anything else → nil. Keep
`TicketInfo.Related`'s type and JSON tag unchanged.

### 2. New file `internal/wsdoc/tickets_graph.go`

Types (unexported):

```go
type ticketGraph struct {
    byStem      map[string]TicketInfo // whole board
    children    map[string][]string   // parent stem -> child stems
    specAnchors map[string]bool
}

func loadTicketGraph(root string) (*ticketGraph, error)
```

`loadTicketGraph` = `scanTickets(root, ticketScanOptions{IncludeDone: true,
IncludeDropped: true})` + `scanSpecs(root)`; propagate either error. Build
`children` from each ticket's non-empty `Parent`. Both scans are already sorted, so
`children` slices come out stem-sorted within each status for free; still sort rows
at render time by `ticketStatusRank` then stem.

Entry point:

```go
func ticketGraphAdvisories(root string, verified []verifiedTicket) ([]VerifyAdvisory, error)
```

where `verifiedTicket{Path, Status, Stem string}` is what `TicketVerify`'s loop
already computes. Returns the load error unchanged; step 4 is what swallows it.

Output order within the returned slice: the board block first (one advisory), then
the integrity advisories.

**Integrity checks** (subject = each verified ticket's own frontmatter, in the
order paths were passed; skip any verified stem absent from the graph):

| Condition | Kind | Message (commit-neutral) |
|---|---|---|
| `Parent != ""` and not in `byStem` | `fix` | ``parent: `<stem>` resolves to no ticket stem. Correct or remove the entry.`` |
| `related` key in neither `byStem` nor `specAnchors` | `fix` | ``related: `<stem>` resolves to no ticket stem and no spec anchor. Correct or remove the entry.`` |
| `parent:` chain revisits a stem | `check` | report the cycle path; do not prescribe which edge is wrong |
| parent resolves, `ticketCategoryRE` category != `epic` | `check` | ``parent: `<stem>` resolves to a ticket whose category is `<cat>`, not `epic`. A parent must be an epic; confirm the intended parent.`` |

Iterate `related` keys in sorted order — Go map iteration is random and the tests
need determinism. Cap the integrity list at **5**, then append a plain
`... +N more` advisory.

Use a small wrapper for the settled two-column shape rather than hand-wrapping
format strings (stems vary 30-60 chars):

```go
func wrapAdvisory(prefix, body string) string // prefix "FIX:   "/"CHECK: ", 7-space continuation, ~72 cols
```

**Ancestor walk** (`walkAncestors(graph, stem) (chain []string, cyclic bool)`):
follow `Parent` upward, unbounded depth, `seen` map cycle guard. On a cycle: return
`cyclic = true` and emit **no** board entries for that verified ticket — the
`CHECK:` cycle advisory is its whole output.

**Board block** — accumulate ancestors across all verified tickets, deduplicated by
stem (first occurrence wins, keeping its depth label). Emit a single advisory
string beginning `## Parent Board` if and only if at least one ancestor entry
rendered.

Per-ancestor rendering, decided purely from status (`gated` = the verified ticket
that reached this ancestor sits under `.done/` or `.dropped/`):

1. **Ancestor closed** (`.done`/`.dropped`) — always:
   `Parent [N]: <stem> [.done] - parent already closed`, blank line, then the
   path-neutral NOTE (2-space `NOTE:`, 4-space continuation). No rows. The NOTE
   must not claim anything closed just now — the same block renders on an ordinary
   `todo/`-path commit.
2. **All children closed** — always: header `- all M child tickets closed`, closed
   rows (`.done` then `.dropped`), then ACTION tier 1.
3. **All `ready`/`todo` children closed, ≥1 `idea` child** — always: header
   `- X of M closed, Y idea/ remaining`, closed rows then `idea` rows, then ACTION
   tier 2.
4. **≥1 open `ready`/`todo` child** — **only when `gated`**: header
   `- N of M child tickets still open`, open rows only (closed omitted), no ACTION
   line. When not `gated`, render nothing at all for this ancestor.

Row format (ASCII, 4-space indent): `%-8s| %s` on status then stem, then optional
parenthetical. Parentheticals, from the settled examples: `  (just now)` for a row
whose stem is one of the verified stems; `  (epic)` when the child's category is
`epic`; `  (epic, closable - see above)` when that epic child already rendered as a
nearer ancestor carrying an ACTION line. Row cap **5**, overflow line at the same
4-space indent with per-status counts in sort order:

- sibling listing (all hidden rows open) — ticket-literal
  `... +3 more open (1 todo, 2 idea)`;
- tiers 2/3, where hidden rows may be closed — same shape minus the `open` word:
  `... +3 more (2 .done, 1 idea)`. (Corner the ticket does not exemplify; chosen
  because `open` would be false there.)

Close a chain with `  No further ancestors.` as the settled examples do.

### 3. Advisory carrier (`internal/wsdoc/tickets_verify.go`)

```go
type VerifyAdvisory struct {
    Kind string // "fix" | "check" | "board"
    Text string
}
// VerifyResult gains: Advisories []VerifyAdvisory
```

**Use the struct, not `[]string`.** Prior Art suggests `[]string` "for example",
but the commit layer must append the amend recipe to mechanical-remedy advisories
only; a bare `[]string` forces `verifyAdapter` to re-parse the `FIX:` prefix, which
makes the settled output format load-bearing for behavior. `Kind` keeps format and
contract separate at a one-field cost. `Advisories` never affects `OK` — the
`result.OK = len(result.Findings) == 0` line is unchanged.

### 4. Wire the graph pass with the degrade-to-silence path (`tickets_verify.go`)

In `TicketVerify`, collect `verifiedTicket` entries inside the existing path loop
(it already has `status`, `stem`, `path`), then **after** the loop and **before**
`result.OK = ...`:

```go
// A whole-board graph load or spec-anchor scan can fail on a malformed file
// unrelated to this commit. Such a failure drops the advisories and lets the
// commit proceed; it must never become an error return, or the non-blocking
// invariant would be violated by the very code meant to honor it.
if advisories, err := ticketGraphAdvisories(root, verified); err == nil {
    result.Advisories = advisories
}
```

That `if err == nil` is the exact named degrade-to-silence path. `TicketVerify`'s
error return stays caller-input-only. Keep the comment — it is the reviewable
statement of the invariant.

### 5. Commit-path rendering (`internal/mcp/server.go` `verifyAdapter`, ~L2708)

- Change the short-circuit at `#L2717-L2719` from `len(result.Warnings) == 0` to
  `len(result.Warnings) == 0 && len(result.Advisories) == 0`.
- Emit existing `WARN [...]` lines first (unchanged), then one entry per
  `result.Advisories`.
- For `Kind == "fix"` only, append `"\n       Then git commit --amend --no-edit."`
  (7-space continuation, matching `wrapAdvisory`). `check` and `board` get nothing.
- Update the adapter doc comment to state that the recipe is a commit-path-only
  presentation detail, so the identical-verdict guarantee of
  `{#260723-tickets-verify-tool}` stays visibly preserved.
- No change to `internal/mcp/format.go` or to either `git.commit` entry point.

### 6. Standalone rendering (`internal/mcp/server.go` `formatTicketVerify`, ~L2735)

After the warning lines and **before** the `next_instruction` switch, emit a blank
line then each advisory's `Text` verbatim — no extra indentation, since the block
carries its own 4-space row indent and `formatGitCommit`'s 2-space pass handles the
commit side. **Do not touch the `next_instruction` switch**: advisories must not
trigger "should be addressed or explicitly accepted".

### 7. Spec (`ai-docs/spec/mcp-tools.md`)

At closeout, extend `{#260723-tickets-verify-tool}` (`ai-docs/spec/mcp-tools.md`
around L955-L975) with the cross-file check set, the `FIX:`/`CHECK:` shapes, the
`## Parent Board` block, the `.done`/`.dropped` sibling-listing gating, the
non-blocking invariant, and the degrade-to-silence rule. Add the cross-reference
from `{#260727-git-commit-verify-advisories}` (L1179) noting that the commit path
additionally appends the amend recipe. State the two-namespace resolution rule
explicitly, so `related:`-to-spec-anchor reads as intended rather than tolerated.

## Verification Plan

`cd /home/swkang/devenv/agents-plugin-tool && go build ./... && go test ./...
-count=1` (12 packages) must stay green.

New synthetic tests in `agents-plugin-tool/internal/wsdoc/tickets_graph_test.go`
(same package, `t.TempDir()` + `mustWrite`, following `tickets_verify_test.go`'s
pattern; a shared fixture builder that writes an epic plus N children keeps these
short):

**Integrity checks (4)**

1. `parent:` naming a stem with no ticket file → one `fix` advisory.
2. `related:` key resolving to neither a ticket nor a spec anchor → one `fix`.
3. `parent:` cycle (A→B→A) → one `check` naming the cycle, **and no
   `## Parent Board` advisory**.
4. `parent:` resolving to a `refactor`-category ticket → one `check`.

**Board block, all five renderings (5)**

5. All children closed → header + closed rows + ACTION tier 1 + `(just now)` on the
   verified row.
6. `idea/`-only remaining → tier-2 header + closed rows then `idea` rows + ACTION
   tier 2.
7. Siblings remain, verified path under `.done/` → `N of M ... still open` header +
   open rows only, no ACTION line, closed children absent.
8. Cap applied → 6+ open siblings render exactly 5 rows plus
   `... +N more open (<per-status>)` in sort order.
9. Ancestor already in `.done/` → NOTE, no rows, and the NOTE text asserts nothing
   about when a child closed.

**Rules (6)**

10. Two-namespace resolution: `related: <spec-anchor-stem>` (fixture writes
    `ai-docs/spec/x.md` containing `{#260513-...}`) emits nothing, while the same
    stem used as `parent:` emits a `fix` — pins tickets-only `parent:` resolution.
11. Ancestor dedup: two verified child paths sharing one parent → the parent block
    appears exactly once.
12. No `parent:` → **no `## Parent Board` advisory at all** (not an empty one).
13. Emission gating: an identical fixture verified at a `todo/` path emits no
    sibling listing; at a `.done/` path it does. Assert both directions in one test.
14. Integrity cap: 6+ dangling `related:` keys → 5 advisories plus `... +N more`.
15. Row sort order: mixed `ready`/`todo`/`idea` siblings render in that order.
16. List-form `related:` (step 1's normalisation) is actually checked: a
    `- <dangling-stem>  # note` frontmatter entry produces a `fix`. Pins the
    known-limitation closure.

**Degrade to silence (1)**

17. `ai-docs/spec/` absent (so `scanSpecs` errors) with a valid `.done` ticket under
    a populated `ai-docs/tickets/` → `TicketVerify` returns **nil error**, `OK`
    true, `len(Advisories) == 0`. Add the mirror case with an unreadable ticket file
    if it can be produced portably; otherwise the missing-spec-dir case alone pins
    the invariant.

**Deliberate negative cases (3) — each must emit nothing**

18. `related:` key resolving to a spec anchor (shares the fixture with 10; assert
    zero advisories explicitly, not merely absence of `fix`).
19. An epic with zero `parent:`-linked children, verified directly → no advisory.
20. An epic body naming a child stem in prose with no `parent:` back-link → no
    advisory (proves no body is read).

**Renderer / commit-path tests (`agents-plugin-tool/internal/mcp/server_test.go`)**

21. `verifyAdapter` over a fixture producing one `fix` and one `check`: the `fix`
    line ends with `Then git commit --amend --no-edit.` and the `check` does not.
22. `formatTicketVerify` over the same result: the board block and both advisories
    render, the amend sentence is **absent**, and the "should be addressed or
    explicitly accepted" `next_instruction` does not fire when `Warnings` is empty.
23. One end-to-end MCP-dispatch test modelled on
    `TestServeStdioGitCommitSurfacesTicketVerifyWarningsAsAdvisories`
    (`agents-plugin-tool/internal/mcp/server_test.go#L1781`): a commit touching a
    `.done/` child of an epic lands, `OK` is unchanged, the `## Parent Board` block
    appears in the text response, and JSON mode carries no advisory field.

Assert on substrings and line counts, not whole-block equality, so the wrapping
width stays an implementation detail. Board numbers from the ticket's Output Format
are illustrative — never assert against the live board.

## Escalations

- None. Confidence: high. Every policy decision the phase needs is settled in the
  ticket; the only judgment calls left are the two flagged inline (normalise the
  `related:` parse — recommended, zero non-test consumers; and the overflow-line
  wording for closed-inclusive tiers, which the ticket does not exemplify).
