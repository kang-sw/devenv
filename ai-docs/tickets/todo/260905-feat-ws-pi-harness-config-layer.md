---
title: "Add `pi` as a first-class harness bucket in ws-mcp (config.tune, tier→model aliases, rsrc harness variants)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor — the Pi adapter is a harness peer of Claude/Codex, so the harness-keyed config layer should know it
  260903-feat-ws-pi-subagent-rpc-ux: sibling — owns the adapter-side model catalog (`model-catalog.json` alias → provider model) that stays separate from the ws-config tier→model layer this ticket extends
related-mental-model:
  - mcp-runtime
sage-review-design: skipped
---

# Add `pi` as a first-class harness bucket in ws-mcp (config.tune, tier→model aliases, rsrc harness variants)

## Background

ws-mcp keys several config surfaces by **harness** — the host that is driving
the MCP session — so that the same project config can carry different values
per host: prompt override points (`config.tune` `prompt.*`), the fixed
tier→model alias table (`agents.tier`, which is what `playbook.render`'s
`recommended-model` and the `SmallTierModel`/`MediumTierModel`/... playbook
variables resolve through), and rsrc playbook harness variants
(`<name>.<harness>.md` overlays and `<include>.<harness>.md` includes). The
`lead-tune` skill is a thin front on `config.tune`, so anything not reachable
through the harness selector is not reachable through `lead-tune` either.

Today the harness set is a closed enum of `codex` and `claude` (plus `*`/all
and the implicit `default` bucket). Search `normalizedHarness` — it exists in
both `internal/wsconfig/config.go` and `internal/mcp/server.go` — and
`promptHarnessEnum` in `internal/mcp/config_registry.go`. Detection is a
substring match on the `initialize` request text (`codex`, `claude`,
`anthropic`) and on per-call `_meta` (Codex workspace roots).

The Pi adapter (`agents-plugin-pi/`) initializes ws-mcp with clientInfo name
`ws-pi-bridge`, matches none of those substrings, and therefore runs every
session with an **empty detected harness**. Observable consequences on Pi:

- `config.tune` with `harness: "pi"` is rejected (`harness must be one of
  claude, codex, or *`); a Pi user can only write prompt overrides into `*`.
- Harness-applicable knobs resolve to the `default` bucket, so a Pi-specific
  tier→model mapping cannot be expressed; `playbook.render`'s
  `recommended-model` on Pi is always the default-bucket value.
- rsrc playbook loading never selects a `.pi.md` overlay because the loader is
  called with an empty harness.

Note the adapter already carries its **own** model table for spawns
(`model-catalog.json`, alias → provider model, read fresh per `ws-agent-spawn`;
spec `pi-adapter-runtime` "Model resolution: name alias, not tier" and "Model
catalog data file"). That table is a different layer — it maps the model
*names* a Pi lead passes to spawn tools — and does not consult ws config. This
ticket does not merge the two; it makes the ws-config tier→model layer
addressable from Pi so that `lead-tune` and playbook tier variables work the
same way on Pi as they do on Claude/Codex. Whether the Pi catalog should later
be derived from the ws tier table is a follow-up decision, recorded below as
out of scope.

## Decisions

- **Harness identity comes from clientInfo, not substring luck.** Extend
  detection so an MCP client whose `initialize` `clientInfo.name` is
  `ws-pi-bridge` (or, more generally, a name carrying a `pi` marker the bridge
  and server agree on) is observed as harness `pi`. Keep the existing
  substring detection for Codex/Claude unchanged. Rejected: having the Pi
  bridge spoof the string `claude` in its clientInfo — it would silently
  inherit Claude-tuned prompt overrides and tier mappings, which is exactly the
  cross-host bleed the harness layer exists to prevent.
- **Closed enum stays closed; it gains one member.** `pi` is added alongside
  `codex` and `claude` in every place the enum is spelled (both
  `normalizedHarness` copies, `promptHarnessEnum`, `aliasTargetKey`'s error
  text, and any `config.list` rendering of harness buckets). A free-form
  harness string is rejected: unknown hosts must fall to `default`, not create
  ad-hoc buckets.
- **Default bucket semantics unchanged.** A Pi session with no `pi`-keyed
  value still resolves through the existing fallback chain (`pi` → `default`),
  so upgrading ws-mcp without touching config is a no-op for current Pi users.
- **Golden rule for the Pi track is respected by sequencing, not violated.**
  This is a ws-mcp Go change and lands on `develop` through the normal ws
  release flow; the Pi adapter needs no code change beyond what its clientInfo
  already sends, unless the agreed marker differs from the current name.

## Constraints

- Any change to `agents-plugin/skills/` or shared rsrc text that names the
  harness enum must go through the wsflow mirroring check
  (`ai-docs/manuals/wsflow-mirroring.md`) and the skills-manifest regen step.
- `config.tune`'s "warning-only for keys that do not vary by harness" rule
  (the "Decision 5" comment in `server.go`'s `config.tune` handler) must keep
  holding for the new bucket.
- Adding a harness bucket must not change the stored key shape for existing
  `prompt.<point>.<harness>` overrides (`*` is stored as `all`).

## Phases

### Phase 1: `pi` harness bucket end to end

Add `pi` to the harness enum and detection, wire it through `config.tune`
(prompt overrides and `agents.tier` with `harness: "pi"`), the tier→model
resolver used by `playbook.render` and the tier playbook variables, and the
rsrc loader's harness-variant selection. Verify with Go tests covering:
detection from a `ws-pi-bridge` clientInfo; `config.tune` accepting
`harness: "pi"` for a prompt override and for `agents.tier`; `config.list`
showing the bucket; `playbook.render` returning the `pi`-keyed model when set
and the default when not; a `.pi.md` overlay being selected only under a
detected `pi` harness. Run the full Go suite plus
`WSRSRC_REGEN_SKILLS=1 go test ./internal/wsrsrc/...` if any rsrc text changes.
Record in the Result whether the Pi bridge's clientInfo name had to change and,
if so, land that adapter change on the Pi track with a matching spec note in
`pi-adapter-runtime`.

## Non-goals

- Merging the Pi adapter's `model-catalog.json` with the ws tier→model table,
  or having one derive from the other. Capture the outcome of Phase 1 first;
  a unification, if wanted, is its own ticket.
- Authoring any `.pi.md` playbook overlays. This ticket makes them selectable;
  it does not write them.
- Changing how Claude or Codex are detected.
