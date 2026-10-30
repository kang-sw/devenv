---
title: "lead-coauthor-paged-document: co-author a human-facing paged document with the user, page by page, to PDF"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 54f3b7497690ecf9
sage-review-design-reviewed: 54f3b7497690ecf9
completed: 2026-10-30
---

# lead-coauthor-paged-document: co-author a human-facing paged document with the user, page by page, to PDF

## Background

A training deck (`ai-docs/presentation/261029-wsflow-advanced-seminar.*`,
gitignored, not in this repository's history) was built in one long lead
session with a working method the user wants to reuse as a shipped skill:
settle the topic and the form first, keep a plan file as the authority, build
the document from the front one or two sections at a time, render-check every
page, take the user's review, record each correction as a decision, and bake a
PDF at the end. The method survived several compactions because the plan file,
not the conversation, held the state.

The skill packages that method for any downstream project. Its output is a
document people read (a slide deck or an A4 report), not code and not a
working document an AI rereads; `lead-audit-doc` keeps that second domain.

## Decisions

Discussion IDs from the authoring conversation are kept in brackets.

- **Name and placement [(54), (58)].** The skill is
  `lead-coauthor-paged-document`, shipped in both `ws` and `wsflow` (the wsflow
  copy is the uniform thin shim over the shared playbook, like every other
  lead skill except the deliberately inline `lead-revive`
  (`agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py`
  `EXPECTED_INLINE_SKILLS`, 260626-bug-wsflow-lead-revive-skill-inventory-drift))
  [(36)]. The skill description leads with "co-author a human-facing paged
  document (slide deck or A4 report) with the user, page by
  page, and output a PDF", and states that code, AGENTS.md, manuals, and other
  AI-reread working documents route to `lead-audit-doc`.
  Rejected: `lead-create-presentation` (too narrow once A4 reports joined the
  scope); `lead-deck` (a deck is only one of the two forms); `lead-paged-doc`,
  `lead-print-doc`, `lead-showcase-doc` (the user found none conveyed
  co-authoring; `showcase` also reads as promotional); `lead-coauthor`
  (blends into coding contexts); `lead-coauthor-document` and
  `lead-coauthor-human-readable-document` (`-doc` would collide in domain with
  `lead-audit-doc`; "human-readable" did not land with the user);
  `lead-coauthor-report` (narrower than training material).
- **Procedure.** The playbook walks these steps; the user decides at each
  confirmation point:
  1. Discuss the topic: audience, purpose, length or talk time.
  2. Settle the form: slide deck (16:9 landscape) or A4 document (portrait)
     [(49)]; the structural elements (cover, part dividers, appendix, and so
     on); text density; any reference document the user supplies. Confirm
     before step 3.
  3. Allocate the working folder, default `ai-docs/paged/<stem>/` [(52)],
     where `<stem>` follows the ticket stem form `YYMMDD-<slug>` [(37)].
     Ask the user explicitly whether they want the folder committed, then
     create the folder; unless the answer is "commit", write a `.gitignore`
     inside it whose content is `*` [(47)]. The default is not committed, and
     a "commit" answer means no `.gitignore` is written [(53)]. A later change
     of mind is handled by the user asking to delete that `.gitignore`.
  4. Create `plan.md` and `document.html` in the folder [(55), (65)].
  5. Discuss the overall order and direction; once the user confirms it,
     record it in `plan.md`.
  6. After the user confirms, build the document from the front, one or two
     sections per round: render-check each new page, stop for the user's
     review, correct, and repeat [(45)].
  7. Keep the progress section (below) current as the document's last part.
  8. When the document is complete, ask whether to bake a PDF, and where and
     under what file name [(56)].
- **One plan file; the folder `.gitignore` alone controls tracking [(47)(a),
  (65)].** The plan is the single `plan.md`, beside `document.html`; both
  share one fate, ignored by the folder's `*` `.gitignore` by default and both
  tracked when the user chose "commit" at step 3. The plan file is the
  authority for the outline, confirmed decisions with their `(nn)` IDs, the
  page-number map, and the review-pending list; after compaction the lead
  rereads it before resuming [(39)].
  Rejected: `.local` suffixes (`plan.local.md`, `document.local.html`) as a
  second guard (a downstream root `.gitignore` from lead-bootstrap ignores
  `ai-docs/**/*.local.md`, so under a "commit" answer the plan would silently
  stay untracked while the document is committed; nothing ignores
  `*.local.html`, so the guard was partial anyway); names that switch with the
  commit answer, keeping only the plan local under "commit", or force-adding
  the plan (each lets something other than the folder `.gitignore` decide
  tracking); a two-stage local draft promoted to a tracked `plan.md` (the plan
  carries the same possibly confidential content as the document); a
  project-root `.gitignore` pattern or a `git check-ignore` probe [(40)]
  (the folder-local `*` needs no root edit and no check).
- **Visual style is the model's first call, not the playbook's [(42)].** The
  playbook ships no HTML template, no color tokens, and no style guidance; the
  model drafts the look and the user corrects it after the first page. The
  playbook guides only the mechanics below [(50)].
  Rejected: shipping the seminar deck's skeleton as a template include.
- **Mechanics the playbook guides [(50)].**
  - Each page is one fixed-size block: 1280×720 px for slides, 210×297 mm for
    A4. A print `@page` rule uses the same size, so the PDF matches the screen.
  - A `?s=N` query renders only page N, for render checks.
  - Render check: find an available headless browser (Chrome, Edge,
    Chromium, or Playwright), screenshot each new page, and inspect it for
    overflow and collisions; with no browser available, tell the user the
    page is unverified and ask them to look [(41)].
  - Page numbers are assigned automatically in document order; reviews refer
    to pages by the on-screen number [(45)].
  - CSS classes for a page's own layout carry a per-page prefix, so they do
    not collide with shared classes [(45)].
  - The progress section [(43)]: the document's last part, not formatted as a
    page and allowed to be long; it shows the outline with per-item status
    (done / under review / not started), the page-number map, the
    review-pending list, and open questions, regenerated from the plan file
    after each round; CSS hides it in print and in presentation mode, so the
    finished PDF never carries it.
  - Presentation mode (F/Enter to start, arrow keys, Esc) is included for the
    slide form only.
  - PDF: print through the headless browser; with none available, instruct
    the user to use the browser's Print → Save as PDF, which the `@page` rule
    makes faithful [(41)].
- **Output is PDF only [(48)].** Rejected: `.pptx` export (no layout fidelity
  guarantee).
- **The lead writes the document itself [(44)].** The draft → review →
  correct loop is the point of the skill, and delegating pages to a worker
  would break it; the Explore agent may gather source material.
- **Working practices the playbook states [(45)].** One or two sections per
  round, then stop for review; every user correction becomes a recorded
  decision in the plan file; the form discussion (step 2) also asks for talk
  time and density.
- **Playbook kind [(61)].** A lead playbook the lead executes inline
  (`kind: print`, like `lead-use-mailbox`), since decision (44) keeps the lead
  as the author. Rejected: a worker or delegate playbook (no worker procedure
  exists).
- **One phase [(60)].** The playbook, ws skill, wsflow shim, manifests, and
  tests land together. Rejected: several phases (the parts have no sequential
  dependency and review as one unit).
- **Verification [(62)].** The existing package tests (rsrc and skills
  manifests, wsflow shim drift, playbook rendering) plus a rendered
  `playbook.read` of the new playbook in ws and wsflow product modes.
  Rejected: new behavior tests (the change is skill text with no new code
  path).
- **Commit timing [(66)].** The lead commits the paged folder only when the
  user chose "commit" at step 3 or asks, following the project's commit rules;
  a baked PDF is committed only when the user asks, wherever it is written,
  including inside a committed folder.
  Rejected: leaving commit timing unstated (the lead could commit possibly
  confidential material on its own).
- **Placement [(59)].** Standalone `feat` ticket with no parent; no epic owns
  lead-skill additions.
- **Pi package ships it too [(63)].** The playbook joins the byte-identical
  `agents-plugin-pi/rsrc/` mirror and its manifest (enforced by
  `TestPiMirrorUpToDate` in
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`); that mirror is the
  only tracked Pi change. The Pi skill is not written by hand:
  `agents-plugin-pi/skills/` is gitignored and generated at pack time by
  `agents-plugin-pi/scripts/copy-skills.mjs` from `agents-plugin/skills/`,
  which also checks playbook targets against the Pi rsrc manifest.
- **Single-call shim [(64)].** The ws, wsflow, and Pi skills call only
  `playbook.read(name: "lead-coauthor-paged-document")`, like lead-audit-doc
  and lead-tune; the procedure keeps its state in the plan file and uses no
  session-keyed ws primitive. The hardcoded skill lists in
  `agents-plugin/tests/test_skill_dispatch_contracts.py` and
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` are updated to
  match. Rejected: the lead-discuss two-call form with `workflow_manual`.

## Constraints

- The playbook is shipped text: it must not name anything only this
  repository has, such as this machine's Playwright cache path or the seminar
  deck (Architecture Rule 4, `ai-docs/manuals/shipped-surface-boundary.md`).
- Apply `ai-docs/manuals/skill-authoring.md` to the new skill and playbook,
  and `ai-docs/manuals/wsflow-mirroring.md` for the wsflow shim and its
  shipped-skill list.
- The rsrc loader reads only `.md` files (`internal/wsrsrc/loader.go`); the
  playbook is Markdown only, which the no-template decision already implies.
- Out of scope: `.pptx` output, a shipped visual template, any change to
  `lead-audit-doc`, and any change to `lead-bootstrap`'s root `.gitignore`
  entries.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 260913-feat-cross-session-mailbox-wake (2026-09-13, Result): "Manifests and drift guards: regenerated agents-plugin/rsrc/manifest.json, agents-plugin/skills/manifest.json, and the byte-identical agents-plugin-wsflow/rsrc/manifest.json via WSRSRC_REGEN/WSRSRC_REGEN_SKILLS/WS_REGEN_WSFLOW_RSRC" — bearing: supports
- 261023-bug-claude-mod-sentinel-key-mailbox-push (2026-10-23, Decisions): "The ws, wsflow, and pi lead-use-mailbox skill shims take each package's lead-discuss two-call parallel form: playbook.read together with workflow_manual(...)" — bearing: constrains
- 261020-chore-retire-mcp-server-repair-skill (2026-10-20, Result): "the wsflow generic shim test excludes lead-use-mailbox and checks it separately (renamed POINTER_TAIL_TITLES to SESSION_KEY_SHIM_TITLES)" — bearing: constrains
- 260917-feat-mailbox-wait-rearm-nudge-and-concrete-command (2026-09-18, Result): "lead-use-mailbox's SKILL.md (ws + pi byte-identical; wsflow hand-edited per its curated-shim rule)" — bearing: constrains
- 260619-feat-ws-lead-tune-skill (2026-06-20, Result): "Added ws:lead-tune as the 13th user-invocable lead entry skill across both surfaces: thin-shim SKILL.md and the kind:print umbrella manual agents-plugin/rsrc/lead-tune/lead-tune.md" — bearing: supports
- 260616-refactor-wsflow-product-mode-convergence (2026-06-17, Result): "Collapsed all shipped agents-plugin-wsflow/skills/lead-* SKILL.md files to thin entry shims ... preserve package-local bare name: lead-* frontmatter" — bearing: supports
- 260626-bug-wsflow-lead-revive-skill-inventory-drift (2026-06-26, Resolution): "lead-revive is a deliberate inline-body wsflow skill, not a thin playbook.print shim, because revive bootstraps the workflow primitives after compaction" — bearing: constrains
- 09de73e8 (2026-05-13, commit): "Generated presentation PDFs should remain local build artifacts, so .gitignore now covers ai-docs/presentation/*.pdf." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-coauthor-paged-document/, agents-plugin/skills/lead-coauthor-paged-document/SKILL.md, agents-plugin-wsflow/skills/lead-coauthor-paged-document/SKILL.md, agents-plugin/tests/test_skill_dispatch_contracts.py, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, ai-docs/manuals/wsflow-mirroring.md, agents-plugin/rsrc/manifest.json, agents-plugin/skills/manifest.json, agents-plugin-wsflow/rsrc/ mirror and manifest, agents-plugin-pi/rsrc/ mirror required by TestPiMirrorUpToDate in agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go |
| scope.surface | public-interface | new user-invocable skill ws:lead-coauthor-paged-document and wsflow:lead-coauthor-paged-document plus a new playbook.read name |
| scope.new_public_symbol | yes | skill and playbook name lead-coauthor-paged-document |
| scope.new_type_contract | no | no new MCP tool, schema, or Go signature; kind print playbook reuses the existing playbook.read contract |
| scope.test_surface | existing | agents-plugin/tests/test_skill_dispatch_contracts.py and agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py hardcode lead skill lists; agents-plugin-tool/internal/wsrsrc/skills_mirror_test.go and pi_mirror_test.go are drift guards |
| complexity.reuse_points | confirmed | playbook shape: lead-use-mailbox kind print playbook at agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md; shim shape: the single-call lead-audit-doc and lead-tune SKILL.md shims (not lead-use-mailbox's two-call shims) |
| complexity.side_effect_risk | low | adds new skill text and regenerated manifests; no existing code path changes |
| risk.correctness | low | prose-only playbook with no new code path; correctness is the procedure text matching Decisions |
| risk.fit | moderate | skill description routing against lead-audit-doc; shim shape and Pi package placement settled in Decisions |
| risk.test | moderate | two Python hardcoded skill lists, the wsflow skills mirror guard, and the Pi rsrc byte-identity guard must all be updated |
| risk.security_or_contract | moderate | shipped downstream text under Architecture Rule 4 and a new public skill name in both products |

## Phases

### Phase 1: Ship lead-coauthor-paged-document in ws, wsflow, and Pi

Add the shared playbook under `agents-plugin/rsrc/lead-coauthor-paged-document/`,
the ws skill entry under `agents-plugin/skills/`, the wsflow thin shim under
`agents-plugin-wsflow/skills/`, and the Pi rsrc mirror under
`agents-plugin-pi/rsrc/`, with every manifest, inventory, and generated copy
those trees require (the regenerated rsrc, skills, wsflow, and Pi manifests;
the hardcoded skill lists in the two Python test files; the
`## Shipped wsflow Skills` list in `ai-docs/manuals/wsflow-mirroring.md`), so the skill is invocable in both products and
renders with each product's own namespace. The playbook carries the procedure,
plan-file authority, mechanics, and working practices recorded in
`## Decisions`, and nothing about visual style.

Verification: the package test suites that cover the rsrc and skills
manifests, the wsflow shim, the Pi mirror (`TestPiMirrorUpToDate`), and
playbook rendering pass; `playbook.read` of the
new playbook renders cleanly in ws and in wsflow product mode, with wsflow
output carrying no `ws/` tool or `ws:` skill references.

### Result (4a3677a73) - 2026-10-30

Landed the kind:print playbook
`agents-plugin/rsrc/lead-coauthor-paged-document/lead-coauthor-paged-document.md`
(procedure steps 1-7 with the progress-section upkeep folded into step 6 and
the Mechanics section; plan-file authority; folder-`.gitignore`-only tracking;
commit timing; page, progress, render-check, and PDF mechanics; no visual
style), the single-call ws and wsflow skill shims, regenerated rsrc and skills
manifests, byte-identical wsflow and Pi rsrc mirrors, both hardcoded Python
skill inventories, and the wsflow-mirroring shipped-skill list.

Verification:
- `python3 -m unittest discover agents-plugin/tests`: 80 tests OK.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 18 tests OK.
- `go test ./... -count=1` in agents-plugin-tool: all 17 packages ok
  (includes TestPiMirrorUpToDate, TestWsflowRsrcMirrorUpToDate,
  TestSkillsManifestDriftIsVisible).
- `printPlaybook` render via a scratch (uncommitted) test, ws and wsflow
  product modes on claude and codex harnesses: renders cleanly; ws emits
  `ws:lead-audit-doc`, wsflow emits `wsflow:lead-audit-doc` and carries no
  `ws/`, `ws:`, or `ws.` token.
- Lite review: clean; one optional minor (talk time is asked in step 1 rather
  than step 2) left as is, since the ticket's step 1 already names talk time.

Decisions:
- The playbook declares the `ExploreAgent` variable but not `delegates: true`;
  the lead authors the document, so the delegate continuity tip does not fit.
- The skill description names bare `lead-audit-doc` so ws and wsflow share one
  namespace-free description.

## Sage Review Round 1 (2026-10-30)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | A commit answer silently leaves plan.local.md untracked downstream (bootstrap root ignores ai-docs/**/*.local.md); .local suffix rationale partial | important | missing |
| 2 | Pi skill copy is a generated gitignored pack-time artifact; only the Pi rsrc mirror is tracked | minor | autonomous |
| 3 | Inventory surfaces beyond the named test lists (wsflow-mirroring shipped list, regen env vars) | minor | autonomous |
| 4 | When the lead commits the folder or PDF is not stated | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Phase title omits the Pi package | minor |
| 2 | Route Facts scope.span does not list every target file | minor |
| 3 | Reuse point names lead-use-mailbox shims which use the rejected two-call form | minor |
| 4 | Step 3 ask-then-write order of .gitignore is loose | minor |
| 5 | wsflow free-of-ws-names check wording open | minor |
| 6 | Stray formatting in name-and-placement decision | minor |
