// CONTRACT (Decision A, ticket 260723-feat-dashboard-terminal-lifetime-daemon
// -decouple): this is a dedicated, private daemon<->helper control protocol,
// deliberately a *separate* Rust type hierarchy from the browser-facing
// `TerminalWebSocketServerMessage`/`TerminalWebSocketClientMessage` in
// `terminal.rs`. Nothing in this module may be reused as, or converted
// directly from, the browser wire types - the daemon translates between the
// two explicitly. This keeps the browser-facing WebSocket contract stable
// even as this private helper protocol evolves.
//
// Framing: NDJSON (one `serde_json`-encoded line per message), mirroring
// `codex_app_server.rs`'s `BufReader::lines()` / `write_all(line + "\n")`
// shape (see `terminal_helper_ipc.rs`). Transport is a Unix domain socket on
// Unix and (Stage 2) a Windows named pipe; both carry the same message enums.

use serde::{Deserialize, Serialize};

#[derive(Clone, Copy, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum TerminalHelperStatus {
    Running,
    Exited,
    Terminated,
    Error,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TerminalHelperOutputChunk {
    pub sequence: u64,
    pub data: String,
}

// CONTRACT: sent by the helper immediately after a daemon connects to its
// IPC listener, carrying the helper's *real* PID and OS-reported start time
// (post double-fork/detach) - this is the identity the daemon must persist
// and later verify-before-kill (see `terminal_platform`). The daemon must
// NOT trust any PID observed before this message (e.g. a `Command::spawn()`
// return value), since that PID belongs to the short-lived middle process of
// the double fork, not the long-lived detached helper.
#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum HelperToDaemonMessage {
    Handshake {
        pid: u32,
        start_time: u64,
    },
    Output(TerminalHelperOutputChunk),
    Status {
        status: TerminalHelperStatus,
        next_sequence: u64,
    },
    Exit {
        status: TerminalHelperStatus,
        next_sequence: u64,
    },
    BackfillResponse {
        request_id: u64,
        chunks: Vec<TerminalHelperOutputChunk>,
        next_sequence: u64,
        status: TerminalHelperStatus,
    },
    // CONTRACT (260729 helper liveness probe): the ONLY reply to
    // `DaemonToHelperMessage::LivenessProbe`, and STRICTLY request-gated -
    // this variant is never emitted unsolicited. That rule is what keeps a
    // rollback safe in the other direction: an old daemon never sends
    // `LivenessProbe`, so it can never receive this variant, which on that
    // build would decode-fail, take its IPC connection down and escalate all
    // the way to `kill_verified`.
    //
    // `attached` is what makes the daemon-side predicate three-way rather
    // than two-way: a helper that answers at all is alive, but only a helper
    // that answers *and* reports nobody attached for longer than the grace is
    // a genuine orphan. `unattached_for_ms` is measured BY THE HELPER (from
    // its own last daemon disconnect), because only the helper knows when
    // that was - a daemon-side timer cannot tell "unattached for an hour"
    // from "I just started and have not adopted it yet". `None` means
    // "attached" (nothing to measure); a `None` alongside `attached: false`
    // is read conservatively daemon-side as within-grace, never as an orphan.
    LivenessProbeResponse {
        attached: bool,
        unattached_for_ms: Option<u64>,
    },
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum DaemonToHelperMessage {
    // CONTRACT: registry-write-before-shell-spawn ordering (see ticket
    // "Registry-write ordering") is only half the orphan-leak fix - the
    // helper additionally waits for this explicit ack before spawning the
    // shell, so a daemon that connects but crashes before processing the
    // handshake never leaves an unsupervised shell running under a helper
    // the daemon doesn't know about.
    HandshakeAck,
    Input {
        data: String,
    },
    Resize {
        columns: u16,
        rows: u16,
    },
    RequestBackfill {
        request_id: u64,
        after: u64,
    },
    GracefulShutdown,
    // CONTRACT (260729 helper liveness probe): a lightweight request the
    // helper answers CONCURRENTLY with an attached session (see
    // `terminal_helper_process.rs::serve_session`'s probe-only accept arm),
    // so "connect succeeded but nobody answered the handshake" stops being
    // read as death. It is NOT an attach: the helper never spawns a shell,
    // never flips `shell_started`, and never resets its unattached clock in
    // response to one.
    //
    // HARD RULE: the daemon may only send this to a helper whose registry
    // entry declares `supportsLivenessProbe` (see
    // `terminal_registry_file.rs`). Sending it to a helper that predates the
    // variant makes that helper's read fail, which on ITS build propagates
    // with `?`, which makes `run_terminal_helper` run
    // `kill_shell_if_running()` + `delete_registry_entry()` - i.e. it
    // SIGKILLs the user's shell. Appending the variant is safe; sending it
    // unguarded is not.
    //
    // AMENDED (260729 review round 3, finding A): a helper built from this
    // tree onwards drops only the CONNECTION on an unknown variant, so this
    // rule now protects the pre-upgrade population specifically rather than
    // every helper. Do not relax it on the strength of the new guarantee -
    // the helpers it exists for are the ones that do not have it.
    LivenessProbe,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn helper_to_daemon_messages_round_trip_through_json() {
        let messages = [
            HelperToDaemonMessage::Handshake {
                pid: 4242,
                start_time: 123_456,
            },
            HelperToDaemonMessage::Output(TerminalHelperOutputChunk {
                sequence: 7,
                data: "hello".to_owned(),
            }),
            HelperToDaemonMessage::Status {
                status: TerminalHelperStatus::Running,
                next_sequence: 8,
            },
            HelperToDaemonMessage::Exit {
                status: TerminalHelperStatus::Exited,
                next_sequence: 9,
            },
            HelperToDaemonMessage::BackfillResponse {
                request_id: 1,
                chunks: vec![TerminalHelperOutputChunk {
                    sequence: 1,
                    data: "x".to_owned(),
                }],
                next_sequence: 2,
                status: TerminalHelperStatus::Running,
            },
            HelperToDaemonMessage::LivenessProbeResponse {
                attached: true,
                unattached_for_ms: None,
            },
            HelperToDaemonMessage::LivenessProbeResponse {
                attached: false,
                unattached_for_ms: Some(1234),
            },
        ];
        for message in messages {
            let json = serde_json::to_string(&message).expect("serialize helper message");
            let decoded: HelperToDaemonMessage =
                serde_json::from_str(&json).expect("deserialize helper message");
            assert_eq!(decoded, message);
        }
    }

    #[test]
    fn daemon_to_helper_messages_round_trip_through_json() {
        let messages = [
            DaemonToHelperMessage::HandshakeAck,
            DaemonToHelperMessage::Input {
                data: "ls\n".to_owned(),
            },
            DaemonToHelperMessage::Resize {
                columns: 80,
                rows: 24,
            },
            DaemonToHelperMessage::RequestBackfill {
                request_id: 3,
                after: 5,
            },
            DaemonToHelperMessage::GracefulShutdown,
            DaemonToHelperMessage::LivenessProbe,
        ];
        for message in messages {
            let json = serde_json::to_string(&message).expect("serialize daemon message");
            let decoded: DaemonToHelperMessage =
                serde_json::from_str(&json).expect("deserialize daemon message");
            assert_eq!(decoded, message);
        }
    }

    // CONTRACT (260729 helper liveness probe, wire-format compatibility):
    // both enums are `#[serde(tag = "type")]`, so variant tags are NAMES, not
    // ordinals - appending (or inserting) a variant must never renumber or
    // rename an existing one. Every pre-probe variant's on-the-wire tag is
    // pinned here literally, so a rename/reorder that would make an existing
    // helper or daemon fail to parse a message it used to understand fails
    // loudly rather than silently escalating to a SIGKILL of the user's shell.
    #[test]
    fn pre_probe_variant_tags_are_unchanged_by_the_appended_probe_variants() {
        let expected: [(String, &str); 5] = [
            (
                serde_json::to_string(&DaemonToHelperMessage::HandshakeAck)
                    .expect("serialize"),
                r#"{"type":"handshakeAck"}"#,
            ),
            (
                serde_json::to_string(&DaemonToHelperMessage::GracefulShutdown)
                    .expect("serialize"),
                r#"{"type":"gracefulShutdown"}"#,
            ),
            (
                serde_json::to_string(&HelperToDaemonMessage::Handshake {
                    pid: 1,
                    start_time: 2,
                })
                .expect("serialize"),
                r#"{"type":"handshake","pid":1,"start_time":2}"#,
            ),
            (
                serde_json::to_string(&HelperToDaemonMessage::Status {
                    status: TerminalHelperStatus::Running,
                    next_sequence: 3,
                })
                .expect("serialize"),
                r#"{"type":"status","status":"running","next_sequence":3}"#,
            ),
            (
                serde_json::to_string(&HelperToDaemonMessage::Exit {
                    status: TerminalHelperStatus::Exited,
                    next_sequence: 4,
                })
                .expect("serialize"),
                r#"{"type":"exit","status":"exited","next_sequence":4}"#,
            ),
        ];
        for (actual, expected) in expected {
            assert_eq!(
                actual, expected,
                "a pre-probe variant's wire form changed; every helper alive across the \
                 upgrade depends on these bytes"
            );
        }
    }
}
