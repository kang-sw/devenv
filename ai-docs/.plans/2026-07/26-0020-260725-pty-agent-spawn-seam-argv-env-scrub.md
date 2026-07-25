# Plan: 260725-feat-dashboard-pty-agent-attention-notification — Phase 1: argv/env passthrough and environment scrub at the spawn seam

## Relevant Ticket Contract

- Extend `TerminalHelperArgs` and `spawn_shell` (`terminal_helper_process.rs:391-407`,
  actually `:393-409` in current source) to carry an explicit command argv and
  env overlay instead of hardcoding `default_shell()`; add a scrub/allowlist
  step applied at BOTH hops — the daemon's helper spawn (`terminal.rs:817-844`)
  and the helper's own shell spawn.
- The scrub list belongs to a vendor-neutral profile seam; only the Claude
  marker set is populated in this phase.
- HARD CONSTRAINT: do not add a field to `TerminalRegistryEntry`
  (`terminal_registry_file.rs:14-25`). If one ever becomes unavoidable it must
  be `Option<T>` + `#[serde(default)]` (`260725-bug-dashboard-terminal-registry-schema-evolution-orphans-helpers`).
- Verification the ticket names: unit tests asserting scrubbed markers are
  absent from the constructed env, AND that a default (no-argv) spawn still
  produces the existing shell behaviour so ordinary terminals are provably
  unchanged (this second half is the regression guard, not optional).
- Constraint: "No PTY wheel reinvention" — this phase adds passthrough at the
  EXISTING spawn seam; it must not fork the terminal substrate or add a
  second helper kind.
- Constraint: env scrub must apply at both hops or the helper's own inherited
  environment (which seeds the shell CommandBuilder's base env — see
  Codebase Findings) stays dirty even if the shell-spawn hop scrubs correctly.
- Scope note (ticket "Scope boundary" section): this phase lands part of
  `260624-feat-ws-dashboard-managed-cli-terminal`'s Phase 1 (argv/env
  commonization only) — output ring, cursoring, resize, status, close/reap,
  fallback reads already exist and are untouched.

## Out of Scope

- Phase 2 (agent spawn path from the browser): `CreateTerminalRequest`
  profile field, the vendor profile registry (command argv template + env
  scrub list + hook config shape), toolbar wiring, `SurfaceKind` recording.
  This phase only builds the plumbing Phase 2 will populate.
- Phase 3 steps 2-3 (hook config materialization under
  `agent-profiles/<terminal_id>/`, the hidden `terminal-notify` subcommand)
  and the turn-start verification spike (already resolved and recorded
  inline in the ticket).
- Phase 4-8 in full (token store/callback endpoint, attention event stream,
  tab indicator, nav-row presentation, browser notification).
- Any `TerminalRegistryEntry` schema change of any kind, `Option<T>` or not.
- `claude_cli.rs`'s existing inline-argv `--settings` hook injection
  (`:473-497`, `:752-764`) — it lives in the agent-GUI surface being wired
  out by `260725-refactor-dashboard-agent-gui-physical-module-isolation`
  (still `idea/`, not landed) and is read-only prior art per the ticket, not
  a dependency to extend.
- Codex profile / any non-Claude vendor marker set (ticket's Deferred scope:
  "Phase 1 keeps the profile seam vendor-neutral; only the Claude profile is
  implemented here").

## Codebase Findings

- `ws-dashboard/crates/daemon/src/cli.rs:30-50` — `TerminalHelperArgs` (clap
  derive `Parser`) currently has no argv/env fields; all fields are plain
  `#[arg(long)]` scalars. New fields must be optional/repeated so the
  existing manual-argv test at
  `crates/daemon/tests/terminal_windows_reaper_acceptance.rs:264-287` (which
  builds the helper's argv by hand, comment: "Mirrors
  `terminal.rs::TerminalSession::spawn`'s arg shape exactly") keeps parsing
  without modification.
- `terminal.rs:801-848` — `TerminalSession::spawn` builds
  `std::process::Command::new(helper_binary)` with a fixed `.arg(...)` chain
  and never calls `.env()`/`.env_clear()`, so it inherits the daemon's full
  process environment implicitly. Its only production caller is
  `create_terminal` (`terminal.rs:646-657`); no other call site exists
  (`terminal_lifetime.rs`/`terminal_windows_reaper_acceptance.rs`'s spawns
  are direct `Command` construction for test harnesses, not this function).
- `terminal_helper_process.rs:393-409` — `spawn_shell` builds a
  `portable_pty::CommandBuilder::new(crate::terminal::default_shell())`,
  sets `cwd` and `TERM` only, then spawns. `CommandBuilder::new`
  (`portable-pty-0.8.1/src/cmdbuilder.rs:209-217`, `get_base_env` at
  `:72-95`) seeds its internal env map from **this process's own**
  `std::env::vars_os()` at construction time — i.e. the helper's inherited
  env, which is itself inherited wholesale from the daemon via the hop
  above. This is exactly the "helper's own inherited environment" the
  ticket says stays dirty if only one hop scrubs, and it is why hop 1
  (daemon → helper) must independently scrub too: hop 1 is defense-in-depth
  against a hop-2 regression, not merely redundant, because hop 2's own
  scrub step is a separate explicit construction the two hops do not share
  code with unless a common module is introduced (below).
- `terminal.rs:1385-1394` and `:1396-1401` — `default_shell()` and
  `browser_pty_term()` are the codebase's existing pattern for testable
  env-dependent logic: both take an injectable `env: impl Fn(&str) ->
  Option<...>` closure instead of reading `std::env::var`/`var_os` directly,
  specifically so tests don't mutate real process env (parallel-test-unsafe)
  and stay deterministic — see the test module at `terminal.rs:1403-1553+`.
  The new scrub/argv logic should follow the same injected-source shape
  rather than reading `std::env::vars_os()` inline.
- `portable-pty-0.8.1/src/cmdbuilder.rs:310-318` (`env_remove`, `env_clear`)
  and `:354-372` (`iter_extra_env_as_str`/`iter_full_env_as_str`),
  `:328-337` (`get_env`) — the inspection surface needed to unit-test a
  `CommandBuilder`'s resulting env without spawning a process.
  `std::process::Command::get_envs()` (stable std API) gives the equivalent
  inspection surface for the hop-1 `std::process::Command`, returning only
  vars set explicitly via `.env()`/`.envs()` — a `Command` that never calls
  those (today's default-path code) yields an empty iterator, which is
  itself the byte-for-byte-unchanged assertion for that branch.
- `terminal_registry_file.rs:14-25` — `TerminalRegistryEntry` fields:
  `terminal_id, work_root_id, pid, start_time, socket_path, created_at_ms,
  title, cwd_hint, columns, rows`. Confirmed no argv/env/command field
  exists today and none should be added (hard constraint).
- `crates/daemon/src/lib.rs:1-31` — module declaration list for the daemon
  crate; a new `agent_env_profile` module needs `pub mod agent_env_profile;`
  added here (alphabetically before `auth`).
- No existing scrub/allowlist/env_clear/CLAUDE_CODE code exists anywhere in
  `crates/daemon/src` today (`grep` returned nothing) — this is genuinely
  new plumbing, not an extension of something partial.
- `terminal_platform.rs:55-70` (unix `spawn_detached`) and `:347-352`
  (windows `spawn_detached`) both take an already-built `std::process::Command`
  and only add detach/job-object semantics; env manipulation done on the
  `Command` before calling `spawn_detached` is unaffected by either leaf.
- Risk signal (shortcut risk: public contract mismatch): the ticket's
  Background section frames scrub as "helper argv carries file PATHS only"
  for the *token* delivery path (Phase 3/4 concern) — this phase must not
  be misread as needing to build that token-file plumbing now; Phase 1's
  argv/env fields are generic (any command + any overlay), not
  callback-token-specific.
- `cargo check -p ws-dashboard-daemon` exits 0 on this machine as of survey
  time, confirming the ticket's "macOS block cleared" note and that
  `cargo test -p ws-dashboard-daemon --lib` is a valid verification command
  here.

### Design decisions this plan pins (so the executor does not have to invent them)

1. **Marker set shape: enumerated list, not a prefix rule.** The ticket's own
   verified evidence is itself an enumerated list (the ten `CLAUDE`/`CLAUDECODE`-named
   vars dumped from this machine's parent environment), and the Phase 3
   spike explicitly used an enumerated ten-variable filtered environment
   rather than a prefix rule. A prefix rule on `CLAUDE` would also be both
   over- and under-scoped at once: over-scoped because it would match any
   future unrelated `CLAUDE*`-named var without a deliberate decision to
   include it, and under-scoped because it would silently exclude
   `AI_AGENT` (no `CLAUDE` prefix) by shape of the filter rather than by a
   stated decision — exactly the trap flagged in the task. An enumerated
   `&'static [&'static str]` list is also what the existing codebase pattern
   already favors for this kind of small, deliberately-curated set (see
   `TerminalRegistryEntry`'s explicit field list, `select_terminal_shell`'s
   explicit source enum). The tradeoff (a list can go stale as the vendor
   adds variables) is accepted deliberately: Phase 2's profile registry is
   the natural place to revisit/extend the list, and going stale silently
   in the *safe* direction (a var that should be scrubbed isn't yet) is
   preferable to a prefix rule silently *including* an unrelated var no one
   decided to scrub.
2. **`AI_AGENT` is included in the Claude marker set.** It is not
   `CLAUDE`-prefixed, but its value shape (`claude-code_<version>_agent`,
   confirmed present on this machine in the Phase 3 spike's own environment
   note) is Claude-vendor identity/session metadata of exactly the kind this
   scrub exists to remove — it identifies the process as running inside a
   Claude Code agent, which is the same class of signal as
   `CLAUDE_CODE_CHILD_SESSION` (the specific var the Background section
   blames for the observed "Transcript saving is off" breakage). The spike
   record notes stripping it was not *necessary* to prove hook delivery
   fires — but that spike was proving a narrower, different claim (hook
   fire, not nested-agent detection suppression), and its own text says the
   residual `AI_AGENT` "could only have suppressed a fire, never fabricated
   one" — i.e. it is not evidence that leaving it in is safe for the actual
   nested-agent-detection failure mode this ticket targets. Since the
   profile is explicitly "the Claude marker set" (not "the `CLAUDE`-prefix
   marker set"), scoping it by vendor intent rather than by string shape is
   the more defensible reading. The resulting list (11 entries): `CLAUDECODE`,
   `CLAUDE_CODE_BRIDGE_SESSION_ID`, `CLAUDE_CODE_CHILD_SESSION`,
   `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_CODE_EXECPATH`,
   `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS`, `CLAUDE_CODE_SESSION_ID`,
   `CLAUDE_EFFORT`, `CLAUDE_PID`, `CLAUDE_WATCHER_TOKEN`, `AI_AGENT`.
3. **Scrub is subtractive (deny-list) over the full inherited environment,
   not a positive allowlist.** An allowlist would have to anticipate every
   var an ordinary shell user's toolchain needs (PATH, HOME, LANG, `nvm`/
   `rbenv`-style PATH prefixes, `SSH_AUTH_SOCK`, locale vars, ...) and the
   evidence only motivates removing a small, specific, named set. Deny-list
   also directly satisfies the "plain shell terminal must keep working
   byte-for-byte" requirement for free: the default (no explicit command)
   path never runs the deny-list logic at all (see decision 4), so nothing
   about an ordinary shell terminal's env construction changes.
4. **Scrub triggers on explicit-argv presence, not a separate flag.** Both
   hops key scrubbing off "was an explicit command passed" (`Option::is_some`)
   rather than a separate boolean, so there is exactly one signal to keep in
   sync and no way for argv and scrub-applicability to disagree. In Phase 1,
   nothing populates argv yet (Phase 2 adds the browser-facing profile
   selector), so in production this phase changes zero observable runtime
   behavior — it only adds tested-but-currently-unreachable plumbing. This
   is what makes the "default spawn unchanged" verification requirement
   satisfiable by construction rather than by careful parity-testing of a
   reconstructed default path.
5. **Both hops share one compiled-in module, not a duplicated list.** The
   daemon and the helper are the same binary (`ws-dashboard`, `lib.rs`
   `pub mod` list) — `terminal.rs` (hop 1) and `terminal_helper_process.rs`
   (hop 2) can both `use crate::agent_env_profile::{CLAUDE, scrub_env_os}`
   directly. No wire/argv transport of the scrub list itself is needed;
   only the resulting command/env-overlay values cross the process boundary
   (via new CLI flags), not the scrub list.
6. **Env source is injected, not read inline**, mirroring
   `default_shell`/`browser_pty_term`'s existing closure-injection pattern,
   so the new pure builder functions are unit-testable without mutating
   real process env under parallel `cargo test` execution.

## Implementation Plan

1. **New module** `crates/daemon/src/agent_env_profile.rs`:
   - `pub struct EnvScrubProfile { pub name: &'static str, pub markers: &'static [&'static str] }`.
   - `pub const CLAUDE: EnvScrubProfile = EnvScrubProfile { name: "claude", markers: &[...] }`
     with the 11-entry list from design decision 2.
   - `pub fn scrub_env_os(env: impl IntoIterator<Item = (OsString, OsString)>, profile: &EnvScrubProfile) -> Vec<(OsString, OsString)>`
     — exact-name (case-sensitive) exclusion, pure, no I/O.
   - Add `pub mod agent_env_profile;` to `crates/daemon/src/lib.rs` (near
     the top, alphabetically before `auth`).
   - Doc-comment the module as the seam Phase 2's profile registry (command
     argv template + env scrub list + hook config shape) will compose over,
     so Phase 2 extends this rather than replacing it.

2. **`cli.rs`**: extend `TerminalHelperArgs` (`:30-50`) with:
   - `#[arg(long)] pub command: Option<String>` — explicit program; `None`
     means "use `default_shell()`" (today's behavior).
   - `#[arg(long = "command-arg")] pub command_args: Vec<String>` — repeated,
     order-preserving; only meaningful when `command` is `Some`.
   - `#[arg(long = "env-overlay", value_parser = parse_env_overlay)] pub env_overlay: Vec<(String, String)>`
     — repeated `KEY=VALUE` pairs; add a small pure `fn parse_env_overlay(raw: &str) -> Result<(String, String), String>`
     (split on first `=`, error on missing `=`) as the clap `value_parser`.
   All three are optional/absent-by-default, so existing manual-argv test
   fixtures (`terminal_windows_reaper_acceptance.rs:264-287`) keep parsing
   unmodified.

3. **`terminal.rs` hop 1** (`:801-848`, `TerminalSession::spawn`):
   - Add two new parameters to `spawn`: `command: Option<(String, Vec<String>)>`
     and `env_overlay: Vec<(String, String)>`.
   - Extract the `std::process::Command` construction (`:818-844`) into a
     pure `fn build_helper_command(helper_binary: &Path, registry_dir: &Path,
     terminal_id: &str, work_root_id: &str, spawn_cwd: &Path, title: &str,
     columns: u16, rows: u16, cwd_hint: Option<&str>, socket_path: &Path,
     command: Option<&(String, Vec<String>)>, env_overlay: &[(String, String)],
     host_env: impl IntoIterator<Item = (OsString, OsString)>) -> std::process::Command`.
     - When `command` is `None`: build the command exactly as today (same
       `.arg(...)` chain, same stdio redirection, **no** `.env()`/`.env_clear()`
       calls at all) — this is the literal byte-for-byte-unchanged path.
     - When `command` is `Some((program, args))`: same base arg chain, plus
       append `--command <program>`, one `--command-arg <arg>` per arg (in
       order), and one `--env-overlay KEY=VALUE` per overlay pair; then call
       `.env_clear()` followed by `.envs(scrub_env_os(host_env, &agent_env_profile::CLAUDE))`.
   - `spawn` calls `build_helper_command(..., std::env::vars_os())` at the
     real call site, inside the existing `spawn_blocking` closure (`:845-848`
     already offloads the blocking work; env read stays sync/cheap).
   - Update `create_terminal` (`:646-657`) to pass `None, vec![]` for the two
     new arguments (Phase 2 wires a real value once `CreateTerminalRequest`
     gains a profile field — out of scope here).

4. **`terminal_helper_process.rs` hop 2** (`:393-409`, `spawn_shell`):
   - Extract the `CommandBuilder` construction into a pure
     `fn build_shell_command(args: &TerminalHelperArgs, host_env: impl IntoIterator<Item = (OsString, OsString)>, term: String) -> CommandBuilder`.
     - When `args.command` is `None`: `CommandBuilder::new(crate::terminal::default_shell())`,
       then `.cwd(&args.cwd)` and `.env("TERM", term)` — identical to
       today's two calls, nothing else touched.
     - When `args.command` is `Some(program)`: `CommandBuilder::new(program)`,
       `.args(&args.command_args)`, `.env_clear()`, then
       `.env(k, v)` for each pair from `scrub_env_os(host_env, &agent_env_profile::CLAUDE)`,
       then `.env(k, v)` for each pair in `args.env_overlay`, then the same
       `.cwd(&args.cwd)` / `.env("TERM", term)` calls as the `None` branch
       (TERM is not a scrub marker, so ordering relative to scrub doesn't
       matter, but applying it last keeps one code path for both branches).
   - `spawn_shell` calls `build_shell_command(args, std::env::vars_os(), browser_pty_term(...))`
     and passes the result to `pair.slave.spawn_command(...)` exactly as
     today.

5. **Tests** (all new, pure/unit — no real process spawn, no `std::env::set_var`):
   - `agent_env_profile.rs`: a fixture host-env `Vec<(OsString, OsString)>`
     containing all 11 markers plus ordinary vars (`PATH`, `HOME`); assert
     `scrub_env_os` removes exactly the 11 and preserves the rest untouched
     (including value equality, not just presence).
   - `cli.rs`: a `TerminalHelperArgs`/`Cli::parse_from` round-trip test
     asserting `--command`, repeated `--command-arg`, and repeated
     `--env-overlay KEY=VALUE` parse into the expected `Option`/`Vec`
     shapes, plus a `parse_env_overlay` unit test for the missing-`=` error
     case.
   - `terminal_helper_process.rs`:
     - `spawn_shell_default_no_command_matches_existing_behaviour`: `args.command = None`;
       call `build_shell_command`; assert the resulting `CommandBuilder`'s
       program equals `default_shell()`, `get_cwd()` matches `args.cwd`, and
       `iter_extra_env_as_str()` (non-base-env entries) contains only
       `TERM` — proving no `env_clear`/scrub ran.
     - `spawn_shell_explicit_command_scrubs_claude_markers_and_applies_overlay`:
       `args.command = Some("printf")`, `args.command_args = ["hi"]`,
       `args.env_overlay = [("FOO", "bar")]`; fixture `host_env` containing
       all 11 markers plus `PATH`; assert `get_argv() == ["printf", "hi"]`,
       `get_env(<each marker>) == None` for all 11, `get_env("PATH")` still
       present (deny-list, not allowlist), `get_env("FOO") == Some("bar")`,
       and `get_env("TERM")` present.
   - `terminal.rs`:
     - `helper_spawn_default_no_command_matches_existing_arg_shape`:
       `command = None`; call `build_helper_command`; assert
       `.get_envs().next().is_none()` (no explicit env calls at all) and
       that the arg list contains no `--command`/`--command-arg`/
       `--env-overlay` tokens, matching today's exact arg chain.
     - `helper_spawn_with_command_scrubs_claude_markers_and_forwards_argv`:
       `command = Some(("agent-cli".into(), vec!["--flag".into()]))`,
       `env_overlay = [("BASE_URL".into(), "http://x".into())]`, fixture
       `host_env` with all 11 markers plus `PATH`; assert `.get_envs()`
       contains no marker key and does contain `PATH`, and the arg list
       contains `--command agent-cli`, `--command-arg --flag`, and
       `--env-overlay BASE_URL=http://x` in order.

## Verification Plan

Run from `/Users/kang-sw/devenv/.git/wt/dashboard/ws-dashboard`:

```
cargo check -p ws-dashboard-daemon > /tmp/phase1-check.log 2>&1
echo $?
```

```
cargo test -p ws-dashboard-daemon --lib > /tmp/phase1-lib-tests.log 2>&1
echo $?
```

If a faster scoped run is wanted first, the new/touched test modules can be
targeted directly (still followed by the full `--lib` run above before
calling the phase done):

```
cargo test -p ws-dashboard-daemon --lib -- agent_env_profile terminal_helper_process terminal:: cli:: > /tmp/phase1-scoped-tests.log 2>&1
echo $?
```

Existing integration tests (`terminal_lifetime.rs`,
`terminal_windows_reaper_acceptance.rs`, `routes.rs`) are not expected to
need edits — the manual-argv fixture at
`terminal_windows_reaper_acceptance.rs:264-287` should still parse against
the extended `TerminalHelperArgs` since all new fields are optional/
default-empty. Re-run them only if the `--lib` run above passes and time
allows:

```
cargo test -p ws-dashboard-daemon --test terminal_lifetime > /tmp/phase1-terminal-lifetime.log 2>&1
echo $?
```

The macOS compile block (`260725-bug-dashboard-terminal-platform-macos-unsupported`)
is closed and `cargo check -p ws-dashboard-daemon` was confirmed to exit 0
on this machine during survey, so these commands are expected to be
directly runnable rather than needing a cross-compile escape hatch.

## Escalations

None.
