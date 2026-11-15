---
title: "Launcher proxy tests read the checkout's local-devenv marker and fail only in the dogfood root"
related:
  260724-bug-ws-mcp-local-devenv-compat-stamp-not-persisting: "same agents-plugin/.local-devenv-runtime dogfood marker"
---

# Launcher proxy tests read the checkout's local-devenv marker and fail only in the dogfood root

## Observation (2026-11-15)

`python3 -m unittest tests.test_ws_mcp_launcher_proxy` from `agents-plugin/`
fails 4 subtests of
`LauncherProxyTest.test_all_valid_stdio_flag_shapes_preserve_mcp_ids_and_root`
with `AssertionError: unexpected repair`. It fails only in the main checkout,
which holds the ignored `agents-plugin/.local-devenv-runtime` marker. In clean
pooled worktrees at `00db6cd9a` and `b7ebf2d20`, the same file passes.
This dogfood setup is the usual way the suite is run, so a real
regression in these subtests would be hidden by the expected red.

The test loads the launcher with the real `agents-plugin` directory as its
plugin dir. `main()` then calls `local_devenv_runtime_enabled(plugin_dir, ...)`
(`agents-plugin/bin/ws-mcp-launcher.py`, around `main` and
`read_local_devenv_contract`). That reads the marker from the checkout and forces
`repair_runtime(force=True)`, which `run_main(..., forbid_install=True)`
mocks to raise (`agents-plugin/tests/test_ws_mcp_launcher_proxy.py`,
`run_main`).

Separately, `test_cold_stalled_download_reaches_framed_mcp_errors` asserts
completion under 2.0s wall time and failed once at 2.05s under a parallel
load. It passes when run alone.

## Direction (unsettled)

- Isolate the tests from the checkout: point the launcher at a temp plugin
  dir, or patch `local_devenv_runtime_enabled` / the marker lookup in
  `run_main`, so the dev opt-in on disk cannot change test behavior.
- Consider loosening or making the 2.0s timing bound load-tolerant.
