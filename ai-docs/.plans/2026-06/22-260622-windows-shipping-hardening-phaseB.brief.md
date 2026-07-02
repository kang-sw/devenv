# Brief: 260622-chore-windows-shipping-hardening — Phase B (launcher cold-load robustness)

## Intent

Harden the first-install / first-load paths of the plugin-managed Python launcher
`agents-plugin/bin/ws-mcp-launcher.py` so a cold Windows install does not break on
(a) the rsrc tree not yet being materialized when the launcher derives its env, and
(b) transient file-access stalls (AV scan holds, concurrent installer) during the
runtime-contract read and the runtime-binary replace. Land only clearly-correct,
cheap fixes; empirical confirmation is deferred to Phase C on a real Windows host.

## Scope Boundary

- **In scope:** `agents-plugin/bin/ws-mcp-launcher.py` only, plus new Python unit
  tests under `agents-plugin/tests/`.
- **Phase B item 1:** rsrc-materialization race — wait for the rsrc tree before the
  one-shot `apply_rsrc_root_env` seam decision.
- **Phase B item 2:** cheap robustness — AV/transient tolerance on the runtime-contract
  read; bounded retry on the runtime-binary `os.replace` (keep the existing
  compatible-binary fallback as final recovery).
- **Explicitly OUT of scope (do NOT touch):**
  - `agents-plugin-wsflow/bin/ws-mcp-launcher.py`. The two launchers already
    intentionally diverge (the wsflow copy never received the `260524`
    `wait_for_runtime_contract` fix and lacks the `bootstrap_runtime_forced` refactor).
    wsflow is the non-user-facing agentless derivative and is NOT the Windows shipping
    target of this epic. The lead captures the wsflow gap as a separate follow-up
    ticket; the implementer must not port anything into wsflow.
  - Phase A Go fixes (already landed), Phase C empirical acceptance.
  - Any speculative redesign (e.g. rename-aside / sidelining a locked running `.exe`).
    The realistic Windows running-`.exe` case is already covered (see Strategy); do not
    add an unproven dance. Phase C reveals what actually bites.

## Caller-Visible Contract

None changes. These are behavior-preserving robustness fixes that make the launcher
tolerate timing/contention conditions the cross-platform startup contract already
intends to survive. No new env var, CLI surface, or contract field. On the happy path
(everything already materialized, replace succeeds first try) behavior is byte-for-byte
unchanged: every new wait/retry returns immediately when its success condition already
holds.

## Contract Instructions

File: `agents-plugin/bin/ws-mcp-launcher.py`.

### Item 1 — rsrc materialization wait

- Add a module-level `wait_for_rsrc_tree(plugin_dir: Path, *, timeout_seconds: float = ..., interval_seconds: float = 0.05) -> None` modeled on the existing `wait_for_runtime_contract` (line ~60).
  - Success sentinel: `plugin_dir / "rsrc" / "manifest.json"` is a file. (`manifest.json` ships at the rsrc-tree root and proves the tree is *populated*, not merely an empty dir created early in extraction.)
  - If the sentinel is already present, return immediately (no sleep) — keeps the happy path unchanged.
  - Otherwise poll until `timeout_seconds`; on first appearance, `note(...)` and return.
  - **On timeout: do NOT `fail()`.** Emit a `note(...)` and return. Rationale: `apply_rsrc_root_env` already no-ops gracefully when `<plugin>/rsrc` is absent (falling back to the runtime's derived path); the wait's job is to close the *race*, not to introduce a new hard-fail install-corruption path (scope discipline). This deliberately differs from `wait_for_runtime_contract`, which must fail because the contract is mandatory to even read.
- Call `wait_for_rsrc_tree(plugin_dir)` in `main()` immediately before the existing `apply_rsrc_root_env(plugin_dir, os.environ)` call (line ~780), so the one-shot `is_dir()` check inside `apply_rsrc_root_env` sees a materialized tree.

### Item 2a — runtime-contract read tolerance (AV / transient holds)

- `wait_for_runtime_contract` (line ~60): make `timeout_seconds` OS-aware. Keep the
  current value on non-Windows; use a longer default on Windows (AV scans the
  freshly-extracted package on cold install, delaying file visibility). Detect Windows
  with `os.name == "nt"`. Do not change `interval_seconds`. Preserve the existing
  fail-on-timeout message contract.
- `read_runtime_contract` (line ~72): the file can *exist* yet be momentarily
  unreadable (AV sharing-hold) → `read_text`/`json.loads` raises transiently. Wrap the
  read in a small bounded retry (a few attempts, short backoff) that retries on
  `OSError` (covers `PermissionError`) and on JSON decode errors, then `fail(...)` with
  the existing message shape if still failing. A first-try success must behave exactly
  as today (no added latency).

### Item 2b — runtime-binary replace contention

- `install_tmp_runtime` (line ~377): wrap the `os.replace(tmp, binary)` in a bounded
  retry on `OSError` (sharing violation from a concurrent installer or a briefly-locked
  target), mirroring the Phase A Go-side `MoveFileEx` bounded retry (≈5 attempts, ~10ms
  exponential backoff). Harmless on POSIX (where `os.rename`/`os.replace` does not raise
  the sharing error, so the retry never triggers). Only after the retry budget is
  exhausted, fall through to the **existing** compatible-binary fallback
  (`runtime_fully_compatible(binary, ...)` → reuse, `return False`) and finally `fail`.
  Do not remove or weaken the existing fallback. Do not add rename-aside logic.

## Integration Test Instructions

New tests in `agents-plugin/tests/` (extend `test_ws_mcp_launcher_capabilities.py` or
add a sibling `test_ws_mcp_launcher_coldload.py`; match the existing
`load_launcher()` importlib + `unittest.mock` style):

1. `wait_for_rsrc_tree`:
   - returns immediately when `<plugin>/rsrc/manifest.json` already exists;
   - with a tiny `timeout_seconds`, returns (does NOT raise) when the sentinel never
     appears;
   - returns once the sentinel appears mid-wait (you may pre-create it to keep the test
     deterministic rather than threading).
2. `read_runtime_contract` retry: monkeypatch `Path.read_text` (or the file) to raise
   `PermissionError` once/twice then succeed → contract is returned, no `SystemExit`.
   Also assert a persistently-unreadable file still `fail`s (SystemExit).
3. `install_tmp_runtime` replace retry: monkeypatch `os.replace` to raise `OSError`
   once/twice then succeed → returns `True` (installed), no `SystemExit`. Keep/confirm
   an existing-or-added test that a persistent `OSError` with an incompatible existing
   binary still `fail`s, and with a compatible existing binary reuses it (`return False`).

Run: `python3 -m unittest discover agents-plugin/tests`

Also sanity-run `python3 -m py_compile agents-plugin/bin/ws-mcp-launcher.py` and
`python3 -m unittest discover agents-plugin-wsflow/tests` to confirm no incidental
breakage (you are not editing wsflow, so the latter must stay green untouched).

## Implementation Strategy Decisions

- **Canonical launcher only.** Settled: do not edit the wsflow launcher; the divergence
  is pre-existing and the lead owns the follow-up ticket.
- **rsrc wait is best-effort (note, not fail) on timeout.** Settled per scope discipline.
- **Item 2b is replace-retry + keep existing compatible fallback.** The realistic
  Windows running-`.exe` case is the *same-version reinstall* race: binaries are
  installed under version+hash-stamped names (`runtime_binary_name`), so an upgrade
  targets a *new* filename (not the running one); the only locked-target case is a
  concurrent reinstall of the identical versioned binary, where the existing target is
  byte-compatible and the existing fallback already reuses it. The retry absorbs the
  brief concurrent-installer window. No rename-aside.
- **Happy-path invariance.** Every added wait/retry must short-circuit when its success
  condition already holds, so steady-state launches incur zero added latency.

## Rejected Alternatives

- Porting Phase B (and back-porting `260524`) into the wsflow launcher — rejected:
  expands scope beyond the Windows shipping target; wsflow divergence is pre-existing
  and captured as a follow-up.
- Hard-fail when the rsrc tree never materializes — rejected: introduces a new
  unproven fail path; `apply_rsrc_root_env`'s graceful no-op is preserved.
- Rename-aside / unlink-then-replace of a running `.exe` — rejected for Phase B:
  unreachable under version-stamped binary names; speculative. Defer to Phase C iff
  observed.

## Approach

- Add `wait_for_rsrc_tree`; call it before `apply_rsrc_root_env` in `main()`.
- Make `wait_for_runtime_contract` timeout OS-aware; add bounded read retry in
  `read_runtime_contract`.
- Add bounded `os.replace` retry in `install_tmp_runtime` ahead of the existing
  compatible-binary fallback.
- Add the three unit-test groups.

## Constraints

- Edit only `agents-plugin/bin/ws-mcp-launcher.py` and `agents-plugin/tests/`.
- No caller-visible contract change; happy path unchanged and zero added steady-state
  latency.
- Preserve the existing `install_tmp_runtime` compatible-binary fallback and all
  existing `note`/`fail` message contracts.
- **Live-host safety (hard, inherited):** this slice adds no process termination; do not
  introduce any. (No `taskkill`, no image-name kill, nothing process-killing at all.)

## Out of scope

- wsflow launcher; Phase A Go code; Phase C empirical acceptance; rename-aside recovery;
  any new env var/CLI/contract field.

## Details

- `wait_for_runtime_contract(path, *, timeout_seconds=<os-aware>, interval_seconds=0.05)`.
- `wait_for_rsrc_tree(plugin_dir, *, timeout_seconds=<os-aware or fixed>, interval_seconds=0.05)`; sentinel `plugin_dir/"rsrc"/"manifest.json"`.
- `read_runtime_contract`: bounded retry on `OSError`/JSON errors, then `fail`.
- `install_tmp_runtime`: bounded `os.replace` retry on `OSError`, then existing
  `runtime_fully_compatible` reuse, then `fail`.

## Verification Contract

- `python3 -m unittest discover agents-plugin/tests` green (new tests included).
- `python3 -m py_compile agents-plugin/bin/ws-mcp-launcher.py` clean.
- `python3 -m unittest discover agents-plugin-wsflow/tests` green (untouched).
- Report the exact commands run and their output.

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `agents-plugin/bin/ws-mcp-launcher.py` — [Must] the only file to edit; functions named above.
- `agents-plugin/tests/test_ws_mcp_launcher_capabilities.py` — [Must] test style: `load_launcher()` importlib + `unittest.mock`.
- `ai-docs/mental-model/plugin-runtime.md` — [Must] rsrc tree / runtime.json contract / launcher repair model (read its `index.md` ancestor first if present).
- `ai-docs/ref/wsflow-mirroring.md` — [Maybe] why the wsflow launcher is out of scope (the divergence + follow-up-ticket allowance).
- `ai-docs/.plans/2026-06/22-260622-chore-windows-shipping-hardening.brief.md` — [Maybe] Phase A brief; the bounded `MoveFileEx` retry that item 2b's replace-retry parallels.
