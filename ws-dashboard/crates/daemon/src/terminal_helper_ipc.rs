// NDJSON reader/writer generic over `AsyncRead + AsyncWrite`, mirroring the
// exact framing shape `codex_app_server.rs` uses for the Codex app-server
// stdio protocol (`BufReader::new(reader).lines()` /
// `write_all(line + "\n")` under a mutex - see `codex_app_server.rs:360-368,
// 247-262`). Kept generic (not hard-wired to `UnixStream`) so the same code
// serves both the Unix-socket transport (Stage 1) and the Windows named-pipe
// transport (Stage 2).

use serde::de::DeserializeOwned;
use serde::Serialize;
use tokio::io::{AsyncBufReadExt, AsyncRead, AsyncWrite, AsyncWriteExt, BufReader, Lines};

/// CONTRACT (260729 review round 3, THE INVARIANT): every way reading a
/// message can fail is attributable to THE PEER'S CONNECTION, so reading has
/// exactly one error type and it is always a peer fault. This is a
/// classification BY SOURCE, deliberately replacing the previous
/// classification by `io::ErrorKind` - which was structurally incapable of
/// being complete, because a set of kinds can always be missing one.
///
/// The three sources, and why each is the peer's and not ours:
/// - `Io`: the read syscall on the peer's connection failed. Whatever the
///   kind, the connection is what broke.
/// - `InvalidUtf8`: the peer put bytes on the wire that are not text. The
///   common producer is a peer SIGKILLed mid-`write_all`, which leaves a
///   truncated final line that `Lines::next_line()` hands back at EOF.
/// - `Malformed`: the line is text but not a message this protocol knows -
///   truncated JSON from that same half-written line, or a variant tag from a
///   NEWER peer. Both are the peer's connection desyncing, not this process
///   failing.
///
/// The rule this type exists to make structural: **no error attributable to
/// the peer's connection may end the helper process or kill its shell. It
/// ends the CONNECTION; the helper returns to its accept loop.** See
/// `terminal_helper_process::handle_connection`, which is the site that used
/// to escalate all three of these into `kill_shell_if_running()`.
#[derive(Debug)]
pub enum PeerFault {
    Io(std::io::Error),
    InvalidUtf8(std::io::Error),
    Malformed(serde_json::Error),
}

impl std::fmt::Display for PeerFault {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Io(error) => write!(formatter, "peer connection I/O failed: {error}"),
            Self::InvalidUtf8(error) => {
                write!(formatter, "peer sent bytes that are not valid UTF-8: {error}")
            }
            Self::Malformed(error) => write!(
                formatter,
                "peer sent a line this protocol cannot decode: {error}"
            ),
        }
    }
}

impl std::error::Error for PeerFault {}

/// CONTRACT (260729 review round 3): writing has TWO sources, and only one of
/// them is the peer's.
///
/// - `Peer`: the write/flush syscall on the peer's connection failed. Every
///   kind belongs here, including the ones an ErrorKind allow-list kept
///   missing: `WriteZero` (what `write_all` actually returns on a short
///   write), and Windows `ERROR_OPERATION_ABORTED` (995).
/// - `Serialize`: THIS process could not serialize its own message. Nothing
///   about the peer caused it and no reconnect can fix it, so it stays fatal.
///
/// Splitting by source is what makes "peer-caused" a closed set rather than a
/// list someone has to remember to extend.
#[derive(Debug)]
pub enum WriteFault {
    Serialize(serde_json::Error),
    Peer(std::io::Error),
}

impl std::fmt::Display for WriteFault {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Serialize(error) => write!(
                formatter,
                "failed to serialize an outgoing message (this process's own fault): {error}"
            ),
            Self::Peer(error) => write!(formatter, "peer connection I/O failed: {error}"),
        }
    }
}

impl std::error::Error for WriteFault {}

pub struct NdjsonReader<R> {
    lines: Lines<BufReader<R>>,
}

impl<R: AsyncRead + Unpin> NdjsonReader<R> {
    pub fn new(reader: R) -> Self {
        Self {
            lines: BufReader::new(reader).lines(),
        }
    }

    /// Reads the next non-empty NDJSON line and decodes it. Returns `Ok(None)`
    /// on a clean EOF (peer closed the connection); returns `Err(PeerFault)`
    /// for anything else, because anything else is the peer's connection
    /// misbehaving - see `PeerFault`.
    ///
    /// AMENDED (260729 review round 3): this used to flatten a parse failure
    /// into `io::Error::other` "so a malformed peer is treated as a transport
    /// failure, not silently skipped". Treating it as a transport failure was
    /// right; expressing it as an `ErrorKind::Other` was not, because the one
    /// caller that had to tell peer faults from its own faults could then only
    /// do so by enumerating kinds - and `Other` was on the fatal side of that
    /// enumeration. A daemon SIGKILLed mid-write leaves a truncated final line,
    /// so this was the COMMON crash signature, not an exotic one, and it made
    /// the helper SIGKILL the user's shell. The desync is still surfaced; it
    /// is simply surfaced as what it is.
    pub async fn read_message<T: DeserializeOwned>(
        &mut self,
    ) -> Result<Option<T>, PeerFault> {
        loop {
            let next = self.lines.next_line().await.map_err(|error| {
                // `Lines`/`BufReader::read_line` reports a non-UTF-8 line as
                // `InvalidData`. That is a peer that wrote bytes, not a fault
                // of this process, and it is exactly what a truncated final
                // line looks like when the cut lands mid-codepoint.
                if error.kind() == std::io::ErrorKind::InvalidData {
                    PeerFault::InvalidUtf8(error)
                } else {
                    PeerFault::Io(error)
                }
            })?;
            match next {
                Some(line) if line.trim().is_empty() => continue,
                Some(line) => {
                    let message = serde_json::from_str(&line).map_err(PeerFault::Malformed)?;
                    return Ok(Some(message));
                }
                None => return Ok(None),
            }
        }
    }
}

pub async fn write_ndjson<W, T>(writer: &mut W, message: &T) -> Result<(), WriteFault>
where
    W: AsyncWrite + Unpin,
    T: Serialize,
{
    let mut line = serde_json::to_string(message).map_err(WriteFault::Serialize)?;
    line.push('\n');
    writer
        .write_all(line.as_bytes())
        .await
        .map_err(WriteFault::Peer)?;
    writer.flush().await.map_err(WriteFault::Peer)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::terminal_helper_protocol::{HelperToDaemonMessage, TerminalHelperOutputChunk};

    #[tokio::test]
    async fn write_then_read_round_trips_a_message() {
        let (client, mut server) = tokio::io::duplex(4096);
        let (read_half, mut write_half) = tokio::io::split(client);
        let mut reader = NdjsonReader::new(read_half);

        write_ndjson(
            &mut server,
            &HelperToDaemonMessage::Output(TerminalHelperOutputChunk {
                sequence: 1,
                data: "hi".to_owned(),
            }),
        )
        .await
        .expect("write ndjson message");

        let decoded: HelperToDaemonMessage = reader
            .read_message()
            .await
            .expect("read ndjson message")
            .expect("message present");
        assert_eq!(
            decoded,
            HelperToDaemonMessage::Output(TerminalHelperOutputChunk {
                sequence: 1,
                data: "hi".to_owned(),
            })
        );

        drop(server);
        let eof: Option<HelperToDaemonMessage> =
            reader.read_message().await.expect("read after close");
        assert!(eof.is_none());
        let _ = write_half.shutdown().await;
    }

    #[tokio::test]
    async fn blank_lines_between_messages_are_skipped() {
        let (client, mut server) = tokio::io::duplex(4096);
        let (read_half, _write_half) = tokio::io::split(client);
        let mut reader = NdjsonReader::new(read_half);

        server
            .write_all(b"\n\n")
            .await
            .expect("write leading blank lines");
        write_ndjson(&mut server, &HelperToDaemonMessage::Handshake {
            pid: 1,
            start_time: 2,
        })
        .await
        .expect("write handshake");

        let decoded: HelperToDaemonMessage = reader
            .read_message()
            .await
            .expect("read past blank lines")
            .expect("message present");
        assert_eq!(
            decoded,
            HelperToDaemonMessage::Handshake {
                pid: 1,
                start_time: 2
            }
        );
    }

    // CONTRACT (260729 review round 3): the COMMON crash signature, spelled
    // out as a fixture. A daemon SIGKILLed mid-`write_all` leaves a truncated
    // final line with no newline; `Lines::next_line()` hands those trailing
    // bytes back as `Some(line)` at EOF, so the peer's death arrives as a
    // decode failure rather than as a clean `None`. It must classify as a PEER
    // fault - the previous `io::Error::other` spelling put it on the fatal
    // side of the helper's classification, which is what made a SIGKILLed
    // daemon SIGKILL the user's shell.
    #[tokio::test]
    async fn a_truncated_final_line_at_eof_is_a_peer_fault() {
        let (client, mut server) = tokio::io::duplex(4096);
        let (read_half, _write_half) = tokio::io::split(client);
        let mut reader = NdjsonReader::new(read_half);

        server
            .write_all(br#"{"type":"output","sequ"#)
            .await
            .expect("write truncated line");
        drop(server);

        let fault = reader
            .read_message::<HelperToDaemonMessage>()
            .await
            .expect_err("a truncated tail cannot decode");
        assert!(
            matches!(fault, PeerFault::Malformed(_)),
            "a half-written line from a killed peer is the peer's connection dying, \
             not this process failing: {fault:?}"
        );
    }

    // The same cut landing mid-codepoint. `read_line` reports it as
    // `ErrorKind::InvalidData`, which an ErrorKind allow-list rejected.
    #[tokio::test]
    async fn an_invalid_utf8_tail_is_a_peer_fault() {
        let (client, mut server) = tokio::io::duplex(4096);
        let (read_half, _write_half) = tokio::io::split(client);
        let mut reader = NdjsonReader::new(read_half);

        server
            .write_all(&[0xf0, 0x9f, 0x92])
            .await
            .expect("write a truncated multi-byte codepoint");
        drop(server);

        let fault = reader
            .read_message::<HelperToDaemonMessage>()
            .await
            .expect_err("invalid utf-8 cannot decode");
        assert!(
            matches!(fault, PeerFault::InvalidUtf8(_)),
            "a cut mid-codepoint is still the peer's connection dying: {fault:?}"
        );
    }

    // A NEWER peer sending a variant this build has never heard of. The
    // ticket's Decisions made the registry capability flag the defence against
    // this; classifying it by source is the STRUCTURAL guarantee underneath -
    // the flag is now defence-in-depth.
    #[tokio::test]
    async fn an_unknown_message_variant_is_a_peer_fault() {
        let (client, mut server) = tokio::io::duplex(4096);
        let (read_half, _write_half) = tokio::io::split(client);
        let mut reader = NdjsonReader::new(read_half);

        server
            .write_all(b"{\"type\":\"somethingFromTheFuture\"}\n")
            .await
            .expect("write unknown variant");

        let fault = reader
            .read_message::<HelperToDaemonMessage>()
            .await
            .expect_err("an unknown variant cannot decode");
        assert!(
            matches!(fault, PeerFault::Malformed(_)),
            "an unknown variant is a peer speaking a dialect we do not know: {fault:?}"
        );
    }

    // Non-vacuity for the write split: the peer's connection failing and THIS
    // process failing to serialize its own message must not be the same value,
    // or "keep genuinely internal failures fatal" cannot be expressed.
    #[tokio::test]
    async fn a_write_to_a_closed_peer_is_a_peer_fault_but_serialization_is_ours() {
        let (client, server) = tokio::io::duplex(64);
        drop(server);
        let (_read_half, mut write_half) = tokio::io::split(client);

        // A message large enough that the write cannot be absorbed by the
        // duplex buffer of a dropped peer.
        let big = HelperToDaemonMessage::Output(TerminalHelperOutputChunk {
            sequence: 1,
            data: "x".repeat(4096),
        });
        let fault = write_ndjson(&mut write_half, &big)
            .await
            .expect_err("writing to a dropped peer must fail");
        assert!(
            matches!(fault, WriteFault::Peer(_)),
            "a write failure on the peer's connection is the peer's: {fault:?}"
        );

        // A map with a non-string key has no JSON representation, so this
        // fails inside `serde_json::to_string` before a single byte reaches
        // the transport.
        let unserializable =
            std::collections::BTreeMap::from([((1u8, 2u8), 3u8)]);
        let mut sink = Vec::new();
        let fault = write_ndjson(&mut sink, &unserializable)
            .await
            .expect_err("a tuple-keyed map cannot be serialized to JSON");
        assert!(
            matches!(fault, WriteFault::Serialize(_)),
            "a serialization failure is this process's own problem and must stay fatal: {fault:?}"
        );
    }
}
