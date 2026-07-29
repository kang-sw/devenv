# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 2: agent spawn path from the browser

## Relevant Ticket Contract

- Add an optional profile selector to `CreateTerminalRequest`, resolved against
  a small vendor profile registry (command argv, env scrub list, hook config
  shape). Absent profile keeps today's shell behaviour byte for byte.
- Wire the top-right toolbar slot vacated by the Tier 1 agent-GUI suspension
  (`c3f5b42b`) to this spawn path. Must NOT route through
  `registerNewAgentChatPane` (one of the three `AGENT_GUI_SUSPENDED` guard
  depths — it would either no-op or re-open the suspended surface).
- Resulting pane is `SurfaceKind: "persistentTerminal"`; the pane must record
  which profile produced it, so Phase 7 can tell an agent terminal from a
  shell terminal and not double-count it against the nav ticket's terminal
  count.
- Verification: a browser acceptance step spawning a terminal under a DUMMY
  profile (trivial local command, never a real vendor CLI) asserting the pane
  opens with the profile recorded, plus an existing-shell-terminal regression
  step. The acceptance suite must not acquire a dependency on a vendor binary,
  credentials, or network.
- Owns the `#260516-ws-web-dashboard-terminal-pane` spec amendment: the pane
  is "a shell terminal substrate only; it does not hardcode Codex, Claude, or
  other agent presets" (`ai-docs/spec/ws-web-dashboard/index.md:2109-2110`) —
  tier this sentence to permit a sibling profile over the same single-sourced
  plumbing, do not delete it.
- HARD CONSTRAINT (`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`):
  no new field on `TerminalRegistryEntry`
  (`ws-dashboard/crates/daemon/src/terminal_registry_file.rs:16-27`).
- Helper argv is world-readable via `ps`; it must carry file PATHS only, never
  secret values. `--env-overlay` must not become a secret channel (Phase 4's
  callback token in particular). An env overlay may not resurrect a scrubbed
  marker — the scrub always wins (Phase 1 Result, decision 5).
- Constraint: "No PTY wheel reinvention" — this ticket adds argv/env
  passthrough and a config-file write at the EXISTING spawn seam; it must not
  fork the terminal substrate or add a second helper/PTY implementation.

## Out of Scope

- Phase 3 steps 2-3 (hook config materialization under
  `agent-profiles/<terminal_id>/`, the `ws-dashboard terminal-notify`
  subcommand) and all later phases (4-8: token store/callback route, SSE
  stream, tab-label indicator, nav-row presentation, browser notifications).
- Any `TerminalRegistryEntry` schema change.
- Any real vendor-CLI dependency in the automated acceptance test (a manual
  recorded run with a real agent CLI is Phase 6's job, not Phase 2's).
- Codex profile (Deferred scope — Claude-only in this ticket family).
- Building the actual hook config shape/materialization; Phase 2's registry
  only needs a placeholder/unwired shape field for it, matching "hook config
  shape" language in the ticket's Phase 2 bullet — it is not invoked yet
  because Phase 3 hasn't landed the file-write step.

## Codebase Findings

- `ws-dashboard/crates/daemon/src/agent_env_profile.rs:8-19` — `EnvScrubProfile`
  (`name`, `markers: &'static [&'static str]`) and the populated `CLAUDE`
  constant (11-marker deny-list). Header CONTRACT comment explicitly names
  this as the seam Phase 2's profile registry composes over ("adding a second
  vendor's `EnvScrubProfile` here, or promoting this into a registry keyed by
  profile id, is Phase 2's job, not a rewrite of this seam"). Reuse
  `scrub_env_os` and `CLAUDE` directly; do not re-derive the marker list.
- `ws-dashboard/crates/daemon/src/terminal.rs:561-570` — `CreateTerminalRequest`
  currently `{ columns, rows, title, cwd_hint }`. Add `profile_id:
  Option<String>` (serde `profileId`, `#[serde(default)]`) here.
- `ws-dashboard/crates/daemon/src/terminal.rs:632-677` (`create_terminal`
  handler) and `terminal.rs:646-661` — today hardcodes `None, Vec::new()` for
  `command`/`env_overlay` with a CONTRACT comment ("no profile selector
  exists on `CreateTerminalRequest` yet — Phase 2 wires a real value"). This
  is the exact resolve-and-forward point: look up `request.profile_id` against
  the new registry, and only pass `Some((program, args))` / a non-empty
  overlay when a profile resolved.
- `ws-dashboard/crates/daemon/src/terminal.rs:903-987` (`TerminalSession::spawn`)
  already accepts `command: Option<(String, Vec<String>)>` and
  `env_overlay: Vec<(String, String)>` (Phase 1 surface) and forwards them
  into `build_helper_command` (`terminal.rs:820-881`), which does the
  hop-1 scrub via `agent_env_profile::scrub_env_os(host_env, &CLAUDE)`
  (`terminal.rs:871-874`). Phase 2 does not need to touch this signature —
  only the caller (`create_terminal`) needs to populate the two args from a
  resolved profile.
- `ws-dashboard/crates/daemon/src/terminal.rs:465-475` — `TerminalSessionView`
  (the ONLY browser-facing session wire type, serialized via `session.view()`
  at `terminal.rs:1024-1036`, the single construction site). No `profile`
  field today. This is the correct place for provenance — see "Provenance"
  design answer below.
- `ws-dashboard/crates/daemon/src/terminal.rs:429-440` — `TerminalSession`
  struct (in-memory, per-daemon-process, never persisted). Add a
  `profile_id: Option<String>` field here (mirrored onto
  `TerminalSessionView`, populated in both `from_connection` call sites —
  see next finding).
- `ws-dashboard/crates/daemon/src/terminal.rs:989-1022` (`from_connection`) —
  called from TWO sites: (a) `TerminalSession::spawn` for a fresh
  browser-requested create (`terminal.rs:977`), and (b) `reconcile_entry`'s
  `AdoptLive`/`AdoptGrace` arm on daemon restart (`terminal.rs:221-256`,
  specifically `terminal.rs:242-255`), which builds its session ONLY from a
  `TerminalRegistryEntry` — a struct that (per the hard constraint) will
  never carry a profile id. Confirmed by reading `TerminalRegistryEntry`
  (`terminal_registry_file.rs:16-27`): no profile field exists or may be
  added. **This is the provenance/registry-constraint tension** — see design
  answer 2 below; `from_connection`'s adopt call site has no profile value to
  pass and must pass `None`.
- `ws-dashboard/crates/daemon/src/terminal.rs:196-256` — `boot_reconcile` /
  `reconcile_entry`: confirms adoption path never touches the browser-facing
  `CreateTerminalRequest` or any profile-carrying struct; it is keyed
  entirely off the on-disk registry entry.
- `ws-dashboard/frontend/src/agentGuiSuspended.ts:1-8` — `AGENT_GUI_SUSPENDED =
  true`. Its own comment enumerates the THREE suspended depths: toolbar
  button, `a n` hotkey, and `registerNewAgentChatPane` itself. Phase 2's new
  spawn path must not touch any of the three — it is a parallel path through
  `terminal.create`-family plumbing instead.
- `ws-dashboard/frontend/src/App.tsx:5411-5442` (`registerNewAgentChatPane`)
  — confirmed as one of the three guard depths (`App.tsx:5420`:
  `if (AGENT_GUI_SUSPENDED) { return pane; }` — belt-and-suspenders no-op).
  Do not call this function or anything that calls it
  (`createAgentChatPane` at `App.tsx:5444-5451`,
  `forkAgentChatFromBubble` at `App.tsx:5641`).
- `ws-dashboard/frontend/src/App.tsx:6579-6616` — the exact vacated toolbar
  slot: `terminal.create`'s `ChromeIconButton` (icon `SquareTerminal`,
  6579-6594) sits directly beside the now-`null`-rendered `agentChat.create`
  button (6596-6616, still guarded by `{AGENT_GUI_SUSPENDED ? null : (...)}`,
  disabled on the same `root.activation`/`root.availability` condition). This
  is the toolbar row the ticket means by "vacated slot" — add a new
  `ChromeIconButton` here (reusing the same disabled condition), NOT inside
  the `AGENT_GUI_SUSPENDED ? null : (...)` branch.
- `ws-dashboard/frontend/src/terminals.ts:82-85` — `TerminalCreateOptions =
  { title?, cwdHint? }`; add `profileId?: string`.
- `ws-dashboard/frontend/src/terminals.ts:205-233` (`createTerminal`) — POST
  body is built inline (`columns, rows, title, cwdHint`); add
  `profileId: options.profileId ?? null` conditionally (omit key or send
  `null` — daemon field is `#[serde(default)]` so either is safe, but keep it
  present-when-set to match existing `cwdHint` pattern).
- `ws-dashboard/frontend/src/App.tsx:5364-5399` (`createTerminalPane`) — the
  existing browser-side flow: `createTerminal` → `terminalPaneFromSession`
  → `setTerminalPanes` → `placeTerminalSessions` → focus. Phase 2's new agent
  spawn action should call this SAME function with
  `{ profileId: "claude" }` (or whatever profile id is chosen for the real
  button) rather than duplicating pane-registration logic.
- `ws-dashboard/frontend/src/commands.ts:41`, `:113`, `:547-555`, `:767-768`
  — `terminal.create` command id/payload/builder/log-label 4-tuple pattern.
  Payload is `{ type, serverRoute, workRootId }` — no path/secret data, so a
  `profileId` string is safe to add to a payload by the same "logical
  dashboard target, not host path" rule already governing this file
  (`ws-web-dashboard/index.md:97`).
- `ws-dashboard/frontend/src/workbench/surfaceRegistry.ts:1-12,52-59` —
  `SurfaceKind` includes `"persistentTerminal"` already;
  `persistentTerminal`'s registry entry (`rowPolicy: "pinned"`,
  `lifecycleOwner: "daemonProcess"`, `closePolicy: "detachDaemonResource"`,
  `closeConfirmationPolicy: "confirmSessionClose"`) needs NO change — an
  agent terminal is registered as an ordinary `persistentTerminal`, exactly
  per the ticket's "first spawn produces an ordinary terminal pane" decision.
- `ws-dashboard/frontend/src/workbench/terminalPlacement.ts:1-56` —
  `placeTerminalSessions`/`terminalPlacementState` place every
  `TerminalSessionView` (fresh or restored) uniformly as
  `surfaceKind: "persistentTerminal"`; no profile-aware branching exists or
  is needed here — placement stays profile-blind by design.
- `ws-dashboard/crates/daemon/src/router.rs:79-94` — `AppState` fields; no
  natural slot for request-scoped, stateless data. The profile registry is
  better as a pure static lookup table (mirroring `agent_env_profile::CLAUDE`)
  referenced directly from the `create_terminal` handler, not a new
  `AppState` field — it holds no runtime state, only compile-time data.
- `ai-docs/spec/ws-web-dashboard/index.md:2092-2117` (`Terminal Pane`
  spec) — exact sentence to tier: "The terminal pane is a shell terminal
  substrate only; it does not hardcode Codex, Claude, or other agent
  presets." (line 2109-2110).
- Risk signal: `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` has no
  existing precedent for a daemon-side "test-only" registry entry gate (no
  `WS_DASHBOARD_TEST_*`-style env var controls daemon-internal feature
  registries; the existing `WS_DASHBOARD_TEST_*` vars only steer the Node
  harness's own fixture workRoot paths). A dummy profile therefore cannot be
  conditionally compiled out for the acceptance run without inventing a new
  daemon-side test-gating mechanism this ticket does not otherwise need —
  see design answer 3 for the resolution (always-registered but
  UI-unlisted).

## Implementation Plan

1. **Daemon: profile registry module.** Add
   `ws-dashboard/crates/daemon/src/agent_profile_registry.rs` (new file,
   sibling to `agent_env_profile.rs`). Define:
   ```rust
   pub struct AgentProfile {
       pub id: &'static str,
       pub command: &'static str,
       pub args: &'static [&'static str],
       pub scrub: &'static crate::agent_env_profile::EnvScrubProfile,
       // Placeholder for Phase 3; unused until hook materialization lands.
       pub hook_config: Option<HookConfigShape>,
   }
   pub fn resolve(profile_id: &str) -> Option<&'static AgentProfile>;
   ```
   Populate exactly two entries: `"claude"` (`command: "claude"`, empty
   `args`, `scrub: &agent_env_profile::CLAUDE`, `hook_config: None`) and a
   dummy test entry, e.g. `"dummy-echo"` (`command`: a portable no-vendor
   local command — see step 2 for the exact choice — `scrub`: an empty/no-op
   `EnvScrubProfile`). `resolve` is a pure `match`, unit-testable without I/O.
   CONTRACT comment: mirror `agent_env_profile.rs`'s header — this module is
   the registry the earlier seam's CONTRACT comment predicted, and Phase 3
   extends `hook_config` rather than replacing this shape.
2. **Dummy profile command choice.** Use a cross-platform portable command
   with no vendor dependency and no network. `args`/`command` here is a
   literal argv (not a shell string, since `build_helper_command` invokes
   `std::process::Command::new(program).args(args)` — no shell interposed),
   so pick a real executable rather than a shell one-liner: reuse whatever
   trivial cross-platform binary `terminalCommandPlanForPlatform`
   (`ws-dashboard/frontend/src/terminalCommandPlan.ts`) already relies on for
   POSIX/Windows-split trivial commands in this same acceptance suite, so
   Phase 2 does not invent a second split. Keep the command's only job "run
   and stay alive long enough to be observed as `Running`, print one
   recognizable marker line" — the acceptance assertion only needs to see the
   pane open with the recorded profile, not exercise real shell interaction.
3. **Daemon: wire `CreateTerminalRequest`.** In `terminal.rs:561-570` add
   `profile_id: Option<String>` (`#[serde(default, rename = "profileId")]` or
   equivalent camelCase mapping consistent with the struct's existing
   `#[serde(rename_all = "camelCase")]`). In `create_terminal`
   (`terminal.rs:632-677`), before calling `TerminalSession::spawn`: if
   `request.profile_id` is `Some(id)`, call `agent_profile_registry::resolve`;
   on `None` lookup result (unknown id), return a `TerminalError::BadRequest`
   (mirrors the existing `validate_size` early-return pattern at
   `terminal.rs:642-644`); on a resolved `Some(profile)`, build
   `command = Some((profile.command.to_owned(), profile.args.iter().map(|a|
   a.to_string()).collect()))` and pass it in place of the current hardcoded
   `None`. `env_overlay` stays `Vec::new()` — Phase 2 does not populate it (no
   secret/value needs to travel yet; Phase 4 owns the callback token and is
   explicitly barred from `--env-overlay` regardless). When
   `request.profile_id` is `None` (absent from the request body), behavior is
   BYTE FOR BYTE unchanged (same `None, Vec::new()` call as today) — this is
   the "absent profile" contract and must be a literal no-branch-taken path,
   not a branch that happens to compute the same values.
4. **Daemon: provenance field.** Add `profile_id: Option<String>` to both
   `TerminalSession` (`terminal.rs:429-440`) and `TerminalSessionView`
   (`terminal.rs:465-475`, camelCase `profileId`). Thread it through
   `from_connection`'s parameter list (`terminal.rs:989-998`) and its TWO call
   sites: `TerminalSession::spawn` (`terminal.rs:977-986`, pass
   `request`-derived `Some(profile.id.to_owned())` or `None`) and
   `reconcile_entry`'s adopt arm (`terminal.rs:242-255`, pass `None` —
   `TerminalRegistryEntry` has no profile data to recover, per the hard
   constraint). Update `view()` (`terminal.rs:1024-1036`) to copy the field
   through. This is the full blast radius — `TerminalSessionView` has exactly
   one construction site (confirmed by search).
5. **Frontend: types and API.** Add `profileId?: string` to
   `TerminalCreateOptions` (`terminals.ts:82-85`) and thread it into the POST
   body in `createTerminal` (`terminals.ts:205-233`). `TerminalSessionView`'s
   TypeScript type (mirroring the Rust struct, defined in or re-exported near
   `terminals.ts`) gains `profileId: string | null` to match the new Rust
   field; this flows automatically into `TerminalPaneState.session.profileId`
   via `terminalPaneFromSession` (no separate pane-level field needed — the
   session view IS the pane's `session` property, per `terminals.ts:53-54`).
6. **Frontend: command + button.** Add `"terminal.create.agent"` to
   `DashboardCommandId` (`commands.ts:41`) and a payload variant
   `{ type: "terminal.create.agent"; workRootId: string; serverRoute: string
   }` (`commands.ts:113`) — profile id is NOT in the payload; it is a fixed
   dispatch-time constant (`"claude"`) chosen by the handler, keeping the
   command payload shape parallel to `terminal.create`'s and avoiding a
   free-text profile field in loggable command payloads. Add
   `buildTerminalCreateAgentCommand` mirroring `buildTerminalCreateCommand`
   (`commands.ts:547-555`) and a log-label case (`commands.ts:767-768` area,
   e.g. `"Create agent terminal"`). In `App.tsx`, add a handler analogous to
   `onCreateTerminal` (see `App.tsx:6445,6456,6291`) that calls
   `createTerminalPane({ profileId: "claude" })`. Add the new
   `ChromeIconButton` at `App.tsx:6579-6616`, positioned after the existing
   `terminal.create` button and BEFORE (or replacing) the
   `{AGENT_GUI_SUSPENDED ? null : (...)}` block — do not nest it inside that
   conditional, since it must render regardless of `AGENT_GUI_SUSPENDED`.
   Reuse the same `disabled={root.activation !== "online" ||
   root.availability !== "available"}` condition. Pick an icon distinct from
   `SquareTerminal`/`MessageSquarePlus` (e.g. `Bot`, from the already-imported
   `lucide-react` set — confirm availability before use) and label "New agent
   terminal".
7. **Frontend: dummy-profile UI exposure.** Do NOT add a second button or a
   profile picker for `"dummy-echo"` — it must stay absent from every
   user-facing control (toolbar, command palette, hotkeys) per design answer
   3. Only the daemon-side registry and the Playwright test know its id.
8. **Spec amendment.** Edit
   `ai-docs/spec/ws-web-dashboard/index.md:2109-2110` — tier the sentence to
   something like: "The terminal pane substrate is shell-neutral: it does not
   hardcode a specific agent preset, but MAY be spawned with a resolved
   vendor profile (command argv, env scrub, provenance) over the same
   single-sourced plumbing; see `#260516-ws-web-dashboard-terminal-registry-pty-spawn`
   for the profile registry." Keep the anti-fork clause explicit (no second
   helper kind, no parallel PTY implementation). Also touch
   `#260516-ws-web-dashboard-terminal-registry-pty-spawn`
   (`ai-docs/spec/ws-web-dashboard/index.md:2034` area) to add one sentence
   naming the optional `profileId` request field and that an absent profile
   is unchanged shell behavior.
9. **Ticket bookkeeping.** After landing, append a Phase 2 `### Result`
   section to
   `ai-docs/tickets/ready/260725-feat-dashboard-pty-agent-attention-notification.md`
   per existing Phase 1 `### Result` style (edition hash, files touched,
   verification, non-vacuity note, review outcome placeholder for the review
   cycle).

## Design Question Answers (required by the survey brief)

1. **Registry location and wire representation.** Daemon-side only, as a
   pure static Rust table (`agent_profile_registry.rs`, mirroring
   `agent_env_profile.rs`'s existing `EnvScrubProfile` seam — its own header
   CONTRACT comment predicts exactly this). No frontend-side registry/mirror
   is needed because the frontend never needs to know a profile's command
   argv or scrub list, only its opaque id. Wire representation is an opaque
   string id (`profileId: "claude"`) in the `CreateTerminalRequest` JSON body
   and echoed back read-only on `TerminalSessionView.profileId` — an opaque
   id is safer than a raw command string because it keeps argv/scrub policy
   a daemon-only decision (a browser-supplied literal command would need its
   own validation/allowlist to avoid becoming an arbitrary-process-spawn
   vector, which is strictly more risk for zero benefit here).
2. **Provenance storage and restart lifetime.** Store `profile_id: Option<String>`
   on the in-memory `TerminalSession` / browser-facing `TerminalSessionView`
   (NOT on `TerminalRegistryEntry`, which is hard-forbidden and is also the
   thing that survives to disk). Consequence, stated plainly: profile
   provenance does NOT survive a daemon restart. `reconcile_entry`'s adopt
   path reconstructs a `TerminalSession` purely from the on-disk
   `TerminalRegistryEntry`, which never carries a profile id, so a
   re-adopted agent terminal reports `profileId: null` after a restart even
   though the running process is still the agent CLI. This mirrors an
   already-accepted pattern in this same ticket: turn state itself resets to
   `idle` on adoption ("a turn boundary crossed while the daemon is down is
   LOST... Pinned default: on adoption... state is `idle`, and the next hook
   corrects it"). Profile provenance has no equivalent self-correction signal
   (nothing re-announces "I am a claude profile terminal" after adopt), so
   Phase 7's post-restart agent counter will UNDER-count until that terminal
   is closed and a fresh one spawned. This plan does not invent a fix (e.g.
   sniffing the process's own argv/cmdline via OS APIs) because that is a new
   mechanism, not "extend the seam," and the ticket's constraints prioritize
   the no-registry-field rule over restart-survival of this specific
   metadatum. This limitation must be stated explicitly in the Phase 2
   `### Result` and is a reasonable candidate for a follow-up `idea/` ticket
   rather than a blocker here — the ticket's own turn-state design already
   accepts the identical class of restart-loss for a value one phase over,
   so treating profile provenance the same way is a consistent, not a novel,
   risk acceptance.
3. **Dummy profile packaging.** A real registry entry (`"dummy-echo"`),
   always present in the compiled daemon binary (no `#[cfg(test)]` gate is
   viable — see Codebase Findings: the acceptance suite drives the real
   production binary via `daemonHarness.ts`, not a test-cfg build). It ships
   in the registry but is kept OUT of every user-facing surface: no toolbar
   button, no command-palette entry, no hotkey references it (only
   `"claude"` does, per step 6). The Playwright acceptance test reaches it by
   calling the terminal-create HTTP route directly (authenticated `fetch`
   from the test, exactly like other direct-API-plus-browser-assertion
   patterns already used in this suite for fixture setup) with
   `profileId: "dummy-echo"`, then asserts the resulting pane's presence and
   recorded profile through the real rendered DOM — satisfying the
   mental-model's "browser-level verification, not curl-only" rule for the
   UI half while not requiring a UI control to exist for an id that must
   never be user-visible. This is "a real registry entry marked for
   testing," the third option the survey brief offered, chosen over a
   fixture-injected registry (would need a new daemon test-mode flag this
   ticket doesn't otherwise need) and over a hard `#[cfg(test)]` entry (not
   reachable from the production binary the acceptance suite runs).
4. **Testing "absent profile unchanged" as a mutation-caught guard, not an
   assertion.** Phase 1 already hit this exact vacuity trap once (`terminal.rs`
   Result section, finding 4: `get_envs()`/`iter_extra_env_as_str()` could
   not distinguish "no env manipulation" from "clear then re-add," and the
   original guard was silently vacuous until replaced). Phase 2's equivalent
   guard is at a different layer (`CreateTerminalRequest` → `create_terminal`
   → `TerminalSession::spawn` call arguments) and needs its own non-vacuous
   proof:
   - Unit test: `profile_id: None` in `CreateTerminalRequest` must produce
     the IDENTICAL `command = None, env_overlay = Vec::new()` call arguments
     that `create_terminal` passed before this phase — assert this at the
     smallest testable seam (extract the "resolve profile_id to (command,
     overlay)" logic as a pure function, e.g.
     `resolve_create_command(profile_id: Option<&str>) -> Result<(Option<(String,
     Vec<String>)>, Vec<(String, String)>), TerminalError>`, and unit-test
     it directly rather than only through the full HTTP handler — this keeps
     the guard testable without spawning a process, matching the pattern
     `validate_command_env_overlay_pairing` and `build_helper_command`
     already set in Phase 1).
   - Non-vacuity proof (state this explicitly when implementing, per this
     ticket family's own established practice): mutate the resolver so an
     absent `profile_id` accidentally resolves to `Some(default_profile)` —
     the "no branch taken" unit test above must fail. Revert. A second
     mutation: make a present `profile_id` fail to look up its scrub list
     (i.e. skip `agent_env_profile::CLAUDE`) — the "profile path uses the
     Claude deny-list" unit test must fail. Revert. Recording both mutations
     (and their reverts) in the Phase 2 `### Result`, exactly as Phase 1 did
     ("Non-vacuity" bullet with three named mutations), is what makes the
     "byte for byte" claim in the ticket's own Phase 2 bullet actually
     checked rather than asserted.
   - Browser/integration level: the existing `terminal.create` (no-profile)
     Playwright coverage in `dashboard-acceptance.spec.ts` is the
     regression step — it must keep passing UNCHANGED (no new assertions
     needed there; its continued pass IS the byte-for-byte proof at the
     browser layer) alongside a NEW spec/test scoped with `--grep` for the
     dummy-profile spawn.

## Verification Plan

- `cargo test -p ws-dashboard-daemon --lib` — new unit tests for
  `agent_profile_registry::resolve` (known id → profile, unknown id → `None`)
  and the extracted `resolve_create_command`-style pure function's
  none/some/mutation-caught behavior described in design answer 4. Compare
  pass count against the Phase 1 baseline (140 passed / 0 failed / 2 ignored)
  and report the new count.
- `cargo test -p ws-dashboard-daemon --test terminal_lifetime` — confirm no
  regression (Phase 1 baseline: 4 passed, 0 failed).
- `cargo check -p ws-dashboard-daemon --tests` — exit 0.
- Frontend pure-TS: `npm run test:*` route/unit tests covering
  `TerminalCreateOptions`/`createTerminal` body construction with and without
  `profileId`, and the new command builder/log-label (`commands.ts`). Not
  sufficient alone for the UI half — see next bullet.
- **Browser-level Playwright (MANDATORY for the UI half, per the
  `ws-web-dashboard` mental model's binding rule — a green build/tsc/curl is
  NOT sufficient).** Add a new `test.step`/case inside
  `ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` (or a small
  sibling spec file if isolation is preferred, but reuse `daemonHarness.ts`
  either way) that:
  1. Calls the real terminal-create route directly with
     `{ profileId: "dummy-echo" }` against a known online/available workRoot
     (authenticated via the existing pairing/session flow the harness
     already establishes).
  2. Reloads/re-lists terminals in the actual browser page and asserts the
     resulting Dockview pane renders (title/tab visible, `persistentTerminal`
     surface), and that the pane's recorded profile is observable from
     browser-visible state (e.g. via a `data-*` attribute or exposed session
     field the implementation wires through — implementer's choice of
     concrete DOM hook, but it must be assertable from the rendered page, not
     only from the network response).
  3. Separately asserts the EXISTING no-profile `terminal.create` toolbar
     button flow still opens an ordinary terminal pane with `profileId`
     absent/null — this is the regression half of "byte for byte," verified
     in the browser, not merely inferred from unit tests.
  4. Runs the "New agent terminal" toolbar button once (spawns the real
     `"claude"` profile) far enough to assert the pane opens and is recorded
     as `profileId: "claude"` — it does NOT need to assert the underlying
     `claude` binary actually starts successfully (Phase 3+ own hook/behavior
     verification); if `claude` is not installed in the CI/dev environment,
     scope this specific step to skip gracefully (mirroring the existing
     `WS_DASHBOARD_TEST_GIT_WORKROOT`-style conditional-skip pattern already
     used in this spec file) rather than failing the whole suite, and say so
     explicitly in the test's skip message.
  Command:
  ```
  npm run test:browser -- --grep "agent spawn profile" > /tmp/phase2-browser.log 2>&1
  echo $?
  ```
  (adjust the grep string to whatever step title is actually chosen). Judge
  success by the FAILURE SITE, not raw exit code: `dashboard-acceptance.spec.ts`
  is serial-mode and carries a KNOWN UNRELATED failure at ~line 3779 (fitNow
  short-viewport, tracked separately) — the whole-suite exit code is 1 even
  when Phase 2's own steps are green. Do NOT attempt to fix line 3779. Read
  the log for the grepped test case's own pass/fail line rather than trusting
  the process exit code.
- macOS sequencing note: `260725-bug-dashboard-terminal-platform-macos-unsupported`
  is CLOSED (`.done/`) as of 2026-07-25 per this ticket's own header —
  `cargo check -p ws-dashboard-daemon` exits 0 on this machine, so the daemon
  crate now compiles and unit/integration tests may run normally here; no
  cross-compile-only fallback is needed for this phase.

## Escalations

- None.
