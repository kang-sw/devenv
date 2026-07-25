# Plan: 260710-bug-project-index-ticket-focus-stale-status — Phase 1: remove Ticket Focus and its machinery across ws, wsflow, specs, and the bundled convention

## Relevant Ticket Contract
- Decision: remove `## Ticket Focus` entirely; retire every surface that reads,
  writes, or maintains it; rely on filesystem-backed discovery (`tickets.list`,
  `project_tree`, the status directories) plus each ticket's own body.
- Preserve immutable migration history (the v0041/v0004 "Ticket Queue ->
  Ticket Focus" notes) — supersede with a **new** template-version entry, never
  rewrite the old one.
- Keep the ws (`agents-plugin/`) and wsflow (`agents-plugin-wsflow/`) trees
  mirror-consistent; the mirror byte-contract test is the gate.
- Deferred / out of scope per ticket: no replacement pointer list, no new
  frontmatter field; this phase does not change `related:`/`parent:` semantics
  or the `lead-goal-step` selection logic.
- Managed-template propagation: add a new `AGENTS.template.md` version entry
  (ws next after v0041 = v0042; wsflow next after v0004 = v0005) so managed
  `AGENTS.md`/`WORKFLOW.md` consumers regenerate without the section. This repo
  is itself such a consumer (currently v0041) — after the bump, regenerate its
  own managed `AGENTS.md`/`WORKFLOW.md` through the bootstrap upgrade path, not
  a hand-edit.
- Verification boundary (from ticket): repo-wide `grep -ri 'ticket focus'`
  returns only immutable migration-history entries; `ws/convention.read(name:
  "ticket-conventions")` no longer names the section; a fresh
  `lead-write-ticket`/`executor-wrapup` render carries no focus step; wsflow
  rsrc mirror test green; rsrc + skills manifest tests green; `go build ./...`,
  `go vet ./...`, `internal/mcp` + `cmd/ws-mcp` + `internal/wsrsrc` suites
  green; wsflow python bundle suite green; bootstrap/AGENTS template-version
  test green after the bump.
- Spec Impact: `ai-docs/spec/documentation-system.md` removes the `## Ticket
  Focus` description from the `_index.md` structure/lifecycle prose, stating
  active attention is discovered from the status directories via
  `tickets.list`/`project_tree`, not a cached section. `ai-docs/spec/workflow-skills.md`
  removes the "`Ticket Focus` entries are maintained…" clause and any
  writer/cleaner behavior contract referencing it.
- Constraints: mirror byte-contract is the gate; do not rewrite immutable
  history; no new frontmatter field or replacement section; the
  downstream-facing managed-template change was already approved in
  discussion (2026-07-25).

## Out of Scope
- Running `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>` — note it as
  a required step, but the bump itself is a separate dev-merge action, not part
  of this plan's execution.
- `CHANGELOG.md:522` ("Replace ticket queue project memory with ticket
  focus…") — immutable release history, do not touch; it is an expected
  survivor of the verification grep-sweep.
- The `v0041` (ws) / `v0004` (wsflow) `AGENTS.template.md` migration-checklist
  bullets describing the historical Ticket Queue → Ticket Focus replacement —
  immutable, superseded by a new entry, never rewritten.
- `ai-docs/tickets/todo/260713-workset-workflow-dogfood-bugs.md:20` — describes
  this very bug as a dogfood observation; historical/descriptive text, not live
  machinery; leave untouched (expected grep-sweep survivor).
- This ticket's own body (`ai-docs/tickets/ready/260710-...md`) — necessarily
  names "Ticket Focus" throughout; expected grep-sweep survivor, not an edit
  target.
- `related:`/`parent:` frontmatter semantics and the `lead-goal-step` selection
  logic that consumes them — ticket explicitly states this phase does not
  change them.
- Any new frontmatter field or replacement pointer/attention list — rejected
  alternative per the ticket's Decision section.
- Any phase beyond Phase 1.

## Codebase Findings
- `ai-docs/_index.md#L210-L541` — `## Ticket Focus` heading is at line 211; the
  section runs through line 540; `## Session Notes` starts immediately at line
  541 with no blank-line gap. Delete lines 211-540 verbatim; the existing blank
  line 210 remains as the separator before `## Session Notes`.
- `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md#L99-L107` — "On:
  Spec-address Check": step 4 (line 106) ends "...do not move to `ready/` or
  add a `Ticket Focus` entry; restore pre-invocation edits..." (drop only the
  "or add a `Ticket Focus` entry" clause, keep the rest); step 5 (line 107) is
  the writer in full ("ensure `ai-docs/_index.md ## Ticket Focus` carries
  ...") — delete the entire step. Line 108 is a blank line, then "## On:
  Output Handoff" — step 5 is the section's last item, no renumbering needed.
- `agents-plugin/rsrc/executor-wrapup.md#L48` — numbered step 4, "Remove
  completed tickets from the `## Ticket Focus` section in `ai-docs/_index.md`."
  — delete; it is the last item in its list (steps 1-4), no renumbering
  needed.
- `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md#L102` — "On: user
  approves index cleanup" step 3: "Keep summary, stack, workspace, build/test
  commands, read-before-edit pointers, active inventory, `Ticket Focus`, and
  compact notes." — drop `, Ticket Focus` from the list.
- **Mirror mechanism (reuse, do not hand-duplicate)**:
  `agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go` (`TestWsflowRsrcMirrorUpToDate`
  / `TestRegenerateWsflowRsrcMirror`) proves `agents-plugin-wsflow/rsrc/` must
  be a **byte-identical** copy of `agents-plugin/rsrc/` (confirmed via `diff`:
  `agents-plugin/rsrc/manifest.json` and the wsflow copy are currently
  byte-identical too). Do **not** hand-edit
  `agents-plugin-wsflow/rsrc/executor-wrapup.md`,
  `.../lead-write-ticket/lead-write-ticket.md`, or
  `.../lead-bootstrap/lead-bootstrap.md` — edit only the three canonical
  `agents-plugin/rsrc/...` files above, then regenerate the mirror.
- `agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go#L17-L106` —
  `TestShippedManifestUpToDate` / `TestRegenerateShippedManifest` require
  `agents-plugin/rsrc/manifest.json` regen after any rsrc edit via
  `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
  (run before the wsflow mirror regen below, since that regen copies the whole
  tree including the refreshed manifest.json; `GenerateManifest` excludes
  `manifest.json` itself from hashing, so ordering is safe either way but
  manifest-first matches the ticket's stated mechanics).
- `agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go#L14-L18` —
  `substitutionMirroredSkills` lists only `lead-goal-step`,
  `lead-prefer-subagent`, `lead-verify-discussion`; `lead-bootstrap` is **not**
  substitution-mirrored. `agents-plugin/skills/lead-bootstrap/{AGENTS.template.md,WORKFLOW.md}`
  and their `agents-plugin-wsflow` counterparts must each be hand-edited
  independently — no auto-mirror covers these two files.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L79` —
  reader-instruction line "Check `## Ticket Focus` in `ai-docs/_index.md`
  before starting implementation…" — delete.
- `agents-plugin/skills/lead-bootstrap/AGENTS.template.md#L178-L185` — the
  `v0041` migration-checklist entry (historical Ticket Queue → Ticket Focus
  replacement) — immutable, do not rewrite; append a new `v0042` entry after it
  instructing bootstrap to drop the reader-instruction line on upgrade and not
  re-add the section.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L80` — same
  reader-instruction line, wsflow copy — delete.
- `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md#L152-L159` —
  the `v0004` entry (immutable) — append a matching new `v0005` entry.
- `agents-plugin/skills/lead-bootstrap/WORKFLOW.md#L47-L49,#L107,#L120-L121` —
  three references: `## Tickets` semantics bullet ("`_index.md` `## Ticket
  Focus` lists selected active attention items…", lines 47-49 — remove the
  whole bullet, it has no independent value once the section is gone); keep-list
  mention inside the index-cleanup numbered step (line 107 — drop just
  `` `Ticket Focus`, `` from the list, keep the rest of the step); routing-map
  bullet (line 120 — drop just `Ticket Focus membership, ordering, and` /
  adjust the sentence so it still routes readiness/status wording to the
  lead-write-ticket procedure without naming the section).
- `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md#L47-L49,#L107,#L120` —
  identical structure and line numbers in the wsflow copy (uses `wsflow:`
  namespace prefixes instead of `ws:`); same three edits.
- `agents-plugin/skills/manifest.json#L5-L7` — hashes
  `lead-bootstrap/AGENTS.template.md` (line 5) and `lead-bootstrap/WORKFLOW.md`
  (line 7); editing either requires
  `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`
  (per `skills_manifest_test.go#L27-L44`). `agents-plugin-wsflow/skills/` has
  **no** parallel manifest.json (confirmed absent) — no regen needed there.
- `ai-docs/spec/documentation-system.md#L18,#L117-L118,#L248` — three prose
  references: index-content list ("…active ticket list, ticket focus, and
  compact session notes." — drop "ticket focus, "); ticket-document-system
  paragraph ("`## Ticket Focus` lists selected active attention items; only
  `ready/` entries are direct implementation targets." — replace with wording
  stating discovery is via `tickets.list`/`project_tree` over the status
  directories, not a cached section); lead-write-ticket behavior contract
  ("updates `## Ticket Focus` for selected active attention items," — delete
  clause, keep the rest of the sentence).
- `ai-docs/spec/workflow-skills.md#L325-L326` — "`Ticket Focus` entries are
  maintained for selected active attention items; only `ready/` entries are
  direct implementation targets." — delete sentence; surrounding spec-address-gate
  contract text stays.
- `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md#L18` —
  "`ai-docs/_index.md ## Ticket Focus` is the selected active attention list;
  only `ready/` entries are direct implementation targets." — this is the
  Go-embedded (`go:embed`, see `agents-plugin-tool/internal/wsdoc/conventions.go`)
  source that `ws/convention.read(name: "ticket-conventions")` serves live; no
  hash/manifest gate found for this specific file (confirmed via grep across
  `*_test.go`), so a plain source edit + `go build`/`go vet` covers it. Replace
  with filesystem-discovery wording consistent with the spec edit above.
- `AGENTS.md` (root, generated, devenv's own template consumer)#L199 —
  reader-instruction line; `AGENTS.md#L224` — `<!-- Template Version: v0041
  -->` marker. **Do not hand-edit either** — per ticket, regenerate through the
  bootstrap upgrade path after the template bump, so the tag advances to
  `v0042` and the reader line is dropped by the new migration-checklist entry
  (same mechanism as any downstream consumer).
- `ai-docs/WORKFLOW.md` (root, generated)#L47,#L107,#L120-L121 — same three
  references as the template's `WORKFLOW.md` (confirmed byte-for-byte the
  rendered form of `agents-plugin/skills/lead-bootstrap/WORKFLOW.md` with
  `{{.SkillNamespace}}`→`ws` substitution); also regenerated via the bootstrap
  upgrade path, not hand-edited.
- `CHANGELOG.md#L522` — immutable release-history line; left as-is; this is
  one of the entries the ticket's verification grep-sweep expects to still
  match.
- Risk signal: the verification boundary "grep returns only immutable
  migration-history entries" is narrower in the ticket's own wording than
  reality — the sweep also legitimately matches
  `ai-docs/tickets/todo/260713-workset-workflow-dogfood-bugs.md:20` and this
  ticket's own body (`260710-...md`), neither of which is a "version note" but
  both of which are expected, non-machinery survivors. The executor should
  treat any survivor under `ai-docs/tickets/**` (any status), `CHANGELOG.md`,
  or a `v0041`/`v0004`-and-earlier `AGENTS.template.md` migration bullet as
  expected; anything else surviving the sweep is a miss requiring a follow-up
  edit.

## Implementation Plan
1. Delete `ai-docs/_index.md` lines 211-540 (`## Ticket Focus` through its last
   entry), keeping the existing blank line 210 and the following `##
   Session Notes` heading unchanged.
2. Edit `agents-plugin/rsrc/lead-write-ticket/lead-write-ticket.md`: in step 4
   (line 106) drop the "or add a `Ticket Focus` entry" clause, keeping the rest
   of the sentence; delete step 5 (line 107) entirely.
3. Edit `agents-plugin/rsrc/executor-wrapup.md`: delete numbered step 4 (line
   48).
4. Edit `agents-plugin/rsrc/lead-bootstrap/lead-bootstrap.md`: in "On: user
   approves index cleanup" step 3 (line 102), drop `, Ticket Focus` from the
   keep-list.
5. Regenerate the rsrc manifest and the wsflow rsrc mirror from steps 2-4, in
   this order, from `agents-plugin-tool/`:
   - `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateShippedManifest`
   - `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`
   Do not hand-edit `agents-plugin-wsflow/rsrc/...`; this regen is the only
   mutation path for that tree.
6. Edit `agents-plugin/skills/lead-bootstrap/AGENTS.template.md`: delete the
   reader-instruction line (line 79); append a new `v0042` migration-checklist
   entry after the existing `v0041` entry (~line 185) instructing bootstrap to
   drop the Ticket Focus reader-instruction line on upgrade and not re-add the
   section — do not edit the `v0041` entry text itself.
7. Edit `agents-plugin-wsflow/skills/lead-bootstrap/AGENTS.template.md`: delete
   the reader-instruction line (line 80); append a matching new `v0005` entry
   after the existing `v0004` entry (~line 159).
8. Edit `agents-plugin/skills/lead-bootstrap/WORKFLOW.md`: remove the `##
   Tickets` semantics bullet (lines 47-49); drop the `Ticket Focus` mention
   from the index-cleanup keep-list (line 107) and from the routing-map bullet
   (line 120), preserving the rest of each bullet's wording.
9. Edit `agents-plugin-wsflow/skills/lead-bootstrap/WORKFLOW.md` with the same
   three edits (confirm exact wording at edit time — `wsflow:` namespace
   prefixes apply).
10. Regenerate `agents-plugin/skills/manifest.json` after steps 6 and 8:
    `cd agents-plugin-tool && WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/... -run TestGenerateRealSkillsManifest -v`.
    No parallel regen for `agents-plugin-wsflow/skills/` (no manifest.json
    there).
11. Edit `ai-docs/spec/documentation-system.md`: drop "ticket focus, " from the
    index-content list (line 18); replace the `## Ticket Focus` sentence
    (lines 117-118) with filesystem-discovery wording (`tickets.list` /
    `project_tree` over the status directories); delete the "updates `##
    Ticket Focus` for selected active attention items," clause from the
    lead-write-ticket behavior paragraph (line 248).
12. Edit `ai-docs/spec/workflow-skills.md`: delete the "`Ticket Focus` entries
    are maintained…" sentence (lines 325-326).
13. Edit `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`:
    replace line 18 with filesystem-discovery wording consistent with step 11
    (no section name).
14. Bump the plugin version per the standard dev-merge rule — **note only, do
    not run as part of this plan**: `agents-plugin-tool/scripts/bump-ws-version.sh <X.Y.Z>`
    is the single bump surface (both `plugin.json` pairs, both `runtime.json`,
    `main.go`, release assets, `_index.md`); this happens as a separate
    dev-merge step.
15. After the version bump (separate step), regenerate this repo's own managed
    `AGENTS.md`/`WORKFLOW.md` through the bootstrap upgrade path (invoke the
    `ws:lead-bootstrap` skill against this repo) so the `v0041`→`v0042`
    migration applies the new checklist entry: drops the reader-instruction
    line and advances the `<!-- Template Version: v0041 -->` tag at
    `AGENTS.md:224` to `v0042` — do not hand-edit `AGENTS.md` or
    `ai-docs/WORKFLOW.md` directly.

## Verification Plan
- `grep -rin 'ticket focus' /home/swkang/devenv --include='*.md' --include='*.go' --include='*.json' | grep -v '/\.worktree/'`
  — expect only: `CHANGELOG.md` (release history), the `v0041`/`v0004`
  migration-checklist bullets in both `AGENTS.template.md` files, and
  `ai-docs/tickets/**/260713-workset-workflow-dogfood-bugs.md` +
  `260710-bug-project-index-ticket-focus-stale-status.md` (ticket bodies
  describing the bug). Any other survivor is a miss.
- `grep -rin 'ticket focus' agents-plugin agents-plugin-wsflow agents-plugin-tool ai-docs/spec ai-docs/_index.md AGENTS.md ai-docs/WORKFLOW.md`
  — narrower sweep over exactly the surfaces this phase owns; expect zero hits
  outside the two immutable version-checklist bullets.
- `cd agents-plugin-tool && go build ./... && go vet ./...`
- `cd agents-plugin-tool && go test ./internal/mcp/... ./cmd/ws-mcp/... ./internal/wsrsrc/...`
  — covers `TestWsflowRsrcMirrorUpToDate`, `TestShippedManifestUpToDate`,
  `TestSkillsManifestDriftIsVisible`, and the bootstrap/template-version tests
  in `internal/mcp/bootstrap_alarm_test.go`.
- `cd agents-plugin-wsflow && python3 -m pytest tests/` — covers
  `test_wsflow_skill_bundle.py` / `test_wsflow_runtime_contract.py`.
- Manual: render `ws/convention.read(name: "ticket-conventions")` and confirm
  the output no longer names `## Ticket Focus`.
- Manual: `ws/playbook.print(name: "lead-write-ticket")` and a fresh
  `executor-wrapup` render carry no Ticket Focus writer/cleaner step.
- Manual (after step 15's bootstrap-upgrade regen): confirm `AGENTS.md:224`
  reads `<!-- Template Version: v0042 -->`, the reader-instruction line near
  line 199 is gone, and `ai-docs/WORKFLOW.md` no longer carries the three
  semantics/keep-list/routing-map mentions.

## Escalations
- None.
