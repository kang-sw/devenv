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
        ];
        for message in messages {
            let json = serde_json::to_string(&message).expect("serialize daemon message");
            let decoded: DaemonToHelperMessage =
                serde_json::from_str(&json).expect("deserialize daemon message");
            assert_eq!(decoded, message);
        }
    }
}
