use ratatui::{
    Frame,
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
    text::Span,
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap},
};

use crate::app::{App, LEFT_PANEL_PERCENT};

/// Draw the full TUI layout — pure read of `App` state, no side effects.
pub fn draw(frame: &mut Frame, app: &App) {
    let area = frame.area();
    let chunks = Layout::horizontal([
        Constraint::Percentage(LEFT_PANEL_PERCENT),
        Constraint::Percentage(100 - LEFT_PANEL_PERCENT),
    ])
    .split(area);

    draw_session_list(frame, app, chunks[0]);
    draw_content_panel(frame, app, chunks[1]);
}

fn draw_session_list(frame: &mut Frame, app: &App, area: ratatui::layout::Rect) {
    let items: Vec<ListItem> = app
        .sessions
        .iter()
        .map(|s| {
            let ts = s.modified.format("%m/%d %H:%M").to_string();
            let text = format!("{} ({})", s.label, ts);
            let style = if s.active {
                Style::default().fg(Color::Green)
            } else {
                Style::default()
            };
            ListItem::new(Span::styled(text, style))
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

fn draw_content_panel(frame: &mut Frame, app: &App, area: ratatui::layout::Rect) {
    // content_panel_height is maintained by the event loop via
    // App::update_content_height — we only read it here.
    let inner_height = app.content_panel_height;

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
    //
    // max_scroll is computed via a reverse visual-row walk so that wrapped
    // lines are accounted for: we find the lowest logical-line offset such
    // that the content below it fills the panel.
    let pw = area.width.saturating_sub(2) as usize;
    let max_scroll = {
        let mut visual_acc: usize = 0;
        let mut max = app.rendered_lines.len();
        for (i, line) in app.rendered_lines.iter().enumerate().rev() {
            let w: usize = line.spans.iter().map(|s| s.content.chars().count()).sum();
            let vr = if pw == 0 || w == 0 { 1 } else { (w + pw - 1) / pw };
            if visual_acc + vr > inner_height {
                max = i + 1;
                break;
            }
            visual_acc += vr;
            max = i;
        }
        max
    };
    let scroll = app.scroll_offset.min(max_scroll).min(u16::MAX as usize) as u16;

    // Clone the pre-rendered lines into the Paragraph.  Line<'static> spans
    // own their content as Strings so this allocates, but the session content
    // is only re-rendered when the file changes — not every frame.
    let p = Paragraph::new(app.rendered_lines.clone())
        .block(block)
        .wrap(Wrap { trim: false })
        .scroll((scroll, 0));

    frame.render_widget(p, area);
}
