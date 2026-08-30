# Plan: 260824-feat-review-watermark-ledger — Phase 1: Ledger format + marker read/append + explicit bootstrap

## Relevant Ticket Contract

- Storage: a single tracked dotfile ledger under `ai-docs/`, proposed path
  `ai-docs/.review-ledger.md` (final name an implementation choice — confirmed
  not excluded by `.gitignore`, which only ignores `ai-docs/**/*.local.md`).
  Each review appends `<base>..<head>: verdict`.
- Marker = the latest entry's through-SHA, read by content via a **line-scoped
  parse of the last entry** — never derived from "the last commit that touched
  the ledger" and never by graph-walking.
- **The parser must skip non-entry lines (comments/banners) from the start**
  (design review, 2026-08-29): Phase 3 adds a top-of-file banner and a
  tail-anchor comment; the "latest entry" scan must ignore those lines now, not
  as a later retrofit, or it will parse a banner as the latest entry and yield
  a garbage marker SHA.
- Non-pass entries (`concern`/`block`) carry a **routed-ticket-stem reference**
  (e.g. `<base>..<head>: block -> 260901-bug-…`). The append surface
  **requires a stem on a `block` entry** on the happy path. A malformed
  un-routed block is corrected by an **append-only follow-up entry**
  (`<same range>: routed -> <stem>`) — never an in-place edit.
- Range key is always a commit SHA, never a wall-clock timestamp; a timestamp
  may ride as optional metadata only.
- **Bootstrap is explicit, never silent.** When no marker exists, insert one at
  current `HEAD` and surface that prior history is *review-skipped*, not
  *reviewed*.
- Ledger honesty: verification must guard against any code path recording an
  unreviewed range as reviewed.
- Ticket's own Phase 1 verification boundary: append/read round-trips; marker
  resolves to the last entry's through-SHA under line-scoped parse even when
  the ledger file was touched by an unrelated edit; bootstrap on an empty
  ledger emits the explicit surface.

## Out of Scope

- Phase 2 (checkpoint recompute/nudge, standalone sweep, wiring into
  `tickets.close`/`workflow_manual`/`enter.*`, nudge size/staleness scaling,
  `_review.local.md` staleness knob) — this ticket's own next phase.
- Phase 3 (ledger canary conflict semantics, exact self-documenting banner
  text/placement, no-squash/landing-topology guidance) — this ticket's own
  Phase 3. This plan still honors the Phase-1 design constraint Phase 3
  depends on: the parser skips comment/banner lines from the start (see
  above), and non-pass entries already carry a routed-ticket-stem reference
  shape.
- `260824-feat-review-release-gate-policy` (④): `AGENTS.md` review-track/
  boundary fields, `_review.local.md` review-mechanics config, the release
  gate itself. Not touched by this phase.
- Any MCP tool registration/schema (`server.go`) or checkpoint wiring
  (`workflow_manual.go`, `implement_resolver.go`'s `tickets.close` path) — the
  ticket's own epic explicitly rejects a merge-time MCP hook, and wiring into
  observed checkpoints is Phase 2's job. Phase 1 delivers composable Go
  primitives only.
- Concurrency hardening (flock/atomic-rename) for the append path — the
  ticket's Write Discipline section describes the single-writer serial case as
  non-racing by construction; no live caller exists yet in this phase.

## Codebase Findings

- `.gitignore` — only `ai-docs/**/*.local.md` is ignored under `ai-docs/`;
  `ai-docs/.review-ledger.md` is not excluded, confirming the ticket's
  proposed path is viable as-is.
- `agents-plugin-tool/internal/mcp/implement_resolver.go#L442-466` —
  `implementCloseMergeReviewNudge`, whose comment at L454 explicitly states it
  "leaves room for epic 260824's later review-watermark hook to compose
  without rework." This is the confirmed Phase-2 wiring point for
  `tickets.close`; it constrains this phase to keep the ledger API as plain,
  composable functions (`root string` in, value/error out) rather than
  anything checkpoint-shaped.
- `agents-plugin-tool/internal/mcp/bootstrap_alarm.go` and
  `agents-plugin-tool/internal/mcp/doc_coverage_alarm.go` — the established
  "read-only detector, gated by a `wsconfig.Resolver` off-switch, injected at
  a checkpoint" pattern this repo already uses for advisory banners. Confirms
  Phase 2's nudge will likely follow this same shape later; not needed by
  Phase 1's read/append/bootstrap primitives themselves, since no checkpoint
  wiring happens in this phase.
- `agents-plugin-tool/internal/wsgit/git.go#L520-530` — `Client.Commit` already
  resolves the just-made commit's SHA via
  `runner.RunGit(ctx, root, "rev-parse", "HEAD")`; `implement_resolver.go`
  additionally shows the free-standing call idiom
  `(wsgit.ExecRunner{}).RunGit(ctx, root, "rev-parse", "--verify", ...)`
  outside the `Client` wrapper. Reuse this exact idiom
  (`wsgit.ExecRunner{}.RunGit(ctx, root, "rev-parse", "HEAD")`) to resolve
  `HEAD` for bootstrap — no new git-primitive abstraction needed.
- `agents-plugin-tool/internal/wsnote/store.go` (whole file, esp.
  `RepoDir`#L77-82, `Load`#L84-96, `rmw`#L152-209) — the closest existing
  precedent for a tracked per-repo append-ish store
  (`ai-docs/ws-notes/`), including a "missing file = empty state, not an
  error" read contract worth mirroring. **Risk signal — do not reuse its
  shape wholesale**: it is JSON via flock + temp-file + atomic-rename RMW,
  but the ledger's decisions require a plain-text, line-oriented,
  git-conflict-legible format (concurrent appends must textually conflict —
  that conflict *is* Phase 3's canary). A JSON store would auto-merge or
  silently corrupt instead of forcing the intended STOP, so only the
  "missing file → empty state" read contract is transferable, not the
  encoding or the RMW write path.
- `agents-plugin-tool/internal/wsdoc/legacy_marker.go` (whole file) —
  unrelated domain (retiring a legacy `🚧` spec marker), but shows this repo's
  existing appetite for CommonMark-aware line scanning (fence/HTML-comment/
  frontmatter tracking). **Risk signal, in reverse**: the ledger format is a
  flat, single-line-per-entry format with a trivial comment convention (lines
  starting with `#`, per the ticket's own Phase-3 banner example
  `# ⚠ If you are resolving a CONFLICT here...`) — importing this file's
  fence/indent machinery would be over-engineering for Phase 1's actual
  contract.
- `grep` across `agents-plugin-tool` and `ai-docs/spec|mental-model` for
  `review-ledger|ReviewLedger|wsreview|watermark` — no existing package, spec,
  or mental-model file references this mechanism yet except the one forward
  comment in `implement_resolver.go` above. This is new territory; no existing
  code to migrate or preserve compatibility with.
- `ai-docs/tickets/.done/260824-feat-lead-review-range-scenario.md` — the
  prerequisite (②) is landed: `git.diff(range:)`/`git.log(range:)` are already
  available on `wsgit.Client` for the eventual sweep. Confirms Phase 1 itself
  does not need to touch diff/log machinery — only marker storage/parsing.
- `agents-plugin-tool/go.mod` — module path is `github.com/kang-sw/devenv`
  rooted at `agents-plugin-tool/`; a new package belongs at
  `agents-plugin-tool/internal/wsreview/`, importable as
  `github.com/kang-sw/devenv/internal/wsreview`.
- `agents-plugin-tool/internal/wsgit/git_test.go#L1315-1341` (`sparseTestInitGit`
  /`sparseTestRunGit`/`sparseTestRunGitOutput`) — existing real-temp-git-repo
  test fixture idiom, reusable for a bootstrap test that needs an actual
  `HEAD` SHA rather than a fake one.

## Implementation Plan

1. Create `agents-plugin-tool/internal/wsreview/ledger.go` (new package
   `wsreview`). Define:
   - `LedgerPath(root string) string` → `filepath.Join(root, "ai-docs", ".review-ledger.md")`,
     mirroring `wsnote.RepoDir`'s join style
     (`agents-plugin-tool/internal/wsnote/store.go#L77-82`).
   - `type Entry struct { Base, Head, Verdict, Ref string }` — `Verdict` one of
     `pass`, `concern`, `block`, `routed`, `bootstrap` (the last two are not
     genuine review outcomes: `routed` is the append-only corrective
     follow-up, `bootstrap` is the explicit-bootstrap surfacing token — see
     step 5). `Ref` carries the routed ticket stem.
   - An entry-line regex, e.g.
     `^([0-9a-f]{7,40})\.\.([0-9a-f]{7,40}): (pass|concern|block|routed|bootstrap)(?: -> (\S+))?\s*$`.
     Any line that does not match — including every `#`-prefixed
     comment/banner line and blank lines — is skipped by the parser. This is
     the Phase-3-readiness requirement: design it now, not as a retrofit.
2. `ParseLatest(content string) (Entry, bool)` — scans lines top to bottom,
   keeping the last regex match; returns `(Entry{}, false)` when no line
   matches (empty or comment-only ledger).
3. `Read(root string) (Entry, bool, error)` — reads `LedgerPath(root)`; a
   missing file returns `(Entry{}, false, nil)`, not an error (mirrors
   `wsnote.Load`'s "no file yet" contract,
   `agents-plugin-tool/internal/wsnote/store.go#L84-96`); on success, delegates
   to `ParseLatest`.
4. `Append(root string, e Entry) error` — validates `Base`/`Head` are
   non-empty SHA-shaped strings, `Verdict` is a known token, and `Ref` is
   non-empty whenever `Verdict` is `block` (the ticket's explicit "the append
   surface requires a stem on a block entry" invariant). A `concern` entry MAY
   carry a `Ref` but is NOT required to — the routed-stem requirement is
   **block-only** (lead adjudication, 2026-08-30): the release gate (④) forcing
   function blocks promotion on an unresolved *blocking* finding only, so a
   non-blocking `concern` needs no mandatory resolution path, and requiring one
   would over-constrain beyond the ticket's literal "requires a stem on a block
   entry." On success, opens
   `LedgerPath(root)` with `O_APPEND|O_CREATE|O_WRONLY` (create parent
   `ai-docs/` dir if absent) and writes one formatted line. No mutation of
   existing lines under any code path (append-only is structural, not just
   documented).
5. `Bootstrap(ctx context.Context, root string) (Entry, bool, error)` — calls
   `Read`; if an entry already exists, no-op and return it with `created =
   false` (idempotent — a second bootstrap call is safe). If absent (file
   missing, or present but zero parseable entries — e.g. banner-only),
   resolve current `HEAD` via
   `wsgit.ExecRunner{}.RunGit(ctx, root, "rev-parse", "HEAD")` (same idiom as
   `agents-plugin-tool/internal/wsgit/git.go#L526`), then append an
   `Entry{Base: head, Head: head, Verdict: "bootstrap"}` line and return it
   with `created = true`. The distinct `bootstrap` verdict token *is* the
   explicit surface: any future reader (Phase 2's nudge) can render "prior
   history is review-skipped, not reviewed" straight off the verdict, with no
   separate flag needed.
6. Add `agents-plugin-tool/internal/wsreview/ledger_test.go` covering:
   - Append then Read round-trips `Base`/`Head`/`Verdict`/`Ref` unchanged.
   - A ledger with a `#`-prefixed banner line before and after real entries —
     `ParseLatest` still returns the last real entry, never the banner.
   - A ledger touched by an unrelated edit (a line that does not match the
     entry regex, inserted after the last real entry) — marker resolution is
     unaffected (this is the ticket's own literal Phase-1 verification
     wording).
   - `Bootstrap` on an absent ledger creates the file and seeds an entry at
     the real git `HEAD` of a temp fixture repo (reuse the
     `sparseTestInitGit`/`sparseTestRunGit` idiom from
     `agents-plugin-tool/internal/wsgit/git_test.go#L1315-1341`); a second
     `Bootstrap` call on the now-seeded ledger is a no-op (`created == false`,
     same entry returned).
   - `Append` with `Verdict: "block"` and empty `Ref` returns an error.
   - `Append` with `Verdict: "concern"` and empty `Ref` **succeeds** (stem is
     block-only; concern's Ref is optional — locks the 2026-08-30 lead
     adjudication).
   - A `routed` corrective entry appended over the same range as an earlier
     `block` entry appends a new line without touching the earlier one
     (append-only/never-edit, both lines readable from the raw file).
7. Do not touch `agents-plugin-tool/internal/mcp/server.go`,
   `workflow_manual.go`, or `implement_resolver.go` in this phase — no MCP
   tool, schema, or checkpoint wiring. Phase 2 owns composing this package's
   functions into the observed-checkpoint recompute/nudge.

## Verification Plan

- `cd agents-plugin-tool && go test ./internal/wsreview/...` — new package's
  round-trip, comment/banner-skip, bootstrap-idempotency, block-without-stem
  rejection, and append-only-corrective-entry cases.
- `cd agents-plugin-tool && go build ./...` — confirm no regressions
  elsewhere; nothing else imports the new package yet, so this is a pure
  additive-compile check.
- Manual/no-op: no spec or manual doc claims caller-visible behavior yet
  (Phase 1 ships no MCP-visible surface), so no spec-drift check is needed
  this phase; `ai-docs/spec/mcp-tools.md`/`workflow-skills.md` updates belong
  to Phase 2 per the ticket's own Spec Impact section.

## Escalations

- None.
