mod app;
mod parser;
mod process;
mod renderer;
mod session;
mod ui;

use app::{App, LEFT_PANEL_PERCENT};
use crossterm::{
    event::{
        self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode, KeyModifiers, MouseButton,
        MouseEvent, MouseEventKind,
    },
    execute,
};
use std::io::stdout;
use std::time::Duration;

/// Frame budget in milliseconds (~60 fps); the event loop sleeps this long
/// after each draw to avoid busy-spinning.
const FRAME_MS: u64 = 16;
/// How often the session list is refreshed from disk.
const SESSION_REFRESH_SECS: u64 = 1;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut terminal = ratatui::init();

    // Augment the panic hook installed by ratatui::init() so that a panic also
    // disables mouse capture before the terminal is restored.
    let prev_hook = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let _ = execute!(stdout(), DisableMouseCapture);
        prev_hook(info);
    }));

    // EnableMouseCapture writes an escape sequence; errors here are non-fatal.
    let _ = execute!(stdout(), EnableMouseCapture);

    let result = run_app(&mut terminal);

    // Disable mouse capture on normal exit and on error return, in addition to
    // the panic hook above.
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
        // Run background periodic tasks at their own intervals.
        if app.last_refresh.elapsed() >= Duration::from_secs(SESSION_REFRESH_SECS) {
            app.refresh_sessions();
            app.maybe_reload_content();
            app.last_refresh = std::time::Instant::now();
        }
        app.poll_processes_if_due();
        app.poll_token_results();

        // Drain all pending events without blocking so input is processed
        // immediately, eliminating up to one frame of latency.
        while event::poll(Duration::ZERO)? {
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

        // Update cached dimensions before draw.  draw_content_panel writes
        // scroll_offset and syncs right_panel_inner_width from the actual
        // ratatui layout rect, so the cached values here serve as a fallback
        // for interactive scroll (scroll_down / scroll_page_down).
        match crossterm::terminal::size() {
            Ok((w, h)) => {
                let left_w = (w as u32 * LEFT_PANEL_PERCENT as u32 / 100) as u16;
                let right_inner_w = (w - left_w).saturating_sub(2);
                app.left_panel_width = left_w;
                app.right_panel_inner_width = right_inner_w;
                app.update_content_height(h.saturating_sub(2) as usize);
            }
            Err(_) => {
                // Keep previously cached dimensions on transient failure.
                // Do NOT call update_content_height here: right_panel_inner_width
                // may still be 0 from initialisation, and consuming
                // needs_scroll_to_bottom with pw=0 would place the viewport
                // at the wrong position.  The flag is resolved on the next
                // successful size query.
            }
        }

        terminal.draw(|f| ui::draw(f, &mut app))?;

        // Sleep for one frame budget.  We use thread::sleep rather than
        // event::poll so the intent is unambiguous: this is a fixed-duration
        // pause, not an event wait.  Events that arrive during this sleep are
        // picked up at the top of the next iteration's drain loop.
        std::thread::sleep(Duration::from_millis(FRAME_MS));
    }

    Ok(())
}

/// Handle a mouse event.
///
/// Scroll events (wheel up/down) delegate to the same scroll methods used by
/// the j/k keys.  A left-click inside the left panel selects the session at
/// that visual row, accounting for the list's current scroll offset.  Clicks
/// outside the left panel are silently ignored.
fn handle_mouse_event(app: &mut App, event: MouseEvent) {
    match event.kind {
        MouseEventKind::ScrollDown => app.scroll_down(),
        MouseEventKind::ScrollUp => app.scroll_up(),

        MouseEventKind::Down(MouseButton::Left) => {
            // Reconstruct full terminal height: content panel height + 2 borders.
            let term_height = (app.content_panel_height + 2) as u16;
            if let Some(idx) = compute_session_index(
                event.row,
                event.column,
                app.left_panel_width,
                term_height,
                app.selected,
                app.sessions.len(),
            ) {
                app.selected = idx;
                app.enqueue_if_needed(idx);
                app.needs_scroll_to_bottom = true;
            }
        }

        _ => {}
    }
}

/// Map a mouse click position to a session list index.
///
/// Returns `None` if the click lands on a border row, outside the left panel,
/// or past the end of the session list.  The `left_panel_width` and
/// `term_height` parameters are injected so this function is pure and testable
/// without a live terminal.
///
/// List scroll-offset formula mirrors ratatui's `ListState` adjustment, which
/// starts from `offset = 0` each frame and adjusts to keep the selected item
/// visible:
///   `selected >= visible_height` → `offset = selected − visible_height + 1`
///   otherwise                    → `offset = 0`
fn compute_session_index(
    row: u16,
    col: u16,
    left_panel_width: u16,
    term_height: u16,
    selected: usize,
    session_count: usize,
) -> Option<usize> {
    // Clicks outside the left panel are ignored.
    if col >= left_panel_width {
        return None;
    }
    // Row 0 is the top border; row `term_height - 1` is the bottom border.
    if row == 0 || row + 1 >= term_height {
        return None;
    }
    let visual_index = (row - 1) as usize;
    let visible_height = (term_height as usize).saturating_sub(2);
    let list_offset = selected.saturating_sub(visible_height.saturating_sub(1));
    let session_index = visual_index + list_offset;
    if session_index < session_count {
        Some(session_index)
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Terminal geometry shared across tests: 80 columns × 24 rows.
    //   left_panel_width = 80 * LEFT_PANEL_PERCENT / 100 = 24
    //   visible_height   = TH - 2 = 22
    const TERM_W: u16 = 80;
    const TH: u16 = 24;
    const LPW: u16 = TERM_W * LEFT_PANEL_PERCENT / 100;
    const VH: usize = (TH - 2) as usize;

    #[test]
    fn first_page_no_scroll_offset() {
        // selected=0 → list_offset=0; click row=1 → session 0.
        assert_eq!(compute_session_index(1, 5, LPW, TH, 0, 10), Some(0));
    }

    #[test]
    fn last_item_on_first_page() {
        // selected=VH-1=21 → list_offset=21.saturating_sub(21)=0
        // click row=VH=22 → visual_index=21, session_index=21
        assert_eq!(
            compute_session_index(VH as u16, 5, LPW, TH, VH - 1, 30),
            Some(VH - 1)
        );
    }

    #[test]
    fn first_scroll_position() {
        // selected=VH=22 → list_offset=22-21=1
        // click row=1 → visual_index=0, session_index=1
        assert_eq!(compute_session_index(1, 5, LPW, TH, VH, 30), Some(1));
    }

    #[test]
    fn deep_scroll() {
        // selected=25 → list_offset=25-21=4
        // click row=6 → visual_index=5, session_index=9
        assert_eq!(compute_session_index(6, 5, LPW, TH, 25, 30), Some(9));
    }

    #[test]
    fn right_panel_click_rejected() {
        // col == left_panel_width → boundary is outside
        assert_eq!(compute_session_index(5, LPW, LPW, TH, 0, 10), None);
        // col > left_panel_width
        assert_eq!(compute_session_index(5, LPW + 1, LPW, TH, 0, 10), None);
    }

    #[test]
    fn top_border_rejected() {
        assert_eq!(compute_session_index(0, 5, LPW, TH, 0, 10), None);
    }

    #[test]
    fn bottom_border_rejected() {
        assert_eq!(compute_session_index(TH - 1, 5, LPW, TH, 0, 10), None);
    }

    #[test]
    fn oob_session_index_is_none() {
        // session_count=3; click would map to index=5 → None
        assert_eq!(compute_session_index(6, 5, LPW, TH, 0, 3), None);
    }
}
