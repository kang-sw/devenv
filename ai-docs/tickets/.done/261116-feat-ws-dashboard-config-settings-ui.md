---
title: "대시보드 4단계 설정 패널과 읽기 전용 기본값 보기"
related:
  261119-feat-ws-config-git-sync: "future synced layer and its settings surface"
  260924-bug-ws-config-project-scope-is-machine-wide: "project scope storage and migration are handled separately"
  261116-feat-ws-dashboard-generalized-mentions: "related dashboard interaction work"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 41988953c096efd7
sage-review-completeness-reviewed: 41988953c096efd7
completed: 2026-11-21
---

# Dashboard settings: four scopes and builtin inspection

## Background

The owner wants a dedicated settings view for directly inspecting and tuning workflow and adapter settings. The approved v2 visual and interaction reference is `ai-docs/assets/mockups/261121/config-settings-ui/mockup.html`; `ai-docs/assets/mockups/261116/config-settings-ui/` is its historical predecessor. The v2 reference includes the owner’s 3+1 refinements. The git-sync ticket is currently in `todo/`, not `idea/`.

Verified source facts (line numbers describe this checkout and may drift):

- The registry is `agents-plugin-tool/internal/mcp/config_registry.go:39-92,243-247`; prompt entries are discovered rather than static (`:564-589`). `tuningField` and `buildTuningCatalog` now live in `agents-plugin-tool/internal/mcp/server_config.go:264,286`, not `server.go`; `server_dispatch.go:393` builds the MCP listing. The existing `config.list` and `config.tune` contract must remain unchanged.
- Resolver order is session > project > repo > global > builtin (`agents-plugin-tool/internal/wsconfig/resolver.go:76-131`), subject to global-only rules and bucket resolution (`mcp/config_registry.go:222`); do not infer a winner from the visual order. `wsconfig/config.go:592-598` resolves project to `CacheRoot/config.json`, without project identity: it is currently machine-wide. `global.go:17-29` resolves global to `$WS_CONFIG_HOME/config.json` or `~/.ws/config.json`. `repo.go:40-68` derives the worktree repo file. Structured session writes are unsupported (`structured.go:105-115`).
- Current project/global writes `updateConfigFile` decode a fixed `Config` and re-marshal it (`wsconfig/config.go:35-47,783-805`): unknown struct members are **not** preserved. Entries in the known `overrides` and `structured_overrides` maps survive. Repo updates use raw top-level members (`repo.go:100-130`). This distinction is material to mixed-version safety.
- The four canonical manifests contain 12 Claude, 2 AgY, 5 Dashboard, and 15 Pi keys respectively (`agents-plugin/config-manifest.json`, `agy-config-manifest.json`, `dashboard-config-manifest.json`, `agents-plugin-pi/config-manifest.json`). `wspackage/package.go:19,48-89` embeds generated package files; `scripts/generate-package-assets.py:17` selects files for generated packages, but the present generated index contains only the placeholder and the normal package generator must be inspected: Pi's manifest is not automatically available merely because ws's manifest is embedded. Add a build-owned Pi manifest input and verify a built binary without a checkout. Never depend on caller cwd, a live MCP session, or repository-relative runtime paths.
- `wsdashboard/access.go:204-211` supplies `TrustedWrite`; the dashboard embeds its template and asset families in `pages.go:27`. `wsi18n/i18n.go:30` embeds 11 locale tables. `mcp/playbook_overrides.go:135-139,186` discovers override points and resolves overrides; `playbook_tools.go:22-24` describes seed rendering. `wsdashboard/session_hire.go:398-409` demonstrates daemon mailbox posting; derive active worktree lead from the daemon's registration/index, not a client-supplied mailbox address.

## Decisions

- **(65)=a:** `/settings` is an agent-maintained, opinionated catalog, not an auto-generated schema UI. Localized labels, explanations, and accessibility strings live in dashboard `wsi18n`, never in `config.list`. A registry/manifests-to-UI bidirectional drift guard and locale completeness test enforce maintenance. Add a `summary:`-frontmatter manual `ai-docs/manuals/config-settings-ui.md` and an `AGENTS.md` Implementation Conventions row covering registry/manifests and dashboard settings files.
- **(67)=a:** One view serves global, project, repo, and session. Bottom rail settings opens global. Workspace, worktree, and session row gears open project, repo, and session respectively. The target badge shows the actual resolved path; project explicitly says this machine-wide cache file is shared until `260924-bug-ws-config-project-scope-is-machine-wide` changes storage. No migration here. Global/project/session save directly; repo disk is read-only and edits form a proposal. Session values unsupported by existing writers and global-only keys are disabled with a localized explanation.
- **(69)=a:** Read-only `worktree.profile.*` and `agent.profile.*` summary cards with expandable raw JSON; union all four adapter manifests for every harness, including absent/inactive adapters; prompt override editor shows original seed and live line-by-line seed versus override diff.
- **(70)=ok:** Row gears are borderless, opacity .32 at rest and 1 on row hover, keyboard `:focus-visible`, or active scope; persistent bottom Settings remains visible. Precedence shows clickable read-only builtin, disabled planned synced slot, global, repo, project, session in that order. The global general section starts with a disabled “Git Sync Layer (coming soon)” card referencing `261119-feat-ws-config-git-sync`. Raw keys use `code.key-chip`, separate from translated description. Higher-scope shadow badges also appear in builtin inspection. All Q1–Q8 and G1–G10 are resolved by these decisions and the contracts below.
- **(75)=a — access and inspection:** `/settings` and snapshot reads are restricted to authenticated local control-authorized callers; LAN-token viewers receive 403. Writes also require local same-origin `TrustedWrite`. Redact credentials server-side from all `agent.profile.*` representations and read/validation errors. In opaque `mcp_servers` objects, preserve JSON structure and keys while default-masking scalar leaves as `"[redacted]"`; only explicitly safe metadata may be exposed as specified below. Never transmit raw profile secrets to the browser.
- **Inspection context:** A session entry carries its session and bound worktree; a worktree entry carries that worktree only; project, global and direct entry carry neither. Preserve this context across scope switches, including builtin. Missing context is unevaluated, not an absent override; stale or mismatched IDs produce an explicit notice without substitution.
- **Existing dashboard preferences:** Preserve the footer quick-preferences popover and all its controls and handlers, with a clear additional entry to `/settings`. This also preserves the existing staging location for sync-now and last-sync status in `261119-feat-ws-config-git-sync` Decision 19 until that ticket adopts the full sheet.
- **Compatibility tier T0, bounded:** The new dashboard uses only already-defined `Config` members and existing key/value encodings. Old readers (`wsconfig/config.go:105-121`, `resolver.go:76-131`) read those values; old writers (`config.go:783-805`) retain arbitrary entries in the known maps, and a new reader reopening sees them. Agent tier writes use existing `agents` shapes and must preserve unrelated known leaves; test both directions. No manifest field, schema version, MCP tool/argument, or hello changes. Old daemon/browser pairs never call new `/api/config/*` routes; a new client against an old daemon must detect a 404 and show an unavailable/update state without writes. New routes and assets are additive; all mutations require `TrustedWrite`. No synced/project storage migration. **This is not a claim of general unknown-field preservation:** old global/project struct writers drop unknown top-level and nested struct fields and cannot recover them on reopen. New writes must preserve such fields, but mixed-version preservation of hypothetical future fields is outside this T0 verdict and requires its own compatibility decision; adding such fields here invalidates this verdict. Repo raw-map preservation does not repair old global/project writers. Test the actual old-style rewrite and reopen on the permitted existing shapes.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Read `ai-docs/manuals/release-compatibility.md`, `shipped-surface-boundary.md`, `ws-mcp.md`, and `performance.md` before editing their matched paths; also `skill-authoring.md` before the new manual or AGENTS convention row, and `wsflow-mirroring.md` if mirrored plugin files are touched. Apply each matching `AGENTS.md` Implementation Conventions row.
- Zero file writes on read, including missing files and builtin mode. Keep state, catalog and drafts in memory; no polling. Follow performance manual storage selection and file-touch accounting; invalidate on existing events and explicit save. No config reads on every keystroke, and no per-key repeated filesystem scans.
- Embed or package canonical manifests in the daemon's build-owned assets, with reproducible generation and drift tests. Respect downstream-safe resource boundaries and package-specific ws/wsflow defaults. A standalone release binary with no checkout and no sessions must expose all four namespaces. Do not copy this repository's manuals into shipped behavior.
- Follow approved v2 mockup for layout and interactions; capture and compare side-by-side light and dark at equal viewport and content. Record commands, captures and concrete discrepancies in the Phase 3 result. Honor reduced motion, keyboard navigation and readable contrast.
- API targets are opaque server-issued row identifiers, resolved against live daemon registrations; reject unknown or mismatched workspace/worktree/session relations. Never accept filesystem paths or mailbox recipients as authority from the browser. All user text is rendered as text, not HTML.

## Prior Decisions

- 261119-feat-ws-config-git-sync (2026-11-19, Decisions): "A minimal sync-now button with last-sync status in the existing settings popover, behind a daemon route gated by TrustedWrite; the full settings sheet adopts it later." — bearing: constrains; footer popover and sync staging are preserved
- 261118-feat-ws-embed-rsrc-in-daemon (2026-11-19, commit): "Production playbooks follow the daemon build, not connecting plugin roots." — bearing: constrains
- 261114-refactor-ws-split-oversized-source-files (2026-11-17, commit): "Move-only same-package relocation of config and tuning declarations; no behavior or shared-format change (T0)." — bearing: constrains
- 261116-feat-ws-daemon-plugin-package-select (2026-11-16, Decisions): "The package selection is one global-scope config key stored in the global config.json overrides, written through config.tune." — bearing: supports
- 261020-feat-worktree-profiles (2026-10-20, commit): "The repo writer edits a raw member map instead of round-tripping Config" — bearing: constrains
- 260929-bug-config-tune-agents-tier-lost-update (2026-09-30, commit): "Missing-file no-ops ... check configFileExists before calling the helper, so they create no directory or lock file" — bearing: constrains
- 260619-feat-ws-layered-config-scope-substrate (2026-06-19, Decisions): "Read-modify-write on project and global config must be serialized (flock + temp-write + atomic-rename)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/config_registry.go, agents-plugin-tool/internal/wsconfig/, agents-plugin-tool/internal/wsdashboard/, agents-plugin-tool/internal/wsi18n/ |
| scope.surface | public-interface | new /settings, /api/config/snapshot, /api/config/set, /api/config/reset and /api/config/proposal HTTP contracts |
| scope.new_public_symbol | no | no new exported symbol specified |
| scope.new_type_contract | yes | snapshot, mutation and proposal JSON request and response shapes |
| scope.test_surface | new-files | new browser tests and config route tests in agents-plugin-tool/internal/wsdashboard/ |
| complexity.reuse_points | confirmed | config registry and resolver, TrustedWrite, embedded dashboard assets, mailbox posting and existing browser tests |
| complexity.side_effect_risk | high | configuration writes and mailbox delivery cross session and worktree boundaries |
| risk.correctness | high | scope and bucket precedence, target isolation and unknown-member preservation must hold |
| risk.fit | high | new settings view integrates catalog, navigation, packaged resources and sync plans |
| risk.test | high | cross-version writes, standalone packaging, browser behavior and visual comparison require broad coverage |
| risk.security_or_contract | high | sensitive reads require local control authorization and profile credential redaction; writes and mailbox proposals additionally require TrustedWrite and validated targets |

## Phases

### Phase 1: Shared config service, adapter union and daemon routes

Implement the backend independently of the settings page. Touch `agents-plugin-tool/internal/mcp/config_registry.go`, `server_config.go`, `config_manifest.go`, `server_dispatch.go` and relevant tests as needed; extract reusable catalog, validation, resolution and mutation logic into `agents-plugin-tool/internal/wsconfig` or a new non-cyclic `internal/wsconfigservice` package. Keep legacy MCP response shapes, validation, selectors, lead/session gates and behavior unchanged; exercise existing MCP tests. Touch `internal/wsconfig/{config,resolver,global,repo,structured}.go` only as necessary to expose snapshots and preserve unknown members during new writes under the existing lock/atomic replacement; preserve nested unknown leaves too. Keep repo writes unavailable to the HTTP mutation route. Use existing resolver semantics for bucketed and tier values, including global-only and session eligibility.

Extend `agents-plugin-tool/scripts/generate-package-assets.py`, `internal/wspackage/`, and its tests with a canonical Pi manifest input embedded alongside the selected ws/wsflow package. Keep canonical files the source of truth and test generated bytes against all four inputs; do not require Pi's installed package. Resolve package-specific defaults from the selected package and report conflicting declarations as an error, never silently choose by map iteration. Static registry keys plus discovered prompt/profile families form the service catalog. Add a Go drift-guard scaffold that reads the real source catalog and manifests, normalizes parameterized families, detects duplicate keys, and can compare a supplied UI catalog; phase 2 completes the UI assertions.

Add `internal/wsdashboard/config_routes.go` and register routes with the daemon's existing router. JSON contract:

- `GET /settings?scope={builtin|global|project|repo|session}&target=<opaque-id>&worktree=<opaque-id>&session=<opaque-id>` and `GET /api/config/snapshot` use the same query contract; `scope` defaults to `global`. `target` identifies the selected project/worktree/session for the edit target when applicable and is distinct from inspection context. `worktree` and `session` are optional opaque IDs; session requires its bound worktree. Project/global/direct entry supply neither context ID, repo entry supplies worktree, session entry supplies both. Scope navigation preserves both context IDs, including when selecting builtin; opening a new rail entry replaces context according to its origin. Resolve each ID from the daemon index; reject stale, unknown or mismatched inspection context with HTTP 400 `invalid_context`, without selecting an alternative. A missing or expired required edit target returns HTTP 404 `target_not_found`, including on set/reset. Never treat IDs as paths or session keys.
- Snapshot returns `{scope,target,context:{worktree,session},path,entries:[{key,selectors,default,value,source_scope,source_bucket,layers:[{scope,bucket,evaluated,status,present,value}],editable,disabled_reason}]}`. `path` is the actual target file path for global/project/repo and null for session/builtin. An absent worktree makes repo `evaluated:false,status:"context_missing"`; an absent session does likewise for session. Omit `present` and `value` for unevaluated layers. For partially evaluated precedence, identify any winner as conditional on evaluated layers and never claim a complete effective value or absence of shadow for missing higher layers. Snapshot includes all adapter namespaces; sensitive session keys never leave the daemon. Missing file means no override only for an evaluated layer; malformed files produce a sanitized error. Project target path equals `wsconfig.Path`.
- `POST /api/config/set` body `{scope,target,context:{worktree,session},key,selectors,value}` returns `{entry:<refreshed-entry>,warnings:[{code,message}]}`. `POST /api/config/reset` takes the same body without `value` and returns the same shape. Selectors and values follow the existing registry writer, including structured values and tier leaves. For `agents.tier`, reset accepts `selectors:{tier,harness}` and translates tier to the existing MCP writer's reset `value:{tier}` at the service boundary (`mcp/config_registry.go:524-541`, `server_config.go:638-649`), leaving MCP schema untouched. Share the existing orchestrate unverified-harness whitelist check (`server_dispatch.go:655-658`) with the service: a successful set of `workflow.profile=orchestrate` for an unverified harness returns a non-blocking `warnings` entry with stable code `unverified_harness`; the UI localizes it. Reject builtin/repo and unsupported scope/key combinations; session target resolves to the daemon-owned live session override store.
- Before serving either read, enforce authenticated local control authorization using the existing `canControl`/local-viewer pattern (`access.go:123-166,204-211`); do not require an Origin header on normal same-host top-level GET navigation. Both `/settings` and snapshot return 403 to a LAN-token-only viewer. All POST endpoints require both local and same-origin results from `TrustedWrite` as well as outer authentication, returning 403 on failure. Redact `agent.profile.*` server-side before any snapshot field, layer, profile summary, error or log emitted from this API. Outside `mcp_servers`, retain standard profile metadata but recursively mask scalar leaves beneath token/api_key/secret/password/authorization-like keys (case-insensitive, normalized separators). Inside each opaque `mcp_servers.<server>` subtree, recursively preserve object/array structure and key names while masking **all** scalar leaves as `"[redacted]"` by default. The only allowlisted scalar metadata is `type`, `transport`, and the executable name alone from `command`; strip any path, arguments or other command content before exposing that name. Never expose `args` leaves, `url`, `env`, `headers`, or unknown scalar leaves. Apply the same allowlist and default mask to errors; malformed JSON and validation errors must not echo raw profile input or opaque values. Do not persist the redacted copy. `wsagent/profile.go:95-98,269-278,332-357` shows arbitrary MCP server objects and validation; `wsagent/claude.go:262-274` forwards server objects unchanged.
- Errors are JSON `{error:{code,message}}`: 400 `invalid_request` for malformed JSON/selectors/key/value/scope, 403 `forbidden` for failed `TrustedWrite`, 404 `target_not_found` for a missing/expired edit target, 400 `invalid_context` for stale, unknown, missing or mismatched inspection context, 409 `scope_unsupported` for known but nonwritable scope/key, 500 `config_error` for read/write failure. Reject wrong HTTP methods with 405. No partial writes on error. Reads must not create dirs, locks, files, or mailbox state.

Tests: LAN-token viewer gets 403 on both settings and snapshot; snapshot layers and error strings never contain fixture secrets from nested agent profile env, headers, credential keys, an `args` entry `--api-key=secret-123`, or an endpoint `url` `https://user:secret@example.com/mcp?token=abc`, while preserving JSON shape and only exposing allowlisted safe metadata; authenticated local reads and same-origin writes succeed. Exercise direct/no-context, repo-to-builtin and session-to-builtin snapshots, stale and mismatched context, and verify conditional effective/shadow results. Reset one agents.tier harness/tier without changing siblings; orchestrate on an unverified harness emits the warning without blocking save. Also cover table-driven scope/target/selector validation, actual path and precedence including harness-specific bucket versus common scope, session isolation, error/security and repo denial, existing unknown-member preservation, old-writer/new-reader compatible data, and manifest union in an embedded-asset subprocess without source checkout. Verification from repository root: `python3 agents-plugin-tool/scripts/generate-package-assets.py`, `python3 agents-plugin-tool/scripts/generate-package-assets.py --check`, `python3 -m unittest discover -s agents-plugin-tool/scripts -p 'test_generate_package_assets.py'`, `cd agents-plugin-tool && go test ./internal/wsconfig/... ./internal/mcp/... ./internal/wspackage/... ./internal/wsdashboard/...`.

### Result (d347e280f) - 2026-11-21

- Added authenticated settings snapshots and same-origin set/reset. The service
  exposes evaluated layers, exact overrides, conditional winners, and sanitized
  profile values. Each snapshot reads applicable file layers once and reads one
  session lineage without updating its activity. Repo changes use proposals.
- Embedded the canonical Pi manifest alongside the other adapter manifests in
  both packages (`2cee6b4bc`). A standalone ws-mcp started without a checkout
  or a session and returned 12 Claude, 2 AgY, 5 Dashboard, and 15 Pi entries.
- Package generation, `--check`, and the 10 generator tests passed. The full
  wsconfig and wspackage suites and the scoped MCP config and dashboard route
  tests passed. The compatibility test verified unknown member preservation and
  reopen after a legacy struct writer rewrote the supported shape.
- The full Go sweep reproduced the Explore delegation-tip and tracker related
  scope failures on `develop` (`1c180f21a`). The broad MCP run also timed out
  during a tracker Git fixture. The lead accepted scope-specific verification
  after the baseline comparison.

### Phase 2: Settings view, navigation and catalog

Depends on Phase 1. Touch `internal/wsdashboard/pages.go`, `assets/dashboard.tmpl`, `assets/dashboard-rail.js`, `assets/dashboard-rail.css`, new `assets/dashboard-settings.js` and `assets/dashboard-settings.css`, page/route tests, and all 11 `internal/wsi18n/tables/*.json` with `wsi18n` tests. Serve `/settings` inside the existing dashboard shell and support direct navigation, back/forward and contextual reload. The rail opens the selected target scope with its opaque ID, preserving existing row navigation. Give every gear a localized accessible name, native button semantics and visible focus; no row click propagation. Keep the global bottom button persistent, with a clear `/settings` entry alongside the existing footer quick-preferences popover. Preserve layout mode, input behavior, theme, UI language, login link, LAN controls, and their existing handlers; retain the sync staging path under Decision 19.

Maintain an explicit UI catalog mapping every static registry key and every manifest key to a section, control, label/description i18n IDs and selectors; represent dynamic prompt/profile families explicitly. Implement workflow/general, tiers, compaction and all four adapter sections. Use correct types, limits, enums and per-scope edit restrictions from the shared service; unrecognized values remain inspectable, never silently overwritten. Per-key controls expose `data-config-key`, scope selectors `data-settings-scope`, and the page root `data-settings-view`; raw names use `code.key-chip`. Render provenance, selected-layer override and higher-scope shadow badges from backend resolution, including bucket name; builtin is read-only and shows shadows. In both precedence strip and each row, render context-missing layers as localized “컨텍스트 미선택 (미평가)” (not evaluated), never no-override. Show conditional effective values and invalid/missing-context notices, never choose a sibling target. Preserve context in URL and snapshot requests across every scope switch and browser history. Render structured save warnings localized and non-blocking. The planned synced slot and global general sync card are visibly disabled and never issue requests. Show actual target path and machine-wide project explanation. Set/reset updates just the affected entry on success; errors keep the user's input and show a localized message. Save controls do not write on render. Before Phase 3, repo controls are disabled with an explanation rather than writing.

Finish the Go drift guard: compare UI catalog to the registry and four actual embedded manifests in both directions, allow only declared parameterized families, fail on duplicate/missing/extra keys, and verify every referenced label, description, disabled reason and common UI string exists and is nonempty in each of 11 locale tables. Include tests deliberately removing a key and locale translation to prove failure. Test preserved footer-popover controls and handlers, rail hover/focus/active CSS and click routing, direct/no-context and repo/session-origin builtin inspection with context preservation and correct unevaluated markers, scope-specific controls, paths, bad requests and old-daemon 404 fallback. Verification: `cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsi18n/... ./internal/mcp/... ./internal/wspackage/...` and `node --test internal/wsdashboard/frontend-*.test.js` from `agents-plugin-tool`.

### Result (ed920bb24) - 2026-11-21

- Shipped the scoped settings frame, rail entry points, typed controls, conditional
  provenance, and catalog drift checks across both packages and all 11 locale
  tables. The old preferences popover retains its controls and handlers.
- The worktree pool is read-only in the catalog because it participates in
  resolution through worktree tools and has no config.tune writer. Review follow-up
  `ca0597e311` retains the project edit target in browser history and refreshes
  the other selectors affected by a bucket write.
- Scoped dashboard settings, shell and rail layout tests passed. The full wsi18n
  suite and all five settings Node tests passed. The full frontend run passed 31
  tests and reproduced three existing failures on `develop` in the sheet opener,
  lifecycle layout, and idle CSS fixtures.

### Phase 3: Repo proposals, prompts, profiles and visual acceptance

Depends on Phases 1–2. Touch `internal/wsdashboard/config_routes.go`, settings assets/template, daemon mailbox/index integration, relevant `internal/mcp/playbook_overrides.go` shared extraction and tests, `internal/wsi18n/tables/*.json`, `ai-docs/manuals/config-settings-ui.md`, and `AGENTS.md`. The manual's `summary:` and convention row cover `agents-plugin-tool/internal/mcp/config_registry.go`, `agents-plugin/{config-manifest,agy-config-manifest,dashboard-config-manifest}.json`, `agents-plugin-pi/config-manifest.json`, and `agents-plugin-tool/internal/wsdashboard/` settings UI files; explain catalog/locale/manifest updates and drift verification. Follow skill authoring rules for the row and manual.

Repo edits are in-memory per target and represented as ordered, deduplicated `{key,selectors,action,value}` operations; reset is an explicit operation and re-edit replaces the previous operation for the same key/selectors. Show sticky proposal bar, count, expandable human-readable patch, clear, and unsaved navigation warning; never write the repo file. `POST /api/config/proposal` takes `{target,operations:[...]}` and uses the same validation as mutation, rejects empty or invalid operations, and requires `TrustedWrite`. The daemon resolves the selected worktree and an active lead at send time; choose the active lead deterministically from the existing index, never a child or another worktree. Post one structured message containing worktree identity, actual repo config path, operations and a request for the lead to review/apply; invoke the existing mailbox wake path. Return `{delivered:true}` only after successful post. If the lead disappeared, return 409 `{error:{code:"no_active_lead",message}}`; retain draft and offer copy. Other failures retain draft and show error; never claim delivery. When no active lead is present, offer copying the same structured JSON plus readable diff to clipboard, with accessible manual selection fallback. Do not include session credentials in message or clipboard. Verify mailbox delivery, target isolation, stale lead, no-lead copy and no repo file writes.

Expose seed plus scoped override data from the same daemon-owned packaged playbook resources and discovery logic as the MCP renderer. Prompt UI shows a read-only original seed, harness selector and editable override, with a live escaped, line-by-line seed-to-override diff updated in memory without losing focus; set/reset obey scope semantics, with repo routed to proposal. Render profile summaries from effective `worktree.profile.*` and `agent.profile.*` data, with accessible collapsible server-redacted raw JSON and no editing affordances, including repo and session views.

Add Playwright E2E using the repository's existing dashboard browser-test harness and isolated temporary homes/cache: test direct and rail navigation, all five inspection modes, write/reset and forbidden origins, project path, disabled scope explanations, repo send/copy/no-write, seed diff, profile expansion, keyboard focus and all adapter namespaces without an active session. Take light/dark captures at the mockup's viewport and compare side-by-side against `ai-docs/assets/mockups/261121/config-settings-ui/mockup.html`, including rail gears, cards, chips, badges and proposal bar; record viewport, capture paths, differences and fixes. Verification: `cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsconfig/... ./internal/mcp/... ./internal/wsi18n/...` and run the added browser tests with `go test ./internal/wsdashboard -run 'TestSettings.*Browser' -count=1`; run `python3 agents-plugin-tool/scripts/generate-package-assets.py --check` from the repository root. Browser dependencies and capture commands must be documented in the test or phase result; do not mark visual acceptance complete without inspecting both comparisons.

### Result (2cbbee08a) - 2026-11-21

- Delivered in-memory worktree proposals with explicit reset operations, mailbox
  delivery to the active lead, and copy and manual selection fallback. No repo
  config file writes occurred. Prompt controls show the packaged seed and an
  escaped live line diff. Profile cards show server-redacted metadata and JSON.
- The independent Lite review's findings were addressed in `ca0597e311`. Tests
  cover common and specific bucket refresh, inherited and local session values,
  legacy tier seeds and single-leaf reset, project navigation, and Chromium with
  both the packaged catalog and the production session store.
- `go vet ./...` passed in `agents-plugin-tool`. Full wsconfig, wspackage, and
  wsi18n suites passed. Scoped Go tests passed for MCP config and prompts,
  dashboard settings and rail layout, agent profiles and tiers, and the daemon
  command. The opt-in legacy empty-root browser fixture reproduced the existing
  deferred language reload on `develop` when waiting for the active landing
  frame. The settings-specific browser tests both passed.
- Browser verification used cached Playwright and Chromium 1223 with
  `PLAYWRIGHT_BROWSERS_PATH` pointing at the installed browsers. From the module
  directory, run
  `WS_PLAYWRIGHT_MODULE=<cached-module> WS_CONFIG_SETTINGS_CAPTURES=/tmp/config-settings-ui-captures go test ./internal/wsdashboard -run '^TestSettings(Interaction|Production)Browser$' -count=1 -v`.
  Both themes used 1440×960 and the same workflow controls and two repo draft
  operations. The reference in each pair was rendered from
  `ai-docs/assets/mockups/261121/config-settings-ui/mockup.html`.

| Scope and theme | Implementation | Reference | Inspected result |
|---|---|---|---|
| Global dark | `/tmp/config-settings-ui-captures/implementation-global-dark.png` | `/tmp/config-settings-ui-captures/reference-global-dark.png` | Layout and interaction states match |
| Global light | `/tmp/config-settings-ui-captures/implementation-global-light.png` | `/tmp/config-settings-ui-captures/reference-global-light.png` | Layout and interaction states match |
| Repo dark | `/tmp/config-settings-ui-captures/implementation-repo-dark.png` | `/tmp/config-settings-ui-captures/reference-repo-dark.png` | Layout and expanded proposal state match |
| Repo light | `/tmp/config-settings-ui-captures/implementation-repo-light.png` | `/tmp/config-settings-ui-captures/reference-repo-light.png` | Layout and expanded proposal state match |

The first comparison exposed a collapsed rail, a wrapped target label, and a
proposal bar below the viewport. The final captures show the full-height rail,
a single-line target label, the aligned settings stage, and a sticky proposal
bar with a visible primary action. The live page shows actual paths and session
rows, retains the quick-preferences entry, and adds the required context and
conditional provenance. The repo preview renders ordered diff operations. The
mockup uses a JSON object for the same example edits. Captures remain outside Git.

## Verification

- Phase 1 service and handler tests must assert LAN token-only 403 on both reads, local control access, same-origin write authorization, default-mask opaque MCP server leaves, including credential-bearing args and URL, in snapshot and errors, context validation and conditional resolution, single tier/harness reset isolation, and non-blocking orchestrate warnings. Run the Phase 1 commands above.
- Phase 2 UI tests must exercise direct settings without context, repo-origin → builtin and session-origin → builtin with preserved context, correct unevaluated markers in strip and rows, conditional shadow attribution, localized warning display, and all existing footer-popover preferences and handlers. Run the Phase 2 commands above.
- Phase 3 browser tests must verify the credential-bearing args and URL are absent from the read-only raw JSON inspector, snapshot and error strings, and repeat the authorization, redaction and context paths, proposal flow, footer controls and light/dark visual comparisons; run the Phase 3 commands above and record captures and inspection results.
