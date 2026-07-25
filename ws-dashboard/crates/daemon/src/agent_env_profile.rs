// CONTRACT (260725 Phase 1, pty-agent spawn-seam argv/env scrub): this
// module is the seam Phase 2's vendor profile registry (command argv
// template + env scrub list + hook config shape) will compose over, not a
// standalone feature. Phase 1 populates only the Claude marker set; adding a
// second vendor's `EnvScrubProfile` here (or promoting this into a registry
// keyed by profile id) is Phase 2's job, not a rewrite of this seam.

use std::ffi::OsString;

/// A named, vendor-scoped set of environment variable names to strip from a
/// spawned agent CLI's environment before it runs, so a nested agent process
/// does not inherit its parent's own agent-session identity markers (which
/// otherwise cause the nested process to misidentify itself as running
/// inside an existing agent session - see ticket
/// `260725-feat-dashboard-pty-agent-attention-notification` Background).
#[derive(Debug, Clone, Copy)]
pub struct EnvScrubProfile {
    pub name: &'static str,
    pub markers: &'static [&'static str],
}

// CONTRACT: enumerated marker list, not a prefix rule - see plan design
// decision 1/2 (`260725-pty-agent-spawn-seam-argv-env-scrub` plan). A prefix
// rule on `CLAUDE` would be simultaneously over-scoped (matches any future
// unrelated `CLAUDE*` var without a deliberate decision to include it) and
// under-scoped (silently excludes `AI_AGENT`, which carries no `CLAUDE`
// prefix but is the same class of Claude-vendor identity signal). Going
// stale in the safe direction (a var that should be scrubbed isn't yet) is
// accepted deliberately; Phase 2's profile registry is where this list gets
// revisited/extended.
pub const CLAUDE: EnvScrubProfile = EnvScrubProfile {
    name: "claude",
    markers: &[
        "CLAUDECODE",
        "CLAUDE_CODE_BRIDGE_SESSION_ID",
        "CLAUDE_CODE_CHILD_SESSION",
        "CLAUDE_CODE_ENTRYPOINT",
        "CLAUDE_CODE_EXECPATH",
        "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS",
        "CLAUDE_CODE_SESSION_ID",
        "CLAUDE_EFFORT",
        "CLAUDE_PID",
        "CLAUDE_WATCHER_TOKEN",
        "AI_AGENT",
    ],
};

/// Removes every entry whose key exactly (case-sensitively) matches one of
/// `profile.markers` from `env`, preserving the relative order of the
/// remaining entries. Subtractive (deny-list) over the full inherited
/// environment, not a positive allowlist - see plan design decision 3: an
/// allowlist would have to anticipate every var an ordinary shell user's
/// toolchain needs (PATH, HOME, LANG, `nvm`/`rbenv`-style PATH prefixes,
/// `SSH_AUTH_SOCK`, locale vars, ...), while the evidence here only
/// motivates removing a small, specific, named set. Pure and I/O-free so it
/// stays unit-testable without touching real process env.
pub fn scrub_env_os(
    env: impl IntoIterator<Item = (OsString, OsString)>,
    profile: &EnvScrubProfile,
) -> Vec<(OsString, OsString)> {
    env.into_iter()
        .filter(|(key, _value)| {
            !profile
                .markers
                .iter()
                .any(|marker| key.to_str() == Some(*marker))
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn os(value: &str) -> OsString {
        OsString::from(value)
    }

    #[test]
    fn scrub_env_os_removes_exactly_the_claude_markers_and_preserves_the_rest() {
        let mut fixture: Vec<(OsString, OsString)> = CLAUDE
            .markers
            .iter()
            .map(|marker| (os(marker), os("marker-value")))
            .collect();
        fixture.push((os("PATH"), os("/usr/bin:/bin")));
        fixture.push((os("HOME"), os("/home/example")));

        let scrubbed = scrub_env_os(fixture, &CLAUDE);

        assert_eq!(
            scrubbed,
            vec![
                (os("PATH"), os("/usr/bin:/bin")),
                (os("HOME"), os("/home/example")),
            ]
        );
    }

    #[test]
    fn scrub_env_os_is_a_no_op_when_no_markers_are_present() {
        let fixture: Vec<(OsString, OsString)> = vec![
            (os("PATH"), os("/usr/bin:/bin")),
            (os("HOME"), os("/home/example")),
        ];

        let scrubbed = scrub_env_os(fixture.clone(), &CLAUDE);

        assert_eq!(scrubbed, fixture);
    }
}
