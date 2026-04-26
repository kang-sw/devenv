//! PTY session lifecycle: spawn, read, write, resize, kill.
//!
//! `PtySession` owns the master PTY handle, the child process, and a
//! background reader thread that forwards bytes to the main loop via an
//! unbounded mpsc channel.  The channel is unbounded so the reader thread
//! never blocks on bursts; the main loop drains it completely each frame.

use std::io::{Read, Write};
use std::sync::mpsc;
use std::thread;

use portable_pty::{CommandBuilder, PtySize};

pub struct PtySession {
    master: Box<dyn portable_pty::MasterPty + Send>,
    child: Box<dyn portable_pty::Child + Send + Sync>,
    writer: Box<dyn Write + Send>,
    rx: mpsc::Receiver<Vec<u8>>,
    size: PtySize,
    reader_thread: Option<thread::JoinHandle<()>>,
}

impl PtySession {
    /// Spawn `claude` in `cwd` at the given PTY dimensions.
    pub fn spawn(cwd: &std::path::Path, size: PtySize) -> anyhow::Result<Self> {
        Self::spawn_with_args(cwd, size, &[])
    }

    /// Spawn `claude` with extra command-line arguments in `cwd`.
    ///
    /// Use this when you need flags such as `--dangerously-skip-permissions`.
    /// `args` is appended after the binary name in order.
    pub fn spawn_with_args(
        cwd: &std::path::Path,
        size: PtySize,
        args: &[&str],
    ) -> anyhow::Result<Self> {
        let pty_system = portable_pty::native_pty_system();
        let pair = pty_system.openpty(size)?;

        let mut cmd = CommandBuilder::new("claude");
        cmd.cwd(cwd);
        for arg in args {
            cmd.arg(arg);
        }
        // Env inherits from the parent process by default.

        let child = pair.slave.spawn_command(cmd)?;
        let mut writer = pair.master.take_writer()?;
        // Enable button-motion tracking so the subprocess receives drag events
        // (e.g. claude's own TUI mouse selection).
        let _ = writer.write_all(b"\x1b[?1002h");
        let mut reader = pair.master.try_clone_reader()?;

        // Drop slave BEFORE spawning the reader thread: if the child exits
        // before the thread starts, master will see EOF only after all slave
        // holders are gone.  Dropping here (parent's copy) ensures no
        // slave-fd is held by this process once the thread is running.
        drop(pair.slave);

        // Unbounded channel — reader thread never blocks on bursts.
        let (tx, rx) = mpsc::channel::<Vec<u8>>();

        let reader_thread = thread::spawn(move || {
            let mut buf = [0u8; 4096];
            loop {
                match reader.read(&mut buf) {
                    Ok(0) => break, // EOF — child exited
                    Ok(n) => {
                        if tx.send(buf[..n].to_vec()).is_err() {
                            break; // Receiver dropped (app exiting)
                        }
                    }
                    Err(_) => break,
                }
            }
        });

        Ok(PtySession {
            master: pair.master,
            child,
            writer,
            rx,
            size,
            reader_thread: Some(reader_thread),
        })
    }

    /// Write bytes to the child's stdin.
    pub fn write(&mut self, bytes: &[u8]) -> std::io::Result<()> {
        self.writer.write_all(bytes)
    }

    /// Try to receive one chunk from the reader thread.  Returns `None` when
    /// the channel is empty.  Call in a `while let Some` loop to drain.
    pub fn try_recv_chunk(&self) -> Option<Vec<u8>> {
        self.rx.try_recv().ok()
    }

    /// Issue a `TIOCSWINSZ` ioctl to update the PTY dimensions.
    pub fn resize(&mut self, size: PtySize) -> anyhow::Result<()> {
        self.size = size;
        self.master.resize(size)
    }

    /// Non-blocking exit-status poll.  Returns `Some(status)` once the child
    /// has exited.
    pub fn try_wait(&mut self) -> Option<portable_pty::ExitStatus> {
        self.child.try_wait().ok().flatten()
    }

    /// Kill the child process (best-effort; errors are suppressed).
    pub fn kill(&mut self) {
        let _ = self.child.kill();
    }

    /// Return the current PTY dimensions.
    pub fn size(&self) -> PtySize {
        self.size
    }
}

impl Drop for PtySession {
    fn drop(&mut self) {
        // Enforce "subprocesses terminate when claude-dash exits" (ticket Decision).
        let _ = self.child.kill();
        // Join the reader thread so it does not outlive the session.
        // After kill(), the child's slave fd closes (child was the only remaining
        // slave holder — we dropped our copy in spawn()), causing the master
        // reader to receive EOF/EIO and exit cleanly.
        if let Some(t) = self.reader_thread.take() {
            let _ = t.join();
        }
    }
}

// ---------------------------------------------------------------------------
// Key encoding
// ---------------------------------------------------------------------------

/// Translate a crossterm `KeyEvent` into the VT byte sequence that the PTY
/// child expects.  Returns `None` for keys with no meaningful encoding
/// (e.g. lone modifier keys).
///
/// Phase-1 coverage: printable chars, basic control keys, cursor keys,
/// function keys 1–12.  App-level hotkeys (Ctrl+Q, Ctrl+B) are filtered
/// out by `handle_event` before this function is called.
pub fn encode_key(key: crossterm::event::KeyEvent) -> Option<Vec<u8>> {
    use crossterm::event::{KeyCode, KeyModifiers};

    match key.code {
        KeyCode::Char(c) => {
            if key.modifiers.contains(KeyModifiers::CONTROL) {
                let lower = c.to_ascii_lowercase() as u8;
                if (b'a'..=b'z').contains(&lower) {
                    return Some(vec![lower & 0x1f]);
                }
                // Characters outside a–z with Ctrl: fall through to UTF-8.
            }
            let mut buf = [0u8; 4];
            let s = c.encode_utf8(&mut buf);
            Some(s.as_bytes().to_vec())
        }

        KeyCode::Enter => Some(b"\r".to_vec()),
        KeyCode::Backspace => Some(vec![0x7f]),
        KeyCode::Tab => Some(b"\t".to_vec()),
        KeyCode::BackTab => Some(b"\x1b[Z".to_vec()),
        KeyCode::Esc => Some(vec![0x1b]),

        KeyCode::Up => Some(b"\x1b[A".to_vec()),
        KeyCode::Down => Some(b"\x1b[B".to_vec()),
        KeyCode::Right => Some(b"\x1b[C".to_vec()),
        KeyCode::Left => Some(b"\x1b[D".to_vec()),

        KeyCode::Home => Some(b"\x1b[H".to_vec()),
        KeyCode::End => Some(b"\x1b[F".to_vec()),
        KeyCode::PageUp => Some(b"\x1b[5~".to_vec()),
        KeyCode::PageDown => Some(b"\x1b[6~".to_vec()),
        KeyCode::Delete => Some(b"\x1b[3~".to_vec()),

        KeyCode::F(n) => Some(match n {
            1 => b"\x1bOP".to_vec(),
            2 => b"\x1bOQ".to_vec(),
            3 => b"\x1bOR".to_vec(),
            4 => b"\x1bOS".to_vec(),
            5 => b"\x1b[15~".to_vec(),
            6 => b"\x1b[17~".to_vec(),
            7 => b"\x1b[18~".to_vec(),
            8 => b"\x1b[19~".to_vec(),
            9 => b"\x1b[20~".to_vec(),
            10 => b"\x1b[21~".to_vec(),
            11 => b"\x1b[23~".to_vec(),
            12 => b"\x1b[24~".to_vec(),
            _ => return None,
        }),

        _ => None,
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use crossterm::event::{KeyCode, KeyEvent, KeyEventKind, KeyEventState, KeyModifiers};

    fn key(code: KeyCode, modifiers: KeyModifiers) -> KeyEvent {
        KeyEvent {
            code,
            modifiers,
            kind: KeyEventKind::Press,
            state: KeyEventState::NONE,
        }
    }

    fn plain(code: KeyCode) -> KeyEvent {
        key(code, KeyModifiers::NONE)
    }

    fn ctrl(c: char) -> KeyEvent {
        key(KeyCode::Char(c), KeyModifiers::CONTROL)
    }

    // --- Ctrl+a–z encoding ---

    #[test]
    fn encode_ctrl_a() {
        assert_eq!(encode_key(ctrl('a')), Some(vec![0x01]));
    }

    #[test]
    fn encode_ctrl_z() {
        assert_eq!(encode_key(ctrl('z')), Some(vec![0x1a]));
    }

    #[test]
    fn encode_ctrl_c() {
        assert_eq!(encode_key(ctrl('c')), Some(vec![0x03]));
    }

    #[test]
    fn encode_ctrl_uppercase_folds_to_lower() {
        // 'A' and 'a' with CONTROL should produce the same byte.
        let via_lower = encode_key(ctrl('a'));
        let via_upper = encode_key(key(KeyCode::Char('A'), KeyModifiers::CONTROL));
        assert_eq!(via_lower, via_upper);
    }

    // --- Plain printable chars ---

    #[test]
    fn encode_plain_char() {
        assert_eq!(encode_key(plain(KeyCode::Char('h'))), Some(b"h".to_vec()));
    }

    #[test]
    fn encode_plain_unicode() {
        // '→' (U+2192) encodes to its 3-byte UTF-8 representation.
        let bytes = encode_key(plain(KeyCode::Char('→'))).unwrap();
        assert_eq!(bytes, "→".as_bytes().to_vec());
    }

    // --- Special keys ---

    #[test]
    fn encode_enter() {
        assert_eq!(encode_key(plain(KeyCode::Enter)), Some(b"\r".to_vec()));
    }

    #[test]
    fn encode_backspace() {
        assert_eq!(encode_key(plain(KeyCode::Backspace)), Some(vec![0x7f]));
    }

    #[test]
    fn encode_tab() {
        assert_eq!(encode_key(plain(KeyCode::Tab)), Some(b"\t".to_vec()));
    }

    #[test]
    fn encode_esc() {
        assert_eq!(encode_key(plain(KeyCode::Esc)), Some(vec![0x1b]));
    }

    // --- Cursor keys ---

    #[test]
    fn encode_arrow_keys() {
        assert_eq!(encode_key(plain(KeyCode::Up)), Some(b"\x1b[A".to_vec()));
        assert_eq!(encode_key(plain(KeyCode::Down)), Some(b"\x1b[B".to_vec()));
        assert_eq!(encode_key(plain(KeyCode::Right)), Some(b"\x1b[C".to_vec()));
        assert_eq!(encode_key(plain(KeyCode::Left)), Some(b"\x1b[D".to_vec()));
    }

    #[test]
    fn encode_page_keys() {
        assert_eq!(
            encode_key(plain(KeyCode::PageUp)),
            Some(b"\x1b[5~".to_vec())
        );
        assert_eq!(
            encode_key(plain(KeyCode::PageDown)),
            Some(b"\x1b[6~".to_vec())
        );
    }

    #[test]
    fn encode_function_keys() {
        assert_eq!(encode_key(plain(KeyCode::F(1))), Some(b"\x1bOP".to_vec()));
        assert_eq!(
            encode_key(plain(KeyCode::F(12))),
            Some(b"\x1b[24~".to_vec())
        );
    }
}
