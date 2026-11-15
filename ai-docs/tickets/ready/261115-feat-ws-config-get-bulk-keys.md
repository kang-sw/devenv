---
title: "Bulk config.get keys[] with band fallback to single reads"
related:
  261114-refactor-ws-common-session-interface: "later composition through its shared session context; not part of its Phase 1, whose Decision 16 (ready version, 19573be27 on exec4/csi-design) forbids MCP changes"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 665ffcf93cf6d847
sage-review-completeness-reviewed: 665ffcf93cf6d847
---

# Bulk config.get keys[] with band fallback to single reads

## Background

The Claude mod issues one `config.get` MCP call per setting at startup:
eight primary setting-read sites across its features, not eight owned by the
band, with additional reads when the compaction advisory or captions are
enabled (`agents-plugin/hooks/claude-mod/register.ts#L179-L186`,
`agents-plugin/hooks/claude-mod/register.ts#L1249-L1252`,
`agents-plugin/hooks/claude-mod/band.tsx#L885-L900`,
`agents-plugin/hooks/claude-mod/captions.tsx#L655-L665`). The startup config-call flood was reported from the
FIX-share lane and routed here by MAIN per (919)=a.

`config.list` cannot replace those reads with parity: its declared catalog
`current.value` is a raw string with no typed/default validation, its
per-key resolver error is discarded
(`agents-plugin-tool/internal/mcp/config_manifest.go:464-485`;
`server.go:2862-2865,3255-3271`), and it is refused to delegate/leaf keys
while `config.get` is allowed (`server.go:5733-5751`). `config.get`
resolves through `resolveConfigGet` with typed validation, typed declared
defaults for invalid stored values, special structured/harness-bucketed
branches and wrapped error propagation (`config_manifest.go:329-359,448-459,513-569`;
`server.go:1247-1266`).

Investigation report (non-authoritative evidence, outside the repo):
`/tmp/exec4/cfg-flood.md`.

## Decisions

1. **Optional `keys[]` on `config.get` (MAIN 09:05 = option (a); owner
   (1084) announced as technical).** A request carries exactly one of the
   legacy `key` or a nonempty `keys` string array; empty, duplicate,
   non-string or blank entries are request-level errors. The legacy
   single-key request and response stay byte-for-byte unchanged. Rejected:
   a `resolved` mode on `config.list` - it would have to bypass the catalog
   build to avoid unrelated failures, an ignore-unknown older server would
   return a successful ordinary catalog that looks plausible, and it stays
   unavailable to delegate/leaf keys.
2. **Response envelope with a mode discriminator.** A `keys` request returns
   `{"mode":"resolved-v1","results":[...]}` with one row per requested key in
   request order: `{"key","ok":true,"value","scope","warnings"?}` or
   `{"key","ok":false,"error":{"message"}}`. Each row is exactly what a
   single-key `config.get` for that key and the same `session_key` returns:
   value (any existing type, including null and objects), scope, warnings,
   and the same wrapped `config.get: <cause>` error text. A per-key error does
   not fail its siblings; malformed requests, authorization and transport
   failures stay request-level errors. Text output (non-JSON format) renders
   one line per row in the same order. Rejected: atomic snapshot semantics
   or eager all-scope validation - rows are independent single-key
   resolutions.
3. **Band fallback to single calls.** The band accepts a bulk result only
   when `mode == "resolved-v1"` and there is exactly one well-formed row per
   requested key; anything else (an MCP error from an older server, a missing
   discriminator, a row-count or key mismatch) makes it issue the existing
   single-key calls with the same session context. A valid row with
   `ok:false` is a rejected read for that key, handled by that feature's
   existing failure policy (for example band-off), never a fallback or a
   default success. The band never sends a dummy legacy `key` alongside
   `keys`: an ignore-unknown server could answer just that key.
4. **Compatibility tier: T0 with the band fallback.** Old band -> new server:
   unchanged legacy calls and results. New band -> old server: today's server
   rejects the request (missing `key`, unknown `keys`;
   `server.go:1254-1256,4746-4757`; `tool_arguments.go:70-128`), and the
   band falls back to single calls. No stored config, manifest or record
   shape changes. Without the fallback this would be a T3 wire boundary; the
   fallback is therefore part of the contract, not an optimization.
5. **Batch scope in the band.** Batch only the startup reads of one keyless
   start generation, keeping each read's existing conditional gate (hosted,
   interactive, watchdog-enabled, advisory/captions-enabled): conditional
   follow-up keys may be a second bulk call after the gating value resolves.
   The keyed `claude.status_band_enabled` re-read after key adoption or a
   model-origin `config_tune` stays a fresh read guarded by its existing
   sequence/stale-result checks (`register.ts:1384-1387,1424-1431`;
   `band.tsx:885-945`). Rejected: reusing startup values after adoption or
   tuning.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Mirror any band change the wsflow package carries.
- Reuse `resolveConfigGet` per row; do not fork resolution logic, and do not
  change single-key resolution, scopes, defaults, warnings or error text.
- The tool schema and description gain `keys` additively; existing callers
  and argument validation for `key` are unchanged. Delegate/leaf
  authorization for `config.get` applies to `keys` exactly as to `key`.
- Activity-mtime touches from keyed reads keep today's behavior per resolved
  key (`session_auth.go:535-546`); no new side effect.
- No new config key, manifest field or stored shape.

## Prior Decisions

- fdb391e6 (2026-11-08, commit): "The config.get branch calls wsagent.ResolveProfiles directly; formatConfigGet renders wsagent.Profile as JSON like worktreeProfile." — bearing: constrains
- 261105-feat-whiteboard-knob-harness-and-session-scope (2026-11-05, commit 9ca6f003): "config.get becoming bucket-aware is not in the ticket's list of readers. Without it, config.get would report only the legacy or builtin value." — bearing: constrains
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, commit 99a94b8a): "D7: the three new knobs are read without session_key (the manual knob before any key exists); tests answer `true` only to a keyed read and assert the feature stays off." — bearing: constrains
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "passing the held session key when there is one so a session-scope `lead-tune` is honored" — bearing: constrains
- 261022-feat-claude-mod-tool-call-captions (2026-10-22, commit 1c2313ba): "Settings read: the switch first; the other four keys (model, style, context tokens, workflow.lang) only when it reads true, so an off session costs one config_get." — bearing: constrains
- 261020-feat-worktree-profiles (2026-10-20, commit 1cdd2f6f): "config.list reports a bad profile as the knob's error instead of failing the whole listing, so the lead can still see and fix it; acquire and the manual (later phase) fail loud as the ticket requires." — bearing: constrains
- 261002-feat-ws-config-adapter-schema-extension (2026-10-02, commit 1ff2e25a): "config.get is exempt from the config.* delegate/leaf gate (read-only) so a spawned child reads the session value its lead tuned through the parent chain; config.tune/list stay lead-gated." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | MCP handler/schema in agents-plugin-tool/internal/mcp/server.go; resolver/results in agents-plugin-tool/internal/mcp/config_manifest.go; startup callers in agents-plugin/hooks/claude-mod/register.ts and its wsflow mirror. |
| scope.surface | public-interface | config.get gains an MCP argument and a new response mode; agents-plugin-tool/internal/mcp/server.go#L4746-L4757 currently requires only key. |
| scope.new_public_symbol | yes | New public MCP argument config.get.keys and response discriminator mode=resolved-v1; no new tool name is planned. |
| scope.new_type_contract | yes | Decisions 1–3 define mutually exclusive key/keys requests and ordered success/error result rows with preserved value types. |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/config_manifest_test.go#L305-L388 covers precedence, types, warnings, unset and text; agents-plugin/hooks/claude-mod/band-engine.test.tsx and engine.test.ts provide existing engine fixtures. |
| complexity.reuse_points | confirmed | Read resolveConfigGet and configGetResult in agents-plugin-tool/internal/mcp/config_manifest.go#L496-L569; readSetting/callText in agents-plugin/hooks/claude-mod/register.ts#L855-L879 and the keyed band edge at #L1384-L1387. |
| complexity.side_effect_risk | moderate | Config reads do not rewrite settings, but keyed resolution touches session activity (agents-plugin-tool/internal/mcp/session_auth.go#L518-L546); batching also changes asynchronous startup call timing. |
| risk.correctness | moderate | Per-key error isolation and strict bulk validation must preserve conditional reads and prevent stale startup results from replacing keyed re-reads; agents-plugin/hooks/claude-mod/band.tsx#L885-L945. |
| risk.fit | moderate | Reuses the existing narrow adapter-read surface, but startup scope spans multiple mod features. |
| risk.test | moderate | Existing Go and engine fixtures are reusable, but current tests exercise single-key responses; verification must add bulk/fallback cases across feature gates and stale-result paths. |
| risk.security_or_contract | moderate | Public MCP validation and delegate/leaf authorization must remain equivalent (agents-plugin-tool/internal/mcp/tool_arguments.go#L65-L128; agents-plugin-tool/internal/mcp/server.go#L5733-L5751); T0 depends on the specified old-server fallback. |

## Phases

### Phase 1: Bulk keys on config.get and band adoption

Add the optional `keys` argument and `resolved-v1` envelope to `config.get`
(Decisions 1, 2, 4) and switch the Claude mod band's startup reads to bulk
calls with the single-call fallback (Decisions 3, 5).

Verification:

- server table tests: legacy `key` unchanged (JSON and text); `keys` rows equal
  per-key single calls for session/project/global/builtin/unset scopes, an
  invalid stored value (typed default + warning), a structured/harness-bucketed
  key and a resolver error; request-level errors for both/neither of
  `key`/`keys`, empty, duplicate and non-string entries; delegate/leaf key
  authorization parity;
- band tests (the mod's existing test runner): bulk success path; fallback on
  an MCP error, a missing discriminator and a row-count/key mismatch, each
  issuing exactly the legacy single calls; an `ok:false` row follows the
  feature's existing failure policy; no request ever carries both `key` and
  `keys`; the keyed band re-read after adoption/tuning remains a fresh call
  with stale-result guards;
- runs: `go test ./internal/mcp -run Config` (scoped) and the mod's test
  command; never the full `./internal/wsdashboard` package.
