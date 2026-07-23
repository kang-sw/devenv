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
    /// on a clean EOF (peer closed the connection); returns `Err` for a real
    /// I/O failure or a line that fails to parse as `T` (a malformed peer is
    /// treated as a transport failure, not silently skipped, since this is a
    /// private 1:1 control channel where a malformed line indicates a
    /// protocol desync worth surfacing).
    pub async fn read_message<T: DeserializeOwned>(&mut self) -> std::io::Result<Option<T>> {
        loop {
            match self.lines.next_line().await? {
                Some(line) if line.trim().is_empty() => continue,
                Some(line) => {
                    let message = serde_json::from_str(&line).map_err(std::io::Error::other)?;
                    return Ok(Some(message));
                }
                None => return Ok(None),
            }
        }
    }
}

pub async fn write_ndjson<W, T>(writer: &mut W, message: &T) -> std::io::Result<()>
where
    W: AsyncWrite + Unpin,
    T: Serialize,
{
    let mut line = serde_json::to_string(message).map_err(std::io::Error::other)?;
    line.push('\n');
    writer.write_all(line.as_bytes()).await?;
    writer.flush().await
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
}
