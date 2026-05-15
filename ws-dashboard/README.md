# ws-dashboard

`ws-dashboard/` is the product surface for the personal ws-aware web dashboard.
The current implementation provides the first protected browser entrypoint,
fixture-backed resource shell, local root-picker backend substrate, and
fixture-backed instance event stream scaffold. No public API is stable yet.

Current layout:

- `crates/core/` - dashboard resource model primitives.
- `crates/harness-core/` - reusable harness abstractions and secret-filtering
  interfaces.
- `crates/harness-cli/` - future standalone harness binary wrapper, intended to
  remain callable by other ws runtime surfaces.
- `crates/daemon/` - future local dashboard daemon.
- `frontend/` - future browser UI package.

## Development

Use the wrapper script from this directory:

```bash
./dev.sh run
```

`run` builds the frontend production assets and starts the protected local
daemon with those assets. Open the owner pairing URL printed by the daemon.

Other commands:

```bash
./dev.sh build
./dev.sh test
./dev.sh frontend-dev
```
