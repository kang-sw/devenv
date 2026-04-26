//! claude-dash — Interactive worktree-scoped Claude TUI multiplexer.
//!
//! Entry point: initialises the crossterm raw-mode terminal, runs the event
//! loop, and restores the terminal on exit or panic.

mod agent;
mod app;
mod pty;
mod ui;
mod vt;
mod worktree;

use std::io;
use std::time::Duration;

use anyhow::Context;
use crossterm::{
    event::{self, Event, KeyCode, KeyModifiers},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use portable_pty::PtySize;
use ratatui::{backend::CrosstermBackend, Terminal};

use app::{App, ExitedModal};

/// Target frame budget in milliseconds (~100 fps, keeping PTY responsive).
const FRAME_MS: u64 = 10;
/// `crossterm::event::poll` timeout per iteration.
const POLL_MS: u64 = 5;

fn main() -> anyhow::Result<()> {
    // Augment the default panic hook so that a panic also restores the terminal
    // before printing the panic message.
    let default_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let _ = disable_raw_mode();
        let mut stderr = io::stderr();
        let _ = execute!(stderr, LeaveAlternateScreen);
        default_hook(info);
    }));

    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen).context("enter alternate screen")?;

    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let cwd = std::env::current_dir().context("get cwd")?;
    let mut app = App::new(&cwd)?;

    let result = run_loop(&mut terminal, &mut app);

    // Restore terminal on normal exit and on error.
    let _ = disable_raw_mode();
    let _ = execute!(terminal.backend_mut(), LeaveAlternateScreen);
    let _ = terminal.show_cursor();

    result
}

fn run_loop<B: ratatui::backend::Backend>(
    terminal: &mut Terminal<B>,
    app: &mut App,
) -> anyhow::Result<()>
where
    <B as ratatui::backend::Backend>::Error:
        std::error::Error + Send + Sync + 'static,
{
    loop {
        // 1. Drain crossterm events.
        while event::poll(Duration::from_millis(POLL_MS))? {
            let evt = event::read()?;
            handle_event(app, evt)?;
        }

        // 2. Drain PTY output and feed into the VT screen model.
        {
            let mut all: Vec<u8> = Vec::new();
            if let Some(ref s) = app.session {
                while let Some(chunk) = s.try_recv_chunk() {
                    all.extend_from_slice(&chunk);
                }
            }
            if !all.is_empty() {
                app.vt.feed(&all);
            }
        }

        // 3. Poll for child process exit.
        if app.exited_modal.is_none() {
            if let Some(ref mut s) = app.session {
                if let Some(status) = s.try_wait() {
                    app.exited_modal = Some(ExitedModal { status });
                }
            }
        }

        // 4. Resize check: compare approximate inner panel size derived from
        //    the current terminal dimensions against the cached last draw size.
        //    The authoritative inner dims are written by ui::draw each frame,
        //    so one frame of lag is acceptable (plan §1.7 resize-check note).
        {
            let (term_cols, term_rows) =
                crossterm::terminal::size().unwrap_or((80, 24));
            // Phase-1 layout: 1-row top bar + borders (2 rows, 2 cols).
            let panel_cols = term_cols.saturating_sub(2);
            let panel_rows = term_rows.saturating_sub(3);
            let (last_cols, last_rows) = app.last_panel_size;
            if panel_cols != last_cols || panel_rows != last_rows {
                if panel_cols > 0 && panel_rows > 0 {
                    app.vt.resize(panel_cols, panel_rows);
                    if let Some(ref mut s) = app.session {
                        let new_size = PtySize {
                            rows: panel_rows,
                            cols: panel_cols,
                            pixel_width: 0,
                            pixel_height: 0,
                        };
                        s.resize(new_size)?;
                    }
                    app.last_panel_size = (panel_cols, panel_rows);
                }
            }
        }

        // 5. Draw.
        terminal.draw(|f| ui::draw(f, app))?;

        // 6. Quit check.
        if app.should_quit {
            break;
        }

        // 7. Sleep for the remainder of the frame budget.
        std::thread::sleep(Duration::from_millis(FRAME_MS));
    }
    Ok(())
}

/// Dispatch a single crossterm event to the application.
fn handle_event(app: &mut App, event: Event) -> anyhow::Result<()> {
    match event {
        Event::Key(key) => {
            // --- Modal active ---
            if app.exited_modal.is_some() {
                match key.code {
                    KeyCode::Char('r') | KeyCode::Char('R') => {
                        app.exited_modal = None;
                        let cwd = std::env::current_dir()?;
                        let (cols, rows) = app.last_panel_size;
                        let size = PtySize {
                            rows,
                            cols,
                            pixel_width: 0,
                            pixel_height: 0,
                        };
                        app.vt.resize(cols, rows);
                        app.session = Some(pty::PtySession::spawn(&cwd, size)?);
                    }
                    KeyCode::Char('q') | KeyCode::Char('Q') => {
                        app.should_quit = true;
                    }
                    _ => {}
                }
                return Ok(());
            }

            // --- Global hotkeys (never forwarded to PTY) ---
            if key.code == KeyCode::Char('q')
                && key.modifiers.contains(KeyModifiers::CONTROL)
            {
                app.should_quit = true;
                return Ok(());
            }

            // Phase-2 tab-cycle hotkeys (Ctrl+[ and Ctrl+]) are handled here
            // in later phases; for now they fall through to PTY forwarding with
            // the caveat documented in the plan (Ctrl+[ == Esc on many terminals).

            // --- Forward to PTY ---
            if let Some(bytes) = pty::encode_key(key) {
                if let Some(ref mut s) = app.session {
                    s.write(&bytes)?;
                }
            }
        }

        // Resize events are handled in the main loop's resize-check step
        // (step 4), not here.
        Event::Resize(_, _) => {}

        _ => {}
    }
    Ok(())
}
