//! Word-wrap helpers for accurate scroll-offset calculation.
//!
//! Ported verbatim from `claude-watch::app::visual_rows`.  The function
//! simulates ratatui's `WordWrapper` with `trim: false` to count how many
//! visual rows a logical `Line` occupies at a given panel width.  This is
//! used to compute the correct `max_scroll` value for the JSONL panel
//! scrollbar so the thumb position matches the visible content.

use ratatui::text::Line;

/// Return the number of visual rows a logical `line` occupies when rendered
/// inside a panel of `panel_width` columns.
///
/// Simulates ratatui's `WordWrapper` with `trim: false`: text is split into
/// whitespace-delimited tokens (each token includes its trailing whitespace),
/// and tokens that would overflow the current row are wrapped to the next row.
/// Words longer than `panel_width` are hard-broken at the column boundary.
/// This matches ratatui's behaviour closely enough to produce accurate
/// scroll-to-bottom offsets.
pub(crate) fn visual_rows(line: &Line, panel_width: usize) -> usize {
    if panel_width == 0 {
        return 1;
    }
    let text: String = line.spans.iter().map(|s| s.content.as_ref()).collect();
    if text.is_empty() {
        return 1;
    }

    let mut rows = 1usize;
    let mut col = 0usize;

    // split_inclusive keeps the whitespace delimiter attached to the preceding
    // token, matching the pending_whitespace + pending_word flush order in
    // ratatui's WordWrapper (trim=false).
    for token in text.split_inclusive(|c: char| c.is_whitespace()) {
        let token_w: usize = token
            .chars()
            .map(|c| unicode_width::UnicodeWidthChar::width(c).unwrap_or(0))
            .sum();
        if token_w == 0 {
            continue;
        }
        if col + token_w > panel_width {
            if col == 0 {
                // Token wider than the whole panel — hard-break inside it.
                rows += token_w / panel_width;
                col = token_w % panel_width;
            } else {
                // Normal word wrap: move token to the next row.
                rows += 1;
                if token_w > panel_width {
                    rows += token_w / panel_width;
                    col = token_w % panel_width;
                } else {
                    col = token_w;
                }
            }
        } else {
            col += token_w;
        }
    }

    rows
}
