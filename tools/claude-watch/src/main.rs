mod app;
mod parser;
mod process;
mod renderer;
mod session;
mod ui;

use app::App;
use crossterm::event::{self, Event, KeyCode, KeyModifiers};
use std::time::Duration;

/// How often the event loop polls for terminal input.
const EVENT_POLL_MS: u64 = 100;
/// How often the session list is refreshed from disk.
const SESSION_REFRESH_SECS: u64 = 1;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut terminal = ratatui::init();
    let result = run_app(&mut terminal);
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
            if let Event::Key(key) = event::read()? {
                match (key.modifiers, key.code) {
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
                }
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
