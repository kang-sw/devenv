use ratatui::{
    Frame,
    layout::{Constraint, Layout},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{Block, Borders, List, ListItem, ListState, Paragraph, Wrap},
};
use crate::app::App;

pub fn draw(frame: &mut Frame, app: &mut App) {
    let area = frame.area();
    let chunks = Layout::horizontal([
        Constraint::Percentage(30),
        Constraint::Percentage(70),
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

fn draw_content_panel(frame: &mut Frame, app: &mut App, area: ratatui::layout::Rect) {
    let inner_height = area.height.saturating_sub(2);
    app.content_panel_height = inner_height;

    // Apply scroll-to-bottom when flagged.
    if app.needs_scroll_to_bottom {
        app.needs_scroll_to_bottom = false;
        let max = (app.rendered_lines.len() as u16).saturating_sub(inner_height);
        app.scroll_offset = max;
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
        let p = Paragraph::new(msg).block(block);
        frame.render_widget(p, area);
        return;
    }

    let max_scroll = (app.rendered_lines.len() as u16).saturating_sub(inner_height);
    let scroll = app.scroll_offset.min(max_scroll);

    let text: Vec<Line> = app.rendered_lines.clone();
    let p = Paragraph::new(text)
        .block(block)
        .wrap(Wrap { trim: false })
        .scroll((scroll, 0));

    frame.render_widget(p, area);
}
