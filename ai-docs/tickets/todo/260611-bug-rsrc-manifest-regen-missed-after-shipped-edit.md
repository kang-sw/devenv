---
title: rsrc manifest regen missed after a shipped-file edit slips past verification
related:
  260609-refactor-ws-spawn-runtime-deletion-session-auth: surfaced during Phase 2c baseline verification
---

# rsrc manifest regen missed after a shipped-file edit slips past verification

## Problem

Phase 2b commit `bb2d3558` edited a shipped rsrc file
(`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`) in the doc
pre-pass but did not regenerate `agents-plugin/rsrc/manifest.json`. The stale
sha256 left `TestPlaybookPrintGoldenLeadWorkflowManual` and `TestValidateRealTree`
red at the branch tip; the 2b "green" claim predated the late doc-pre-pass edit,
so the regression was committed undetected and only caught at the Phase 2c
baseline run (fixed in `a21241e6`).

## Why it slipped

- The manifest regen step (`WSRSRC_REGEN=1 go test -run TestGenerateRealManifest`)
  is a separate manual action with no enforcement at edit time.
- The doc pre-pass / `lead-update-spec` / shipped-rsrc edit paths do not prompt
  for regen, and a final `-count=1` suite was apparently not re-run after that
  specific late edit.

## Possible follow-ups

- A pre-commit or `ws/git.commit` guard: when a staged path is under
  `agents-plugin/rsrc/**` (excluding `manifest.json`), refuse the commit unless
  `manifest.json` is also staged and current.
- Make the doc pipeline / executor-wrapup explicitly regenerate the manifest when
  any shipped rsrc file changed.
- Document the regen requirement in the rsrc edit guidance so editors cannot miss it.

## Notes

Low severity (caught by the tree-sync gate on the next full run), but it means a
red tree can be committed and a "green" claim made if the suite is not re-run
after the final edit. The real lesson: re-run the full `-count=1` suite after the
LAST edit of a phase, not mid-phase.
