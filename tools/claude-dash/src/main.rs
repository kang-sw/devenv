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

use app::{App, ExitedModal, WORKTREE_POLL_SECS};

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

    let mut app = App::new()?;

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

        // 2. Drain PTY output for ALL tabs (background tabs keep running).
        for tab in &mut app.tabs {
            tab.drain_pty();
        }

        // 3. Poll for child process exit on ALL tabs.
        for tab in &mut app.tabs {
            tab.poll_exit();
        }

        // 4. Worktree polling (every WORKTREE_POLL_SECS).
        if app.last_worktree_poll.elapsed() >= Duration::from_secs(WORKTREE_POLL_SECS) {
            let new_list = worktree::discover_worktrees();
            if !new_list.is_empty() {
                app.reconcile_worktrees(new_list);
            }
            app.last_worktree_poll = std::time::Instant::now();
        }

        // 5. Resize check: compare approximate inner panel size (derived from
        //    terminal dimensions) against the active tab's last-known size.
        if !app.tabs.is_empty() {
            let (term_cols, term_rows) =
                crossterm::terminal::size().unwrap_or((80, 24));
            // Phase-2 layout: 1-row tab bar + borders (2 rows, 2 cols).
            let panel_cols = term_cols.saturating_sub(2);
            let panel_rows = term_rows.saturating_sub(3);
            if panel_cols > 0 && panel_rows > 0 {
                let tab = &app.tabs[app.active_tab];
                let (last_cols, last_rows) = tab.last_inner_size;
                if panel_cols != last_cols || panel_rows != last_rows {
                    let idx = app.active_tab;
                    app.tabs[idx].maybe_resize(panel_cols, panel_rows)?;
                    app.tabs[idx].last_inner_size = (panel_cols, panel_rows);
                }
            }
        }

        // 6. Draw.
        terminal.draw(|f| ui::draw(f, app))?;

        // 7. Quit check.
        if app.should_quit {
            break;
        }

        // 8. Sleep for the remainder of the frame budget.
        std::thread::sleep(Duration::from_millis(FRAME_MS));
    }
    Ok(())
}

/// Dispatch a single crossterm event to the application.
fn handle_event(app: &mut App, event: Event) -> anyhow::Result<()> {
    match event {
        Event::Key(key) => {
            // --- Modal active on the current tab ---
            if let Some(tab) = app.tabs.get_mut(app.active_tab) {
                if tab.exited_modal.is_some() {
                    return handle_modal_key(app, key);
                }
            }

            // --- Global hotkeys (never forwarded to PTY) ---
            // Ctrl+Q — quit app.
            if key.code == KeyCode::Char('q')
                && key.modifiers.contains(KeyModifiers::CONTROL)
            {
                app.should_quit = true;
                return Ok(());
            }

            // Ctrl+[ — previous tab.
            if key.code == KeyCode::Char('[')
                && key.modifiers.contains(KeyModifiers::CONTROL)
            {
                let new_idx = app.active_tab.saturating_sub(1);
                app.activate_tab(new_idx);
                return Ok(());
            }

            // Ctrl+] — next tab.
            if key.code == KeyCode::Char(']')
                && key.modifiers.contains(KeyModifiers::CONTROL)
            {
                let new_idx = (app.active_tab + 1).min(app.tabs.len().saturating_sub(1));
                app.activate_tab(new_idx);
                return Ok(());
            }

            // Ctrl+1 — select main slot (Phase 3+; no-op in Phase 2).
            if key.code == KeyCode::Char('1')
                && key.modifiers.contains(KeyModifiers::CONTROL)
            {
                return Ok(());
            }

            // Ctrl+2–9 — select named-agent slot (Phase 3+; no-op in Phase 2).
            if let KeyCode::Char(c) = key.code {
                if key.modifiers.contains(KeyModifiers::CONTROL)
                    && ('2'..='9').contains(&c)
                {
                    return Ok(());
                }
            }

            // --- Forward remaining keys to the active tab's PTY ---
            if let Some(bytes) = pty::encode_key(key) {
                if let Some(tab) = app.tabs.get_mut(app.active_tab) {
                    if let Some(ref mut s) = tab.session {
                        s.write(&bytes)?;
                    }
                }
            }
        }

        // Resize events are handled in the main loop's resize-check step.
        Event::Resize(_, _) => {}

        _ => {}
    }
    Ok(())
}

/// Handle a key press while the exit modal is active on the current tab.
fn handle_modal_key(
    app: &mut App,
    key: crossterm::event::KeyEvent,
) -> anyhow::Result<()> {
    let idx = app.active_tab;
    let tab = &app.tabs[idx];
    let is_removed = tab.worktree.is_removed;
    let modal_status_owned = tab.exited_modal.as_ref().map(|_| ());

    if modal_status_owned.is_none() {
        return Ok(());
    }

    match key.code {
        // [R] Restart — only when worktree is not removed.
        KeyCode::Char('r') | KeyCode::Char('R') if !is_removed => {
            let tab = &mut app.tabs[idx];
            tab.exited_modal = None;
            let (cols, rows) = tab.last_inner_size;
            let size = PtySize { rows, cols, pixel_width: 0, pixel_height: 0 };
            let cwd = tab.worktree.path.clone();
            tab.vt.resize(cols, rows);
            tab.session = Some(pty::PtySession::spawn(&cwd, size)?);
        }

        // [X] Close tab — drop the tab.
        KeyCode::Char('x') | KeyCode::Char('X') => {
            app.tabs.remove(idx);
            if app.tabs.is_empty() {
                app.should_quit = true;
            } else {
                app.active_tab = app.active_tab.min(app.tabs.len() - 1);
            }
        }

        // [Q] Quit app.
        KeyCode::Char('q') | KeyCode::Char('Q') => {
            app.should_quit = true;
        }

        // Ctrl+Q always quits regardless of modal state.
        KeyCode::Char('q') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.should_quit = true;
        }

        _ => {}
    }
    Ok(())
}
