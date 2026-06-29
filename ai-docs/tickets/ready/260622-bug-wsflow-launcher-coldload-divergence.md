---
title: wsflow launcher diverges from canonical — missing cold-load hardening
sage-review: skipped
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-chore-windows-shipping-hardening: surfaced this divergence; Phase B hardened the canonical launcher only
  260523-bug-ws-mcp-launcher-runtime-repair-race: adjacent launcher runtime-repair race investigation
related-mental-model:
  - plugin-runtime
---

# wsflow launcher diverges from canonical — missing cold-load hardening

## Problem

`agents-plugin-wsflow/bin/ws-mcp-launcher.py` has drifted behind the canonical
`agents-plugin/bin/ws-mcp-launcher.py`. Discovered during `260622` Phase B: the
wsflow copy

- never received the `260524` `wait_for_runtime_contract` materialization wait
  (it still `fail`s immediately if `runtime.json` is absent), and
- did not receive the `260622` Phase B cold-load hardening:
  `wait_for_rsrc_tree`, OS-aware contract-read timeout, `read_runtime_contract`
  `(OSError, ValueError)` retry, and the `install_tmp_runtime` `os.replace`
  bounded retry.

The launchers are curated (not byte-identical, unlike the generated rsrc subtree),
so this is real divergence, not a generation gap.

## Why deferred

Phase B was scoped to the Windows **shipping** target, which is the full ws
plugin (`agents-plugin/`). wsflow is the non-user-facing agentless derivative and
is not part of the epic's Windows shipping gate, so porting the fixes into wsflow
would have expanded Phase B scope. The `wsflow-mirroring.md` reference explicitly
permits a follow-up ticket when a launcher behavior change cannot be mirrored in
the same logical change.

## Open questions (research before acting)

- Is the wsflow launcher ever run on Windows / cold-install paths in practice, or
  only in agentless/Linux contexts? If it has no Windows cold-load exposure, the
  divergence may be acceptable to document rather than fix.
- If parity is wanted: port the `260524` wait + the three Phase B robustness
  fixes, keeping wsflow text non-ws-aware, and add/extend
  `agents-plugin-wsflow/tests` coverage.
- Should the two launchers be unified behind a shared module (with a render-time
  or import-time product transform) to stop future drift, or kept curated with an
  explicit divergence checklist?

## Notes

- Canonical Phase B reference commits: `ab1460d4`, `da1047fb`.
- No caller-visible contract change is involved; these are robustness/conformance
  fixes.

## Spec Impact

Target spec area: none — robustness fixes with no caller-visible behavior change.
Contract-first spec: no

## Phases

### Phase 1: Verify wsflow launcher parity and close

Port the `260524` `wait_for_runtime_contract` materialization wait and the four
`260622` Phase B cold-load hardening fixes (`wait_for_rsrc_tree`, OS-aware
contract-read timeout, `read_runtime_contract (OSError, ValueError)` retry,
`install_tmp_runtime os.replace` bounded retry) into the wsflow launcher if not
already present. Verify byte-level parity is not required — the launchers are
curated — but functional parity for the listed fixes is the target.

Completion boundary: wsflow launcher has all five robustness fixes; diff against
canonical is zero for the named functions.
Deferred: launcher unification behind a shared module (open question); wsflow test
coverage extension (open question).
Verification: `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py` exits 0.
