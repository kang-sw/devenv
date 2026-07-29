// CONTRACT (260725 Phase 2, browser spawn profile): this module is the
// registry `agent_env_profile.rs`'s header CONTRACT comment predicted -
// "adding a second vendor's `EnvScrubProfile` here, or promoting this into a
// registry keyed by profile id, is Phase 2's job, not a rewrite of this
// seam." `agent_env_profile.rs` keeps owning the env-scrub marker lists;
// this module composes a profile id onto a scrub list plus a spawn argv, and
// stays a pure, I/O-free, unit-testable static lookup table - no `AppState`
// field, no runtime mutation (see plan Codebase Findings on `router.rs`'s
// `AppState`). Phase 3 extends `hook_config` rather than replacing this
// shape.

use crate::agent_env_profile::{self, EnvScrubProfile};

/// Vendor hook-event -> turn-state pairs to materialize into a `--settings`
/// file at spawn time (260725 Phase 3 step 2). `events` is `(event name,
/// turn state)`, e.g. Claude's `[("UserPromptSubmit", "working"), ("Stop",
/// "ready")]` per the ticket's pinned three-state vocabulary and the closed
/// Phase 3 step-1 spike, which measured both events firing in a real
/// interactive PTY. `Copy` because `AgentProfile` itself is `Copy` and this
/// is the type Phase 2 reserved a slot for without defining its contents.
#[derive(Debug, Clone, Copy)]
pub struct HookConfigShape {
    pub events: &'static [(&'static str, &'static str)],
}

/// A vendor (or test-only) spawn profile: the literal argv to run in place
/// of the default interactive shell, plus the env-scrub list to apply to the
/// spawned process's environment. See `terminal.rs::resolve_create_command`
/// for the single call site that turns a `profile_id` into these values.
#[derive(Debug, Clone, Copy)]
pub struct AgentProfile {
    pub id: &'static str,
    pub command: &'static str,
    pub args: &'static [&'static str],
    pub scrub: &'static EnvScrubProfile,
    // Placeholder for Phase 3; unused until hook materialization lands.
    pub hook_config: Option<HookConfigShape>,
}

// CONTRACT: `command`/`args` are a literal argv, never a shell string -
// `build_helper_command` (`terminal.rs`) forwards them straight into
// `std::process::Command::new(program).args(args)` with no shell interposed
// (confirmed at hop 2 too: `terminal_helper_process.rs`'s `CommandBuilder`
// execs `program` directly). Picking `program: "sh"`/`"cmd.exe"` with an
// explicit `-c`/`/C` script argument is therefore a deliberate two-element
// argv naming a real interpreter executable, not "a shell one-liner" being
// naively fed through as a single unparsed command string - it mirrors the
// same per-platform shell choice `select_terminal_shell` already makes for
// the *default* (no-profile) spawn path in this same file, and the marker +
// long-running-then-observable-Running shape mirrors the POSIX/cmd-exe
// command choices `terminalCommandPlanForPlatform`
// (`frontend/src/terminalCommandPlan.ts`) already uses for this same
// acceptance suite (`printf`/`sleep`, `ping -n 30 127.0.0.1 > nul`).
#[cfg(unix)]
const DUMMY_ECHO_COMMAND: &str = "/bin/sh";
#[cfg(unix)]
const DUMMY_ECHO_ARGS: &[&str] = &[
    "-c",
    "printf '%s\\n' DUMMY_ECHO_MARKER; sleep 30",
];

#[cfg(windows)]
const DUMMY_ECHO_COMMAND: &str = "cmd.exe";
#[cfg(windows)]
const DUMMY_ECHO_ARGS: &[&str] = &[
    "/C",
    "echo DUMMY_ECHO_MARKER & ping -n 31 127.0.0.1 > NUL",
];

// CONTRACT (260725 Phase 6): same shape as `DUMMY_ECHO_ARGS` (a real
// interpreter argv, marker line then a long-running sleep - see the CONTRACT
// above `DUMMY_ECHO_COMMAND`), with a LONGER sleep, and that difference is
// load-bearing rather than cosmetic. The Phase 6 indicator is suppressed at
// render time for any session whose `status` is no longer `"running"` (the
// stale-indicator fix), so the spawned dummy process must outlive the whole
// browser spec: with the 30s sleep `dummy-echo` uses, the helper would exit
// mid-run and the indicator under test would legitimately disappear. 180s
// matches the Playwright per-test timeout (`playwright.config.ts`), which
// bounds how long an orphaned sleep can survive an aborted run.
#[cfg(unix)]
const DUMMY_ECHO_HOOKED_ARGS: &[&str] = &[
    "-c",
    "printf '%s\\n' DUMMY_ECHO_MARKER; sleep 180",
];

#[cfg(windows)]
const DUMMY_ECHO_HOOKED_ARGS: &[&str] = &[
    "/C",
    "echo DUMMY_ECHO_MARKER & ping -n 181 127.0.0.1 > NUL",
];

// CONTRACT (260725 Phase 3 step-1 spike, closed positive): both event names
// are the exact strings the spike measured firing under a real interactive
// PTY (`UserPromptSubmit`, `Stop`). Turn-state vocabulary is pinned by the
// ticket's `## Decisions` ("Concrete mechanics"): `working` / `ready` / `idle`.
const CLAUDE_HOOK_CONFIG: HookConfigShape = HookConfigShape {
    events: &[("UserPromptSubmit", "working"), ("Stop", "ready")],
};

const CLAUDE_PROFILE: AgentProfile = AgentProfile {
    id: "claude",
    command: "claude",
    args: &[],
    scrub: &agent_env_profile::CLAUDE,
    hook_config: Some(CLAUDE_HOOK_CONFIG),
};

// CONTRACT (plan design answer 3): always present in the compiled daemon
// binary - the acceptance suite drives the real production binary via
// `daemonHarness.ts`, not a `#[cfg(test)]` build, so a test-cfg-gated entry
// would be unreachable from the browser gate. Kept OUT of every user-facing
// surface (toolbar, command palette, hotkeys) instead: only this module and
// the Playwright acceptance test know the id `"dummy-echo"`. Never a vendor
// CLI, never a network dependency, never a shell one-liner fed as an
// unparsed program string - see the CONTRACT above `DUMMY_ECHO_COMMAND`.
const DUMMY_ECHO_PROFILE: AgentProfile = AgentProfile {
    id: "dummy-echo",
    command: DUMMY_ECHO_COMMAND,
    args: DUMMY_ECHO_ARGS,
    scrub: &agent_env_profile::NONE,
    hook_config: None,
};

// CONTRACT (260725 Phase 6, tab-label indicator): a SECOND test-only
// profile, identical to `DUMMY_ECHO_PROFILE` except that it carries a
// hook config. `TerminalSession::spawn` gates callback-token generation on
// `hook_config.is_some()` (`terminal.rs`), so `dummy-echo` - which asserts
// `hook_config.is_none()` in its own Phase 2 test on purpose - can never be
// used to drive the Phase 4 turn-state callback route from a browser
// acceptance spec: it never gets a token. This profile exists solely so
// `agent-attention-indicator.spec.ts` can spawn a terminal that HAS a real
// `terminal-tokens/<terminal_id>.json`, without depending on a vendor CLI.
//
// The event list is deliberately EMPTY: `materialize_hook_config`
// (`agent_hook_config.rs`) loops over `shape.events`, so an empty slice
// writes `{"hooks":{}}` and appends a `--settings <path>` pair that
// `/bin/sh -c <script>` (or `cmd.exe /C <script>`) simply takes as extra
// positional arguments - the dummy script's behavior is unchanged, and no
// hook can ever fire because the dummy command is not a vendor CLI. Only
// `hook_config.is_some()` is load-bearing here.
//
// Same visibility contract as `DUMMY_ECHO_PROFILE`: always compiled in
// (the acceptance suite drives the real production binary), never exposed
// on any user-facing surface, id known only to this module and its own
// Playwright spec.
const DUMMY_ECHO_HOOKED_PROFILE: AgentProfile = AgentProfile {
    id: "dummy-echo-hooked",
    command: DUMMY_ECHO_COMMAND,
    args: DUMMY_ECHO_HOOKED_ARGS,
    scrub: &agent_env_profile::NONE,
    hook_config: Some(HookConfigShape { events: &[] }),
};

const PROFILES: &[AgentProfile] = &[CLAUDE_PROFILE, DUMMY_ECHO_PROFILE, DUMMY_ECHO_HOOKED_PROFILE];

/// Pure `match`-shaped lookup, no I/O - unit-testable without spawning a
/// process. `None` on an unknown id (including an empty string), which
/// `create_terminal`'s caller turns into a `TerminalError::BadRequest`.
pub fn resolve(profile_id: &str) -> Option<&'static AgentProfile> {
    PROFILES.iter().find(|profile| profile.id == profile_id)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_finds_the_claude_profile_and_wires_the_claude_scrub_list() {
        let profile = resolve("claude").expect("claude profile must be registered");
        assert_eq!(profile.command, "claude");
        assert!(profile.args.is_empty());
        assert_eq!(profile.scrub.name, "claude");
    }

    #[test]
    fn resolve_finds_the_dummy_echo_profile_with_a_no_op_scrub() {
        let profile = resolve("dummy-echo").expect("dummy-echo profile must be registered");
        assert!(!profile.command.is_empty());
        assert!(profile.scrub.markers.is_empty());
    }

    #[test]
    fn claude_profile_hook_config_registers_both_spike_verified_events() {
        let profile = resolve("claude").expect("claude profile must be registered");
        let hook_config = profile.hook_config.expect("claude profile must carry hook config");
        assert!(hook_config
            .events
            .iter()
            .any(|(event, state)| *event == "UserPromptSubmit" && *state == "working"));
        assert!(hook_config
            .events
            .iter()
            .any(|(event, state)| *event == "Stop" && *state == "ready"));
    }

    #[test]
    fn dummy_echo_profile_has_no_hook_config() {
        let profile = resolve("dummy-echo").expect("dummy-echo profile must be registered");
        assert!(profile.hook_config.is_none(), "test-only profile must not carry hooks");
    }

    #[test]
    fn resolve_finds_the_dummy_echo_hooked_profile_with_a_no_op_scrub() {
        let profile =
            resolve("dummy-echo-hooked").expect("dummy-echo-hooked profile must be registered");
        assert!(!profile.command.is_empty());
        assert!(profile.scrub.markers.is_empty());
    }

    #[test]
    fn dummy_echo_hooked_profile_carries_an_empty_hook_config() {
        let profile =
            resolve("dummy-echo-hooked").expect("dummy-echo-hooked profile must be registered");
        let hook_config = profile
            .hook_config
            .expect("the hooked test profile must carry a hook config so spawn mints a token");
        assert!(
            hook_config.events.is_empty(),
            "the hooked test profile must register no vendor events - only `is_some()` is \
             load-bearing (see this profile's CONTRACT)"
        );
    }

    #[test]
    fn the_two_dummy_profiles_share_a_program_and_differ_in_hook_config_and_lifetime() {
        let plain = resolve("dummy-echo").expect("dummy-echo profile must be registered");
        let hooked =
            resolve("dummy-echo-hooked").expect("dummy-echo-hooked profile must be registered");
        assert_eq!(plain.command, hooked.command);
        assert!(plain.hook_config.is_none());
        assert!(hooked.hook_config.is_some());
        // The hooked profile's process must outlive the whole browser spec
        // (see its args CONTRACT): a shorter-lived dummy would exit
        // mid-run, flip the session off `"running"`, and make the Phase 6
        // indicator legitimately disappear while under test.
        assert_ne!(
            plain.args, hooked.args,
            "the hooked profile deliberately runs longer than dummy-echo"
        );
        assert!(!hooked.args.is_empty());
    }

    #[test]
    fn resolve_returns_none_for_an_unknown_or_empty_id() {
        assert!(resolve("not-a-real-profile").is_none());
        assert!(resolve("").is_none());
    }
}
