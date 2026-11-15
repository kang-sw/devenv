---
title: "Launcher proxy tests read the checkout's local-devenv marker and fail only in the dogfood root"
related:
  260724-bug-ws-mcp-local-devenv-compat-stamp-not-persisting: "same agents-plugin/.local-devenv-runtime dogfood marker"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0b0745dfa5e1c050
sage-review-completeness-reviewed: 0b0745dfa5e1c050
---

# Launcher proxy tests read the checkout's local-devenv marker and fail only in the dogfood root

## Background

Observed on 2026-11-15:

`python3 -m unittest tests.test_ws_mcp_launcher_proxy` from `agents-plugin/`
fails 4 subtests of
`LauncherProxyTest.test_all_valid_stdio_flag_shapes_preserve_mcp_ids_and_root`
with `AssertionError: unexpected repair`. It fails only in the main checkout,
which holds the ignored `agents-plugin/.local-devenv-runtime` marker. In clean
pooled worktrees at `00db6cd9a` and `b7ebf2d20`, the same file passes.
A checkout-specific expected red can obscure real proxy-test regressions.

The test loads the launcher with the real `agents-plugin` directory as its
plugin dir. `main()` then calls `local_devenv_runtime_enabled(plugin_dir, ...)`
(`agents-plugin/bin/ws-mcp-launcher.py`, around `main` and
`read_local_devenv_contract`). That validates the checkout marker; for interactive stdio, an active marker
and a compatibility-stamp miss select `repair_runtime(force=True)`, whose
`install_runtime` call `run_main(..., forbid_install=True)` mocks to raise
(`agents-plugin/bin/ws-mcp-launcher.py#L481-L531`,
`agents-plugin/bin/ws-mcp-launcher.py#L1882-L1925`,
`agents-plugin/tests/test_ws_mcp_launcher_proxy.py#L900-L932`).

Separately, `test_cold_stalled_download_reaches_framed_mcp_errors` asserts
completion under 2.0s wall time. That bound is outside this fix.

## Decisions

- MAIN's 2026-11-15 08:10:44Z triage explicitly assigns the small test-only fix: patch `local_devenv_runtime_enabled` in `run_main`, so the checkout's ignored opt-in marker cannot change proxy-test behavior.
- Keep production launcher behavior unchanged. The separate cold-download wall-clock observation is outside this fix; do not relax its bound.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)

## Prior Decisions

- 3d1fe8d9 (2026-11-13, commit): "The marker file stays the only opt-in (lead constraint)." — bearing: constrains
- fa267b5f (2026-11-13, commit): "R1-C1: parse existing Go serve flag shapes, including root-before-stdio, single dash, and assigned booleans; all valid stdio invocations use the live MCP stream, not CLI final framing." — bearing: constrains
- 09b041ed (2026-11-10, commit): "The pre-existing capabilities test leaked os.replace and time.sleep replacements onto the shared modules, which made the new proxy tests fail under discover; scoped with mock.patch.object." — bearing: constrains
- 260724-bug-ws-mcp-local-devenv-compat-stamp-not-persisting (2026-07-26, commit): "Scoped strictly to the marker path so release integrity is unchanged: the default False preserves exact-match everywhere the download/bootstrap paths run." — bearing: constrains
- 260525-bug-local-runtime-contract-marker (2026-05-25, commit): "User corrected that .local-devenv-runtime is intentional local dogfood, not stale state to remove." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | Only agents-plugin/tests/test_ws_mcp_launcher_proxy.py changes; agents-plugin/bin/ws-mcp-launcher.py and agents-plugin/.local-devenv-runtime are evidence, not edit targets (Decisions, Phase 1). |
| scope.surface | internal | Test helper isolation only; production launcher behavior stays unchanged. |
| scope.new_public_symbol | no | None; existing LauncherProxyTest.run_main helper, agents-plugin/tests/test_ws_mcp_launcher_proxy.py#L900-L932. |
| scope.new_type_contract | no | None; predicate mock and focused regression in the existing test module. |
| scope.test_surface | existing | agents-plugin/tests/test_ws_mcp_launcher_proxy.py#L1054-L1072 covers four stdio flag shapes; #L1085-L1103 retains the cold-download timing bound. |
| complexity.reuse_points | confirmed | run_main already uses scoped mock.patch.object; explicit local-devenv True mocks exist in agents-plugin/tests/test_ws_mcp_launcher_proxy.py#L956-L998. |
| complexity.side_effect_risk | low | Test-only scoped mocking; run_main already isolates environment and forbids unapproved subprocess/exec calls (#L900-L932); no production edits authorized. |
| risk.correctness | moderate | A default predicate patch must not silently mask the explicit forced-local repair scenarios already exercising run_main (#L956-L998). |
| risk.fit | low | Existing helper, unittest.mock and temporary-directory patterns suffice (#L337-L380, #L900-L932); no new abstraction required. |
| risk.test | moderate | Regression must establish independence from a real marker without weakening explicit local-devenv tests; the separate 2-second wall-clock assertion remains load-sensitive (#L956-L998, #L1085-L1103). |
| risk.security_or_contract | low | No production API, runtime policy or framing changes; existing ID/root assertions remain required (#L1054-L1072). |

## Phases

### Phase 1: Isolate proxy tests from the checkout marker

Patch the local-devenv predicate in the existing `run_main` test helper and add a focused regression that proves the on-disk marker cannot trigger an unexpected repair. Use existing mock and test patterns, preserve proxy framing and argument assertions, and run the launcher proxy test module offline. Do not change production launcher behavior, perform installs or downloads, or alter the separate timing bound.
