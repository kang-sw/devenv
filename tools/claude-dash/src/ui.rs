//! Layout and rendering for all phases.
//!
//! Phase 2 layout:
//! - Row 0 (height 1): tab bar
//! - Row 1+: main VT panel with borders
//!
//! The active tab's VT content fills the main panel.  Background tabs
//! accumulate PTY state but are not drawn.

use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, Clear, Paragraph, Tabs},
    Frame,
};

use crate::app::App;

/// Draw the full TUI for the current phase.
///
/// Updates `tabs[active_tab].last_inner_size` from the authoritative inner
/// rect so the main loop can issue PTY resize ioctls on the next iteration.
pub fn draw(frame: &mut Frame, app: &mut App) {
    if app.tabs.is_empty() {
        return;
    }

    let area = frame.area();

    // Phase-2 layout: tab bar (1 row) + main panel.
    let chunks = Layout::vertical([Constraint::Length(1), Constraint::Min(0)]).split(area);

    // --- Tab bar ---
    draw_tab_bar(frame, app, chunks[0]);

    // --- Main panel ---
    let vt_block = Block::default().borders(Borders::ALL);
    let inner = vt_block.inner(chunks[1]);
    frame.render_widget(vt_block, chunks[1]);

    // Cache authoritative inner dimensions for resize check.
    if let Some(tab) = app.tabs.get_mut(app.active_tab) {
        tab.last_inner_size = (inner.width, inner.height);
        tab.vt.render_into(frame.buffer_mut(), inner);

        // Process-exit modal.
        if let Some(ref modal) = tab.exited_modal {
            draw_exit_modal(frame, area, modal.is_removed_worktree, &format!("{:?}", modal.status));
        }
    }
}

fn draw_tab_bar(frame: &mut Frame, app: &App, area: Rect) {
    let titles: Vec<Line> = app
        .tabs
        .iter()
        .map(|tab| {
            if tab.worktree.is_removed {
                Line::from(vec![
                    Span::raw(tab.worktree.name.clone()),
                    Span::styled(" [removed]", Style::default().fg(Color::Red)),
                ])
            } else {
                Line::from(tab.worktree.name.clone())
            }
        })
        .collect();

    let tabs = Tabs::new(titles)
        .select(app.active_tab)
        .highlight_style(Style::default().add_modifier(Modifier::REVERSED));

    frame.render_widget(tabs, area);
}

/// Draw a centred exit modal overlay.
fn draw_exit_modal(frame: &mut Frame, area: Rect, is_removed: bool, status: &str) {
    let modal_w = (area.width / 3).max(36);
    let modal_h = 5u16;
    let modal_x = area.x + area.width.saturating_sub(modal_w) / 2;
    let modal_y = area.y + area.height.saturating_sub(modal_h) / 2;
    let modal_area = Rect::new(modal_x, modal_y, modal_w, modal_h);

    let text = if is_removed {
        format!("Process exited ({}).\n[X] Close tab", status)
    } else {
        format!("Process exited ({}).\n[R] Restart  [X] Close tab", status)
    };

    let paragraph = Paragraph::new(text)
        .block(Block::default().borders(Borders::ALL).title(" Exit "));

    frame.render_widget(Clear, modal_area);
    frame.render_widget(paragraph, modal_area);
}
