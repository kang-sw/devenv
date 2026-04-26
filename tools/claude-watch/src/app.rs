use std::collections::HashSet;
use std::time::{Duration, Instant};

use ratatui::text::Line;

use crate::parser::parse_turns;
use crate::process::find_active_uuids;
use crate::renderer::{render_turns, RenderOptions};
use crate::session::{SessionEntry, discover_sessions};

// ---------------------------------------------------------------------------
// Tuning constants
// ---------------------------------------------------------------------------

/// Lines scrolled per j / k keypress.
pub(crate) const SCROLL_STEP: usize = 3;
/// How often the process list is polled.
const PROCESS_POLL_SECS: u64 = 2;
/// Default panel height estimate before the first terminal-size query.
const INITIAL_PANEL_HEIGHT_GUESS: usize = 40;

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
    /// When set, the next `update_content_height` call pins scroll to bottom.
    pub(crate) needs_scroll_to_bottom: bool,
    /// UUID of the session currently loaded in the right panel.
    pub(crate) loaded_uuid: Option<String>,
    /// mtime of the loaded session file — used to detect live updates.
    pub(crate) loaded_mtime: Option<std::time::SystemTime>,
    /// Whether thinking blocks are expanded.
    pub(crate) show_thinking: bool,

    // --- process monitor ---
    pub(crate) active_uuids: HashSet<String>,
    pub(crate) last_process_poll: Instant,
}

impl App {
    pub fn new() -> Self {
        let sessions = discover_sessions();
        App {
            sessions,
            selected: 0,
            last_refresh: Instant::now(),
            should_quit: false,

            rendered_lines: Vec::new(),
            scroll_offset: 0,
            content_panel_height: INITIAL_PANEL_HEIGHT_GUESS,
            needs_scroll_to_bottom: true,
            loaded_uuid: None,
            loaded_mtime: None,
            show_thinking: false,

            active_uuids: HashSet::new(),
            last_process_poll: Instant::now(),
        }
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

    pub fn select_prev(&mut self) {
        if self.selected > 0 {
            self.selected -= 1;
            self.needs_scroll_to_bottom = true;
        }
    }

    pub fn select_next(&mut self) {
        if self.selected + 1 < self.sessions.len() {
            self.selected += 1;
            self.needs_scroll_to_bottom = true;
        }
    }

    pub fn selected_session(&self) -> Option<&SessionEntry> {
        self.sessions.get(self.selected)
    }

    // -----------------------------------------------------------------------
    // Scroll
    // -----------------------------------------------------------------------

    /// Called by the event loop before each draw with the current inner panel
    /// height.  Resolves any pending scroll-to-bottom request.
    pub fn update_content_height(&mut self, height: usize) {
        self.content_panel_height = height;
        if self.needs_scroll_to_bottom {
            self.needs_scroll_to_bottom = false;
            self.scroll_offset = self
                .rendered_lines
                .len()
                .saturating_sub(self.content_panel_height);
        }
    }

    pub fn scroll_down(&mut self) {
        let max = self
            .rendered_lines
            .len()
            .saturating_sub(self.content_panel_height);
        self.scroll_offset = self.scroll_offset.saturating_add(SCROLL_STEP).min(max);
    }

    pub fn scroll_up(&mut self) {
        self.scroll_offset = self.scroll_offset.saturating_sub(SCROLL_STEP);
    }

    pub fn scroll_page_down(&mut self) {
        let max = self
            .rendered_lines
            .len()
            .saturating_sub(self.content_panel_height);
        self.scroll_offset = self
            .scroll_offset
            .saturating_add(self.content_panel_height)
            .min(max);
    }

    pub fn scroll_page_up(&mut self) {
        self.scroll_offset = self
            .scroll_offset
            .saturating_sub(self.content_panel_height);
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
        if let Some(session) = self.sessions.get(self.selected).cloned() {
            let mtime = std::fs::metadata(&session.path)
                .ok()
                .and_then(|m| m.modified().ok());

            let turns = parse_turns(&session.path);
            let opts = RenderOptions { show_thinking: self.show_thinking };
            self.rendered_lines = render_turns(&turns, &opts);
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
                let opts = RenderOptions { show_thinking: self.show_thinking };
                self.rendered_lines = render_turns(&turns, &opts);
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
                    let opts = RenderOptions { show_thinking: self.show_thinking };
                    self.rendered_lines = render_turns(&turns, &opts);
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
