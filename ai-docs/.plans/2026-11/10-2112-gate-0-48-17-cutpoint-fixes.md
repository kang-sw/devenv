# Release-gate fix round: cut-point release procedure (ws 0.48.17)

## Request

The ws 0.48.17 release-gate review of fd82c26de..6ddde41cb blocked. On branch
`fix/gate-0.48.17-cutpoint`, fix every item under "Unit B" in the findings file:
- CR-C1, CR-I1, CR-I2 and CR-I3;
- the two "Minor, fix in this round" items: ws.md pre-gate push, and whiteboard
  `fields` implying `full`.

Unit A (the proxy) is out of scope; another delegate fixes it in parallel.

Findings file, the contract for this round:
`/home/swkang/.cache/ws@kang-sw-devenv/proj/dac18b1d/review-paths/fc07fd75-01-findings.md`

Authority for intended behavior:
- `ai-docs/tickets/.done/261110-feat-ws-ship-cut-point-release.md` (D1-D9, plus
  the worktree decisions: the shipped lead-ship never creates worktrees, the
  config decides; this repo's ws.md uses a `ws/worktree.acquire` full worktree).
- For the whiteboard item: `ai-docs/tickets/.done/261110-bug-ws-whiteboard-query-output-noise.md`.

Files in scope:
- `agents-plugin/rsrc/lead-ship/lead-ship.md` and
  `agents-plugin/rsrc/lead-review/lead-review.md`;
- their wsflow and Pi mirrors and the three `rsrc/manifest.json` files;
- `ai-docs/ship/ws.md`;
- `agents-plugin-tool/internal/wswhiteboard/query.go`, its tests, and the
  whiteboard playbook copies, if its text changes.

## Lead decisions (settled; do not reopen)

- CR-C1:
  - A cut point is cleared only by a `pass` or `concern` stamp that this gate's
    own review wrote, which includes a clean-prefix stamp.
  - A `bootstrap` entry written during the gate never counts as cleared.
  - On a non-clearing verdict with no prefix stamped, the gate stops for the
    user's explicit override, the same as step 4. Restore that stop for the
    step-3 review path.
- CR-I1:
  - lead-ship stays generic. It says that the lead itself runs any config step
    that needs a lead-only capability; for this config those are worktree
    acquire/release and branch merges. The delegate runs the mechanical steps.
  - ws.md marks which of its release-branch steps are lead-run.
- CR-I2: make the merge back conflict-safe without touching the shared main
  checkout mid-merge. The expected shape:
  1. In the release worktree, merge `develop` into the release branch and
     resolve any conflict there. The version-bump lines are the usual
     conflicts; resolve them in favor of the higher version, and rerun the
     release-contract check.
  2. Then merge the release branch into develop, which is now conflict-free.

  The D9 merge resolves its conflicts in the release worktree the same way. A
  conflict that is not mechanical goes to lead-delegate, never into a stall
  that every restart repeats.
- CR-I3: one rule. Every stop after Cut releases the release worktree. The Cut
  step also detects and clears a leftover lease or branch from an earlier
  attempt, including `release/pending-<sha>`, before acquiring.
- Pre-flight: no remote push or delete before the final gate. The remote branch
  delete for a restart moves after the gate, or becomes a force-push of the
  recreated branch at push time; pick the simpler one.
- Whiteboard: when `fields` is given and `depth` is not, the depth is `full`.

## Constraints

- Before editing, read:
  - `ai-docs/manuals/skill-authoring.md`, and apply its invariant checklist to
    every changed line;
  - `ai-docs/manuals/shipped-surface-boundary.md`: lead-ship and lead-review are
    shipped, so they must not name this repo's paths, tickets or tools beyond
    the generic MCP namespace templates;
  - `ai-docs/manuals/wsflow-mirroring.md`.
- The ws, wsflow and Pi copies stay byte-identical, and the manifest hashes
  match. `ws-rgo` does not bring generated outputs back, so compute the hashes
  locally and let the drift tests confirm them.
- Never run go build/test/vet locally. Use
  `~/.local/bin/ws-rgo -C /home/swkang/.ws-worktrees/devenv/gamma-washcloth-everyone/agents-plugin-tool -m 30 -- <go args>`.
  Python tests under `agents-plugin/tests` run locally with `nice -n 19`.
- No `git stash`. Do not push. Do not merge. Do not touch the dashboard daemons
  on ports 29417 and 45743.

## Verification

- Remote:
  - `go test ./internal/wsrsrc/... ./internal/wsreview/... ./internal/wswhiteboard/... ./internal/mcp/ -count=1`
    passes, including the playbook golden, mirror and manifest tests, after you
    update the golden pins;
  - `go vet ./...`.
- Local: `test_skill_dispatch_contracts.py`, `test_shipped_surfaces_downstream_neutral.py`,
  and `agents-plugin-wsflow/tests`.
- A hand dry run in a scratch repo:
  - the found:false + review + block case now stops;
  - a red pre-flight stop followed by a restart at the same cut succeeds;
  - a merge back with a version-line conflict against develop resolves in the
    release worktree, with the shared checkout untouched.
- A test for the whiteboard `fields` default.

## Return

Report:
- each item: fixed, or recorded with the reason;
- the commits;
- the verification output;
- anything left open.

Stop and return before any change that would alter a ticket decision beyond the
lead decisions above.
