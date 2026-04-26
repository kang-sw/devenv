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
use clap::Parser;
use crossterm::{
    event::{
        self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyModifiers, MouseButton,
        MouseEventKind,
    },
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use portable_pty::PtySize;
use ratatui::{backend::CrosstermBackend, Terminal};

use app::{App, SlotKind, SCROLL_STEP, WORKTREE_POLL_SECS};
use ui::PAGE_SCROLL;

/// Tmux-style TUI multiplexer for Claude worktrees.
#[derive(Parser, Debug)]
#[command(name = "claude-dash", about = "Claude worktree TUI multiplexer")]
struct Cli {
    /// Pass `--dangerously-skip-permissions` to every spawned `claude` process.
    #[arg(long)]
    dangerously_skip_permissions: bool,
}

/// Target frame budget in milliseconds (~100 fps, keeping PTY responsive).
const FRAME_MS: u64 = 10;
/// `crossterm::event::poll` timeout per iteration.
const POLL_MS: u64 = 5;

fn main() -> anyhow::Result<()> {
    let cli = Cli::parse();

    let default_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let _ = disable_raw_mode();
        let mut stderr = io::stderr();
        let _ = execute!(stderr, LeaveAlternateScreen, DisableMouseCapture);
        default_hook(info);
    }));

    enable_raw_mode().context("enable raw mode")?;
    let mut stdout = io::stdout();
    execute!(stdout, EnterAlternateScreen, EnableMouseCapture)
        .context("enter alternate screen")?;

    let backend = CrosstermBackend::new(stdout);
    let mut terminal = Terminal::new(backend)?;

    let mut app = App::new(cli.dangerously_skip_permissions)?;

    let result = run_loop(&mut terminal, &mut app);

    let _ = disable_raw_mode();
    let _ = execute!(terminal.backend_mut(), LeaveAlternateScreen, DisableMouseCapture);
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

            // --- Ctrl+Q — quit app (always, regardless of slot or prefix mode) ---
            if key.code == KeyCode::Char('q') && key.modifiers.contains(KeyModifiers::CONTROL) {
                app.should_quit = true;
                return Ok(());
            }

            // --- Prefix mode: consume the next key and dispatch a command ---
            if app.prefix_active {
                app.prefix_active = false;
                match key.code {
                    // '1'–'9' → switch worktree tab (0-indexed), clamped.
                    KeyCode::Char(c @ '1'..='9') => {
                        let idx = (c as usize) - ('1' as usize);
                        let clamped = idx.min(app.tabs.len().saturating_sub(1));
                        app.activate_tab(clamped);
                    }
                    // '0' → switch to tab index 9 (10th), clamped.
                    KeyCode::Char('0') => {
                        let clamped = 9usize.min(app.tabs.len().saturating_sub(1));
                        app.activate_tab(clamped);
                    }
                    // 'n' → spawn new claude in current tab.
                    KeyCode::Char('n') => {
                        app.spawn_new_claude_in_tab()?;
                    }
                    // 'q'–'p' row → switch agent slot within the current tab.
                    KeyCode::Char(c) => {
                        let slot: Option<usize> = match c {
                            'q' => Some(0),
                            'w' => Some(1),
                            'e' => Some(2),
                            'r' => Some(3),
                            't' => Some(4),
                            'y' => Some(5),
                            'u' => Some(6),
                            'i' => Some(7),
                            'o' => Some(8),
                            'p' => Some(9),
                            // Unrecognised char → cancel (prefix_active already reset).
                            _ => None,
                        };
                        if let Some(s) = slot {
                            switch_agent_slot(app, s);
                        }
                    }
                    // Esc or any other key → cancel.
                    _ => {}
                }
                return Ok(());
            }

            // --- Ctrl+B — activate prefix mode; do NOT forward to PTY ---
            if key.code == KeyCode::Char('b') && key.modifiers.contains(KeyModifiers::CONTROL) {
                app.prefix_active = true;
                return Ok(());
            }

            // --- Slot-specific key handling ---
            let active_named = app
                .active()
                .map(|t| matches!(t.active_slot, SlotKind::Named(_)))
                .unwrap_or(false);

            if active_named {
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

        // --- Mouse events ---
        Event::Mouse(mouse) => {
            let is_named = app
                .active()
                .map(|t| matches!(t.active_slot, SlotKind::Named(_)))
                .unwrap_or(false);

            if is_named {
                handle_agent_mouse_scroll(app, mouse);
            } else {
                forward_mouse_to_pty(app, mouse)?;
            }
        }

        Event::Resize(_, _) => {}
        _ => {}
    }
    Ok(())
}

/// Switch the active tab to the given agent `slot`.
///
/// Slot 0 is the main PTY; slots 1–N are named agents (0-indexed in
/// `named_agents`).  Out-of-range slots are silently ignored.
fn switch_agent_slot(app: &mut App, slot: usize) {
    if let Some(tab) = app.active_mut() {
        if slot == 0 {
            tab.active_slot = SlotKind::Main;
            tab.agent_view = None;
        } else {
            let agent_idx = slot - 1;
            if agent_idx < tab.named_agents.len() {
                tab.open_agent_view(agent_idx);
            }
        }
    }
}

/// Handle mouse scroll events while a Named (JSONL) slot is active.
fn handle_agent_mouse_scroll(app: &mut App, mouse: crossterm::event::MouseEvent) {
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
    match mouse.kind {
        MouseEventKind::ScrollUp => {
            view.scroll_offset = view.scroll_offset.saturating_sub(SCROLL_STEP);
        }
        MouseEventKind::ScrollDown => {
            view.scroll_offset = view
                .scroll_offset
                .saturating_add(SCROLL_STEP)
                .min(max_scroll);
        }
        _ => {}
    }
}

/// Encode a mouse event as an X10 VT sequence and write it to the active PTY.
///
/// Panel inner origin (fixed layout): column 1, row 3
/// (tab bar 1 row + slot row 1 row + top border 1 row; left border 1 col).
///
/// X10 encoding: `ESC [ M <Cb> <Cx> <Cy>` where each byte has 32 added.
/// Coordinates are 1-indexed; max supported coordinate is 223 (255 − 32).
fn forward_mouse_to_pty(
    app: &mut App,
    mouse: crossterm::event::MouseEvent,
) -> anyhow::Result<()> {
    const PANEL_COL: u16 = 1;
    const PANEL_ROW: u16 = 3;

    if mouse.column < PANEL_COL || mouse.row < PANEL_ROW {
        return Ok(());
    }
    let pty_x = mouse.column - PANEL_COL; // 0-indexed relative to panel inner
    let pty_y = mouse.row - PANEL_ROW;

    // X10 byte = (1-indexed coord) + 32.  Max 1-indexed coord = 255 − 32 = 223.
    // pty_x is 0-indexed, so 1-indexed = pty_x + 1.  Bound: pty_x + 1 ≤ 223
    // → pty_x ≤ 222.
    if pty_x > 222 || pty_y > 222 {
        return Ok(());
    }
    let cx = (pty_x + 33) as u8; // (pty_x + 1) + 32
    let cy = (pty_y + 33) as u8;

    let cb: u8 = match mouse.kind {
        MouseEventKind::Down(MouseButton::Left) => 32,   // button 0 + 32
        MouseEventKind::Down(MouseButton::Middle) => 33, // button 1 + 32
        MouseEventKind::Down(MouseButton::Right) => 34,  // button 2 + 32
        MouseEventKind::Up(_) => 35,                     // release (3 + 32)
        MouseEventKind::ScrollUp => 96,                  // wheel up (64 + 32)
        MouseEventKind::ScrollDown => 97,                // wheel down (65 + 32)
        _ => return Ok(()),
    };

    let bytes = [0x1b_u8, b'[', b'M', cb, cx, cy];
    if let Some(tab) = app.tabs.get_mut(app.active_tab) {
        if let Some(ref mut s) = tab.session {
            s.write(&bytes)?;
        }
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
            // Read fields before taking a mutable borrow so app.spawn_session
            // (which immutably borrows self) does not conflict.
            let (cols, rows) = app.tabs[idx].last_inner_size;
            let cwd = app.tabs[idx].worktree.path.clone();
            let size = PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            };
            let new_session = app.spawn_session(&cwd, size)?;
            let tab = &mut app.tabs[idx];
            tab.exited_modal = None;
            tab.vt.resize(cols, rows);
            tab.session = Some(new_session);
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
