mod app;
mod parser;
mod process;
mod renderer;
mod session;
mod ui;

use app::App;
use crossterm::event::{self, Event, KeyCode, KeyModifiers};
use std::time::Duration;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut terminal = ratatui::init();
    let result = run_app(&mut terminal);
    ratatui::restore();

    if let Err(ref e) = result {
        eprintln!("Error: {e}");
    }
    result
}

fn run_app(
    terminal: &mut ratatui::DefaultTerminal,
) -> Result<(), Box<dyn std::error::Error>> {
    let mut app = App::new();

    // Load the initially selected session immediately.
    app.load_selected_session();

    loop {
        terminal.draw(|f| ui::draw(f, &mut app))?;

        // Poll with a 100 ms timeout so background refresh can fire.
        if event::poll(Duration::from_millis(100))? {
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

        // Refresh session list + content every ~1 second.
        if app.last_refresh.elapsed() >= Duration::from_secs(1) {
            app.refresh_sessions();
            app.maybe_reload_content();
            app.last_refresh = std::time::Instant::now();
        }

        // Poll running processes every ~2 seconds.
        app.poll_processes_if_due();
    }

    Ok(())
}
