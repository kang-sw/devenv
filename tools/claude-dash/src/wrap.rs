//! Word-wrap helpers for accurate scroll-offset calculation.
//!
//! Ported verbatim from `claude-watch::app::visual_rows`.  The function
//! simulates ratatui's `WordWrapper` with `trim: false` to count how many
//! visual rows a logical `Line` occupies at a given panel width.  This is
//! used to compute the correct `max_scroll` value for the JSONL panel
//! scrollbar so the thumb position matches the visible content.
//!
//! A future refactor may extract a shared `claude-jsonl` utility crate; at
//! that point this function and `claude-watch::app::visual_rows` should be
//! unified. For now the copy-then-share-later convention is intentional.

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
                // The token occupies the current row first; additional rows
                // needed = ceil(token_w / panel_width) - 1 = (token_w-1)/panel_width.
                rows += (token_w - 1) / panel_width;
                col = token_w % panel_width;
            } else {
                // Normal word wrap: move token to the next row.
                rows += 1;
                if token_w > panel_width {
                    rows += (token_w - 1) / panel_width;
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::text::{Line, Span};

    fn line(s: &str) -> Line<'static> {
        Line::from(Span::raw(s.to_string()))
    }

    // Branch: panel_width == 0 — degenerate, always returns 1.
    #[test]
    fn zero_panel_width_returns_one_row() {
        assert_eq!(visual_rows(&line("hello"), 0), 1);
    }

    // Branch: empty text — always returns 1 (blank line occupies one visual row).
    #[test]
    fn empty_line_returns_one_row() {
        assert_eq!(visual_rows(&line(""), 10), 1);
    }

    // Common case: short text that fits entirely within the panel.
    #[test]
    fn text_shorter_than_panel_occupies_one_row() {
        assert_eq!(visual_rows(&line("hello"), 10), 1);
    }

    // Common case: normal word-wrap at a token boundary produces two rows.
    #[test]
    fn two_tokens_overflowing_panel_wrap_to_two_rows() {
        // "hello "(6) + "world"(5) = 11 wide; panel = 10.
        // "hello " fits (col=6); "world" doesn't (6+5=11>10), wraps → row 2.
        assert_eq!(visual_rows(&line("hello world"), 10), 2);
    }

    // C1 regression — col==0 path: token_w exact multiple of panel_width.
    // "ABCDEF"(6) with panel=3 → "ABC"/"DEF" → 2 rows.
    // Bug produced 3 (used token_w/panel_width = 6/3 = 2 extra rows instead of 1).
    #[test]
    fn token_exact_multiple_of_panel_width_hard_breaks_to_correct_row_count() {
        assert_eq!(visual_rows(&line("ABCDEF"), 3), 2);
    }

    // Hard-break: token wider than panel that does NOT fill it exactly (non-multiple).
    // "ABCDE"(5) with panel=3 → "ABC"/"DE" → 2 rows.
    #[test]
    fn token_non_multiple_of_panel_width_hard_breaks_correctly() {
        assert_eq!(visual_rows(&line("ABCDE"), 3), 2);
    }

    // C1 regression — col>0 path: long token exact multiple after leading text.
    // "X "(2) fits, col=2; "ABCDEF"(6) with panel=3 wraps to new row then
    // hard-breaks: row for "ABC", row for "DEF" → total 3 rows.
    // Bug produced 4 (used 6/3 = 2 extra rows instead of 1).
    #[test]
    fn long_token_exact_multiple_after_text_hard_breaks_correctly() {
        // "X " → col 2; "ABCDEF" → new row + (6-1)/3=1 extra → rows = 3.
        assert_eq!(visual_rows(&line("X ABCDEF"), 3), 3);
    }

    // Hard-break at col>0, non-exact multiple — should be unaffected by C1 fix.
    // "X "(2) + "ABCDE"(5) with panel=4: col=2; 2+5=7>4 → new row + (5-1)/4=1 extra
    // → rows = 3.
    #[test]
    fn long_token_non_multiple_after_text_hard_breaks_correctly() {
        assert_eq!(visual_rows(&line("X ABCDE"), 4), 3);
    }
}
