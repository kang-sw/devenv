---
title: "Add `pi` as a first-class harness bucket in ws-mcp and unify the Pi adapter's model table on ws config (config.tune, model aliases, rsrc harness variants)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor — the Pi adapter is a harness peer of Claude/Codex, so the harness-keyed config layer should know it
  260903-feat-ws-pi-subagent-rpc-ux: sibling — introduced the adapter-side model catalog (`model-catalog.json`, a translation shim from tier names to Pi model strings) that Phase 4 of this ticket deletes once ws config holds a `pi` tier table
related-mental-model:
  - mcp-runtime
spec:
  - mcp-tools
  - pi-adapter-runtime
sage-review-design: completed
sage-review-design-reviewed: 0419eaf97cb3d6e3
sage-review-completeness: completed
sage-review-completeness-reviewed: b65918c686bc149b
---

# Add `pi` as a first-class harness bucket in ws-mcp and unify the Pi adapter's model table on ws config (config.tune, model aliases, rsrc harness variants)

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

The adapter also carries its **own** model table for spawns
(`model-catalog.json`, `{aliases: {name → provider/id}}`, read fresh per
`ws-agent-spawn`/`ws-fork`; spec `pi-adapter-runtime` "Model resolution: name
alias, not tier" and "Model catalog data file"). Two implicit alias keys are
baked into adapter code: `"small"` (the explore leaf) and `"complex"`
(`ws-execute complex:true`). That table is the same concept as ws config's
`agents.model_aliases` — `{alias → {harness → {backend, model, effort}}}`,
the generic harness-keyed alias map that the fixed tiers are also stored in —
except that it is adapter-local, unkeyed by harness, and invisible to
`lead-tune`, `config.tune`, and `playbook.render`. Today a Pi lead therefore
reads `recommended-model` from one table and must pass `model_name` from
another, reconciling the two by hand. Owner direction (2026-09-05): unify on
ws config as the single source; the adapter-local catalog retires once the
ws side can answer the same lookup.

## Decisions

- **Harness identity comes from a structured `clientInfo` parse, not from the
  substring matcher.** Today's detector (`detectHarnessFromRaw`) lowercases the
  whole `initialize` params blob and substring-matches `codex` / `claude` /
  `anthropic`; it never parses `clientInfo`, and it is shared with the per-call
  `_meta` path (`detectHarnessFromMeta`), where no clientInfo exists. The `pi`
  check is therefore **not** a new substring: at the `initialize` site, parse
  `params.clientInfo.name` and treat exactly `ws-pi-bridge` as harness `pi`.
  Precedence: this structured check runs **before** the substring detector, so
  a bridge whose params happen to carry `codex`/`claude` text cannot
  mis-detect; the substring detector is left byte-identical for Codex/Claude.
  The `_meta` path is unchanged — a `pi` session is identified once at
  `initialize` and the detected harness is held for the session, as it is for
  Claude today. Rejected: having the Pi bridge spoof `claude` in its clientInfo
  (it would silently inherit Claude-tuned prompt overrides and tier mappings,
  the cross-host bleed the harness layer exists to prevent). Rejected for now:
  an explicit harness declaration (a `_meta` field or an env value the
  launcher sets) — the project owns both ends of this connection so it is
  viable, but `clientInfo.name` is the field MCP already reserves for client
  identity and needs no new contract; revisit only if a second Pi-family
  client appears.
- **Closed enum stays closed; it gains one member.** `pi` is added alongside
  `codex` and `claude` in every place the enum is spelled (both
  `normalizedHarness` copies, `promptHarnessEnum`, `aliasTargetKey`'s error
  text, and any `config.list` rendering of harness buckets). A free-form
  harness string is rejected: unknown hosts must fall to `default`, not create
  ad-hoc buckets.
- **Default bucket semantics unchanged.** A Pi session with no `pi`-keyed
  value still resolves through the existing fallback chain (`pi` → `default`),
  so upgrading ws-mcp without touching config is a no-op for current Pi users.
- **One model table: ws config `agents.tier` under harness `pi`, tiers
  only.** The Pi adapter's `model_name` (spawn/fork), the implicit `"small"`
  (explore leaf and `ws-execute complex:false`), and `playbook.render`'s
  `recommended-model`/tier variables all resolve through the same fixed
  four-tier table (`small|medium|large|xlarge`), edited through
  `lead-tune`/`config.tune agents.tier harness:pi`. There is no user-named
  alias concept (see Open Decisions #1); `ws-execute complex:true` inherits
  the lead's model and consults no table. For Pi the `model` value is the
  `provider/id` string Pi's own model registry accepts and `effort` maps onto
  the spawner's `modelEffort` with a fixed table: an empty resolved effort
  (the stored form of "not given" — `normalizeOptionalEffort` collapses both
  `""` and `none` to `""`, so a stored tier never carries `none`) passes no
  `modelEffort` and leaves the child's default, and `low|medium|high|xhigh`
  pass through unchanged; Pi's `off`, `minimal` and `max` levels are not
  representable in ws config and are simply not offered through this layer.
  `playbook.render`'s `recommended-model` line is left as-is on Pi: for an
  unset `pi` tier it still prints the `default` bucket's id, which the Phase 4
  guide tells the lead to ignore in favour of the tier name (the adapter
  resolves the tier through the Phase 3 tool and inherits on a non-`pi`
  answer). Accepted as presentation debt; suppressing the line would need a
  harness-aware rule in `withRecommendedRenderBinding` that also changes
  what undetected hosts see. A caller-supplied `model_effort` spawn parameter keeps
  winning over the config-resolved effort (explicit per-call beats table
  default), matching how `model_name` already overrides. Rejected: keeping `model-catalog.json` and
  syncing it from ws config (two writers, drift), and deriving ws config from
  the catalog (inverts the ownership every other harness uses).
- **The adapter reads the resolution from ws-mcp, not from the config file.**
  The adapter never parses ws's config store directly; it asks ws-mcp through
  a read tool so the fallback chain (`pi` → `default`), legacy-key
  normalization, and `InferBackend` stay in one place. Missing alias stays
  "inherit the parent model", never an error, preserving the adapter's
  never-hard-fail rule.
- **Golden rule for the Pi track is respected by sequencing, not violated.**
  Phases 2–3 are ws-mcp Go changes and land on `develop` through the normal
  ws release flow; Phases 1 and 4 are adapter changes on the Pi track, and
  Phase 4 is gated on a ws release carrying Phases 2–3 (the adapter's version pin already
  enforces this ordering).

## Constraints

- Any change to `agents-plugin/skills/` or shared rsrc text that names the
  harness enum must go through the wsflow mirroring check
  (`ai-docs/manuals/wsflow-mirroring.md`) and the skills-manifest regen step.
- `config.tune`'s "warning-only for keys that do not vary by harness" rule
  (the "Decision 5" comment in `server.go`'s `config.tune` handler) must keep
  holding for the new bucket.
- Adding a harness bucket must not change the stored key shape for existing
  `prompt.<point>.<harness>` overrides (`*` is stored as `all`).

## Spec Impact

- `mcp-tools`: one new read tool (Phase 3) returning `{backend, model,
  effort, resolved_from}` for a fixed tier under the session's detected
  harness; `config.tune`/`config.list` harness enum gains `pi`.
- `pi-adapter-runtime`: harness detection from `clientInfo.name` (if the
  bridge name changes, note it); the bridged-tool inventory gains the
  `mercenary.*` filter (Phase 1); the three model-resolution anchors ("Model
  resolution: name alias, not tier", "Model catalog data file",
  "Unset-catalog advisory") are rewritten in Phase 4 to tier-through-ws-mcp,
  and the "no Pi model strings in the ws-mcp core" sentence becomes "user
  config may carry Pi model strings; adapter and core code may not".

## Phases

### Phase 1: `ws-execute complex:true` inherits the lead model (Pi track)

Independent of the ws-side phases; lands on the Pi track (`agents-plugin-pi/`)
first. Today `complex:true` selects a `"complex"` catalog alias and only
inherits the lead's model by accident (catalog miss → inherit fallback); a
user who adds a `complex` entry would silently change its meaning. Make
`complex:true` pass no `model_name` at all (inherit), keep `complex:false`
on `"small"`, delete the `"complex"` alias from `resolveExecuteModelAlias`,
the tool description, `pi-lead-guide.md`, and the `pi-adapter-runtime`
`ws-execute` wording ("a light-model default; the lead's own model when
set"). Update the tests. Verify with `npm test`.

Also in this phase (Open Decisions #3, adapter half): the bridge filters
`mercenary.*` out of the ws-mcp tool list before registering tools with Pi,
so no Pi process — lead or child — can see or call the mercenary surface.
Record the filter in `pi-adapter-runtime` next to the bridged-tool inventory
and cover it with a bridge test.

### Result (bfa2cbb6) - 2026-09-05

Landed as `87a9961d` (survey plan), `dbc9e05b` (complex:true inherit),
`bfa2cbb6` (mercenary filter), on the implementation branch under the goal
branch.

Behavioral delta:

- `ws-execute complex:true` passes no `model_name`, so the execute worker
  inherits the lead's model through the spawner's existing inherit path;
  `complex:false` and omitted stay on `"small"`. The `"complex"` alias is
  gone from `resolveExecuteModelAlias`, the tool description and parameter
  schema, `pi-lead-guide.md`, and the `ws-execute` wording in
  `pi-adapter-runtime` ("the lead's own model when set"). The three Phase 4
  model-resolution anchors are untouched, as scheduled.
- The bridge drops every raw tool name starting with `mercenary.` right
  after `tools/list`, before Pi registration and before `wsToolNames` is
  built, so neither the lead nor any spawned child sees the mercenary
  surface regardless of the server-side `workflow.prefer_mercenary` knob.
  Recorded in `pi-adapter-runtime` next to the bridged-tool inventory.
- Tests 696/696 (+2): the filter is exercised by a fixture that contains
  mercenary names (the live 60-tool fixture has none); the execute-gateway
  tests assert `undefined` for `complex:true` and `"small"` otherwise.

Review (single, full scope): one Important — the two implementation
commits lacked `## Spec` and `## Ticket Updates` sections — [fixed] by
rewording the local commits (tree unchanged). No Critical.

## Blocked (2026-09-05)

Phases 2 and 3 are ws-mcp Go changes that land on `develop` through the
normal ws release flow, and Phase 4 is gated on a released ws that carries
them. None of the three can advance on the Pi-track goal branch this run
is draining; the next step is a `develop`-based run (or the owner) picking
up Phase 2. Phase 1 is complete.

### Phase 2: `pi` harness bucket end to end

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

Finding (2026-09-05, dogfood via `lead-tune` in another session): the harness
vocabulary is not discoverable before a write, so a lead cannot tell whether
`pi` is accepted. `prompt.*` declares `Enum: promptHarnessEnum` on its
`harness` selector, so `config.list` prints `harness[claude|codex|*]` and
`config.tune` rejects an unknown value up front; `agents.tier`'s `harness`
selector (`config_registry.go`) declares no enum, so `config.list` prints a
bare `harness`, the `config.tune` enum guard is skipped, and an unknown value
is only rejected later by `aliasTargetKey`. The two surfaces also use
different wildcard spellings (`*` for prompt overrides, `default` for
`agents.tier`). Phase 2 should, alongside adding `pi`: give the `agents.tier`
harness selector an explicit enum so `config.list` lists it (`claude`, `codex`,
`pi`, `default`), make the `config.tune` rejection text name the full enum per
key rather than the hard-coded "claude, codex, or *", and have the `lead-tune`
playbook tell the lead to read the selector enum from `config.list` before
proposing a harness value. The playbook change goes through the wsflow
mirroring check.

### Phase 3: Tier resolution read tool for adapters

Depends on Phase 2. Expose one MCP read tool (working name
`config.resolve_agent(tier, harness?)`, or an equivalent machine-readable
mode on an existing config read tool — decide at implementation and record
the choice) that returns the resolved `{backend, model, effort}` for a fixed
tier under the session's detected harness, applying the same fallback chain
and normalization `playbook.render` uses, and reporting which bucket answered
(Open Decisions #2, settled). Register it in the config registry with the
no-agent/harness applicability the other config tools carry, document it in
`mcp-tools.md`, and cover it with Go tests for: each tier under `pi`,
fallback to `default` with the answering bucket reported, and an unknown
tier rejected.

### Phase 4: Pi adapter resolves models through ws-mcp; catalog retires

Depends on Phase 3 being in a released ws the adapter pins. On the Pi track
(`agents-plugin-pi/`): route `resolveModelForAlias` (spawner) and the explore
leaf's / `ws-execute complex:false`'s `"small"` through the bridge to the
Phase 3 tool, treating a non-`pi` answering bucket as inherit; carry `effort`
into `modelEffort`; replace the "model catalog
unset" `workflow_manual` advisory with an "alias table has no `pi` entries"
advisory sourced from the same tool; delete `model-catalog.ts`,
`model-catalog.json`, and their tests; update the `pi-adapter-runtime` spec
anchors ("Model resolution: name alias, not tier", "Model catalog data file",
"Unset-catalog advisory") and `pi-lead-guide.md` so the lead is told to pass
the tier names `lead-tune` shows (and to drop the "anything the user names"
sentence). Verify with the adapter's `npm test` and one live spawn per kind
(a set `pi` tier, an unset tier → inherit, `complex:true` → inherit).

## Non-goals

- Authoring any `.pi.md` playbook overlays. This ticket makes them selectable;
  it does not write them.
- Changing how Claude or Codex are detected.
- Reworking `mercenary` backends to launch Pi processes; `backend` for a `pi`
  entry is informational until a Pi mercenary backend exists.

## Open Decisions (2026-09-05)

The design review's `missing` issues (table below, #1 #2 #3 #5) each need an
owner decision before the phases can be re-cut. Lead recommendation per item,
recorded for the owner to accept or overrule:

1. **Arbitrary alias names have no home in ws config.** `agents.model_aliases`
   looks generic but every writer and reader normalizes the outer key to the
   four fixed tiers (`small|medium|large|xlarge`); `config.tune` has no
   `model_alias` key. The adapter's `"complex"` and any user-named alias would
   be dropped by the catalog-retirement phase as originally written.
   **Settled 2026-09-05 (owner): tier only — there is no user-named alias
   concept.** Tracing the adapter shows only three names ever reach the
   catalog: `playbook.render`'s `recommended-model` (a ws-config tier value),
   the explore leaf's fixed `"small"`, and `ws-execute`'s `"complex"`. No lead
   path invents names and the guide never suggests one. The "generic
   name → `provider/id`" framing came from `260903` D-A as a *translation
   shim*: ws config could only hold codex/claude ids, so the adapter needed a
   table to turn `recommended-model` into a Pi string without placing Pi
   strings in ws-mcp. A `pi` harness bucket removes the shim's reason to
   exist. Consequences, folded into the phases below: the model layer is the
   four fixed tiers under harness `pi`; explore's `"small"` and `ws-execute
   complex:false` are tier `small`; `complex:true` was always meant to
   **inherit the lead's model** (no alias at all — today it only does so by
   accident, through the catalog-miss fallback) and is corrected on the Pi
   track independently of the ws-side phases; no alias writer or non-tier
   resolver is added. The live spec anchor's "anything the user names"
   sentence is amended when the catalog retires.
2. **`pi` → `default` fallback hands Pi a codex model id.** `wsconfig.Load`
   seeds `default`/`codex`/`claude` for every tier, so a fixed tier is never
   "unset"; Pi cannot accept those ids as `--model`. Recommendation: the read
   tool reports **which bucket answered** (`resolved_from: pi|default|…`) and
   the Pi adapter treats a non-`pi` hit as "inherit the parent model" — the
   fallback chain stays intact for Codex/Claude, and Pi degrades to inherit
   exactly as today. Alternative: exempt `pi` from the chain server-side (a
   `pi` lookup never falls to `default`) — simpler for the adapter but bakes a
   harness special case into the resolver.
   **Settled 2026-09-05 (owner): the recommendation.** The read tool reports
   `resolved_from`; the adapter inherits on any non-`pi` answer; the resolver
   gains no harness special case.
3. **`backend` is load-bearing and `normalizedHarness` doubles as the backend
   normalizer.** Adding `pi` there also creates a `pi` *backend* key, and a
   mercenary spawn from a detected `pi` session would resolve the `pi` bucket
   and try to launch a `provider/id` string under a codex/claude backend.
   Recommendation: split the normalizers (harness enum vs backend enum; `pi`
   joins only the harness one), require `pi` entries to carry an explicit
   backend of `pi` (no mercenary backend yet — see Non-goals) and make the
   mercenary path reject a `pi`-backend resolution with a clear error rather
   than attempting a launch. Alternative: keep one normalizer and document the
   mercenary-from-Pi path as unsupported — smaller diff, sharper edge.
   **Settled 2026-09-05 (owner): mercenary is a deprecated path and is not
   touched.** No normalizer split and no new error branch in ws-mcp. The
   harness enum gains `pi` for every harness-keyed surface (prompt overrides,
   `agents.tier`, rsrc harness variants, `config.list`), and whatever backend
   key that incidentally creates stays inert because the Pi adapter never
   exposes the mercenary surface: the bridge drops `mercenary.*` from the
   tool list it registers with Pi (adapter-side filter after `tools/list`,
   independent of the global `workflow.prefer_mercenary` knob, so Codex and
   Claude sessions are unaffected). Playbook blocks tagged mercenary-only
   already render under the same knob; the Pi guide names no mercenary
   route. Mercenary-from-Pi is therefore unreachable from a Pi process,
   not rejected at runtime. The parent epic classes a `tools/list` filter as
   a harness-owned soft guard (containment proper belongs in the keyed
   `tools/call` handler); this ticket knowingly trades against that for a
   deprecated surface — the residual path (`mercenary.register(backend:
   "pi")` issued from a non-Pi session) stays as loud and as unsupported as
   any other misuse of a deprecated tool, and the prose that
   `workflow.prefer_mercenary: on` renders for a Pi lead describes tools it
   cannot see, which the Pi guide's "no mercenary route" line already
   overrides. The adapter half lands in Phase 1.
4. **Golden-rule exception and spec territory.** Phases 2–3 are ws-mcp Go
   changes motivated solely by Pi; branch sequencing does not answer whether
   that is an exception to "ws-mcp Go is never modified for Pi". The live spec
   anchor also says no Pi model strings are placed in the ws-mcp core.
   Recommendation: grant the exception explicitly here, scoped to "the harness
   enum and a harness-neutral alias read tool" (nothing Pi-specific beyond the
   enum member), and amend the anchor to say user *config* may carry Pi model
   strings while *code* may not. Add `spec:` frontmatter and a `## Spec Impact`
   section naming `mcp-tools` (new read tool) and the three `pi-adapter-runtime`
   model-resolution anchors once this is accepted.
   **Settled 2026-09-05 (owner): not an exception but a clause.** The Pi
   track's rule ("ws-mcp Go source untouched") exists to keep the dependency
   one-directional and to keep host-specific logic out of the core. Adding a
   harness to a closed enum and exposing a harness-neutral read tool does
   neither, and the same clause covers any later host (opencode or another)
   that needs to be a peer of Codex/Claude in the harness-keyed surfaces. The
   clause is recorded in `AGENTS.md` (Project Knowledge, Pi direction) and
   the `pi-adapter-runtime` anchor amendment lands with Phase 4. `spec:` and
   `## Spec Impact` added. Design re-review found three live statements
   still contradicting Phases 2–3 (the AGENTS.md bullet's "Go source
   untouched" parenthetical, the `pi-adapter-runtime` preamble's "no ws-mcp
   source is modified for Pi", and the research anchor's "Go source
   untouched"); all three were reworded to "no Pi-specific logic in its Go
   source" plus a pointer to the clause in the same commit that stamps this
   review, so no phase carries them. Only the
   `{#260903-pi-model-catalog-config-file}` sentence remains scheduled
   (Phase 4).

## Blocked (2026-09-05) — resolved, kept as history

Superseded the same day: every `missing` row below was settled under
`## Open Decisions`, and the design re-review returned `concern` with all
issues autonomous. The table is the original round-1 record.

### Design Reviewer — block (round 1)

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | ws config `agents.model_aliases` cannot hold the adapter's alias names — no writer, no resolver (outer key is the fixed four-tier vocabulary; `complex` and user-named aliases have no home after Phase 3) | critical | missing |
| 2 | `pi` → `default` fallback yields codex model ids, so "unmapped alias returns unset" cannot hold (`applyDefaultModelAliases` seeds every tier on load; ids are not portable across harness buckets) | critical | missing |
| 3 | `backend` is load-bearing, not informational; `normalizedHarness` doubles as backend normalizer; mercenary spawn from a detected `pi` session is undefined | important | missing |
| 4 | clientInfo-based detection is not a substring addition; precedence and the `_meta` path are unspecified; explicit harness declaration not weighed | important | autonomous |
| 5 | golden-rule justification answers a different question (branch ordering, not exception grant); no `## Spec Impact` section / `spec:` frontmatter | important | missing |
| 6 | effort vocabulary mismatch (ws none|low|medium|high|xhigh vs Pi off|minimal|…|max) and precedence against the surviving `model_effort` spawn parameter | minor | autonomous |
| 7 | frontmatter `related:` entry for 260903 contradicts the body on the catalog's fate | minor | autonomous |
