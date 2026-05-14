# ws-dashboard

`ws-dashboard/` is the planned product surface for the personal ws-aware web
dashboard. It is intentionally scaffold-only for now: the directory establishes
the long-lived project shape without committing to daemon APIs, frontend
packages, or stable harness protocols.

Current layout:

- `crates/core/` - dashboard resource model primitives.
- `crates/harness-core/` - reusable harness abstractions and secret-filtering
  interfaces.
- `crates/harness-cli/` - future standalone harness binary wrapper, intended to
  remain callable by other ws runtime surfaces.
- `crates/daemon/` - future local dashboard daemon.
- `frontend/` - future browser UI package.

No public API is stable yet.
