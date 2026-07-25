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

/// Placeholder for Phase 3's hook-config-shape materialization
/// (`agent-profiles/<terminal_id>/` file write, `ws-dashboard
/// terminal-notify`). Intentionally empty - every Phase 2 profile sets
/// `hook_config: None` and nothing in this phase reads a populated value.
/// Phase 3 defines the real fields and the write-out step; this type only
/// reserves the slot on `AgentProfile` so that phase is an extension, not a
/// struct-shape rewrite.
#[derive(Debug, Clone, Copy)]
pub struct HookConfigShape;

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

const CLAUDE_PROFILE: AgentProfile = AgentProfile {
    id: "claude",
    command: "claude",
    args: &[],
    scrub: &agent_env_profile::CLAUDE,
    hook_config: None,
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

const PROFILES: &[AgentProfile] = &[CLAUDE_PROFILE, DUMMY_ECHO_PROFILE];

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
    fn resolve_returns_none_for_an_unknown_or_empty_id() {
        assert!(resolve("not-a-real-profile").is_none());
        assert!(resolve("").is_none());
    }
}
