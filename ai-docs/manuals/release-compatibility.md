---
summary: Required when a change touches a cross-version surface (MCP tool or argument, `/mcp` hello, shared store shape, tracker event type, config/rsrc manifest field) - how ws versions meet, the scenario sheet, the T0/T1/T2 tiers, and the release rules
---

# Release Compatibility

Several ws versions run on one machine at once: Claude, Codex, and Pi each
install their own plugin copy and update on their own schedule. This manual
says how those versions meet, what goes wrong, and which changes a release may
carry. A change's compatibility tier is decided while it is discussed and
recorded in the ticket's `## Decisions`, so the release gate only confirms it.

## How Versions Meet

- **One daemon per cache root.** Only its dashboard daemon runs a ws executable;
  Python launchers and holders are the adapters. Starting or replacing that
  daemon is the sole executable lifecycle exception. Within a minor the newest version wins
  (`ensure.go` keepRunning, `version.go` CompareVersions). An older client
  never replaces a newer daemon. Equal version strings fall back to binary
  mtime.
- **Binary and text come from different places.** Hosted MCP tools run in the
  daemon binary, while playbook and skill text comes from the connecting
  plugin's own directory (`WS_RSRC_ROOT` is required). The typical in-minor
  skew is old skill text running against a newer binary. Playbook content
  always matches its own package's skills.
- **A minor mismatch never creates a second server.** The current hello
  rejects it; the Python adapter diagnoses "dashboard daemon not reachable"
  and retries without executable fallback. Cross-minor acceptance and notices
  remain owned by the mixed-version guards ticket.
- **There is effectively no version negotiation.** The `/mcp` hello requires
  `protocol: 1` exactly. The hello's plugin version is used only for the minor
  check and is not recorded per session. `runtime.json` per-tool ranges are
  never evaluated, and no `tools/list_changed` is sent. The comment at
  `version.go:52-54` states that the wire surface changes only in a minor;
  nothing checks it yet.
- **Stores mostly read tolerantly.** Go struct round-trips can drop fields an
  older writer does not know (suspected; no per-writer audit yet). Only the
  config manifest and the rsrc manifest check `schema_version` on read. sqlite
  has no guard against an older binary opening a newer schema. The tracker
  flags unknown event types.
- **Existing mechanisms.** Intake `accepts:` negotiation for optional event
  kinds; the holder v / v-1 window; Pi's version pin (daemon `serverInfo` >=
  bundled version, same minor) with one `tools/list` at startup; the Claude
  mod reconnect watchdog, which fires only when tools disappear.

## Scenario Sheet

| # | Situation | What happens | Tier |
|---|---|---|---|
| 1 | Pi updated a patch, Claude still on the older patch (same minor) | The daemon is replaced by Pi's newer binary; Claude sessions reconnect and run old skill text on the new binary. Fine if tools/args were only added; a removed or renamed arg fails loudly as unknown | added: T0; removed: T2 (today possible even in a patch) |
| 2 | Claude updated, Pi older (same minor) | Symmetric to 1; Pi's pin (daemon >= bundled) passes | same as 1 |
| 3 | Another harness replaces the daemon mid-session | In-flight calls get `-32000`, the launcher reconnects; the tool list is not refreshed, so new tools stay invisible and removed tools fail on call | T1 |
| 4 | Minor mismatch (Pi 0.49 / Claude 0.48) | Current hello rejects it; no direct server is started. Future mixed-version acceptance still needs store guards against old writers dropping fields or opening newer schemas | T2 for non-additive store/protocol changes |
| 5 | Same version string, different build (release vs dev) | mtime decides; a newer file wins even with older code; total order, no flapping, but a wasted replace attempt per session start | T0 (cost only) |
| 6 | Local dev: source newer than binary | Asset-only changes are not picked up (`261112-bug-ws-launcher-fingerprint-skips-embedded-assets`) | dev-only bug |
| 7 | Tracker ledger synced across machines on different versions | The older side flags new event types as errors | T2 (cross-machine; daemon rules cannot prevent it) |

## Tiers

- **T0 transparent.** Playbook and skill content changes; dashboard assets;
  new tools, arguments, and optional fields; new optional store fields, as
  long as read-modify-write preserves fields the writer does not know.
- **T1 reconnect-recoverable.** Tool list refresh after a daemon replacement;
  a tool or argument rename that keeps the old name as an alias for one
  minor.
- **T2 breaking.** Non-additive store changes (field removal or meaning
  change, destructive sqlite migration); the hello protocol; new tracker event
  types; strict config manifest fields; tool or argument removal without an
  alias.

## Release Rules

1. **A patch release carries only T0 and T1.** Within a minor, tools and
   arguments stay; a rename keeps an alias for one minor.
2. **T2 lands only in a minor bump**, raising the affected store's
   `schema_version` or the protocol number. A writer that sees a newer
   `schema_version` than its own refuses to write and fails loud.
3. **The daemon judges skew.** It records each session's hello version. On a
   minor mismatch it injects a strong notice into that session's tool results
   and workflow manual ("plugin compatibility is broken; tell the user to
   update") plus a dashboard badge, and the session keeps working. A patch
   difference shows only a quiet dashboard badge. There is no direct-execution
   fallback. Inspection reads `/health`, or the existing hello version when
   health/version information is missing; it never executes a ws binary or
   starts a daemon just to obtain its version.
4. **A daemon replacement refreshes the tool list.** `tools/list_changed`
   first; where a host ignores it, a forced reconnect (the Claude mod watchdog
   widened to version or content-hash changes; Pi re-fetches `tools/list` on
   reconnect).

## Enforcement State

Rules 1-4 are confirmed (user decisions 2026-11-12); their mechanisms are not
implemented yet. Until they land, the tier recorded at discussion time and the
release gate's review are the only checks. Pending mechanisms, tracked in
ticket `261112-feat-ws-mixed-version-compat-guards`:

- Rule 1: a tool-surface monotonicity test against the previous release's
  `runtime.json` surface.
- Rule 2: newer-`schema_version` write refusal in every shared store writer.
- Rule 3: per-session hello version recording, the skew notice, and the
  dashboard badge.
- Rule 4: `tools/list_changed` emission and the widened reconnect triggers.
