---
related:
  261108-bug-ws-git-merge-directory-rename-misplaces-tickets: "predecessor: gave ws/git.merge the same option (bcc267066)"
title: "The parallel-mode merge preview runs git merge-tree with directory-rename detection on, reporting a false conflict ws/git.merge does not have"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2051e9bb7ecc9cf0
sage-review-completeness-reviewed: 2051e9bb7ecc9cf0
---

# Parallel-mode merge preview reports a false directory-rename conflict

## Background

Observed 2026-11-10 (twice in one devenv lead session): before merging an
impl branch that moved its ticket `ready/` -> `.done/` (leaving `ready/`
empty on that side) while `develop` had meanwhile added another ticket under
`ready/`, the lead's preview reported

```text
CONFLICT (file location): ai-docs/tickets/ready/<new-ticket>.md added in develop
inside a directory that was renamed in impl/..., suggesting it should perhaps
be moved to ai-docs/tickets/.done/<new-ticket>.md.
```

Corrected finding (this ticket was first filed as a `ws/git.merge` bug):
`ws/git.merge` already merges with `-c merge.directoryRenames=false`
(`agents-plugin-tool/internal/mcp/git_merge.go:341-344`, commit bcc267066,
2026-11-08). The false conflict came from the preview command the lead runs
first, which the shipped workflow manual prescribes for parallel mode:
"first diff the target against the tree `git merge-tree --write-tree
<target> <branch>` prints" (`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:197`
and the wsflow and Pi copies at the same line,
`agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md:197`,
`agents-plugin-pi/rsrc/lead-workflow-manual/lead-workflow-manual.md:197`). Plain `git merge-tree` keeps
directory-rename detection on, so the preview disagrees with the merge the
lead is about to make, and a lead that trusts the preview abandons
`ws/git.merge` for a hand-made native merge. Parallel mode hits this
whenever a worker closes the last ticket in a status directory while the lead
promotes another into it, which is the normal parallel shape.

## Decisions

1. **The manual's preview command passes the same option `ws/git.merge`
   uses: `git -c merge.directoryRenames=false merge-tree --write-tree
   <target> <branch>`,** with a short clause saying why (it previews the
   merge `ws/git.merge` makes, which disables directory-rename detection).
   Rejected: a dry-run mode on `ws/git.merge` (new tool surface for a
   one-option mismatch); leaving the command and documenting the false
   conflict (every lead would still be misled first).

## Constraints

- All three copies (`agents-plugin/rsrc/...`, `agents-plugin-wsflow/rsrc/...`,
  and `agents-plugin-pi/rsrc/...`, which `TestPiMirrorUpToDate` requires to be
  byte-identical to `agents-plugin/rsrc/`:
  `agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go#L20-L30`)
  change identically; read `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/wsflow-mirroring.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md` first.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Any golden or prose-pin test over the manual body is updated in the same
  commit.

## Prior Decisions

- 261108-bug-ws-git-merge-directory-rename-misplaces-tickets (2026-11-08, commit bcc26706): "Moving the last file out of a near-empty ready/ looks like a ready/ -> .done/ directory rename to git ... Ticket status moves are always explicit, so the heuristic never helps." — bearing: supports
- bcc26706 (2026-11-08, commit): "git_merge.go has a single merge site; no merge-tree/preflight call exists in the tool or internal/wsgit, so nothing else needed the flag." — bearing: supports
- bcc26706 (2026-11-08, commit): "Rejected `-X no-renames`: it also disables file rename detection, which real merges rely on." — bearing: constrains
- 261030-feat-workflow-manual-worktree-work-mode (2026-10-30, commit 137158e1): "Generic rules (worktree.acquire per unit, release before merge or on an unmerged end, serial merges, merge-tree interaction check, conflicts to lead-delegate) live in the manual" — bearing: constrains
- 5617ff93 (2026-10-30, commit): "git merge-tree --write-tree chosen as the merge preview after the user pointed at a dry-run mode; git.merge has no dry-run parameter." — bearing: supports
- 261030-feat-note-layer-discipline-and-worker-readonly (2026-10-31, Result a893f696): "The manual's Notes paragraph was aligned (rsrc manifest regenerated, wsflow and pi mirrors synced)" — bearing: constrains
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, commit 4ad1e70c): "Merged natively with merge.directoryRenames=false: the branch emptied ai-docs/tickets/ready/ ... ort's directory-rename detection would have moved the newly-ready ... into .done/ as a conflict" — bearing: supports
- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, commit d31ced27): "a delegate merged develop in with -X no-renames (5695e68a4) ... avoiding git's false directory-rename of the two new ready/ tickets into .done/." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin-pi/rsrc/lead-workflow-manual/lead-workflow-manual.md, plus regenerated rsrc manifest.json files |
| scope.surface | public-interface | shipped lead-workflow-manual prose read by downstream leads; no code symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, agents-plugin-tool/internal/wsrsrc/manifest_shipped_test.go, agents-plugin-wsflow/tests; git grep merge-tree outside ai-docs finds only the three manuals, so no prose pin on this sentence |
| complexity.reuse_points | confirmed | the merge.directoryRenames=false option at agents-plugin-tool/internal/mcp/git_merge.go#L341-L344 |
| complexity.side_effect_risk | low | one-sentence prose change in a mirrored manual; manifests regenerate mechanically |
| risk.correctness | low | the flag is an existing git config key already used by ws/git.merge |
| risk.fit | low | aligns the preview with the merge ws/git.merge performs, per recorded rationale |
| risk.test | low | mirror and manifest drift tests catch a missed copy or manifest regen |
| risk.security_or_contract | moderate | changes shipped downstream workflow guidance in three packages that must stay byte-identical |

## Phases

### Phase 1: align the preview command

Edit the parallel-mode paragraph in all three manual copies (ws, wsflow, Pi;
see Constraints) per Decision 1, then run the rsrc after-edit checklist in
`ai-docs/manuals/wsflow-mirroring.md` (regenerate `agents-plugin/rsrc/manifest.json`,
sync the wsflow mirror, and regenerate the Pi mirror so `TestPiMirrorUpToDate`
passes). Verification: `go test ./...` under `agents-plugin-tool` (rsrc and render
goldens) and the wsflow package tests (`python3 -m pytest agents-plugin-wsflow/tests`); a manual check that
`git -c merge.directoryRenames=false merge-tree --write-tree` on a scratch
repo reproducing the empty-directory shape prints no conflict while the plain
form does.
