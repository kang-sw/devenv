//! Layout and rendering for all phases.
//!
//! Phase 3 layout:
//! - Row 0 (height 1): tab bar
//! - Row 1 (height 1): agent slot row
//! - Row 2+: main panel (VT or JSONL viewer) with borders

use ratatui::{
    layout::{Constraint, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span},
    widgets::{
        Block, Borders, Clear, Paragraph, Scrollbar, ScrollbarOrientation, ScrollbarState, Tabs,
        Wrap,
    },
    Frame,
};

use crate::app::{App, SlotKind};

/// Lines per page for PageUp/PageDown in the agent viewer.
pub const PAGE_SCROLL: usize = 20;

/// Draw the full TUI for the current phase.
///
/// Updates `tabs[active_tab].last_inner_size` from the authoritative inner
/// rect so the main loop can issue PTY resize ioctls on the next iteration.
pub fn draw(frame: &mut Frame, app: &mut App) {
    if app.tabs.is_empty() {
        return;
    }

    let area = frame.area();

    // Phase-3 layout: tab bar (1) + slot row (1) + main panel.
    let chunks = Layout::vertical([
        Constraint::Length(1),
        Constraint::Length(1),
        Constraint::Min(0),
    ])
    .split(area);

    draw_tab_bar(frame, app, chunks[0]);
    draw_slot_row(frame, app, chunks[1]);

    // --- Main panel ---
    let tab_idx = app.active_tab;
    let tab = match app.tabs.get_mut(tab_idx) {
        Some(t) => t,
        None => return,
    };

    let vt_block = Block::default().borders(Borders::ALL);
    let inner = vt_block.inner(chunks[2]);
    frame.render_widget(vt_block, chunks[2]);

    // Cache authoritative inner dimensions for resize check.
    tab.last_inner_size = (inner.width, inner.height);

    match tab.active_slot {
        SlotKind::Main => {
            tab.vt.render_into(frame.buffer_mut(), inner);
        }
        SlotKind::Named(_) => {
            draw_agent_view(frame, tab, inner);
        }
    }

    // Process-exit modal.
    if let Some(ref modal) = tab.exited_modal {
        let is_removed = modal.is_removed_worktree;
        let status_str = format!("{:?}", modal.status);
        draw_exit_modal(frame, area, is_removed, &status_str);
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

fn draw_slot_row(frame: &mut Frame, app: &App, area: Rect) {
    let tab = match app.tabs.get(app.active_tab) {
        Some(t) => t,
        None => return,
    };

    let active_slot_idx = match tab.active_slot {
        SlotKind::Main => 0usize,
        SlotKind::Named(i) => i + 1,
    };

    let mut spans: Vec<Span> = Vec::new();

    // Slot 1: main PTY.
    let main_style = if active_slot_idx == 0 {
        Style::default().add_modifier(Modifier::REVERSED)
    } else {
        Style::default()
    };
    spans.push(Span::styled("[1:main]", main_style));

    // Slots 2–9: named agents.
    for (i, agent) in tab.named_agents.iter().enumerate() {
        spans.push(Span::raw("  "));
        let slot_num = i + 2;
        let label = format!("[{}:{}]", slot_num, agent.name);
        let style = if active_slot_idx == i + 1 {
            Style::default().add_modifier(Modifier::REVERSED)
        } else {
            Style::default()
        };
        spans.push(Span::styled(label, style));

        // Token count (Phase 4 adds colour; Phase 3 shows plain).
        if let Some(n) = agent.token_total {
            spans.push(Span::styled(
                format!(" {}", format_tokens(n)),
                Style::default().fg(Color::Cyan),
            ));
        }
    }

    frame.render_widget(Line::from(spans), area);
}

fn draw_agent_view(frame: &mut Frame, tab: &mut crate::app::WorktreeTab, area: Rect) {
    let view = match tab.agent_view.as_mut() {
        Some(v) => v,
        None => return,
    };

    // Leave one column for the scrollbar track.
    let panel_width = area.width.saturating_sub(1) as usize;
    let panel_height = area.height as usize;

    // Compute or reuse total visual rows using word-wrap simulation (Phase 4).
    // Invalidate the cache when panel_width changes.
    let total_visual_rows = match view.cached_visual_rows {
        Some((rows, w)) if w == area.width => rows,
        _ => {
            let rows: usize = view
                .rendered_lines
                .iter()
                .map(|line| crate::wrap::visual_rows(line, panel_width))
                .sum();
            let rows = rows.max(1);
            view.cached_visual_rows = Some((rows, area.width));
            rows
        }
    };

    let max_scroll = total_visual_rows.saturating_sub(panel_height);
    // Saturate before casting: scroll_offset is usize; as u16 would wrap silently
    // for files with >65 535 visual rows.
    let scroll = view.scroll_offset.min(max_scroll).min(u16::MAX as usize) as u16;

    let p = Paragraph::new(view.rendered_lines.clone())
        .wrap(Wrap { trim: false })
        .scroll((scroll, 0));
    frame.render_widget(p, area);

    // Vertical scrollbar when content overflows.
    if max_scroll > 0 {
        let mut scrollbar_state = ScrollbarState::new(max_scroll + panel_height)
            .viewport_content_length(panel_height)
            .position(view.scroll_offset.min(max_scroll));
        frame.render_stateful_widget(
            Scrollbar::new(ScrollbarOrientation::VerticalRight),
            area,
            &mut scrollbar_state,
        );
    }
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

    let paragraph =
        Paragraph::new(text).block(Block::default().borders(Borders::ALL).title(" Exit "));

    frame.render_widget(Clear, modal_area);
    frame.render_widget(paragraph, modal_area);
}

/// Format a token count as a human-readable string.
/// Copied from claude-watch (private fn; cannot import across crates).
/// A future refactor may extract a shared utility crate; for now the
/// copy-then-share-later convention is intentional.
fn format_tokens(n: u64) -> String {
    if n < 1_000 {
        format!("{}", n)
    } else if n < 1_000_000 {
        format!("{:.1}k", n as f64 / 1_000.0)
    } else {
        format!("{:.1}M", n as f64 / 1_000_000.0)
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    // --- format_tokens ---

    #[test]
    fn zero_tokens_formats_as_plain_zero() {
        assert_eq!(format_tokens(0), "0");
    }

    #[test]
    fn token_count_under_1k_formats_without_suffix() {
        assert_eq!(format_tokens(999), "999");
    }

    #[test]
    fn token_count_at_1k_formats_with_k_suffix() {
        assert_eq!(format_tokens(1_000), "1.0k");
    }

    #[test]
    fn token_count_mid_thousands_formats_with_one_decimal() {
        assert_eq!(format_tokens(12_300), "12.3k");
    }

    #[test]
    fn token_count_just_under_1m_formats_as_1000k() {
        assert_eq!(format_tokens(999_999), "1000.0k");
    }

    #[test]
    fn token_count_at_1m_formats_with_m_suffix() {
        assert_eq!(format_tokens(1_000_000), "1.0M");
    }

    #[test]
    fn large_token_count_formats_with_m_suffix() {
        assert_eq!(format_tokens(4_500_000), "4.5M");
    }
}
