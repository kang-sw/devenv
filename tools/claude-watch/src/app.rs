use std::time::{Duration, Instant};
use crate::session::{SessionEntry, discover_sessions};
use crate::parser::parse_turns;
use crate::renderer::{render_turns, RenderOptions};
use crate::process::find_active_uuids;

pub struct App {
    pub sessions: Vec<SessionEntry>,
    pub selected: usize,
    pub last_refresh: Instant,
    pub should_quit: bool,

    // Phase 3: content panel
    pub rendered_lines: Vec<ratatui::text::Line<'static>>,
    pub scroll_offset: u16,
    /// Inner height of the content panel — updated each draw frame.
    pub content_panel_height: u16,
    /// When true the next draw will pin scroll to the bottom.
    pub needs_scroll_to_bottom: bool,
    /// UUID of the session currently loaded in the right panel.
    pub loaded_uuid: Option<String>,
    /// mtime of the loaded session file — used to detect live updates.
    pub loaded_mtime: Option<std::time::SystemTime>,
    /// Whether thinking blocks are visible.
    pub show_thinking: bool,

    // Phase 4: active process UUIDs
    pub active_uuids: std::collections::HashSet<String>,
    pub last_process_poll: Instant,
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
            content_panel_height: 40,
            needs_scroll_to_bottom: true,
            loaded_uuid: None,
            loaded_mtime: None,
            show_thinking: false,

            active_uuids: std::collections::HashSet::new(),
            last_process_poll: Instant::now(),
        }
    }

    /// Reload the session list, preserving the current selection by UUID when
    /// possible.
    pub fn refresh_sessions(&mut self) {
        let current_uuid = self.sessions.get(self.selected).map(|s| s.uuid.clone());

        // Apply active markers before replacing.
        let mut new_sessions = discover_sessions();
        for s in &mut new_sessions {
            s.active = self.active_uuids.contains(&s.uuid);
        }

        self.sessions = new_sessions;

        // Restore selection.
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

    pub fn scroll_down(&mut self) {
        let max = (self.rendered_lines.len() as u16).saturating_sub(self.content_panel_height);
        self.scroll_offset = self.scroll_offset.saturating_add(3).min(max);
    }

    pub fn scroll_up(&mut self) {
        self.scroll_offset = self.scroll_offset.saturating_sub(3);
    }

    pub fn scroll_page_down(&mut self) {
        let max = (self.rendered_lines.len() as u16).saturating_sub(self.content_panel_height);
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

    pub fn toggle_thinking(&mut self) {
        self.show_thinking = !self.show_thinking;
        // Re-render with updated visibility.
        self.reload_content_if_loaded();
    }

    /// Load (or reload) the selected session's turns into rendered_lines.
    pub fn load_selected_session(&mut self) {
        if let Some(session) = self.sessions.get(self.selected).cloned() {
            let uuid = session.uuid.clone();
            let path = session.path.clone();

            let mtime = std::fs::metadata(&path)
                .ok()
                .and_then(|m| m.modified().ok());

            let turns = parse_turns(&path);
            let opts = RenderOptions { show_thinking: self.show_thinking };
            self.rendered_lines = render_turns(&turns, &opts);
            self.loaded_uuid = Some(uuid);
            self.loaded_mtime = mtime;
            self.needs_scroll_to_bottom = true;
        }
    }

    /// Reload content if a session is already loaded (e.g. after toggle_thinking).
    fn reload_content_if_loaded(&mut self) {
        if let Some(uuid) = self.loaded_uuid.clone() {
            if let Some(session) = self.sessions.iter().find(|s| s.uuid == uuid).cloned() {
                let turns = parse_turns(&session.path);
                let opts = RenderOptions { show_thinking: self.show_thinking };
                self.rendered_lines = render_turns(&turns, &opts);
            }
        }
    }

    /// Check if the selected session's file has changed and reload if needed.
    /// On selection change: full reload + scroll-to-bottom.
    /// On mtime change only: reload content, preserve scroll position.
    pub fn maybe_reload_content(&mut self) {
        let session = match self.sessions.get(self.selected).cloned() {
            Some(s) => s,
            None => return,
        };

        // Selection changed — do a full load.
        if self.loaded_uuid.as_deref() != Some(&session.uuid) {
            self.load_selected_session();
            return;
        }

        // Same session — check mtime for live-tail updates.
        if let Ok(meta) = std::fs::metadata(&session.path) {
            if let Ok(mtime) = meta.modified() {
                if self.loaded_mtime != Some(mtime) {
                    let turns = parse_turns(&session.path);
                    let opts = RenderOptions { show_thinking: self.show_thinking };
                    self.rendered_lines = render_turns(&turns, &opts);
                    self.loaded_mtime = Some(mtime);
                    // Preserve scroll on live update.
                }
            }
        }
    }

    /// Poll running processes every ~1-2 seconds and mark active sessions.
    /// Should be called from the main event loop.
    pub fn poll_processes_if_due(&mut self) {
        if self.last_process_poll.elapsed() < Duration::from_secs(2) {
            return;
        }
        self.last_process_poll = Instant::now();
        self.active_uuids = find_active_uuids();

        // Re-apply active markers to sessions.
        for s in &mut self.sessions {
            s.active = self.active_uuids.contains(&s.uuid);
        }
    }
}
