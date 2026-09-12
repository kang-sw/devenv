# Plan: 260908-bug-shipped-surfaces-carry-devenv-only-content — Phase 2: Point leaks

## Relevant Ticket Contract

- Decision 3: "Point leaks are deleted or routed, never gated on detecting
  devenv" — apply the twelve-site table verbatim disposition; no site is
  gated on detecting this repository.
- Phase 2 scope (this survey): the twelve Decision-3 sites, mirror every
  rsrc change into `agents-plugin-wsflow/rsrc/`, add devenv's Landing Lens
  clause to gitignored `ai-docs/_review.local.md`, update
  `TestScopeAnnouncementFiresOnWorkflowManual` and
  `TestLegacyMarkerLinesIgnoreMechanismProseFile` to assert generic text,
  drop the `ai-docs/ref/worktree-ticket-scope.md` pointer phrase from the
  `{#260810-scope-announcement-idea-inclusion}` sentence.
- Phase 2 verification boundary: `go test ./...` `-count=1`; both plugin
  test suites; `ws-mcp doctor --root <scratch>` (ai-docs/ + AGENTS.md only)
  reports OK; `diff -r agents-plugin/rsrc agents-plugin-wsflow/rsrc` empty;
  `grep -rn worktree-ticket-scope ai-docs/spec agents-plugin
  agents-plugin-wsflow` returns nothing.
- Non-goal (Constraints): "The `doctor` and legacy-marker spec sentences do
  not quote the removed text and are unchanged" — `ai-docs/spec/mcp-tools.md`
  line ~1229 (`260726-refactor-retire-spec-planned-marker-mechanism` in the
  legacy-marker section) is prose, not a quoted advisory string; leave as is.

## Out of Scope

- Phase 1 (binding-anchor hook, migration-anchor rename) — already landed at
  `eceadf11`, confirmed via `git log -1` on the checked-out branch.
- Phase 3 (the Python downstream-neutral guard test, the
  `skill-authoring.md` "Resolvable downstream" checklist item) — explicitly
  excluded by the lead's task framing.
- The migration-anchor family sites (Decision 1/2) — separate concept,
  already handled in Phase 1.
- `ai-docs/spec/mcp-tools.md:1229` prose mention of
  `260726-refactor-retire-spec-planned-marker-mechanism` — unchanged per
  Constraints.
- Adding a `.gitignore` entry for `ai-docs/_review.local.md` — already
  covered by the existing `ai-docs/**/*.local.md` pattern (verified: `git
  check-ignore -v` confirms the match).

## Codebase Findings

1. `agents-plugin/rsrc/lead-tune/lead-tune.md:75` (byte-identical mirror at
   `agents-plugin-wsflow/rsrc/lead-tune/lead-tune.md:75`) — "point to
   research ticket `260611-research-ws-per-role-delegation-tuning-config`."
   No test pins this string (confirmed via grep across
   `agents-plugin-tool/`).
2. `agents-plugin/rsrc/lead-update-spec/lead-update-spec.md:22` (identical
   mirror) — `Read \`agents-plugin/rsrc/lead-write-spec/lead-write-spec.md\`.`
   is a raw repo path; every sibling cross-playbook reference in this repo
   uses `{{.McpNamespace}}/playbook.read(name: "<stem>")` (confirmed pattern
   at `lead-discuss.md:61,64,73`, `lead-forge-spec.md:266,272`,
   `lead-goal-fan-out-step.md:18`).
3. `agents-plugin-tool/internal/wsdoc/doctor.go:23` — the `checks` slice
   entry `{"agents-plugin", filepath.Join(root, "agents-plugin"), true}`.
   `smoke` (`cmd/ws-mcp/main.go:124` `runSmoke`) calls the same
   `wsdoc.Doctor`, so removing this one table row fixes both `doctor` and
   `smoke`. No test file references `wsdoc.Doctor`/`DoctorReport` besides
   `main.go` and `doctor.go` itself — no golden test to update. Verified
   live: building the binary and running `doctor --root` against a
   scratch dir with only `ai-docs/` + `AGENTS.md` currently fails with
   `missing agents-plugin: ...` (exit 1); after deleting the row it will
   report all four remaining `ok` lines and exit 0.
4. `agents-plugin/rsrc/lead-review/lead-review.md:101-106` (identical
   mirror) — built-in default Landing Lens body:
   ```
   ## Landing Lens                        ← optional to customize; range scenario always runs it (built-in default below if omitted); branch scenario never runs it
   Diff follows repo conventions (AGENTS.md, skill-authoring, wsflow-mirroring
   where applicable). Caller-visible behavior changes have a matching spec update
   ...
   ```
   Line numbers shifted from the ticket's cited `100-103` to `101-106` (one
   line earlier content added by an unrelated prior edit); content otherwise
   matches the ticket's citation exactly.
5. `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:20` (identical
   mirror) — `- Retired Claude plugin artifacts are out of support for this
   skill; do not reintroduce \`claude-plugin/\`.` Whole-bullet deletion; no
   test pins this string.
6. `agents-plugin-tool/internal/mcp/scope_announcement.go:35` — `sb.WriteString(" See
   ai-docs/ref/worktree-ticket-scope.md; restore full visibility with
   \`git sparse-checkout disable\`.")`. Pinned by
   `scope_announcement_test.go:62`:
   `if !strings.Contains(freshResp, "worktree-ticket-scope.md") { t.Fatalf(...) }`
   inside `TestScopeAnnouncementFiresOnWorkflowManual` (line 38) — this is
   the correct, exactly-matching golden test named by the lead.
7. `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md:14,15,30`
   (identical mirror) — three prose pointers to
   `ai-docs/ref/worktree-ticket-scope.md`:
   - L14: `... unreproduced hazard recorded in \`ai-docs/ref/worktree-ticket-scope.md\` (\`## Unreproduced Hazard\`): no pattern shape is provably safe, ...`
   - L15: `... a raw \`git mv\` into a vanished directory fails even after widening (see \`ai-docs/ref/worktree-ticket-scope.md\`).`
   - L30: `... (\`ai-docs/ref/worktree-ticket-scope.md\` "Cross-Scope \`git mv\`" carries the detail). \`git sparse-checkout disable\` fully restores the worktree at any time.`
   Line numbers match the ticket exactly. `ai-docs/ref/worktree-ticket-scope.md`
   itself is untouched (out of scope; it stays as a reference doc, just no
   longer named from shipped prose).
8. `agents-plugin-tool/internal/wsdoc/legacy_marker.go:408` — prefix string
   `"legacy planned marker (contract-first planned-entry mechanism being
   retired by 260726-refactor-retire-spec-planned-marker-mechanism): %d
   marker(s) %s"`. **Naming discrepancy found**: the ticket (Phase 2 text
   and Constraints) names `TestLegacyMarkerLinesIgnoreMechanismProseFile` as
   the test to update, but that test (lines 213-222, same file) only asserts
   spec-prose files produce zero markers — it contains no reference to the
   stem string and needs no change. The actual golden-string sites are two
   `legacyMarkerAdvisoryPrefix` constants that embed the stem verbatim:
   - `agents-plugin-tool/internal/wsdoc/legacy_marker_test.go:233-234` (used
     by `TestLegacyMarkerAdvisoryFlipsWithLiveTicketState` and
     `TestLegacyMarkerAdvisoryMatchesOwnAnchorReference`, among others via
     the shared constant).
   - `agents-plugin-tool/internal/mcp/legacy_marker_render_test.go:13-14`
     (a second, independently-defined copy of the same constant in the
     `mcp` package, "mirrors wsdoc's note prefix ... so a render-side
     change to the note is caught here too").
   Both constants must drop the `by 260726-refactor-...` clause to match the
   reworded source string. This is a same-decision correction (which test
   file/line actually pins the string), not a scope change — the stem-drop
   requirement is unchanged and, if anything, more completely covered.
9. `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md:133-142`
   — the "Equivalence note" paragraph (blank line + 8 lines of prose ending
   "...tokens substituted for the wsflow-prefixed equivalents." + trailing
   blank line before `- v0001:`) citing commit `599fb453` and ticket
   `260825`. Confirmed absent from the ws counterpart
   (`agents-plugin/skills/lead-bootstrap/AGENTS.template.md` has no
   `599fb453`/`260825`/"Equivalence note" text) — this file is a
   wsflow-package-only scaffold template, not part of the rsrc mirror, so no
   corresponding ws-side edit or rsrc regen applies. No manifest.json exists
   under `agents-plugin-wsflow/skills/`, so no regen step is needed here.
10. `agents-plugin-tool/internal/wsdoc/conventions/mental-model-conventions.md:91`
    — "analogous to `## Architecture Rules` in `CLAUDE.md`, but scoped to the
    domain the doc covers." Single file, no wsflow counterpart (served by the
    Go binary directly, not duplicated). Change `CLAUDE.md` → `AGENTS.md`.
11. `ai-docs/spec/mcp-tools.md:169` and `:496` — two bare citations:
    - L169 (inside a `>` blockquote): `The bootstrap tool name is
      deliberately obscure (260617 obscurity, soft guard): semantically
      disconnected from ...` — delete `(260617 obscurity, soft guard)`;
      the colon clause still reads standalone.
    - L496: `... no new git-mutation MCP verb is added anywhere in this
      family (260605 pivot constraint). A note key can contain arbitrary
      characters ...` — delete `(260605 pivot constraint)`; sentence break
      unaffected.
    Line numbers shifted by ~1 from the ticket's cited `495`/`169` (`496`
    actual for the first) — same sentences, confirmed by grep for
    "pivot constraint" / "obscurity".
12. `agents-plugin-wsflow/skills/lead-revive/SKILL.md:8` — `Recover your ws
    \`session_key\` from the compaction summary, ...`. This file is not
    rsrc-mirrored (it's `EXPECTED_INLINE_SKILLS`, per
    `ai-docs/manuals/wsflow-mirroring.md` "Exception: `lead-revive` ships an
    inline procedure body"); the rest of the file already correctly says
    `wsflow/workflow_manual`, `wsflow/mcp-server-repair` — only this one
    word (`ws` → `wsflow`) is the leak. The ws counterpart
    (`agents-plugin/skills/lead-revive/SKILL.md:8`) correctly says "ws" and
    is unaffected.

Additional Phase-2-scope items (named separately from the twelve-site table
in Phase 2's own text):

13. `ai-docs/spec/mcp-tools.md:688-692`, under
    `{#260810-scope-announcement-idea-inclusion}` (inside the
    `{#260626-workflow-manual-restoration-entry}` section, confirmed at
    line 642) — drop only the clause `pointing to
    \`ai-docs/ref/worktree-ticket-scope.md\`,` from the sentence describing
    the scope-announcement block, keeping "the `git sparse-checkout
    disable` restore path, and a `git sparse-checkout list` pointer ...".
    No new hazard-sentence text is added to the spec — the ticket's
    instruction is a pure drop, not a rewrite.
14. `ai-docs/_review.local.md` does not currently exist in this working
    tree (fresh worktree) and is git-ignored via the existing
    `ai-docs/**/*.local.md` pattern (`.gitignore:16`) — no gitignore change
    needed. Create it carrying exactly the Landing Lens clause being
    removed from the shipped default (site 4's old text, verbatim), so
    devenv's own behavior is unchanged. Minimal structure (`# Review:
    devenv` + `## Landing Lens`) is sufficient: all other Review Config
    Template sections are optional per `lead-review.md:14` ("A present
    config's Review Phases, Checklist, Blocked Paths, and Deep Review
    sections are honored by both scenarios; `## Landing Lens` is honored by
    the range scenario only"), and inventing Remote/Comment-method/etc.
    answers is not part of this ticket's scope.

wsflow-mirroring regen procedure (confirmed against
`ai-docs/manuals/wsflow-mirroring.md`): edit only canonical
`agents-plugin/rsrc/**` files (sites 1, 2, 4, 5, 7), then run, in order:
```
cd agents-plugin-tool
WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
```
This regenerates `agents-plugin/rsrc/manifest.json` and syncs
`agents-plugin-wsflow/rsrc/` byte-for-byte — do not hand-edit files under
`agents-plugin-wsflow/rsrc/` directly. `-count=1` is mandatory on both (env-gated
test bodies with no changing input; go's test cache can return a stale green
`ok` otherwise). Sites 9 and 12 are package-local (wsflow skills tree, not
rsrc) and are edited directly with no regen step; site 10 has no wsflow
counterpart at all.

## Implementation Plan

1. `agents-plugin/rsrc/lead-tune/lead-tune.md:75` — replace the sentence
   with a no-pointer form per Decision 3 ("not a supported knob" with no
   pointer), e.g. drop the "point to research ticket ..." clause entirely,
   keeping step 1's "not a supported tuning knob today" as the full guidance
   for this case.
2. `agents-plugin/rsrc/lead-update-spec/lead-update-spec.md:22` — replace
   `Read \`agents-plugin/rsrc/lead-write-spec/lead-write-spec.md\`.` with
   `Call {{.McpNamespace}}/playbook.read(name: "lead-write-spec")` (matching
   the established cross-playbook-read idiom used elsewhere in this file's
   sibling playbooks).
3. `agents-plugin-tool/internal/wsdoc/doctor.go:23` — delete the
   `{"agents-plugin", filepath.Join(root, "agents-plugin"), true}` row from
   the `checks` slice. No other code change needed; `smoke` inherits the fix
   through the shared `Doctor` call.
4. `agents-plugin/rsrc/lead-review/lead-review.md:102-103` — replace `Diff
   follows repo conventions (AGENTS.md, skill-authoring, wsflow-mirroring
   where applicable).` with the generic form from Decision 3: `Diff follows
   the repo's own conventions (\`AGENTS.md\` and any authoring manual it
   names).` Keep the rest of the paragraph (spec/mental-model update
   clauses) unchanged.
5. `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md:20` — delete the
   bullet `- Retired Claude plugin artifacts are out of support for this
   skill; do not reintroduce \`claude-plugin/\`.` entirely.
6. `agents-plugin-tool/internal/mcp/scope_announcement.go:35` — replace `"
   See ai-docs/ref/worktree-ticket-scope.md; restore full visibility with
   \`git sparse-checkout disable\`."` with a one-sentence inline hazard
   statement plus the restore command, e.g.: `" No pattern shape is
   provably safe against a rare hide-too-much failure, so verify by
   listing after every apply; restore full visibility with \`git
   sparse-checkout disable\`."` (exact wording is the executor's word
   choice — the requirement is: keep the `git sparse-checkout disable`
   command, state the hazard in one inline sentence, drop the file
   pointer). Then update
   `agents-plugin-tool/internal/mcp/scope_announcement_test.go:62` in
   `TestScopeAnnouncementFiresOnWorkflowManual`: replace the
   `strings.Contains(freshResp, "worktree-ticket-scope.md")` assertion (and
   its message) with an assertion on a fragment of the new inline sentence
   (e.g. `"hide-too-much"` or whatever exact phrase step 6 lands on) so the
   test still proves the hazard text renders, without depending on the
   deleted file pointer.
7. `agents-plugin/rsrc/lead-scope-worktree/lead-scope-worktree.md:14,15,30`
   — reword each of the three pointers to state the hazard/detail inline in
   one sentence instead of citing `ai-docs/ref/worktree-ticket-scope.md`:
   - L14: keep "no pattern shape is provably safe" reasoning inline, drop
     `recorded in \`ai-docs/ref/worktree-ticket-scope.md\` (\`##
     Unreproduced Hazard\`)`.
   - L15: state inline that a raw `git mv` into a vanished directory fails
     even after widening, drop `(see \`ai-docs/ref/worktree-ticket-scope.md\`)`.
   - L30: state inline that a raw `git mv` fails with `No such file or
     directory` until the directory is recreated, drop `(\`ai-docs/ref/
     worktree-ticket-scope.md\` "Cross-Scope \`git mv\`" carries the
     detail)`; keep the `git sparse-checkout disable` sentence after it.
8. `agents-plugin-tool/internal/wsdoc/legacy_marker.go:408` — remove ` by
   260726-refactor-retire-spec-planned-marker-mechanism` from the prefix
   format string, leaving `"legacy planned marker (contract-first
   planned-entry mechanism being retired): %d marker(s) %s"`. Then update
   both golden constants to match (see Codebase Finding 8 for the
   discrepancy with the ticket's named test):
   - `agents-plugin-tool/internal/wsdoc/legacy_marker_test.go:233-234`
     (`legacyMarkerAdvisoryPrefix` const)
   - `agents-plugin-tool/internal/mcp/legacy_marker_render_test.go:13-14`
     (second `legacyMarkerAdvisoryPrefix` const)
   Both become `"legacy planned marker (contract-first planned-entry
   mechanism being retired): 1 marker(s) at line 10"`. Run
   `go test ./internal/wsdoc/... ./internal/mcp/... -count=1` afterward to
   confirm every consumer of both constants (the
   `TestLegacyMarkerAdvisory*` family and the `mcp` package render tests)
   still passes.
9. `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md` — delete
   the Equivalence note paragraph at lines 133-142 (blank line through
   "...wsflow-prefixed equivalents." plus the following blank line before
   `- v0001:`), leaving "...merge surgically and mark conflicts instead of
   overwriting." flowing directly (with one blank line) into the `- v0001:`
   list.
10. `agents-plugin-tool/internal/wsdoc/conventions/mental-model-conventions.md:91`
    — change `` `## Architecture Rules` in `CLAUDE.md` `` to `` `##
    Architecture Rules` in `AGENTS.md` ``.
11. `ai-docs/spec/mcp-tools.md` — delete `(260617 obscurity, soft guard)` at
    line 169 and `(260605 pivot constraint)` at line 496 (leaving a single
    space before the following punctuation, matching normal prose spacing).
12. `agents-plugin-wsflow/skills/lead-revive/SKILL.md:8` — change `Recover
    your ws \`session_key\`` to `Recover your wsflow \`session_key\``.
13. `ai-docs/spec/mcp-tools.md:688-692` (`{#260810-scope-announcement-idea-
    inclusion}`) — remove the clause `pointing to
    \`ai-docs/ref/worktree-ticket-scope.md\`,` from the sentence, keeping
    the surrounding "a short block naming the hidden ticket count and
    stems ..., the `git sparse-checkout disable` restore path, and a `git
    sparse-checkout list` pointer to the worktree's active re-include
    patterns" intact.
14. Create `ai-docs/_review.local.md` with:
    ```markdown
    # Review: devenv

    ## Landing Lens
    Diff follows repo conventions (AGENTS.md, skill-authoring, wsflow-mirroring
    where applicable). Caller-visible behavior changes have a matching spec update
    (spec describes caller-visible behavior); workflow-system modification-relevant
    changes have a matching mental-model update (mental model captures
    modification-relevant operational knowledge) — each doc updated per its own
    function, not just "any doc touched."
    ```
    (verbatim copy of the text removed from the shipped default at step 4,
    so devenv's actual review behavior is unchanged). Confirm it is in fact
    git-ignored after creation (`git status` shows it untracked-and-ignored,
    not untracked-and-visible).
15. After steps 1, 2, 4, 5, 7 land (the five canonical `agents-plugin/rsrc/`
    edits), run the wsflow-mirroring regen procedure from Codebase Findings
    to sync `agents-plugin-wsflow/rsrc/` and `agents-plugin/rsrc/manifest.json`:
    ```
    cd agents-plugin-tool
    WSRSRC_REGEN=1 go test ./internal/wsrsrc/... -count=1 -run TestGenerateRealManifest
    WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror
    ```
    Do not hand-edit anything under `agents-plugin-wsflow/rsrc/`.

## Verification Plan

- `cd agents-plugin-tool && go test ./... -count=1` — must be green across
  all packages (exercises steps 3, 6, 8, and the regenerated manifest from
  step 15).
- `python3 -m unittest discover agents-plugin-wsflow/tests` — wsflow package
  test suite (runtime contract + distributed skill bundle), per
  `ai-docs/manuals/wsflow-mirroring.md` "Static Verification".
- `go run ./cmd/ws-mcp doctor --root <scratch>` (or the built binary) against
  a directory holding only `ai-docs/` and `AGENTS.md` — must report OK / exit
  0 (already confirmed the pre-fix failure mode live; re-run post-fix).
- `diff -r agents-plugin/rsrc agents-plugin-wsflow/rsrc` — must be empty
  after the regen step.
- `grep -rn worktree-ticket-scope ai-docs/spec agents-plugin
  agents-plugin-wsflow` — must return nothing.
- `grep -rn "260611-research-ws-per-role-delegation-tuning-config"
  agents-plugin agents-plugin-wsflow` — must return nothing.
- `grep -rn "claude-plugin/" agents-plugin agents-plugin-wsflow` (excluding
  `AGENTS.md`'s own "do not reintroduce" rule at the repo root, which is
  root-repo policy text, not shipped surface) — must return nothing under
  `rsrc/`/`skills/`.
- `grep -rn "599fb453\|260825" agents-plugin-wsflow/skills` — must return
  nothing.
- `grep -n "CLAUDE.md" agents-plugin-tool/internal/wsdoc/conventions/mental-model-conventions.md` — must return nothing.
- `grep -n "pivot constraint\|obscurity, soft guard" ai-docs/spec/mcp-tools.md` — must return nothing.
- Manual: open `ai-docs/spec/mcp-tools.md` `{#260810-scope-announcement-idea-inclusion}` region and confirm the sentence still parses grammatically with the clause removed.

## Escalations

- None.
