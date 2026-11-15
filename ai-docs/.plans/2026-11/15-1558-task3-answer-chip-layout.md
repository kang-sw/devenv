# TASK3 answered-row chip layout follow-up

## Approved outcome

MAIN at 06:57:47 relayed owner rejection1007 of TASK3 visuals and explicitly authorized a fast answer-chip layout fix: implement, scoped tests plus full wsi18n, head/hash; no screenshot matrices. Long answered policy text currently squeezes into a narrow right-hand chip (~12 lines), making the whole history row tall/mostly empty and its expand arrow mid-height. Let the answer use useful available width with natural flow; preserve complete meaningful content rather than solving this by deleting or clamping answers. This is T0 presentation, no store/API/schema or answer semantic change.

## Input and scope

- Baseline e8f5500c0 on fix/exec3-task3-answer-chip-layout in this checkout only. Do not touch the active research P2 checkout decree-thinness-strum or merge develop yourself.
- Owner evidence: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/130568195e3ac446-1.png, -2.png and -3.png. Read first two to understand chip failure; third concerns a separate open-question design question, not authorization to redesign those controls.
- Production scope: answered-history row template/CSS under agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl and the existing split page-*.css owner (discover exact selector), plus affected existing render/style tests. Prefer a small CSS-only change if it solves the actual mechanism. Do not refactor unrelated layout, stream selection or dashboard lifecycle.
- Read applicable manuals and the closed TASK3 ticket/approved density reference as evidence, but do not edit .done tickets or the curated references. Preserve whole-entry content, options/reasons/quotes, per-item controls, time/class identity, narrow natural wrapping and existing question controls. Open-item redesign is not this assignment.
- If you find a confirmed self-triggered history rerender loop while tracing the relevant DOM/CSS seam, immediately report evidence to the lead; EXEC5 owns independent reload slowness/spinner diagnosis. Do not claim a loop fixed without a repro, or expand into performance work.

## Execution limits and verification

Offline only. No providers/network/downloads/credentials, shared-daemon changes, builds/install/reload/push/process signals, screenshot/browser/harness runs, matrices or new review gates. Raw evidence under /tmp. Run focused render/style tests that prove long-answer available-width behavior/structure and existing controls preserved, then full wsi18n and diff checks. Report known baseline reds honestly against this exact baseline; do not change thresholds or bless failures. Lead reads focused diff only, not duplicate raw logs.

## Commit and handoff

Commit this plan and minimal implementation/test changes with ## AI Context via ws/git.commit on the named branch. No branch integration or native merge/commit fallback authorization. Return exact head, commands/exit statuses, useful VERIFIED/INFERRED evidence with file:line citations, focused diff artifact and any remaining risks. Settle clean and quiescent for lead lease release; MAIN owns integration and runtime activation.
