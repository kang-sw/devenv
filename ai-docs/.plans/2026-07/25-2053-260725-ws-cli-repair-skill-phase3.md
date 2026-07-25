# Plan: ws-cli — MCP-independent CLI fallback surface for Windows disconnects — Phase 3: mcp-server-repair skill and entry-point pointers

## Relevant Ticket Contract

- New skill `agents-plugin/skills/mcp-server-repair/SKILL.md` (mirrored as
  `agents-plugin-wsflow/skills/mcp-server-repair/SKILL.md`): description names the
  trigger explicitly (ws tools absent from tool list, or a tool call failing to
  connect); body is **fully self-contained, makes no MCP call**, and covers:
  `ws-cli tools` / `ws-cli tools <name>` / `ws-cli call` usage, the mapping rule
  (`ws/x.y(a: b)` -> `ws-cli call x.y '{"a":"b"}'`), the cold-start
  `workflow_manual` + `obsidian-latch` sequence, the PATH-independent
  `python3 <plugin-root>/bin/ws-mcp-launcher.py` form, a note that a stale
  runtime may make the first call slow (repair runs), and the manual reconnect
  procedure to relay to the user.
- Deliberate non-`lead-` prefix: name is `mcp-server-repair`, not `lead-mcp-server-repair`.
- Two audiences in one body: agent-facing (keep working via `ws-cli`) and
  user-facing (concrete re-enable steps, since the agent cannot re-enable the
  server itself).
- Register the skill on **every** enumeration surface, not just the mirror:
  `substitutionMirroredSkills` (`agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go`),
  `agents-plugin/skills/manifest.json` (guarded by `skills_manifest_test.go`),
  `EXPECTED_SKILLS` in `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`,
  and the shipped-skill list in `ai-docs/ref/wsflow-mirroring.md`. Confirm the
  source passes `guardSubstitutionEligible`, then generate the wsflow mirror
  (never hand-write it).
- Add a one-line pointer to eight front doors: `lead-discuss`, `lead-sprint`,
  `lead-revive`, `lead-proceed` in **both** `agents-plugin/skills/` and
  `agents-plugin-wsflow/skills/`. The wsflow four are hand-written (not
  substitution-mirrored) and use `wsflow-cli` / `/wsflow:mcp-server-repair`.
- **Exact per-file rule** (ticket Decisions, verified against current file
  contents below): wsflow's `lead-proceed` and `lead-revive` currently end with
  a "stop and report that blocker" sentence — the pointer **replaces** that
  sentence. The other six (ws `lead-discuss`/`lead-sprint`/`lead-revive`/`lead-proceed`,
  wsflow `lead-discuss`/`lead-sprint`) carry no such line today and simply
  **gain** the pointer line.
- Constraint: pointer stays a single short line (~15 words), per
  `260630-epic-skill-playbook-diet` front-door budget.
- Constraint: `guardSubstitutionEligible`'s denylist includes the literal
  lowercased substring `"ws."` — this disqualifies the mcp-server-repair
  **source** (full-ws copy only; the four front doors are hand-curated on both
  sides and are not run through this guard) if it contains that substring
  anywhere, including inside words like "shows.", "flows.", "windows.",
  "workflows." (`w`+`s`+`.` at the tail of any of these matches). Also avoid
  `"mercenary"`, `<!-- ws:full-only:...`/`<!-- ws:wsflow-only:...` markers, and
  the literal names `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
  `lead-skill-authoring`.
- Verification (from ticket): skill listed by host with ws MCP server stopped;
  body makes no MCP call; mirror generation succeeds and produces `wsflow-cli`
  in the mirrored body; `go test ./internal/wsrsrc/...` and the wsflow bundle
  test both pass with the new skill registered; each of the eight front doors
  changes by exactly the pointer line (plus, in the two wsflow cases, removal
  of the replaced sentence).
- Depends on Phases 1 & 2 (both landed — `ws-cli`/`wsflow-cli` shims and
  `tools`/`call` subcommands exist and are verified working).

## Out of Scope

- Phases 1 and 2 mechanics (CLI passthrough, shims, launcher lazy-import) —
  already landed (commits `6b1038b7`, `1281b11a`); do not re-touch
  `cmd/ws-mcp/main.go`, `bin/ws-cli*`, `bin/wsflow-cli*`, or the launcher.
- Spec reflection into `mcp-tools.md` / `plugin-runtime.md` / `workflow-skills.md`
  — ticket marks this closeout-only, not per-phase.
- Windows `.cmd` `setlocal`/`endlocal` polish and Windows dogfood measurement —
  recorded Phase 2 residuals, not Phase 3 scope.
- `agents-plugin/skills/lead-discuss/agents/openai.yaml`-style Codex display
  metadata for the new skill — not requested by the ticket; do not add one.
- Any edit under `agents-plugin/rsrc/` or `agents-plugin-wsflow/rsrc/` —
  `mcp-server-repair` has no shared rsrc playbook (same pattern as
  `lead-prefer-subagent`/`lead-verify-discussion`/`lead-goal-step`: inline body,
  no playbook dir), so Phase 3 never needs to touch either `rsrc/` tree.
- **`ai-docs/_index.md` refresh — explicitly skip.** A concurrent unrelated
  session holds uncommitted edits there (see Concurrency Guard below); if the
  doc-update gate would normally touch it, skip that step and instead record
  the new skill in this ticket's own `### Result` section for Phase 3.
- Broader skill-surface-reduction direction from the migration anchor (see
  Codebase Findings) — informational only, not an implementation instruction
  for this phase.

## Concurrency Guard (verified via `git status`, fresh)

Working tree currently has five uncommitted files from a **different, unrelated
session**:
- `agents-plugin-wsflow/rsrc/lead-write-ticket/lead-write-ticket.md`
- `agents-plugin-wsflow/rsrc/manifest.json`
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`
- `agents-plugin/rsrc/manifest.json`
- `ai-docs/_index.md`

Phase 3's file set (skills/, the skills manifest, `skills_mirror_test.go`, the
python bundle test, `wsflow-mirroring.md`, the wsflow skills mirror) is
**entirely disjoint** from this set — Phase 3 never touches any `rsrc/` file,
so there is no path collision. Two same-named-but-different files exist in the
tree; do not confuse them:
- `agents-plugin/rsrc/manifest.json` (rsrc tree manifest) — **concurrent
  session's file, do not touch.**
- `agents-plugin/skills/manifest.json` (skills tree manifest, different path,
  different schema instance) — **this phase's file, edit/regenerate freely.**

Rules for the implementer:
1. Never read-for-edit, edit, stage, or commit the five files listed above.
2. Use explicit-pathspec `git add`/`git commit` only — never `git add -A` or
   `git add .`.
3. Skip any `ai-docs/_index.md` doc-gate refresh; note the new skill in the
   ticket's own Phase 3 `### Result` section instead.

## Codebase Findings

- `ai-docs/tickets/ready/260725-feat-ws-cli-mcp-fallback-surface.md#L468-L502` —
  full Phase 3 contract text and file-set enumeration (source of the
  requirements above).
- `ai-docs/tickets/ready/260725-feat-ws-cli-mcp-fallback-surface.md#L415-L466` —
  Phase 2 Result: shims (`ws-cli`, `wsflow-cli`, `.cmd` variants) exist and are
  verified; `wsCliPattern` already added to `skills_mirror.go`. Phase 3 only
  needs to *register* the new skill; the substitution mechanism itself needs no
  changes.
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-L18` —
  `substitutionMirroredSkills = []string{"lead-goal-step", "lead-prefer-subagent",
  "lead-verify-discussion"}`. Append `"mcp-server-repair"` here (order
  insensitive; the test iterates the slice, no alphabetical requirement).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror.go#L25-L34` —
  `disqualifyingTokens` list used by `guardSubstitutionEligible` (source-text
  substring check, case-insensitive): `"mercenary"`, `"<!-- ws:full-only:"`,
  `"<!-- ws:wsflow-only:"`, `"ws."`, `"lead-write-code"`, `"lead-write-skeleton"`,
  `"lead-salvage"`, `"lead-skill-authoring"`. Verify the drafted
  `mcp-server-repair/SKILL.md` source against this list before regenerating the
  mirror — a hit fails the regen test loudly (`GenerateWsflowSkillBody`
  returns an error, `TestWsflowSkillsMirrorUpToDate` fails).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L94-L114`
  (`TestRegenerateWsflowSkillsMirror`) — regen only writes `SKILL.md` per
  listed skill (creates the directory via `os.MkdirAll` if missing), gated by
  `WS_REGEN_WSFLOW_SKILLS=1`; no other files are copied. Command:
  `cd agents-plugin-tool && WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`
  (the `-count=1` is mandatory or the test cache can return a stale green `ok`
  without writing).
- `agents-plugin/skills/manifest.json` (20 current entries, `schema_version: 1`,
  path -> sha256 hash map) guarded by
  `agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go#L27-L44`
  (`TestSkillsManifestDriftIsVisible`). Regenerate with:
  `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
  after the new file exists and the four ws-side front doors are edited — this
  is a **separate** env var/regen path from the rsrc manifest regen
  (`WSRSRC_REGEN`) and from the wsflow skills mirror regen
  (`WS_REGEN_WSFLOW_SKILLS`); do not conflate the three.
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L16-L52` —
  `EXPECTED_SKILLS` (21 entries) and `EXPECTED_INLINE_SKILLS` (4: `lead-revive`,
  `lead-prefer-subagent`, `lead-verify-discussion`, `lead-goal-step`) need
  `"mcp-server-repair"` added to **both** sets (inline because it is a
  generated self-contained body, not a `playbook.print` shim — same pattern as
  the other three substitution-mirrored skills).
- `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py#L54-L64`
  (`FORBIDDEN_PATTERNS`) — scans every file under the wsflow `skills/` tree for
  `\bws/`, `\bws:`, `\bws\.`, `\bsubquery\b`, `\bagents\.`, and the four
  excluded-skill names. This applies to the **wsflow-side** pointer text in all
  four front doors (hand-written) — phrase them to avoid these tokens
  (recommendation below uses none of them).
- **Risk signal — two hardcoded `re.fullmatch` regex tests will break as soon
  as the pointer line is added**, both in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`:
  - `test_skill_files_are_thin_playbook_shims` (`#L104-L122`): loops over
    `EXPECTED_SKILLS - EXPECTED_INLINE_SKILLS - EXPECTED_PARALLEL_INIT_SKILLS`
    (14 skills incl. `lead-proceed`) and requires an **exact** fullmatch ending
    literally `"...playbook cannot be loaded, stop\nand report that blocker.\n"`
    with nothing after. Confirmed current wsflow `lead-proceed/SKILL.md` ends
    exactly that way (read below) — after this phase's edit (pointer *replaces*
    that sentence) the fullmatch will fail unless the test is updated. The
    other 13 skills in this group are untouched by this phase and must keep
    passing unchanged.
  - `test_parallel_init_skill_files_are_playbook_shims` (`#L124-L144`): loops
    over `EXPECTED_PARALLEL_INIT_SKILLS = {"lead-discuss", "lead-sprint",
    "lead-goal-fan-out-step"}` and requires an exact fullmatch ending at
    `"After both return, execute the procedure returned by \`wsflow/playbook.print\`.\n"`
    with nothing after. `lead-discuss` and `lead-sprint` gain a trailing
    pointer line; `lead-goal-fan-out-step` is untouched and must keep matching
    the un-suffixed form.
  - Neither test currently constrains `lead-revive` (it is in
    `EXPECTED_INLINE_SKILLS`, exempt from both regexes), so its wsflow body
    change needs no test edit beyond the `FORBIDDEN_PATTERNS` scan it already
    goes through.
- Confirmed current exact file contents (read directly, 2026-07-25):
  - `agents-plugin/skills/lead-discuss/SKILL.md` — ends
    `"After both return, execute the procedure returned by \`ws/playbook.print\`.\n"`,
    no blocker sentence. **Gains** pointer.
  - `agents-plugin/skills/lead-sprint/SKILL.md` — same shape. **Gains** pointer.
  - `agents-plugin/skills/lead-revive/SKILL.md` — ends
    `"...to bootstrap.\n"`, no blocker sentence. **Gains** pointer.
  - `agents-plugin/skills/lead-proceed/SKILL.md` — ends
    `"...inline against the user request.\n"`, no blocker sentence. **Gains** pointer.
  - `agents-plugin-wsflow/skills/lead-discuss/SKILL.md` — ends
    `"...returned by \`wsflow/playbook.print\`.\n"`, no blocker sentence. **Gains** pointer.
  - `agents-plugin-wsflow/skills/lead-sprint/SKILL.md` — same shape. **Gains** pointer.
  - `agents-plugin-wsflow/skills/lead-revive/SKILL.md` — ends
    `"...to bootstrap.\nIf the tool cannot be loaded, stop and report that blocker.\n"`.
    Pointer **replaces** the final sentence.
  - `agents-plugin-wsflow/skills/lead-proceed/SKILL.md` — ends
    `"...If the playbook cannot be loaded, stop\nand report that blocker.\n"`.
    Pointer **replaces** the final sentence.
- `ai-docs/ref/wsflow-mirroring.md#L29-L83` — "Shipped wsflow Skills" included
  list (needs `mcp-server-repair` added) and the existing exception note for
  `lead-prefer-subagent`/`lead-verify-discussion`/`lead-goal-step` (`#L75-L83`,
  pattern to follow for the new entry's exception wording).
  `#L123-L165` "Substitution-Mirrored Skill Generation" section's curated list
  needs `mcp-server-repair` appended alongside the note that its full-ws
  counterpart is `agents-plugin/skills/mcp-server-repair/SKILL.md` (a skill
  dir, not an rsrc playbook — like the other three).
- `agents-plugin/rsrc/lead-skill-authoring/lead-skill-authoring.md#L79-L98` —
  Skill Layout: since `mcp-server-repair` has no `enter.*`/routing MCP call
  (that is the entire point — it must work with MCP down), it is a
  **Choreography skill**: `Invariants -> On: X handlers -> Judgments ->
  Templates -> Doctrine`, not the thin routing shape.
- Migration-anchor check (`ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`,
  "Entry-skill keep-list" and "Decision: skill surface reduction" sections):
  the pivot's direction is to shrink the skill surface to thin entry shims with
  internal bodies moved into `playbook.print` content. This ticket's own
  Decisions section (`#L165-L169`) already anticipated and overrode that
  general direction for this one skill specifically: "Every other ws skill's
  front door delegates to `ws/playbook.print`; this one cannot, or it dies at
  the moment it is needed" — i.e. an inline, non-`playbook.print` body is a
  deliberate, ticket-sanctioned exception, not a conflict with the pivot
  doctrine. No blocking conflict found; no further pivot-doc action needed for
  this phase.
- Risk signal (needs an implementation-time lookup, not a codebase fact): the
  ticket requires "the manual reconnect procedure to relay to the user" but
  does not specify the literal Claude Code steps (its own Background section
  states this path "is not discoverable" today). No file in this repo documents
  the exact host-side reconnect mechanism (checked `ai-docs/ref/ws-mcp.md`,
  the `260724-bug-windows-mcp-mid-session-disconnect` and
  `260724-bug-ws-mcp-local-devenv-compat-stamp-not-persisting` tickets — none
  give a concrete reconnect UI sequence). This is a narrow factual lookup
  (Claude Code's actual mechanism to reconnect/re-enable a dropped MCP server),
  not a strategy question — see Implementation Plan step 1.

## Implementation Plan

1. **Author `agents-plugin/skills/mcp-server-repair/SKILL.md`** (Choreography
   layout: Invariants -> On: X handlers -> Judgments -> Templates -> Doctrine).
   Frontmatter: `name: mcp-server-repair`, `description:` naming the trigger
   explicitly (tools absent from the tool list, or a tool call failing to
   connect). Body must cover, self-contained, no MCP call anywhere in it:
   - The mapping rule: `ws/x.y(a: b)` -> `` ws-cli call x.y '{"a":"b"}' ``.
   - `ws-cli tools` (mapping rule + name/description list), `ws-cli tools <name>`
     (schema), `ws-cli call <name> '<json>'` (invoke).
   - Cold-start sequence, verbatim invocable:
     `` ws-cli call workflow_manual '{"session_key":"obsidian-latch","root":"<abs worktree>"}' ``.
   - PATH-independent fallback-to-the-fallback:
     `` python3 <plugin-root>/bin/ws-mcp-launcher.py tools ``.
   - Note: a stale runtime may make the first call slow because repair runs —
     this is expected, not a failure.
   - "For the user" section: concrete manual reconnect steps. **Before writing
     this section**, resolve the current Claude Code mechanism to reconnect or
     re-enable a dropped MCP server (e.g. consult the `claude-code-guide`
     subagent or current Claude Code docs) — this is not documented anywhere in
     this repo today, so do not guess or invent a UI sequence.
   - Before finalizing: grep the drafted file, lowercased, for the literal
     substring `ws.` (e.g. `tr 'A-Z' 'a-z' < SKILL.md | grep -o 'ws\.'`) and
     confirm zero matches — this includes ordinary words like "shows.",
     "flows.", "windows.", "workflows." ending a sentence. Also confirm no
     occurrence of `mercenary`, `<!-- ws:full-only:`, `<!-- ws:wsflow-only:`,
     `lead-write-code`, `lead-write-skeleton`, `lead-salvage`,
     `lead-skill-authoring`.

2. **Register for substitution mirroring**: append `"mcp-server-repair"` to
   `substitutionMirroredSkills` in
   `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L15-L18`.

3. **Generate the wsflow mirror**:
   `cd agents-plugin-tool && WS_REGEN_WSFLOW_SKILLS=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowSkillsMirror`.
   This creates `agents-plugin-wsflow/skills/mcp-server-repair/SKILL.md` with
   `ws:`/`ws/`/`ws-cli` substituted to `wsflow:`/`wsflow/`/`wsflow-cli`; do not
   hand-edit it afterward.

4. **Edit the eight front doors** with a single consistent pointer sentence
   (recommended wording, may be adjusted slightly but keep it one line and
   keep the wsflow four consistent with each other so step 6's regex work is
   simple):
   - ws-side (append after the existing final line, no other change), all four:
     `` `agents-plugin/skills/{lead-discuss,lead-sprint,lead-revive,lead-proceed}/SKILL.md` ``
     append: `If this call fails to connect, run `/ws:mcp-server-repair`.`
   - wsflow-side `lead-discuss`, `lead-sprint`: append the same sentence with
     the wsflow namespace: `If this call fails to connect, run `/wsflow:mcp-server-repair`.`
   - wsflow-side `lead-proceed`: **replace** the final two lines
     (`"If the playbook cannot be loaded, stop\nand report that blocker.\n"`)
     with the single pointer sentence above (wsflow namespace).
   - wsflow-side `lead-revive`: **replace** the final line
     (`"If the tool cannot be loaded, stop and report that blocker.\n"`) with
     the same pointer sentence.
   - Verify none of the wsflow four now contain `ws/`, `ws:`, `ws.`, `subquery`,
     or `agents.` per `FORBIDDEN_PATTERNS` — the recommended wording above
     contains none of these.

5. **Regenerate the skills manifest**:
   `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
   — do this after steps 1-4 so it picks up the new file and the four edited
   ws-side front doors in one pass. Confirm the diff only touches
   `agents-plugin/skills/manifest.json` (never
   `agents-plugin/rsrc/manifest.json` — that path belongs to the concurrent
   session, see Concurrency Guard).

6. **Update `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`**:
   - Add `"mcp-server-repair"` to `EXPECTED_SKILLS` (`#L16-L38`) and to
     `EXPECTED_INLINE_SKILLS` (`#L41-L46`).
   - In `test_skill_files_are_thin_playbook_shims` (`#L104-L122`): exclude
     `"lead-proceed"` from the main loop's skill set (e.g. subtract a new
     `{"lead-proceed"}` alongside the existing subtractions), then add a
     dedicated fullmatch check for `lead-proceed` whose expected tail is the
     pointer sentence instead of the "stop and report that blocker" sentence.
     Keep the original strict regex, unmodified, for the remaining 13 skills.
   - In `test_parallel_init_skill_files_are_playbook_shims` (`#L124-L144`):
     make the expected trailing text conditional on skill name — `lead-discuss`
     and `lead-sprint` require the pointer sentence appended after the existing
     final line; `lead-goal-fan-out-step` keeps the current no-suffix
     requirement, unmodified. Prefer an explicit per-skill exact tail (built
     from a small dict/set keyed by skill name) over a loosely optional regex
     group — an optional match would silently accept a *missing* pointer line
     for `lead-discuss`/`lead-sprint`, weakening the drift guard exactly where
     this phase adds a real invariant.
   - `test_full_skill_inventory_drift_is_visible` and
     `test_skill_files_do_not_reference_full_ws_agent_surface` need no manual
     change; they already generalize over the updated sets/files.

7. **Update `ai-docs/ref/wsflow-mirroring.md`**:
   - Add `mcp-server-repair` to the "Shipped wsflow Skills" Included list
     (`#L29-L52`), with a short exception note following the existing
     `lead-prefer-subagent`/`lead-verify-discussion`/`lead-goal-step` pattern
     (`#L75-L83`): inline body, substitution-generated, no shared rsrc
     playbook, full-ws counterpart is the `agents-plugin/skills/mcp-server-repair`
     skill dir.
   - Add `mcp-server-repair` to the curated list in "Substitution-Mirrored
     Skill Generation" (`#L131-L135`).

8. **Append a Phase 3 `### Result (<short-hash>) - 2026-07-25` section** to
   `ai-docs/tickets/ready/260725-feat-ws-cli-mcp-fallback-surface.md` following
   the Phase 1/2 Result convention (what changed, verification run, deviations,
   review outcome if reviewed). Because the `ai-docs/_index.md` refresh is
   skipped this round (Concurrency Guard), explicitly note the new
   `mcp-server-repair` skill's existence and both invocation forms
   (`/ws:mcp-server-repair`, `/wsflow:mcp-server-repair`) in this Result text so
   the addition is discoverable from the ticket alone until `_index.md` next
   gets a normal refresh.

9. **Commit with explicit pathspecs only** (never `git add -A`/`git add .`):
   the new/edited files from steps 1-8 plus the ticket file from step 8. Do not
   stage `agents-plugin{,-wsflow}/rsrc/lead-write-ticket/lead-write-ticket.md`,
   either `rsrc/manifest.json`, or `ai-docs/_index.md`.

## Verification Plan

- `cd agents-plugin-tool && go build ./... && go vet ./...` — clean.
- `cd agents-plugin-tool && go test ./internal/wsrsrc/... ./cmd/ws-mcp/...` —
  green, including `TestWsflowSkillsMirrorUpToDate` (now covering
  `mcp-server-repair`) and `TestSkillsManifestDriftIsVisible`.
- `python3 -m unittest discover agents-plugin-wsflow/tests` (from repo root) —
  green, including the two updated exact-match tests and the inventory tests.
- Manual/host-level (not a local gate, record result when next run): with the
  ws MCP server stopped, confirm the harness still lists `mcp-server-repair`
  as an invocable skill, and that reading its body requires no tool call.
- `diff` review: confirm each of the eight front-door files changed by exactly
  the pointer line (plus, for the two wsflow cases, removal of the replaced
  sentence) — no unrelated reflow.
- `git status` immediately before commit: confirm the five concurrent-session
  files are still absent from the staged set.

## Escalations

- None.
