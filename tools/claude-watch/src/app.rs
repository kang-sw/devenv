use std::collections::HashSet;
use std::path::PathBuf;
use std::time::{Duration, Instant};

use chrono::{DateTime, Local};

use ratatui::text::Line;

use crate::parser::parse_turns;
use crate::process::find_active_uuids;
use crate::renderer::{render_turns, RenderOptions};
use crate::session::{discover_sessions, parse_session_metadata, SessionEntry};

// ---------------------------------------------------------------------------
// Tuning constants
// ---------------------------------------------------------------------------

/// Lines scrolled per j / k keypress.
pub(crate) const SCROLL_STEP: usize = 3;
/// How often the process list is polled.
const PROCESS_POLL_SECS: u64 = 2;
/// Default panel height estimate before the first terminal-size query.
const INITIAL_PANEL_HEIGHT_GUESS: usize = 40;
/// Width of the left (session list) panel as a percentage of the terminal width.
/// Referenced by both `ui.rs` (layout constraint) and `main.rs` (mouse hit-test).
pub(crate) const LEFT_PANEL_PERCENT: u16 = 30;

// ---------------------------------------------------------------------------
// Application state
// ---------------------------------------------------------------------------

pub struct App {
    // --- session list ---
    pub(crate) sessions: Vec<SessionEntry>,
    pub(crate) selected: usize,
    pub(crate) last_refresh: Instant,
    pub(crate) should_quit: bool,

    // --- content panel ---
    pub(crate) rendered_lines: Vec<Line<'static>>,
    /// Lines scrolled from the top of the content (usize avoids u16 overflow
    /// on sessions with > 65 535 rendered lines).
    pub(crate) scroll_offset: usize,
    /// Inner height of the content panel (rows visible between borders).
    /// Updated by the event loop via `update_content_height` before each draw.
    pub(crate) content_panel_height: usize,
    /// Rendered width of the left (session list) panel in terminal columns.
    /// Updated by the event loop alongside `content_panel_height`.
    pub(crate) left_panel_width: u16,
    /// Inner width of the right (content) panel in terminal columns,
    /// excluding the block borders.  Updated by the event loop.
    pub(crate) right_panel_inner_width: u16,
    /// When set, `draw_content_panel` pins scroll to bottom on the next frame
    /// using the exact panel width from the ratatui layout rect.
    pub(crate) needs_scroll_to_bottom: bool,
    /// UUID of the session currently loaded in the right panel.
    pub(crate) loaded_uuid: Option<String>,
    /// mtime of the loaded session file — used to detect live updates.
    pub(crate) loaded_mtime: Option<std::time::SystemTime>,
    /// Whether thinking blocks are expanded.
    pub(crate) show_thinking: bool,
    /// Cached result of the visual-row sum over rendered_lines.
    /// Tuple is `(total_visual_rows, panel_width_used)`.  Invalidated whenever
    /// `rendered_lines` is reassigned or the panel width changes.
    pub(crate) cached_visual_rows: Option<(usize, u16)>,

    // --- process monitor ---
    pub(crate) active_uuids: HashSet<String>,
    pub(crate) last_process_poll: Instant,

    // --- background token parser ---
    /// Work queue: (uuid, path, mtime-at-enqueue).  The mtime is echoed back in
    /// the result so stale results can be discarded in `poll_token_results`.
    work_tx: std::sync::mpsc::SyncSender<(String, PathBuf, DateTime<Local>)>,
    result_rx: std::sync::mpsc::Receiver<(String, u64, bool, DateTime<Local>)>,
}

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

impl App {
    pub fn new() -> Self {
        let sessions = discover_sessions();

        let (work_tx, work_rx) =
            std::sync::mpsc::sync_channel::<(String, PathBuf, DateTime<Local>)>(64);
        let (result_tx, result_rx) =
            std::sync::mpsc::channel::<(String, u64, bool, DateTime<Local>)>();

        std::thread::spawn(move || {
            while let Ok((uuid, path, mtime)) = work_rx.recv() {
                if let Some((tokens, is_headless)) = parse_session_metadata(&path) {
                    let _ = result_tx.send((uuid, tokens, is_headless, mtime));
                }
            }
        });

        let mut app = App {
            sessions,
            selected: 0,
            last_refresh: Instant::now(),
            should_quit: false,

            rendered_lines: Vec::new(),
            scroll_offset: 0,
            content_panel_height: INITIAL_PANEL_HEIGHT_GUESS,
            left_panel_width: 0, // overwritten at the top of every event-loop iteration
            right_panel_inner_width: 0,
            needs_scroll_to_bottom: true,
            loaded_uuid: None,
            loaded_mtime: None,
            show_thinking: false,
            cached_visual_rows: None,

            active_uuids: HashSet::new(),
            last_process_poll: Instant::now(),

            work_tx,
            result_rx,
        };

        // Queue the first 8 sessions for immediate background parsing.
        for i in 0..app.sessions.len().min(8) {
            let data = {
                let s = &app.sessions[i];
                (s.uuid.clone(), s.path.clone(), s.modified)
            };
            if app.work_tx.try_send(data).is_ok() {
                app.sessions[i].parse_queued = true;
            }
        }

        app
    }

    // -----------------------------------------------------------------------
    // Session list
    // -----------------------------------------------------------------------

    /// Reload the session list, preserving the current selection by UUID when
    /// possible.
    pub fn refresh_sessions(&mut self) {
        let current_uuid = self.sessions.get(self.selected).map(|s| s.uuid.clone());

        let mut new_sessions = discover_sessions();
        for s in &mut new_sessions {
            s.active = self.active_uuids.contains(&s.uuid);
            // Carry over token metadata and parse_queued when mtime is
            // unchanged.  On mtime change (or new session), reset parsing
            // state so the new version is re-queued below.
            let mtime_changed =
                if let Some(old) = self.sessions.iter().find(|old| old.uuid == s.uuid) {
                    if old.modified == s.modified {
                        s.token_total = old.token_total;
                        s.is_headless = old.is_headless;
                        s.parse_queued = old.parse_queued;
                        false
                    } else {
                        // mtime changed → fields stay at defaults (None/false)
                        true
                    }
                } else {
                    // New session
                    true
                };

            // Enqueue only when we know the file has changed (or is new) and
            // no parse is already in flight for this version.
            if s.token_total.is_none() && !s.parse_queued && mtime_changed {
                let data = (s.uuid.clone(), s.path.clone(), s.modified);
                if self.work_tx.try_send(data).is_ok() {
                    s.parse_queued = true;
                }
            }
        }

        self.sessions = new_sessions;

        if let Some(uuid) = current_uuid {
            if let Some(idx) = self.sessions.iter().position(|s| s.uuid == uuid) {
                self.selected = idx;
            } else {
                self.selected = self.selected.min(self.sessions.len().saturating_sub(1));
            }
        }
    }

    /// Drain background token-parse results and apply them to the session list.
    ///
    /// The result carries the mtime that was current when the work item was
    /// enqueued.  If the session's mtime has since advanced the result is stale
    /// and is discarded — a fresh work item will have been (or will be)
    /// enqueued by `refresh_sessions`.
    pub fn poll_token_results(&mut self) {
        while let Ok((uuid, tokens, is_headless, result_mtime)) = self.result_rx.try_recv() {
            if let Some(s) = self.sessions.iter_mut().find(|s| s.uuid == uuid) {
                if s.modified == result_mtime {
                    s.token_total = Some(tokens);
                    s.is_headless = Some(is_headless);
                }
                // mtime mismatch → stale result from a superseded parse; discard.
            }
        }
    }

    /// Enqueue a background token-parse for `sessions[idx]` if it has not
    /// already been queued for its current mtime.  Sets `parse_queued = true`
    /// only when `try_send` succeeds.
    pub(crate) fn enqueue_if_needed(&mut self, idx: usize) {
        if let Some(s) = self.sessions.get(idx) {
            if s.token_total.is_none() && !s.parse_queued {
                let data = (s.uuid.clone(), s.path.clone(), s.modified);
                if self.work_tx.try_send(data).is_ok() {
                    self.sessions[idx].parse_queued = true;
                }
            }
        }
    }

    pub fn select_prev(&mut self) {
        if self.selected > 0 {
            self.selected -= 1;
            self.enqueue_if_needed(self.selected);
            self.needs_scroll_to_bottom = true;
        }
    }

    pub fn select_next(&mut self) {
        if self.selected + 1 < self.sessions.len() {
            self.selected += 1;
            self.enqueue_if_needed(self.selected);
            self.needs_scroll_to_bottom = true;
        }
    }

    pub fn selected_session(&self) -> Option<&SessionEntry> {
        self.sessions.get(self.selected)
    }

    // -----------------------------------------------------------------------
    // Scroll
    // -----------------------------------------------------------------------

    /// Return the visual-row scroll offset that pins the view to the bottom.
    ///
    /// `Paragraph::scroll((n, 0))` with `Wrap` skips `n` **visual rows**
    /// (each wrapped portion of a long line counts as one row).  This function
    /// returns `total_visual_rows - panel_height`, which is exactly the offset
    /// needed to show the last `content_panel_height` visual rows.
    ///
    /// Accepts `pw` as an explicit parameter so callers with access to the
    /// actual ratatui layout rect (e.g. `ui.rs`) can pass the true inner width
    /// rather than the cached `right_panel_inner_width` approximation.
    pub fn scroll_to_bottom_offset_for_width(&mut self, pw: usize) -> usize {
        // Return the cached total when the panel width matches.
        if let Some((total_visual, cached_pw)) = self.cached_visual_rows {
            if cached_pw == pw as u16 {
                return total_visual.saturating_sub(self.content_panel_height);
            }
        }
        let total_visual: usize = self.rendered_lines.iter().map(|l| visual_rows(l, pw)).sum();
        self.cached_visual_rows = Some((total_visual, pw as u16));
        total_visual.saturating_sub(self.content_panel_height)
    }

    /// Convenience wrapper that uses the cached `right_panel_inner_width`.
    /// Prefer `scroll_to_bottom_offset_for_width` when the exact panel rect is
    /// available.
    pub fn scroll_to_bottom_offset(&mut self) -> usize {
        self.scroll_to_bottom_offset_for_width(self.right_panel_inner_width as usize)
    }

    /// Called by the event loop before each draw with the current inner panel
    /// height.  Updates the cached height only; `needs_scroll_to_bottom` is
    /// resolved inside `draw_content_panel` where the exact ratatui layout
    /// rect is available.
    pub fn update_content_height(&mut self, height: usize) {
        self.content_panel_height = height;
    }

    pub fn scroll_down(&mut self) {
        let max = self.scroll_to_bottom_offset();
        self.scroll_offset = self.scroll_offset.saturating_add(SCROLL_STEP).min(max);
    }

    pub fn scroll_up(&mut self) {
        self.scroll_offset = self.scroll_offset.saturating_sub(SCROLL_STEP);
    }

    pub fn scroll_page_down(&mut self) {
        let max = self.scroll_to_bottom_offset();
        self.scroll_offset = self
            .scroll_offset
            .saturating_add(self.content_panel_height)
            .min(max);
    }

    pub fn scroll_page_up(&mut self) {
        self.scroll_offset = self.scroll_offset.saturating_sub(self.content_panel_height);
    }

    // -----------------------------------------------------------------------
    // Content loading
    // -----------------------------------------------------------------------

    pub fn toggle_thinking(&mut self) {
        self.show_thinking = !self.show_thinking;
        self.reload_content_if_loaded();
    }

    /// Load (or reload) the selected session's turns into rendered_lines.
    pub fn load_selected_session(&mut self) {
        // Ensure a background token-parse is enqueued for the session being
        // loaded, covering on-demand selection of sessions beyond the initial 8.
        self.enqueue_if_needed(self.selected);
        if let Some(session) = self.sessions.get(self.selected).cloned() {
            let mtime = std::fs::metadata(&session.path)
                .ok()
                .and_then(|m| m.modified().ok());

            let turns = parse_turns(&session.path);
            let opts = RenderOptions {
                show_thinking: self.show_thinking,
            };
            self.rendered_lines = render_turns(&turns, &opts);
            self.cached_visual_rows = None;
            self.loaded_uuid = Some(session.uuid);
            self.loaded_mtime = mtime;
            self.needs_scroll_to_bottom = true;
        }
    }

    /// Reload content after an in-place change (e.g. toggle_thinking).
    fn reload_content_if_loaded(&mut self) {
        if let Some(uuid) = self.loaded_uuid.clone() {
            if let Some(session) = self.sessions.iter().find(|s| s.uuid == uuid).cloned() {
                let turns = parse_turns(&session.path);
                let opts = RenderOptions {
                    show_thinking: self.show_thinking,
                };
                self.rendered_lines = render_turns(&turns, &opts);
                self.cached_visual_rows = None;
            }
        }
    }

    /// Poll for file changes on the selected session and reload if the mtime
    /// changed.  On a selection change, performs a full load.
    pub fn maybe_reload_content(&mut self) {
        let session = match self.sessions.get(self.selected).cloned() {
            Some(s) => s,
            None => return,
        };

        if self.loaded_uuid.as_deref() != Some(&session.uuid) {
            self.load_selected_session();
            return;
        }

        if let Ok(meta) = std::fs::metadata(&session.path) {
            if let Ok(mtime) = meta.modified() {
                if self.loaded_mtime != Some(mtime) {
                    let turns = parse_turns(&session.path);
                    let opts = RenderOptions {
                        show_thinking: self.show_thinking,
                    };
                    self.rendered_lines = render_turns(&turns, &opts);
                    self.cached_visual_rows = None;
                    self.loaded_mtime = Some(mtime);
                    // Preserve scroll position on live update.
                }
            }
        }
    }

    // -----------------------------------------------------------------------
    // Process monitor
    // -----------------------------------------------------------------------

    /// Poll running processes every ~2 seconds and mark active sessions green.
    pub fn poll_processes_if_due(&mut self) {
        if self.last_process_poll.elapsed() < Duration::from_secs(PROCESS_POLL_SECS) {
            return;
        }
        self.last_process_poll = Instant::now();
        self.active_uuids = find_active_uuids();

        for s in &mut self.sessions {
            s.active = self.active_uuids.contains(&s.uuid);
        }
    }
}

impl Default for App {
    fn default() -> Self {
        Self::new()
    }
}
