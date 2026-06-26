---
title: rsrc manifest/mirror regen test entrypoints silently no-op when go-test-cached
related:
  260611-bug-rsrc-manifest-regen-missed: the up-to-date guard this regen entrypoint feeds; same maintenance loop
completed: 2026-06-20
---

# rsrc manifest/mirror regen test entrypoints silently no-op when go-test-cached

## Background

The shipped-rsrc maintenance loop regenerates two artifacts through env-gated
test entrypoints in `agents-plugin-tool/internal/wsrsrc`:

- `WS_REGEN_MANIFEST=1 go test ./internal/wsrsrc -run TestRegenerateShippedManifest`
  (`WriteManifest` side effect)
- `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror`
  (rewrites the byte-identical wsflow mirror)

## Observed Surprise (dogfood, 2026-06-19)

Running either regen command a second time in a session returned `ok` from the
**go test cache** without executing the test body, so the `WriteManifest` /
mirror-rewrite side effect never ran. The artifacts stayed stale even though the
command reported success; the failure only surfaced later via
`TestShippedManifestUpToDate` / `TestWsflowRsrcMirrorUpToDate` reporting drift.
Re-running with `-count=1` forced execution and fixed it.

Go's test cache keys on the test binary and a fixed set of inputs; an arbitrary
env var like `WS_REGEN_MANIFEST` does not invalidate the cache, and a
side-effecting "test" is exactly the case the cache is not designed for.

## Why it matters

The regen entrypoints are the documented maintenance path after any canonical
rsrc edit (see `ai-docs/ref/wsflow-mirroring.md` and the regen hints in the guard
tests). A maintainer who omits `-count=1` gets a green-but-stale result and can
commit a stale manifest/mirror; the up-to-date guards catch it, but the loop is
confusing and wastes a cycle.

## Candidate fixes (not yet decided)

- Make the regen entrypoints uncacheable (e.g. read a changing input, or move
  regen out of `go test` into a small `go run` generator / `//go:generate`).
- Add `-count=1` to the regen command strings printed by the guard failures and
  documented in `wsflow-mirroring.md`.
- Keep the test-as-entrypoint but document the caveat prominently.

Lowest-effort partial fix is correcting the printed/ documented commands to
include `-count=1`; the cleaner fix is a non-cached generator entrypoint.

## Resolution (2026-06-20)

Took the lowest-effort fix: added `-count=1` to the regen command strings printed
by both guard failures (`manifest_shipped_test.go`, `wsflow_mirror_test.go`) and
to `ai-docs/ref/wsflow-mirroring.md`, where the caveat is now stated explicitly
(the flag is mandatory because the env-gated test body has no changing input).
A maintainer who copies the guard's own failure message or the doc command now
always runs the side effect.

The non-cached generator entrypoint was deliberately not built — it is the
cleaner shape but over-engineering for a footgun fully neutralized by the printed
commands. If a `//go:generate` regen path is wanted later, open a fresh `idea/`
ticket; this one closes as resolved.
