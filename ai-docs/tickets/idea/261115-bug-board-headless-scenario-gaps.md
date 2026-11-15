---
title: "board-headless: adapter history deep-link and shared-controls scenario gaps"
---

# board-headless: adapter history deep-link and shared-controls scenario gaps

## Background

MAIN found these gaps in `scripts/board-headless.mjs` while verifying the
808 r2 merge (`2f08f5171`) on 2026-11-15. This is a static capture relayed by
mail. No run was made for this capture.

1. **Adapter `history` deep-link.** The `history` scenario on the ADAPTER page
   fails with "deep-link target not revealed in the bottom-anchored clip".
   The develop baseline `1512bfad4` fails the same way, so the failure
   predates 808.
2. **`shared-controls` needs an exited terminal card.** The scenario times out
   at a step that needs an already-exited terminal card. `serve` never creates
   one, so the scenario never exercises its fold-independence check.

Side notes:

- `serve` shutdown always exits 1 with `ENOTEMPTY`. HOME is set to the temp
  directory, so `go build` drops its read-only module cache there, and the
  cleanup cannot remove it.
- Combining `--shared-actions` with `--history-entries` makes the
  open-question pill cover the summary.

## Open Questions

- Is (1) a harness clip/anchor expectation bug or a real adapter-page reveal
  defect? Bisect against the baseline before choosing a fix.
- Should `serve` seed an exited terminal card for (2), or should the scenario
  build its own fixture?
- For the `ENOTEMPTY` exit: use a separate GOMODCACHE/GOCACHE outside the temp
  HOME, or chmod the cache before removal?
