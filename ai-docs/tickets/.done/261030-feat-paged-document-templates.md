---
title: "lead-coauthor-paged-document: reusable document templates (create, find, apply)"
related:
  261030-feat-lead-coauthor-paged-document: extends; supersedes its decision (42) where a template is chosen
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6ad994b123c76b48
sage-review-completeness-reviewed: 6ad994b123c76b48
completed: 2026-10-30
---

# lead-coauthor-paged-document: reusable document templates (create, find, apply)

## Background

`lead-coauthor-paged-document` (shipped by
261030-feat-lead-coauthor-paged-document) lets the model draft each
document's visual look from scratch (that ticket's decision (42)). In
practice the main job of a paged document is a consistent look across
documents: the same company deck format reused across talks, reports, and
projects. The skill therefore needs a way to make a template, a fixed place to
find templates, and a step that applies one.

Template lookup needs no new MCP tool. The skill already loads through
`playbook.read`, so ws-mcp is present whenever it runs; the playbook states a
filesystem convention and the model finds templates with its native file
tools. The global location sits in the existing ws home directory, which
already holds the global config, mailbox, and machine notes
(`agents-plugin-tool/internal/wsconfig/global.go`: `$WS_CONFIG_HOME`, else
`~/.ws`).

The same edit carries a minor release followup from the pre-ship range sweep
(ledger commit 89a371c63, followup id `target-handwrite-zoning`): the
playbook's `On: invoke` ends at the PDF question and never states what the
lead reports at the end, unlike the lead-skill layout in
`ai-docs/manuals/skill-authoring.md` and the sibling `lead-audit-doc`.

## Decisions

Discussion IDs from the authoring conversation are kept in brackets.

- **Two lookup locations [(68)].** Templates are searched in the project at
  `ai-docs/paged-templates/<name>/` and globally at
  `<ws home>/paged-templates/<name>/`. The global location is the common case,
  because one template is usually shared across several projects; the project
  location serves a template that belongs to one project or is shared with the
  team through the repository.
  Rejected: global only (no team sharing through the repository); project only
  (no reuse across projects); a new MCP tool for template lookup (the
  playbook's path convention plus native file tools suffice, and the skill
  already runs with ws-mcp present).
- **The ws home path [(75)].** The playbook names the global location as
  `$WS_CONFIG_HOME/paged-templates/` when that variable is set, otherwise
  `~/.ws/paged-templates/`, matching how the ws tooling resolves its home for
  the global config, mailbox, and machine notes.
  Rejected: a bare `~/.ws/paged-templates/` (a user who relocated the ws home
  with `WS_CONFIG_HOME` would find templates split from the rest of that
  home). The lead checks the variable in its own shell; when the variable is
  set only for the MCP server process and the shell does not see it, the lead
  falls back to `~/.ws/paged-templates/`, an accepted edge case because the
  ticket adds no tool.
- **Name collision [(71)].** When both locations hold a template with the same
  name, the project one wins (the narrower scope, as in git config). Every
  template list shown to the user marks each entry as project or global.
  Rejected: global wins (the project could not override a shared template).
- **Two ways to create a template [(69), (74)].**
  - (a) From a finished document: a new final step of `On: invoke`, after the
    PDF question, asks whether to save the document's look as a template.
    When the document used a template and its look changed, it asks instead
    whether to update that template or save the look under a new name. The
    look changed when the user corrected the shared CSS or a page-kind
    skeleton during the work, or a page kind was added; those corrections are
    recorded in `plan.md` as decisions, so the lead reads the change from
    there.
  - (b) From a reference: a standalone template mode. The user supplies
    reference material the model can read (HTML, PDF, screenshots); the model
    builds sample pages in that look, one per page kind the template will
    offer (at least cover, part divider, and body), the user reviews and
    corrects them, and the result is saved as a template. The samples are
    drafted in a working folder allocated as in the document procedure's
    steps 3 and 4 (folder, commit question, `plan.md`, `document.html`), with
    the same render check and correction recording; the template folder is
    written only at the save step.
- **Template mode entry [(79)].** One skill, one playbook: `On: invoke` routes a
  request to make a template to a separate `On: template` procedure for (b);
  every other request takes the document procedure.
  Rejected: a second skill (doubles the shipped skill surface for one optional
  branch of the same domain).
- **Default save location [(70)].** A new template is saved globally by
  default; it goes to the project location when the user says it is for this
  project only or for team sharing. The save step names the location before
  writing.
- **Committing a project template [(80)].** Saving to the project location
  asks whether to commit the template folder; it is committed only on a yes,
  following the project's commit rules. A global template is outside the
  repository and is never committed.
  Unless the answer is yes, the save writes a `.gitignore` containing `*` in
  the template folder, the same guard as the document folder, so an
  uncommitted template cannot be swept into a later broad commit. Updating an
  already-committed project template asks again before committing the change.
  Rejected: always committing a project template (the material may be
  confidential, the same reason the original ticket's decision (66) gates
  committing the document).
- **Template folder contents [(73)].** One folder per template:
  - `template.html`: the shared CSS and one skeleton per page kind (cover,
    part divider, body, table, and so on); it renders as a sample of the look.
  - `template.md`: the page-kind list and when to use each, the page format
    (16:9 slides or A4), and the source of the look (a finished document or a
    reference).
  The skeletons carry placeholder text only, never the source document's real
  content: a global template travels into other projects, so real text would
  carry possibly confidential material out of its project (the confidentiality
  basis of decisions (66) and (80)).
  Rejected: CSS only (loses the page-kind skeletons that carry most of the
  consistency).
- **Applying a template [(72)].** At step 2 (form), list the templates found in
  both locations and let the user pick one or choose a fresh draft. A chosen
  template's CSS and skeletons are copied into `document.html` when step 4
  creates it, and its name and location are written into `plan.md` at the
  same step, since `plan.md` does not exist yet at step 2. The progress section keeps its fixed
  theme whatever template is chosen.
  Rejected: linking `document.html` to the template file (the document must
  stand alone for PDF baking and when moved).
- **Style rule changes only where a template applies [(77)].** The playbook's
  Style invariant becomes: with a chosen template, follow it; without one, the
  look is the model's own first draft. The progress section stays the one
  fixed exception. The playbook still ships no template of its own.
- **Closing output line [(78)].** `On: invoke` and `On: template` each end with
  what the lead reports: for a document, the working-folder path, whether the
  folder is committed, the template used or saved (with its location), and the
  PDF path when one was baked; for a template, its folder path and whether it
  was committed. The commit that lands this resolves followup
  `target-handwrite-zoning`.
- **Skill description [(82)].** The ws and wsflow skill descriptions gain a
  short clause that the skill also makes a reusable template for such
  documents, so a template-only request routes here. The clause names no path
  and no namespace token: `FORBIDDEN_PATTERNS` in
  `agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py` (`\bws/`, `\bws:`,
  `\bws\.`) scans the wsflow skills tree, where `~/.ws/` would match, and the
  ws and wsflow descriptions stay identical.
  Rejected: leaving the description unchanged (a request to make a template
  names no document and could miss the skill).
- **One phase [(81)].** The playbook edit and its mirrors land together,
  following the original ticket's decision (60).
- **Verification [(76)].** The existing package tests (rsrc manifest, wsflow
  and Pi rsrc byte-identity, playbook rendering) plus a rendered
  `playbook.read` of the playbook in ws and wsflow product modes. The wsflow
  render must carry no `ws/` tool or `ws:` skill reference; the ws-home
  filesystem path in any spelling (`~/.ws/paged-templates/`,
  `$WS_CONFIG_HOME/paged-templates/`) is a real path shared by both products,
  not a product reference, and is exempt from that manual check. The
  automated `FORBIDDEN_PATTERNS` guard scans only the wsflow skills tree, not
  rsrc.
  Rejected: new behavior tests (prose-only change with no new code path).

## Constraints

- Shipped text (Architecture Rule 4, `ai-docs/manuals/shipped-surface-boundary.md`):
  name no template, path, or machine detail that only this repository has.
- Out of scope: a template shipped with the plugin; any MCP tool or Go change;
  any change to the skill's name or shim call shape; any change to the
  progress section's fixed theme.
- The `.done/` ticket 261030-feat-lead-coauthor-paged-document is frozen; this
  ticket records the supersession of its decision (42).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (42)): "Visual style is the model's first call, not the playbook's [(42)]. The playbook ships no HTML template, no color tokens, and no style guidance; the model drafts the look and the user corrects it" — bearing: constrains
- 06c3e6e1 (2026-10-30, commit): "the progress section gets a fixed structure and fixed theme shipped verbatim; this narrows decision (42) only for that section, and the document's own pages stay the model's call." — bearing: constrains
- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (66)): "The lead commits the paged folder only when the user chose \"commit\" at step 3 or asks, following the project's commit rules; a baked PDF is committed only when the user asks" — bearing: supports
- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (63)): "The playbook joins the byte-identical agents-plugin-pi/rsrc/ mirror and its manifest (enforced by TestPiMirrorUpToDate ...); the Pi skill is not written by hand" — bearing: constrains
- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (64)): "The ws, wsflow, and Pi skills call only playbook.read(name: \"lead-coauthor-paged-document\"), like lead-audit-doc and lead-tune; ... uses no session-keyed ws primitive." — bearing: constrains
- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (61)): "A lead playbook the lead executes inline (kind: print, like lead-use-mailbox), since decision (44) keeps the lead as the author." — bearing: supports
- 261030-feat-lead-coauthor-paged-document (2026-10-30, Decisions (62)): "The existing package tests (rsrc and skills manifests, wsflow shim drift, playbook rendering) plus a rendered playbook.read of the new playbook in ws and wsflow product modes. Rejected: new behavior tests" — bearing: supports
- 4b8cfa8d (2026-10-30, commit): "261030-feat-lead-coauthor-paged-document: post-close hotfix, platform-neutral progress-theme fonts" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-coauthor-paged-document/lead-coauthor-paged-document.md, agents-plugin-wsflow/rsrc mirror, agents-plugin-pi/rsrc mirror, three rsrc manifests, agents-plugin/skills/lead-coauthor-paged-document/SKILL.md, agents-plugin-wsflow/skills/lead-coauthor-paged-document/SKILL.md, agents-plugin/skills/manifest.json |
| scope.surface | public-interface | shipped skill descriptions and playbook procedure in ws, wsflow, and Pi packages; no code symbol |
| scope.new_public_symbol | no | none; new On: template procedure is playbook prose, skill name and shim call shape unchanged |
| scope.new_type_contract | no | none; template folder layout is a filesystem convention, no code type |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-tool/internal/wsrsrc/skills_manifest_test.go, agents-plugin/tests, agents-plugin-wsflow/tests |
| complexity.reuse_points | confirmed | wsconfig GlobalPath resolution $WS_CONFIG_HOME else ~/.ws in agents-plugin-tool/internal/wsconfig/global.go; existing playbook steps 2 and 7 and the Style invariant |
| complexity.side_effect_risk | moderate | changes downstream lead behavior: new filesystem locations read and written, new commit question for project templates |
| risk.correctness | moderate | several interacting rules: collision precedence, labeling, default save location, update-vs-new on changed look, closing output lines |
| risk.fit | moderate | Rule 4 shipped-text boundary, wsflow render must stay free of ws/ and ws: references, and the global path must not read as a product reference |
| risk.test | low | prose-only change guarded by existing mirror, manifest, and shim drift tests plus rendered playbook.read check |
| risk.security_or_contract | moderate | skill description change alters routing; templates may hold confidential material, gated by the commit question |

## Phases

### Phase 1: Template lookup, creation, and application in the playbook

Edit `agents-plugin/rsrc/lead-coauthor-paged-document/lead-coauthor-paged-document.md`
to carry every decision above: the Style invariant change, the two lookup
locations with collision and labeling rules, template application at step 2,
the save-as-template question at completion, the `On: template` procedure,
the template folder contents, and the closing output lines. Regenerate the rsrc
manifest, the byte-identical wsflow rsrc mirror, and the Pi rsrc mirror with
its manifest (procedure in `ai-docs/manuals/wsflow-mirroring.md`; the Pi tree
is guarded by `TestPiMirrorUpToDate`). Update the description in the ws skill
`agents-plugin/skills/lead-coauthor-paged-document/SKILL.md` and the wsflow
shim `agents-plugin-wsflow/skills/lead-coauthor-paged-document/SKILL.md`, and
regenerate the skills manifest and any skills mirror those trees require.

Verification: as in the Verification decision. The commit resolves followup
`target-handwrite-zoning` through the `resolves` field of `ws/git.commit`.

### Result (b6cc688ed) - 2026-10-30

Landed in 6ffaaf62a and b6cc688ed. The playbook
`lead-coauthor-paged-document` now carries a `## Templates` section
(location: project `ai-docs/paged-templates/` over the global
`$WS_CONFIG_HOME/paged-templates/`, else `~/.ws/paged-templates/`; collision
precedence and project/global labeling; folder contents with a
placeholder-only rule; save rules with default-global, update-in-place,
project commit question, and `.gitignore` guard). `On: invoke` routes template
requests to a new `On: template` procedure, lists templates at step 2, copies
the chosen template into `document.html` and records it in `plan.md` at step 4,
asks the save / update-or-new question at step 8, and reports at step 9.
`On: template` ends with its own report line. The Style invariant follows a
chosen template; the progress theme stays fixed. The ws and wsflow skill
descriptions gained an identical template clause. rsrc manifest, wsflow rsrc
mirror, Pi rsrc mirror (copied byte-for-byte; no regen entrypoint), and skills
manifest regenerated. Followup `target-handwrite-zoning` resolved by 6ffaaf62a.

Verification: `go test ./internal/wsrsrc/... ./internal/mcp/... -count=1` ok;
`python3 -m unittest discover agents-plugin-wsflow/tests` OK (18);
`python3 -m unittest discover agents-plugin/tests` OK (80); a temporary
`printPlaybook` render of the playbook in ws and wsflow product modes differs
only in the `ws:`/`wsflow:` skill namespace, and the wsflow render has no
`ws/` or `ws:` reference besides the exempt `~/.ws/paged-templates/` path.
Lite review: clean, three minor wording findings fixed in b6cc688ed.

Decisions: step 8 skips the save question when a used template's look did not
change (a literal reading of (69a) would offer to save a duplicate of it); an
update writes the template back to its own location instead of following the
default-global rule for new templates; `On: template` carries no progress
section, since the ticket names only the render check and correction
recording for template mode.
