# Plan: ticket.verify + commit-gate mechanical backstop, then must-not-forget mutation-tool collapse — Phase 1: verify() + commit-gate backstop (pure addition)

## Relevant Ticket Contract
- Add a deterministic `ticket.verify(paths)` covering: stem regex, status/dir
  consistency, frontmatter fence integrity, ready-landing sage-posture
  presence/value, phase/Result structural presence only, close date-field
  presence.
- spec-address stays soft-warn (matches today's `tickets.move` tip); do not
  promote to hard reject.
- phase/Result check is structural well-formedness only; append-only is
  explicitly out of scope (diff-level property).
- Host verify at `wsgit.Client.Commit`, after staging, before `git commit -m`,
  as a sibling of `validateCommitStatus`; every ticket-touching commit through
  this path is gated. Keep `ticket.verify(paths)` callable standalone for
  mid-edit red/green feedback, with call-site parity (gate and standalone
  return the same verdict for identical input).
- Single source of truth: where a residual mutation-tool check enforces the
  same rule (ready-move sage-posture), it must delegate to verify's logic, not
  duplicate it.
- Escalation terminal: no bypass/override path may let an invalid commit
  through; verify always runs on every gated commit, unconditionally.
- Acceptance: each hard guardrail fires on a deliberately invalid fixture
  (bad stem, wrong status dir, missing ready sage-posture, malformed
  frontmatter, malformed phase/Result headings); a commit staging such a
  ticket is blocked; spec-address emits only a warning; Go tests cover these
  cases.
- Phase 1 is purely additive — existing tools stay unchanged in shape.

## Out of Scope
- Phase 2 (mutation-tool collapse/rename, `tickets.create` rename,
  `tickets.close` free-edit + soft-warn, `sage.stamp` tool, action-time
  obligation prose). Do not touch `TicketCreate`'s stub shape or
  `TicketsClose`'s hard/soft posture here.
- Promoting spec-address to a hard gate (`260723-feat-ready-spec-address-hard-gate`).
- Enforcing the phase/Result append-only convention (diff-level; no snapshot
  check).
- A pre-commit git hook or any non-`ws/git.commit` chokepoint for the raw
  `git commit` bypass path — the ticket sanctions dropping that concern if
  `git.commit` is the practical chokepoint; do not build a hook in Phase 1
  unless research says otherwise (not indicated by survey).
- `tickets.sage_gate`/`tickets.sage_record` internals — reuse their
  posture-reading helpers, do not modify their write paths.

## Codebase Findings

- `agents-plugin-tool/internal/wsgit/git.go#L432-L470` — `Client.Commit`:
  stages (`stagingCommandsForCommit`), re-reads status, calls
  `validateCommitStatus(status, opts.Paths)` (L454-456, pure git-status shape
  check, zero ticket-content awareness), then `git commit -m` (L462). Verify
  must be inserted right after the `validateCommitStatus` call succeeds and
  before `CommitMessage`/`git commit -m`, gated on the already-expanded
  `opts.Paths` (ticket-move rename pairs are expanded into `opts.Paths` at
  L443 via `expandCommitPathsForTicketMoves`, so verify sees both old+new
  paths on a move).
- `agents-plugin-tool/internal/wsgit/git.go#L855-L877` — `ticketStatusStem`
  already parses `ai-docs/tickets/<status>/<stem>.md` paths inside wsgit
  (used only for commit-message ticket-change summaries). Its accepted status
  set is `{idea, todo, ready, wip, .done, .dropped}` — includes `"wip"`,
  which `wsdoc.statusDirs` (tickets_mutate.go#L49-55) does NOT include (only
  idea/todo/ready/.done/.dropped, matching AGENTS.md). This is a pre-existing
  inconsistency between the two packages' idea of valid status dirs; verify's
  status/dir guardrail must follow the AGENTS.md/`wsdoc.statusDirs` five-dir
  set, not wsgit's `wip`-inclusive one. Flagging only — not a Phase 1 fix
  target.
- `ai-docs/mental-model/mcp-runtime.md#L100` (`{#260720-wsdoc-commit-boundary}`)
  — documented invariant: **`internal/wsdoc` must not import
  `internal/wsgit`** (would invert the package dependency direction); the
  established pattern for a wsdoc mutation that also needs a commit is:
  wsdoc computes the write inputs, the MCP dispatch `callTool` case performs
  the actual `wsgit.NewClient().Commit(...)`. Today neither package imports
  the other. The ticket asks for the reverse edge (wsgit calling verify
  logic that naturally lives in wsdoc, since wsdoc owns all ticket-parsing
  helpers). A straight `import "github.com/kang-sw/devenv/internal/wsdoc"`
  inside `wsgit/git.go` is not cyclic (wsdoc has zero wsgit imports) but
  breaks the observed convention that composition of wsgit+wsdoc happens only
  at the MCP/CLI dispatch layer. `wsgit.Client` already has exactly this kind
  of seam for a different dependency: `Client.Runner Runner` with a
  `c.runner()` getter defaulting to `ExecRunner{}` (git.go#L36-47). Mirror
  that pattern for verify: add an optional `Client.Verifier` field (function
  type, see Implementation Plan) defaulting to a no-op when unset, and have
  the MCP dispatch (`git.commit` case) and the CLI (`gitCommit` in
  `cmd/ws-mcp/main.go#L460-505`) construct the client with
  `Verifier: wsdoc.TicketVerify`. This keeps wsgit free of a hard wsdoc
  import, matches the existing DI seam, and is consistent with the ticket's
  note that "exact tool signatures ... are still design-level" (Spec Impact
  section) — the wiring mechanism is an implementation decision, not a
  literal instruction to hard-import.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L12` — `ticketStemRE =
  regexp.MustCompile(`^\d{6}-[\w-]+$`)`; reuse directly for the stem
  guardrail.
- `agents-plugin-tool/internal/wsdoc/tickets.go#L277-297` — `ticketPhases(text)`
  already extracts `### Phase ` headings and `### Result` presence per phase
  by prefix match only (no format validation). Verify's phase/Result
  well-formedness guardrail needs a *stricter* pass: any line with prefix
  `### Phase ` must fully match `^### Phase \d+: .+$`; any `### Result` line
  must match `^### Result \(\S+\) - \d{4}-\d{2}-\d{2}$`; any `#### Edition`
  line must match `^#### Edition \(\S+\) - \d{4}-\d{2}-\d{2}$` (per the
  AGENTS.md commit-rule convention for phase Result/Edition headings). Do not
  modify `ticketPhases`; add a separate well-formedness pass in the new
  verify file so `ticketPhases`'s existing lenient callers (ticket listing)
  are undisturbed.
- `agents-plugin-tool/internal/wsdoc/frontmatter.go#L8-27` — `frontmatter(path)`
  returns `nil` silently both when the file has no leading `---` and when the
  closing fence is missing; it does not distinguish "no frontmatter" from
  "malformed frontmatter" for a caller. Verify needs its own explicit fence
  check (read raw bytes, confirm line 0 is `---` and a later line is exactly
  `---`) rather than inferring malformity from a nil map, since a
  legitimately fenced-but-empty-body ticket must not be flagged.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L48-55` — `statusDirs`
  map: the canonical five-directory set to validate against.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L167-206` —
  `ticketCategoryRE`, `exemptReadyGateCategories`, `readyGateWarning(ticketAbsPath, stem)`:
  already implements the exact spec-address soft-warn semantics the ticket
  asks verify to reproduce ("ready gate is normally enforced by
  lead-write-ticket; no spec addressing detected."). Verify should call this
  function as-is (same package, no refactor needed) rather than
  reimplementing spec-address detection — this alone satisfies "single
  source of truth" for that guardrail.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L242-263` —
  `sageReviewStageRequirement(stem)`: category → (designRequired,
  completenessRequired). Reuse directly.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L282-302` —
  `effectiveSageReviewPostures(fm)`: reads new two-field posture with legacy
  single-field migration-read fallback (does not write). Reuse directly —
  read-only, safe to call from a verify pass that must not mutate files.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L371-395` — the
  ready-terminal-posture check is currently *inlined* inside
  `prepareSageReviewForUpwardMove` (design/completeness must be
  `completed`/`skipped`, else `sageReviewStageError`/`sageReviewBlockedError`).
  This is the "residual mutation-tool check" the ticket's single-source-of-truth
  decision targets: extract the terminal-posture predicate into a small
  shared helper (e.g. `readyPostureProblems(fm map[string]any, stem string) []string`
  returning field-level problem descriptions, no I/O) that both
  `prepareSageReviewForUpwardMove` (via wrapping its existing error type) and
  the new `TicketVerify` call. Keep `prepareSageReviewForUpwardMove`'s
  existing write side-effects and its move-only error types
  (`sageReviewStageError`/`sageReviewBlockedError`/`PartialMutationNotice`)
  untouched; only the pure "is this posture terminal" predicate is shared.
- `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L433-446` —
  `findTicketPath(root, stem)` — not directly usable by verify (verify is
  called with file *paths*, not stems, per the ticket's `ticket.verify(paths)`
  signature), but its status-dir iteration order is the reference list to
  mirror for the guardrail.
- `agents-plugin-tool/internal/mcp/server.go#L890-922` — `git.commit` MCP
  dispatch case: builds `wsgit.CommitOptions` from `params.Arguments` and
  calls `wsgit.NewClient().Commit(...)`. This is where `Verifier:
  wsdoc.TicketVerify` must be wired in (construct `wsgit.Client{Verifier:
  wsdoc.TicketVerify}` instead of the zero-value `wsgit.NewClient()`), and
  where a verify failure's actionable-prose error should surface unchanged
  through the existing `toolTextResponse(req.ID, "", err)` error path — no
  special-casing needed, `Commit` returning an error before `git commit -m`
  already short-circuits identically to today's `validateCommitStatus`
  failures.
- `agents-plugin-tool/cmd/ws-mcp/main.go#L460-505` — `gitCommit` CLI handler:
  same `wsgit.NewClient().Commit(...)` call; needs the same `Verifier` wiring
  for CLI-path parity (the "raw `git commit` from the shell" bypass the
  ticket flags is a different, explicitly out-of-scope concern, but the ws
  CLI wrapper itself must not be a second silent bypass of the MCP tool).
- `agents-plugin-tool/internal/mcp/server.go#L1134-1162` (`tickets.move`) —
  reference dispatch-case shape for a new `tickets.verify` case: root
  resolution via `s.resolveToolRoot`, `stringList(params.Arguments["paths"])`
  argument extraction (pattern already used at L861/L898), then call
  `wsdoc.TicketVerify(root, paths)`, then either `toolJSONResponse` (if
  `wantsJSON`) or a new formatter (mirror `formatSageGate`/`formatTicketMutate`
  at L2429-2456: plain-text summary + per-finding bullets).
- `agents-plugin-tool/internal/mcp/server.go#L3743-3793` — tool schema block
  pattern for `tickets.*` entries (`name`, `description`,
  `inputSchema.properties`, `required`). New `tickets.verify` schema:
  `paths: stringArrayProperty(...)`, `format: stringProperty(...)`,
  `required: []string{"paths"}`.
- `agents-plugin-tool/internal/mcp/server.go#L4076-4093` —
  `toolSchemaRequiresSessionKey` allowlist: must add `"tickets.verify"` here
  (all sibling `tickets.*` tools require `session_key`; matches
  `s.resolveToolRoot` requiring it).
- `agents-plugin-tool/internal/mcp/server.go#L4198-4212` — `roleAllowsTool`:
  `roleLeaf` explicitly excludes `name != "git.commit"` but otherwise allows
  everything not `mercenary.`/`config.`/`session.`-prefixed. `tickets.verify`
  needs no entry here — it is read-only (no mutation, no commit) and should
  stay available to `roleLeaf`/`roleDelegate` for the ticket's stated
  "callable standalone for mid-edit red-green feedback" requirement. Confirm
  this by leaving it out of every hidden/exclusion list, not by adding a new
  allow rule.
- `agents-plugin-tool/internal/wsgit/git_test.go#L222-264`
  (`TestCommitStagesExplicitPathsAndBuildsMessage`) and
  `#L318-332` (`TestCommitRefusesUnrelatedStagedPaths`) — existing `.Commit(...)`
  tests use a fake `root` (`"/repo"`, does not exist on disk) with a
  `sequenceRunner` faking all `RunGit` output; `opts.Paths` in these tests is
  `["src"]` / `["src/file.go"]` — never a `ai-docs/tickets/...` path — so an
  unconditional file-backed verify pass (triggered only for `ai-docs/tickets/`
  paths inside `opts.Paths`) will not touch these tests. Confirms the design
  choice to scope verify strictly to ticket-shaped entries of the *committed*
  `opts.Paths`, not the full repo status, both for correctness and to avoid
  breaking existing non-ticket `Commit` tests.
- `agents-plugin-tool/internal/wsgit/git_test.go#L180-220` — `recordingRunner`/
  `sequenceRunner` test-double patterns for constructing `Client{Runner: ...}`
  with a nil `Verifier` (must default to a no-op, mirroring `c.runner()`'s
  nil-default pattern) so every pre-existing `Client{Runner: ...}` test
  literal keeps compiling and passing unmodified.
- `agents-plugin-tool/internal/wsdoc/tickets_test.go#L1-11` — `mustWrite(t, root, relPath, content)` test helper (writes a file under a temp root, creating parent dirs) is the established pattern for building ticket fixtures in wsdoc tests; reuse for `TicketVerify` fixtures.

## Implementation Plan

1. `agents-plugin-tool/internal/wsdoc/tickets_verify.go` (new file):
   - Define `VerifyFinding{Path, Guardrail, Message string}` and
     `VerifyResult{OK bool, Findings []VerifyFinding, Warnings []VerifyFinding}`.
   - Define `TicketVerify(root string, paths []string) (VerifyResult, error)`:
     for each path, skip if not shaped like `ai-docs/tickets/<seg>/<name>.md`
     (non-ticket paths are not verify's concern); otherwise run, in order:
     stem-regex + status-dir guardrail (hard), file-exists/read guardrail
     (hard), frontmatter-fence guardrail (hard, own fence scan — do not infer
     from `frontmatter()`'s nil), ready-sage-posture guardrail (hard, only
     when status dir == `ready`, via the new shared `readyPostureProblems`
     helper from step 2), close-date-field guardrail (hard, only when status
     dir == `.done`/`.dropped`, field `completed`/`dropped` non-empty),
     phase/Result well-formedness guardrail (hard, regex pass over raw
     lines), spec-address guardrail (soft, only when status dir == `ready`,
     via existing `readyGateWarning`). Aggregate all hard failures into
     `Findings` (`OK = len(Findings) == 0`), soft ones into `Warnings`.
     Return a plain `error` only for caller-input problems (empty `paths`);
     guardrail failures are never a Go `error`, only `Findings`, so the
     commit-gate caller (step 4) is responsible for turning a non-OK
     `VerifyResult` into the blocking `error`.
2. `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`:
   - Extract the terminal-posture predicate currently inlined at the tail of
     `prepareSageReviewForUpwardMove` (L381-394) into a pure helper (no I/O,
     no writes) that both `TicketVerify` and
     `prepareSageReviewForUpwardMove` call, so the ready-posture rule has one
     implementation. Keep `prepareSageReviewForUpwardMove`'s own error
     construction (`sageReviewStageError`, `sageReviewBlockedError`) at its
     call site; the shared helper only reports which field(s) are
     non-terminal.
3. `agents-plugin-tool/internal/wsgit/git.go`:
   - Add `type Verifier func(root string, paths []string) (VerifyResult, error)`
     using a *locally declared* result shape (do not import wsdoc's type —
     keep the field structurally typed, e.g. an interface with an `OK()
     bool` + `Error() string`-producing method, or simplest: declare
     `Verifier func(root string, paths []string) error` and have the MCP/CLI
     wiring adapt `wsdoc.TicketVerify`'s richer result into a single
     formatted error before assigning it — mirrors how `GitRunner` in
     `tickets_mutate.go` is a minimal structural interface rather than an
     import). Add `Verifier Verifier` field on `Client`; add a `verifier()`
     accessor defaulting to a no-op (`func(string, []string) error { return
     nil }`) when unset, mirroring `runner()` at L42-47.
   - In `Commit`, immediately after the `validateCommitStatus` call succeeds
     (after L456, before `detectTicketChanges`/`CommitMessage`), call
     `c.verifier()(root, opts.Paths)`; on non-nil error, return
     `CommitResult{}, err` before staging is committed (staging itself
     already happened — this is intentional per the ticket: "after staging,
     before `git commit -m`" — a blocked commit leaves the invalid state
     staged for the caller to fix and re-verify, not reverted).
4. `agents-plugin-tool/internal/mcp/server.go`:
   - Add a small adapter `func verifyAdapter(root string, paths []string) error`
     that calls `wsdoc.TicketVerify(root, paths)` and, if `!result.OK`,
     formats `Findings` into one multi-line actionable error (guardrail +
     path + message per line); wire `wsgit.Client{Verifier: verifyAdapter}`
     into the `git.commit` case (replace `wsgit.NewClient()` there).
   - Add a `tickets.verify` dispatch case (paths arg via `stringList`,
     `s.resolveToolRoot`, calls `wsdoc.TicketVerify` directly — no wsgit
     involvement for the standalone path), returning JSON on `format=json`
     or a new `formatTicketVerify(result)` plain-text formatter (per-path
     PASS/FAIL + bulleted findings + bulleted warnings), mirroring
     `formatTicketMutate`/`formatSageGate`'s style at L2429-2456.
   - Add the `tickets.verify` schema block (paths + format properties,
     `required: ["paths"]`) near the other `tickets.*` entries (~L3743-3793).
   - Add `"tickets.verify"` to the `toolSchemaRequiresSessionKey` allowlist
     (~L4076-4093).
   - Do not add `tickets.verify` to any hidden/exclusion list (`roleLeaf`
     exclusion at L4198-4212, `permanentlyHiddenTool`, `noAgentHiddenTool`) —
     it must stay available to every role for standalone mid-edit use.
5. `agents-plugin-tool/cmd/ws-mcp/main.go`:
   - Wire the same `Verifier: verifyAdapter`-equivalent into `gitCommit`'s
     `wsgit.NewClient()` call (L490) — either export the adapter from
     `internal/mcp` or duplicate the two-line formatting inline in `main.go`
     (mirrors how other CLI handlers already call `wsdoc.*` directly, e.g.
     `ticketsClose`/`ticketsMove` at L613-651); add a `tickets verify`
     subcommand (`ticketsCommand` switch at L507-529 + a `ticketsVerify(args
     []string)` handler) so the CLI mirrors the MCP tool 1:1, matching the
     `git-workflow-tools` mental model's stated rule ("Git operation
     additions require wsgit, MCP dispatch/schema, CLI handler/usage,
     tests...").
6. `agents-plugin-tool/internal/wsdoc/tickets_verify_test.go` (new):
   cases for each hard guardrail firing independently on a fixture built with
   `mustWrite` (bad stem, wrong/legacy status dir e.g. `wip`, missing/blocked
   ready sage-posture, malformed frontmatter — missing closing fence,
   malformed `### Phase`/`### Result` heading, missing `.done`/`.dropped`
   date field), one case confirming spec-address on a non-exempt ready
   ticket is a `Warnings` entry not a `Findings` entry, one case confirming
   an `epic`/`research`/`workset` ready ticket is exempt from the
   spec-address warning, and one passing-fixture case with `OK == true` and
   empty `Findings`.
7. `agents-plugin-tool/internal/wsgit/git_test.go`: add a `Commit`-level test
   with a real `t.TempDir()` root (not the fake `"/repo"` used elsewhere)
   containing an invalid ticket file staged via `opts.Paths`, a
   `Client{Runner: <fake>, Verifier: <stub returning error>}`, asserting
   `Commit` returns the verify error and never reaches the `commit -m`
   runner call (assert on `sequenceRunner`/call-count, mirroring
   `TestCommitRefusesUnrelatedStagedPaths`'s style); and a passing-verify
   case asserting `Commit` proceeds unchanged when `Verifier` is nil
   (backward-compat / no-op default).
8. `agents-plugin-tool/internal/mcp/server_test.go` (or a new
   `tickets_verify_test.go` beside `tickets_checklist_test.go`): a
   dispatch-level test hitting `tickets.verify` end-to-end (real tmp root,
   invalid + valid fixtures) confirming call-site parity: the same
   fixture's verdict from the standalone `tickets.verify` call matches the
   verdict `git.commit` produces when staging the same ticket path (per the
   ticket's explicit acceptance check).
9. Run `go build ./...` and `go test ./...` from `agents-plugin-tool/`.
10. **Dual-tree sync check**: confirm whether this ticket's scope touches
    `agents-plugin/rsrc/` or `agents-plugin-wsflow/rsrc/` bundled doc/prompt
    text (e.g. a convention doc describing the mutation-tool surface). Phase
    1 is Go-code-only (new tool + gate, no playbook/procedure text change),
    so no rsrc-tree edits are expected — confirm this by grepping both rsrc
    trees for `tickets.create`/`tickets.move`/`git.commit` prose mentions
    before closing the phase, and only touch both trees together if any
    prose needs a `tickets.verify` mention.

## Verification Plan
- `cd agents-plugin-tool && go build ./...`
- `cd agents-plugin-tool && go test ./internal/wsdoc/... ./internal/wsgit/... ./internal/mcp/...`
- Manual acceptance-check pass per the ticket's fixture list: construct one
  ticket file per guardrail violation, confirm `tickets.verify` reports it
  and `git.commit` blocks staging it; confirm a ready ticket lacking
  spec-address only warns (commit still succeeds; check `CommitResult` is
  non-empty and no error returned).
- Confirm `go vet ./...` is clean (repo convention baseline, not
  ticket-specific).

## Escalations
- None.
