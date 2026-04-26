//! claude-dash — Interactive worktree-scoped Claude TUI multiplexer.
//!
//! Entry point: initialises the crossterm raw-mode terminal, runs the event
//! loop, and restores the terminal on exit or panic.

mod agent;
mod app;
mod parser;
mod pty;
mod renderer;
mod session;
mod ui;
mod vt;
mod worktree;
mod wrap;

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

use app::{App, SlotKind, SCROLL_STEP, WORKTREE_POLL_SECS};
use ui::PAGE_SCROLL;

/// Target frame budget in milliseconds (~100 fps, keeping PTY responsive).
const FRAME_MS: u64 = 10;
/// `crossterm::event::poll` timeout per iteration.
const POLL_MS: u64 = 5;

fn main() -> anyhow::Result<()> {
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
    <B as ratatui::backend::Backend>::Error: std::error::Error + Send + Sync + 'static,
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

        // 2b. Drain background token-parse results.
        app.drain_token_results();

        // 3. Poll for child process exit on ALL tabs.
        for tab in &mut app.tabs {
            tab.poll_exit();
        }

        // 4. Check mtime on the active Named slot (live update from agent writes).
        if let Some(tab) = app.tabs.get_mut(app.active_tab) {
            if matches!(tab.active_slot, SlotKind::Named(_)) {
                tab.maybe_reload_agent_view();
            }
        }

        // 5. Worktree + agent polling (every WORKTREE_POLL_SECS).
        if app.last_worktree_poll.elapsed() >= Duration::from_secs(WORKTREE_POLL_SECS) {
            let new_list = worktree::discover_worktrees();
            if !new_list.is_empty() {
                app.reconcile_worktrees(new_list);
            }
            app.refresh_named_agents();
            app.last_worktree_poll = std::time::Instant::now();
        }

        // 6. Resize check for active tab.
        if !app.tabs.is_empty() {
            let (term_cols, term_rows) = crossterm::terminal::size().unwrap_or((80, 24));
            // Phase-3 layout: 2-row header (tab + slot) + borders (2 rows, 2 cols).
            let panel_cols = term_cols.saturating_sub(2);
            let panel_rows = term_rows.saturating_sub(4);
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

        // 7. Draw.
        terminal.draw(|f| ui::draw(f, app))?;

        // 8. Quit check.
        if app.should_quit {
            break;
        }

        // 9. Sleep for the remainder of the frame budget.
        std::thread::sleep(Duration::from_millis(FRAME_MS));
    }
    Ok(())
}

/// Dispatch a single crossterm event to the application.
fn handle_event(app: &mut App, event: Event) -> anyhow::Result<()> {
    match event {
        Event::Key(key) => {
            // --- Modal active on the current tab ---
            if app
                .tabs
                .get(app.active_tab)
                .map(|t| t.exited_modal.is_some())
                .unwrap_or(false)
            {
                return handle_modal_key(app, key);
            }

            // --- Ctrl+Q — quit app (always, regardless of slot) ---
            if key.code == KeyCode::Char('q') && key.modifiers.contains(KeyModifiers::CONTROL) {
                app.should_quit = true;
                return Ok(());
            }

            // --- Tab navigation (Ctrl+[ / Ctrl+]) ---
            if key.code == KeyCode::Char('[') && key.modifiers.contains(KeyModifiers::CONTROL) {
                let new_idx = app.active_tab.saturating_sub(1);
                app.activate_tab(new_idx);
                return Ok(());
            }
            if key.code == KeyCode::Char(']') && key.modifiers.contains(KeyModifiers::CONTROL) {
                let new_idx = (app.active_tab + 1).min(app.tabs.len().saturating_sub(1));
                app.activate_tab(new_idx);
                return Ok(());
            }

            // --- Slot selection (Ctrl+1–9) ---
            if key.modifiers.contains(KeyModifiers::CONTROL) {
                if let KeyCode::Char(c) = key.code {
                    if c == '1' {
                        // Ctrl+1 → Main slot.
                        if let Some(tab) = app.active_mut() {
                            tab.active_slot = SlotKind::Main;
                            tab.agent_view = None;
                        }
                        return Ok(());
                    }
                    if ('2'..='9').contains(&c) {
                        let agent_idx = (c as usize) - ('2' as usize);
                        if let Some(tab) = app.active_mut() {
                            if agent_idx < tab.named_agents.len() {
                                tab.open_agent_view(agent_idx);
                            }
                        }
                        return Ok(());
                    }
                }
            }

            // --- Slot-specific key handling ---
            let active_slot = app
                .active()
                .map(|t| matches!(t.active_slot, SlotKind::Named(_)))
                .unwrap_or(false);

            if active_slot {
                // Named slot: consume scroll keys; ignore others (no PTY forward).
                handle_agent_scroll(app, key);
                return Ok(());
            }

            // --- Main slot: forward to PTY ---
            if let Some(bytes) = pty::encode_key(key) {
                if let Some(tab) = app.tabs.get_mut(app.active_tab) {
                    if let Some(ref mut s) = tab.session {
                        s.write(&bytes)?;
                    }
                }
            }
        }

        Event::Resize(_, _) => {}
        _ => {}
    }
    Ok(())
}

/// Handle scroll keys while an AgentView is active.
fn handle_agent_scroll(app: &mut App, key: crossterm::event::KeyEvent) {
    let tab = match app.tabs.get_mut(app.active_tab) {
        Some(t) => t,
        None => return,
    };
    let view = match tab.agent_view.as_mut() {
        Some(v) => v,
        None => return,
    };
    let total_lines = view.rendered_lines.len();
    let panel_height = tab.last_inner_size.1 as usize;
    let max_scroll = total_lines.saturating_sub(panel_height);

    match key.code {
        KeyCode::Up | KeyCode::Char('k') => {
            view.scroll_offset = view.scroll_offset.saturating_sub(SCROLL_STEP);
        }
        KeyCode::Down | KeyCode::Char('j') => {
            view.scroll_offset = view
                .scroll_offset
                .saturating_add(SCROLL_STEP)
                .min(max_scroll);
        }
        KeyCode::PageUp => {
            view.scroll_offset = view.scroll_offset.saturating_sub(PAGE_SCROLL);
        }
        KeyCode::PageDown => {
            view.scroll_offset = view
                .scroll_offset
                .saturating_add(PAGE_SCROLL)
                .min(max_scroll);
        }
        _ => {}
    }
}

/// Handle a key press while the exit modal is active on the current tab.
fn handle_modal_key(app: &mut App, key: crossterm::event::KeyEvent) -> anyhow::Result<()> {
    let idx = app.active_tab;
    if app
        .tabs
        .get(idx)
        .map(|t| t.exited_modal.is_none())
        .unwrap_or(true)
    {
        return Ok(());
    }

    let is_removed = app.tabs[idx].worktree.is_removed;

    match key.code {
        // Ctrl+Q always quits.
        KeyCode::Char('q') if key.modifiers.contains(KeyModifiers::CONTROL) => {
            app.should_quit = true;
        }

        // [R] Restart — only when worktree is not removed.
        KeyCode::Char('r') | KeyCode::Char('R') if !is_removed => {
            let tab = &mut app.tabs[idx];
            tab.exited_modal = None;
            let (cols, rows) = tab.last_inner_size;
            let size = PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            };
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

        _ => {}
    }
    Ok(())
}
