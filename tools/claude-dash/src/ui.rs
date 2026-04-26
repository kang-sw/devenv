//! Layout and rendering for all phases.
//!
//! Phase 1: full-screen layout with a 1-row placeholder bar at the top
//! and the VT panel filling the rest.  Process-exit modal overlaid when
//! the child exits.

use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Modifier, Style},
    widgets::{Block, Borders, Clear, Paragraph},
    Frame,
};

use crate::app::App;

/// Draw the full TUI for the current phase.
///
/// Updates `app.last_panel_size` from the authoritative inner rect so that
/// the main loop can issue PTY resize ioctls on the next iteration.
pub fn draw(frame: &mut Frame, app: &mut App) {
    let area = frame.area();

    // Phase-1 layout: 1-row placeholder bar + remainder for VT panel.
    let chunks = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).split(area);

    // Top bar — placeholder title.
    let bar = Block::default().title(" claude-dash ");
    frame.render_widget(bar, chunks[0]);

    // VT panel.
    let vt_block = Block::default().borders(Borders::ALL);
    let inner = vt_block.inner(chunks[1]);
    frame.render_widget(vt_block, chunks[1]);

    // Cache the authoritative inner dimensions for the resize check.
    app.last_panel_size = (inner.width, inner.height);

    // Render the VT surface cells into the inner area.
    app.vt.render_into(frame.buffer_mut(), inner);

    // Process-exit modal (overlaid on top of everything).
    if let Some(ref modal) = app.exited_modal {
        draw_exit_modal(frame, area, &format!("{:?}", modal.status));
    }
}

/// Draw a centred exit modal overlay.
fn draw_exit_modal(frame: &mut Frame, area: Rect, status: &str) {
    let modal_w = (area.width / 3).max(32);
    let modal_h = 5u16;
    let modal_x = area.x + area.width.saturating_sub(modal_w) / 2;
    let modal_y = area.y + area.height.saturating_sub(modal_h) / 2;
    let modal_area = Rect::new(modal_x, modal_y, modal_w, modal_h);

    let text = format!("Process exited ({}).\n[R] Restart  [Q] Quit", status);
    let paragraph = Paragraph::new(text)
        .block(Block::default().borders(Borders::ALL).title(" Exit "));

    frame.render_widget(Clear, modal_area);
    frame.render_widget(paragraph, modal_area);
}
