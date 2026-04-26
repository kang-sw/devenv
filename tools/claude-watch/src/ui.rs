use ratatui::{
    Frame,
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Scrollbar, ScrollbarOrientation, ScrollbarState, Wrap},
};

use crate::app::{App, LEFT_PANEL_PERCENT};

/// Draw the full TUI layout.  Takes `&mut App` so that `draw_content_panel`
/// can resolve `needs_scroll_to_bottom` using the exact ratatui layout width.
pub fn draw(frame: &mut Frame, app: &mut App) {
    let area = frame.area();
    let chunks = Layout::horizontal([
        Constraint::Percentage(LEFT_PANEL_PERCENT),
        Constraint::Percentage(100 - LEFT_PANEL_PERCENT),
    ])
    .split(area);

    draw_session_list(frame, app, chunks[0]);
    draw_content_panel(frame, app, chunks[1]);
}

/// Format a token count into a human-readable string.
fn format_tokens(n: u64) -> String {
    if n < 1_000 {
        format!("{}", n)
    } else if n < 1_000_000 {
        format!("{:.1}k", n as f64 / 1_000.0)
    } else {
        format!("{:.1}M", n as f64 / 1_000_000.0)
    }
}

fn draw_session_list(frame: &mut Frame, app: &App, area: ratatui::layout::Rect) {
    let items: Vec<ListItem> = app
        .sessions
        .iter()
        .map(|s| {
            let ts = s.modified.format("%m/%d %H:%M").to_string();
            let style = if s.active {
                Style::default().fg(Color::Green)
            } else {
                match s.is_headless {
                    None => Style::default().fg(Color::DarkGray),       // not yet parsed
                    Some(true) => Style::default().fg(Color::White),    // -p / headless
                    Some(false) => Style::default().fg(Color::Yellow),  // interactive
                }
            };
            let main_text = format!("{} ({})", s.label, ts);
            let token_text = if let Some(n) = s.token_total {
                format!(" [{}]", format_tokens(n))
            } else {
                String::new()
            };
            ListItem::new(Line::from(vec![
                Span::styled(main_text, style),
                Span::styled(token_text, Style::default().fg(Color::Cyan)),
            ]))
        })
        .collect();

    let list = List::new(items)
        .block(Block::default().title(" Sessions ").borders(Borders::ALL))
        .highlight_style(Style::default().add_modifier(Modifier::REVERSED))
        .highlight_symbol("> ");

    let mut state = ListState::default().with_selected(if app.sessions.is_empty() {
        None
    } else {
        Some(app.selected)
    });

    frame.render_stateful_widget(list, area, &mut state);
}

fn draw_content_panel(frame: &mut Frame, app: &mut App, area: ratatui::layout::Rect) {
    // Use the exact inner width from the ratatui layout rect.  This is the
    // authoritative source of truth for wrapping calculations and is written
    // back to the cached field so that interactive scroll methods
    // (scroll_down / scroll_page_down) use the same value on the next
    // event-loop iteration.
    let pw = area.width.saturating_sub(2) as usize;
    app.right_panel_inner_width = area.width.saturating_sub(2);

    // Resolve scroll-to-bottom here, where the exact ratatui layout width is
    // known, rather than in update_content_height which uses an approximation.
    if app.needs_scroll_to_bottom {
        app.needs_scroll_to_bottom = false;
        app.scroll_offset = app.scroll_to_bottom_offset_for_width(pw);
    }

    let title = app
        .selected_session()
        .map(|s| format!(" {} ", s.label))
        .unwrap_or_else(|| " Content ".to_string());

    let block = Block::default().title(title).borders(Borders::ALL);

    if app.rendered_lines.is_empty() {
        let msg = if app.sessions.is_empty() {
            "No sessions found."
        } else {
            "Loading…"
        };
        frame.render_widget(Paragraph::new(msg).block(block), area);
        return;
    }

    // Clamp scroll to valid range using usize arithmetic; cast only for the
    // Paragraph::scroll call which requires (u16, u16).
    let max_scroll = app.scroll_to_bottom_offset_for_width(pw);
    let scroll = app.scroll_offset.min(max_scroll).min(u16::MAX as usize) as u16;

    // Clone the pre-rendered lines into the Paragraph.  Line<'static> spans
    // own their content as Strings so this allocates, but the session content
    // is only re-rendered when the file changes — not every frame.
    let p = Paragraph::new(app.rendered_lines.clone())
        .block(block)
        .wrap(Wrap { trim: false })
        .scroll((scroll, 0));

    frame.render_widget(p, area);

    if max_scroll > 0 {
        let mut scrollbar_state = ScrollbarState::new(max_scroll)
            .position(app.scroll_offset);
        frame.render_stateful_widget(
            Scrollbar::new(ScrollbarOrientation::VerticalRight),
            area,
            &mut scrollbar_state,
        );
    }
}
