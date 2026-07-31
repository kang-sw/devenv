# Agent Harness Capability Tiers

Recovered from the retired dashboard line on 2026-07-31. This reference keeps
the fixture-verified harness capability evidence that remains expensive to
re-derive; it does not define a live dashboard contract.

## Tier Definitions

Interactive agent-harness provider/session work must classify each capability
per **(harness, capability) cell**, never per capability as a whole, into one
of four tiers: **Passthrough** (the harness itself officially
documents/exposes the capability for third-party programmatic use), **Overlay**
(an integration layer composes only officially-exposed primitives into new behavior;
no vendor-private state touched), **Hack** (the only reachable path mutates a
harness's private/undocumented on-disk state or relies on
reverse-engineered/unofficial protocol messages), or **Unavailable** (no known
path yet; stays not-implemented rather than silently becoming a Hack). A
capability native to only one harness stays Passthrough for that harness —
cross-harness commonality (common subset vs. per-harness-gated) is a separate,
orthogonal axis, never conflated with tier.

## Fixture-Verified Codex Capability Evidence

**Codex's column is fixture-verified** (2026-07-11, via `codex app-server
generate-json-schema --out <dir> --experimental` against the installed
`codex-cli 0.144.1`, not WebSearch-only): Codex additionally offers
Passthrough manual compaction (`thread/compact/start`, result arrives async via
`thread/compacted`), fork (`thread/fork`), skill listing (`skills/list`),
mid-turn steering (`turn/steer`), and a native goal-state-tracking family
(`thread/goal/set`/`get`/`clear` — bookkeeping only, not an auto-looping
primitive) with no confirmed equivalents elsewhere. `thread/rollback` (rewind)
is Passthrough but **confirmed deprecated for removal** and coarser than
originally assumed (drops N turns from the end, not point-based, does not
revert file changes) — do not design new functionality around it.

## Evidence Boundary

This recovery preserves the four-tier definitions and the fixture-verified
Codex findings from the archived source. The sole semantic-neutrality edit is
the Overlay actor: its retired dashboard-specific wording is rendered as "an
integration layer" so the reusable taxonomy does not retain a dashboard-owned
contract. It intentionally does not preserve the retired dashboard's adapter,
browser, plugin-presence, or provider-lifecycle contracts; those depended on
the removed dashboard architecture rather than on reusable harness evidence.

## Recovery Verification

Run `./ai-docs/ref/verify-dashboard-archive-recovery.sh` before removing the
source branch. It verifies that `archive/ws-dashboard` is an annotated tag,
that its commit has the four captured parents in the required order (main,
helper-liveness, server-scoped-forwarding, then local SQLite activity), and
that `origin` advertises its peeled tag entry. It also compares this file's
four-tier definition and fixture-verified Codex paragraph against
`origin/impl/helper-liveness-probe:ai-docs/mental-model/ws-dashboard-agent-harness.md`.
The comparison normalizes Markdown wrapping and makes only the documented
`the dashboard` → `an integration layer` substitution in the tier definition.
