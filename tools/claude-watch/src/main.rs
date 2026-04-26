mod app;
mod parser;
mod process;
mod renderer;
mod session;
mod ui;

use app::App;
use crossterm::{
    event::{
        self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyModifiers, MouseButton,
        MouseEventKind,
    },
    execute,
};
use std::io::stdout;
use std::time::Duration;

/// How often the event loop polls for terminal input.
const EVENT_POLL_MS: u64 = 100;
/// How often the session list is refreshed from disk.
const SESSION_REFRESH_SECS: u64 = 1;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut terminal = ratatui::init();
    execute!(stdout(), EnableMouseCapture)?;

    let result = run_app(&mut terminal);

    // Disable mouse capture on all exit paths before restoring the terminal.
    let _ = execute!(stdout(), DisableMouseCapture);
    ratatui::restore();

    if let Err(ref e) = result {
        eprintln!("Error: {e}");
    }
    result
}

fn run_app(terminal: &mut ratatui::DefaultTerminal) -> Result<(), Box<dyn std::error::Error>> {
    let mut app = App::new();
    app.load_selected_session();

    loop {
        // Resolve content height and any pending scroll-to-bottom BEFORE draw
        // so the draw function remains a pure read of App state.
        let panel_height = crossterm::terminal::size()
            .map(|(_, h)| h.saturating_sub(2) as usize)
            .unwrap_or(app.content_panel_height);
        app.update_content_height(panel_height);

        terminal.draw(|f| ui::draw(f, &app))?;

        // Poll with a short timeout to allow background refresh.
        if event::poll(Duration::from_millis(EVENT_POLL_MS))? {
            match event::read()? {
                Event::Key(key) => match (key.modifiers, key.code) {
                    (_, KeyCode::Char('q')) => app.should_quit = true,
                    (KeyModifiers::CONTROL, KeyCode::Char('c')) => app.should_quit = true,

                    (_, KeyCode::Up) => app.select_prev(),
                    (_, KeyCode::Down) => app.select_next(),

                    (_, KeyCode::Char('j')) => app.scroll_down(),
                    (_, KeyCode::Char('k')) => app.scroll_up(),
                    (_, KeyCode::PageDown) => app.scroll_page_down(),
                    (_, KeyCode::PageUp) => app.scroll_page_up(),
                    (_, KeyCode::Char('t')) => app.toggle_thinking(),

                    _ => {}
                },
                Event::Mouse(mouse_event) => handle_mouse_event(&mut app, mouse_event),
                _ => {}
            }
        }

        if app.should_quit {
            break;
        }

        // Refresh session list + live-tail content at ~1 s intervals.
        if app.last_refresh.elapsed() >= Duration::from_secs(SESSION_REFRESH_SECS) {
            app.refresh_sessions();
            app.maybe_reload_content();
            app.last_refresh = std::time::Instant::now();
        }

        // Poll running processes at ~2 s intervals.
        app.poll_processes_if_due();
    }

    Ok(())
}

/// Handle a mouse event.
///
/// Scroll events (wheel up/down) delegate to the same scroll methods used by
/// the j/k keys.  A left-click inside the left panel selects the session at
/// that visual row, accounting for the list's current scroll offset.  Clicks
/// outside the left panel are silently ignored.
fn handle_mouse_event(app: &mut App, event: crossterm::event::MouseEvent) {
    match event.kind {
        MouseEventKind::ScrollDown => app.scroll_down(),
        MouseEventKind::ScrollUp => app.scroll_up(),

        MouseEventKind::Down(MouseButton::Left) => {
            // Derive the left panel boundary from the current terminal size,
            // matching the Constraint::Percentage(30) used by the layout.
            let (term_width, term_height) = match crossterm::terminal::size() {
                Ok(s) => s,
                Err(_) => return,
            };
            let left_panel_width = (term_width as u32 * 30 / 100) as u16;

            let col = event.column;
            let row = event.row;

            // Clicks outside the left panel are ignored.
            if col >= left_panel_width {
                return;
            }

            // Row 0 is the top border; row term_height-1 is the bottom border.
            if row == 0 || row + 1 >= term_height {
                return;
            }

            // Convert from visual row to session index, accounting for the
            // list scroll offset that ratatui computes when `selected` is
            // beyond the first viewport page.
            //
            // ratatui starts ListState at offset=0 each frame, then adjusts:
            //   if selected >= visible_height → offset = selected - visible_height + 1
            //   otherwise                     → offset = 0
            let visual_index = (row - 1) as usize;
            let visible_height = (term_height as usize).saturating_sub(2);
            let list_offset = app.selected.saturating_sub(visible_height.saturating_sub(1));
            let session_index = visual_index + list_offset;

            if session_index < app.sessions.len() {
                app.selected = session_index;
                app.needs_scroll_to_bottom = true;
            }
        }

        _ => {}
    }
}

