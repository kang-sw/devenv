use std::time::Instant;
use crate::session::{SessionEntry, discover_sessions};

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
    }
}
