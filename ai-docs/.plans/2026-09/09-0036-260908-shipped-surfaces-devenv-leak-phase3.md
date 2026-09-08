# Plan: 260908-bug-shipped-surfaces-carry-devenv-only-content — Phase 3: Guard

## Relevant Ticket Contract

- Add `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py` per
  Decision 4, docstring cites `AGENTS.md` rule 4 (the test is the rule's
  mechanical form; do not restate the rule elsewhere).
- Decision 4 scan scope: `agents-plugin/rsrc/**`, `agents-plugin/skills/**`,
  their `agents-plugin-wsflow/` equivalents, `agents-plugin-tool/internal/
  wsdoc/conventions/**`, and the string literals (double-quoted and
  backtick, **comments excluded**) of every non-test `.go` file under
  `agents-plugin-tool/`. Walk git-tracked files only. Do not scan `bin/`,
  `runtime.json`, or plugin manifests.
- Decision 4 failure rules (a line fails when any hits):
  1. Ticket-stem-shaped token `26[0-9]{4}-[a-z][a-z0-9-]+` resolves to a
     real ticket under any status dir (`idea/todo/ready/.done/.dropped`) or
     to a `{#...}` spec anchor. A token resolving to neither is an example
     and passes.
  2. A path under `ai-docs/` names a specific file that is git-tracked in
     this repo and is *not* among the files `lead-bootstrap` installs (set
     derived from the bootstrap scaffold list, not hardcoded independent of
     it). A runtime-created gitignored local file (e.g. `_review.local.md`)
     passes. Naming a bare directory (`ai-docs/tickets/`, `ai-docs/
     manuals/`, `ai-docs/ref/`) or a `<status>/<stem>.md` placeholder
     passes.
  3. A bare six-digit `26[0-9]{4}` token (not immediately followed by
     `-[a-z]`, i.e. a citation, not a stem head already judged by rule 1)
     whose date prefix matches a real ticket in this repo. The convention
     docs' `260421-feature-name`-shaped example stems are full stems (rule
     1's job), not bare citations, and pass.
  4. The line contains a commit hash introduced by the word "commit", one
     of this repo's own names as a whole token (`agents-plugin`,
     `agents-plugin-tool`, `agents-plugin-wsflow`, `claude-plugin/`,
     `install.sh`, `skill-authoring`, `wsflow-mirroring` — not preceded or
     followed by `[a-z0-9-]`), or one of the migration phrases (`migration
     anchor`, `native-subagent pivot`, `spawn-removal`, `host-neutral
     migration`, `adapter boundar`, `retired Claude tree`, `Codex-first`).
- No allowlist mechanism, ever (explicitly rejected in Decision 4 — an
  allowlist is where the next leak hides). Each failure prints file, line,
  token, and matched rule.
- Add the **Resolvable downstream** invariant-checklist item ("resolves in
  a project holding only what bootstrap installs?") to
  `ai-docs/manuals/skill-authoring.md`; "all six" becomes "all seven".
- Phase 3 required verification: test passes on the current (post-Phase-2)
  tree; reinserting the pre-Phase-1 guardrail sentence into
  `session_state.go` fails it; a `// 260605`-style Go comment, the
  `ticket-conventions.md` example stem `260115-feat-foo-bar`, and a line
  naming `ai-docs/manuals/` as a bare directory each pass; the plugin test
  suites (Go + wsflow python bundle) still pass.
- Constraint: Phase order is fixed — Phases 1 and 2 (already landed at
  `95e10a76`) must precede this guard; it must not ship with an allowlist
  to land early (it doesn't need one — the tree is already clean).

## Out of Scope

- Phases 1 and 2 content — already landed and verified clean (`grep -rn
  260605` and `grep -rn 'migration.anchor'` both clean across shipped
  surfaces per their Result sections).
- `migration_anchor`/`binding_anchor` gate behavior, the `260605` anchor's
  content — Phase 1/2 territory, not this guard.
- `kang-sw-devenv` literals in `wsstate/paths.go` and `wsagent/agent.go` —
  ticket's explicit out-of-scope carve-out (tool-created, every downstream
  project has them).
- Any allowlist/exemption list for the scanner — explicitly rejected
  mechanism, not a simplification to reach for under pressure.
- Scanning `ai-docs/spec/**` or `ai-docs/mental-model/**` themselves — they
  are not shipped surfaces; they're consulted only as the resolution
  targets for rule 1 (spec anchors) and are not part of the four scanned
  trees.

## Codebase Findings

- `agents-plugin/tests/test_skill_dispatch_contracts.py:1-11` — established
  pattern to follow: plain `unittest.TestCase`, module-level `Path(__file__)
  .resolve().parents[1] / "skills"` / `"rsrc"` constants, no pytest config
  or conftest in the repo. Tests run via `python3 -m unittest
  tests.test_shipped_surfaces_downstream_neutral -v` from `agents-plugin/`
  (confirmed convention: `ai-docs/tickets/idea/260904-bug-stale-proceed-
  dispatch-contract-test.md:15` and prior plan files use the same
  invocation; `python -m pytest tests/... -q` also works, no pytest-only
  fixtures needed).
- `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py:4` — `subprocess`
  is already an established import in this test dir; use `subprocess.run(
  ["git", "ls-files"], cwd=REPO_ROOT, ...)` for the git-tracked-files walk
  (repo root is `Path(__file__).resolve().parents[2]` from
  `agents-plugin/tests/`).
- `agents-plugin-tool/internal/mcp/session_state.go:391-543` — current
  (post-Phase-1) state: `implementPrepInstruction` takes `verdict
  implementTodoVerdict` and splices in `verdict.BindingAnchorClause`
  (line 534); no devenv literal remains here today.
- Pre-Phase-1 sentence (for the "reinserting it fails the test"
  verification), recovered via `git show 83b6653a^:agents-plugin-tool/
  internal/mcp/session_state.go`:
  `Before edits or dispatch, run mental-model lookup, read returned docs
  ancestors first, read the 260605 migration anchor when target touches
  plugin architecture, host-neutral migration, spawn-removal, or adapter
  boundaries, and read infra.read("impl-playbook"). ` — this single
  sentence trips rule 3 (bare `260605` citation, ticket
  `260605-research-ws-native-subagent-pivot` exists) and rule 4 twice
  (`host-neutral migration`, `spawn-removal`, `adapter boundar`), so it is
  a solid fixture for the "guard fails on the known pre-fix leak"
  verification. This is a manual/reported verification step (temporarily
  edit the constant, run the test, confirm failure, revert), the same
  pattern Phase 1/2 used for their grep-based Result verifications — it is
  not one of the two required permanent unit cases (comment-exclusion is).
- `agents-plugin-tool/internal/wsrsrc/loader.go:26`,
  `internal/wsgit/git.go:40,455,505,734`,
  `internal/wsdoc/tickets_scope.go:19`, `internal/wsdoc/legacy_marker.go:190`,
  `internal/wsdoc/project_tree.go:41`,
  `internal/wsdoc/tickets_verify.go:137`,
  `cmd/ws-mcp/parent_watch_windows.go:17` — **real, currently-present**
  `//` comments naming ticket stems and `{#...}` spec anchors (e.g.
  `{#260720-wsdoc-commit-boundary}`, `260726-refactor-retire-spec-planned-
  marker-mechanism`, `ticket 260724 hypothesis A`). These are load-bearing
  evidence, not synthetic: if the Go extractor did not exclude comments,
  the test would fail on the current clean tree from real, legitimate
  code. This makes comment-exclusion correctness a genuine (not
  paper-only) requirement and the exact reason Phase 3 requires the unit
  case — reuse one of these real lines as a positive control instead of a
  fabricated-only fixture.
- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md:12` —
  `- Reference tickets by **stem only** (e.g., \`260115-feat-foo-bar\`),
  never by full path.` Confirmed no ticket with stem `260115-feat-foo-bar`
  exists under any status dir — correct pass case for rule 1's "resolves
  to neither" branch.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:90` and
  `agents-plugin/rsrc/impl-playbook.md:38` — existing bare-directory
  mentions of `ai-docs/manuals/` (no filename), correct pass cases for
  rule 2's directory-name carve-out.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md:92-112` — the
  bootstrap scaffold's `<!-- MIGRATION: ... -->` comment block is the
  source for deriving rule 2's "files lead-bootstrap installs" set. It
  lists `ai-docs/` sub-entries; only `WORKFLOW.md` (→ `ai-docs/WORKFLOW.md`)
  is a concrete file — everything else (`mental-model/`, `spec/`,
  `manuals/`, `ws-notes/`, `.old/`, `ref/`, `tickets/<status>/`) is a bare
  directory entry, confirmed by `lead-bootstrap.md`'s `## On: fresh`
  handler (steps 1-8: only `AGENTS.md`, `ai-docs/WORKFLOW.md`, and
  `CLAUDE.md` are concrete files written; everything else is directory
  scaffolding or `.gitignore` entries). Derive the installed-file set by
  parsing this migration block (lines inside the `<!-- MIGRATION:` comment
  that have no trailing `/` and end in a plausible filename) rather than
  hardcoding a literal list disconnected from the source — the ticket says
  "the test derives that set from the bootstrap skill's scaffold list."
  Concretely the derived set today is `{ai-docs/WORKFLOW.md}` (root
  `AGENTS.md`/`CLAUDE.md` are not under `ai-docs/`, so irrelevant to this
  particular rule).
- `ai-docs/manuals/skill-authoring.md:71` — exact line to edit: `Every
  invariant must pass all six: **Falsifiable** ... · **Doctrine-aligned**
  (re-derives from the file's doctrine?).` Append **Resolvable downstream**
  (resolves in a project holding only what bootstrap installs?) as a
  seventh criterion and change "all six" to "all seven".
- `ai-docs/spec/*.md` anchors use the `{#<slug>}` format (e.g.
  `ai-docs/spec/api-documentation-cache.md:13:{#260505-api-docs-mcp-
  surface}`); anchor slugs are ticket-stem-shaped and must be checked
  against a set of all `{#...}` anchors collected across `ai-docs/spec/`
  for rule 1.
- Repo scale for the Go scan: `find agents-plugin-tool -name "*.go" ! -name
  "*_test.go" | wc -l` → 100 files; the scan need not be fast, correctness
  matters more than speed for a test that runs occasionally.
- Confirmed current tree is already clean per Phase 1/2 Result sections
  and a fresh spot-check: `grep -rn 260605 agents-plugin agents-plugin-
  wsflow agents-plugin-tool` (excluding `_test.go`) hits only a
  pre-existing, out-of-scope wsflow Python test comment
  (`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py:9`, not under
  the scanned trees); `grep -rn 'migration.anchor'` and `grep -rn
  worktree-ticket-scope` are both clean across shipped surfaces.

## Implementation Plan

1. In `agents-plugin/tests/test_shipped_surfaces_downstream_neutral.py`
   (new file), set up module constants: `REPO_ROOT = Path(__file__)
   .resolve().parents[2]`, the four scanned trees (`agents-plugin/rsrc`,
   `agents-plugin/skills`, `agents-plugin-wsflow/rsrc`,
   `agents-plugin-wsflow/skills`, `agents-plugin-tool/internal/wsdoc/
   conventions`), and the Go source root
   (`agents-plugin-tool`, excluding `*_test.go`). Docstring at module top
   cites `AGENTS.md` rule 4 verbatim intent (downstream-neutral shipped
   text) and states the rule lives only in `AGENTS.md`.
2. Add a `git_tracked_files(repo_root)` helper using `subprocess.run(["git",
   "ls-files"], cwd=repo_root, capture_output=True, text=True, check=True)`
   to get the git-tracked set for both rule 2 (path-exists check) and to
   restrict the scan itself to tracked files (ignore
   `.local-devenv-runtime`-style gitignored files).
3. Add `iter_ticket_stems(repo_root)`: walk `ai-docs/tickets/{idea,todo,
   ready,.done,.dropped}/*.md` (tracked only) and collect stems (filename
   without `.md`).
4. Add `iter_spec_anchors(repo_root)`: regex `\{#([a-zA-Z0-9_-]+)\}` over
   `ai-docs/spec/*.md` (tracked only), collect the anchor slugs.
5. Add `bootstrap_installed_ai_docs_files(repo_root)`: parse the
   `<!-- MIGRATION: ... -->` block in `agents-plugin/skills/lead-bootstrap/
   AGENTS.template.md`; for each listed entry line under the `ai-docs/`
   heading that does not end in `/` and is not the `tickets/<status>/`
   line, emit `ai-docs/<entry>`. Confirm output is `{"ai-docs/WORKFLOW.md"}`
   against the current template (assert this in a small unit test so a
   template edit that silently changes the derived set is caught).
6. Add `extract_go_string_literals(source_text)`: strip Go comments (`//
   ...` to end of line, `/* ... */` block, both comment-stripped before any
   literal extraction — do not use a plain string-literal regex on raw
   source, since a stripped-comment `"` inside a comment could otherwise
   confuse a naive quote-scanner) — walk the source with the same
   line/column bookkeeping so failures can still report source line
   numbers — then extract `"..."` (handling `\"` escapes) and
   `` `...` `` raw string literals, yielding `(line_no, literal_text)`
   pairs. Comment-stripping must be string-literal-aware in the other
   direction too (a `//` or `/*` inside a real string literal is not a
   comment start) — a small hand-rolled character scanner tracking
   in-string/in-comment state is simpler and more reliable here than
   nested regexes.
7. Add a dedicated unit test for step 6 using one of the real comment
   lines found in survey (e.g. a literal Python triple-quoted string
   mirroring `internal/wsdoc/legacy_marker.go:190`'s `// 260726-refactor-
   retire-spec-planned-marker-mechanism requires the` comment plus a
   sibling real string literal on the next line) proving the extractor
   yields the string literal but not the `260726...` token from the
   comment. Also include a synthetic `// 260605` case per the ticket's
   named verification.
8. Add `classify_line(text) -> Optional[FailureReason]` implementing
   Decision 4 rules 1-4 in order, taking the pre-built ticket-stem set,
   spec-anchor set, tracked-file set, and bootstrap-installed set as
   inputs. Reuse one `26[0-9]{4}-[a-z][a-z0-9-]+` regex for rule 1 and a
   `(?<!-)26[0-9]{4}(?!-[a-z])` (or manual boundary check, Python `re`
   lacks variable-length lookbehind for the stem-head exclusion — check
   neighboring chars manually) for rule 3's bare-citation form. Implement
   rule 4's name-boundary check as "not preceded or followed by
   `[a-z0-9-]`" via manual index checks, not just `\b` (word boundary
   treats `-` as non-word, which would wrongly allow `lead-skill-
   authoring` to still match — Decision 4 explicitly calls out this case
   as must-pass).
9. Add the top-level test method(s): walk all Markdown/text files under
   the four non-Go trees line by line, run `classify_line`, collect
   failures; walk Go files, run `extract_go_string_literals` then
   `classify_line` on each literal's text, collect failures. Assert no
   failures, formatting any failure as `<file>:<line>: <token> (rule
   <n>)` in the assertion message so a real future leak is diagnosable
   from CI output alone.
10. Add `ai-docs/manuals/skill-authoring.md:71` edit: append **Resolvable
    downstream** (resolves in a project holding only what bootstrap
    installs?) to the six existing criteria and change "all six" to "all
    seven".
11. Manual verification pass (not committed test code): temporarily
    replace the current `implementPrepInstruction` guardrails clause in
    `agents-plugin-tool/internal/mcp/session_state.go` with the recovered
    pre-Phase-1 sentence, run the new test and confirm it fails on rules
    3 and 4, then revert the file to its committed state before
    finishing.

## Verification Plan

- `cd agents-plugin && python3 -m unittest
  tests.test_shipped_surfaces_downstream_neutral -v` — new test green on
  the current (post-Phase-2) tree.
- Manual: reinsert the pre-Phase-1 sentence into `session_state.go`
  (recovered above), rerun the same command, confirm failure naming the
  `260605`/migration-phrase tokens, then `git checkout --
  agents-plugin-tool/internal/mcp/session_state.go` to revert.
- `cd agents-plugin && python3 -m unittest -v` (or the existing full test
  invocation) — confirm `test_skill_dispatch_contracts`,
  `test_ws_mcp_launcher_capabilities`, `test_ws_mcp_launcher_coldload`
  still pass (unaffected by this addition).
- `cd agents-plugin-tool && go test ./... -count=1` — unaffected, confirm
  still green (no Go source changes in this phase).
- `cd agents-plugin-wsflow && python3 -m unittest discover -s tests -v` (or
  the wsflow bundle's existing invocation) — unaffected, confirm still
  green.
- Spot-check the three named pass cases inside the new test's own
  assertions/fixtures: the `ticket-conventions.md:12` `260115-feat-foo-bar`
  line, an `ai-docs/manuals/` bare-directory line, and a `// 260605`
  Go comment line each produce no failure.

## Escalations

- None.
