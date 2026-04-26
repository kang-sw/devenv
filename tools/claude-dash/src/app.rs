//! Application state — multi-tab, multi-slot (Phases 2+).
//!
//! Each tab owns one worktree + one PTY session + one VT screen.
//! Named-agent slots are added in Phase 3.

use std::time::Instant;

use portable_pty::PtySize;

use crate::pty::PtySession;
use crate::vt::VtScreen;
use crate::worktree::{discover_worktrees, Worktree};

/// How often worktree/agent lists are refreshed (seconds).
pub const WORKTREE_POLL_SECS: u64 = 5;

/// Initial PTY size before the first draw sets the authoritative dimensions.
const INITIAL_SIZE: PtySize = PtySize {
    rows: 24,
    cols: 80,
    pixel_width: 0,
    pixel_height: 0,
};

// ---------------------------------------------------------------------------
// Exit modal
// ---------------------------------------------------------------------------

/// State for the process-exit modal overlay.
pub struct ExitedModal {
    pub status: portable_pty::ExitStatus,
    /// True when the worktree was already removed — disables [R] Restart.
    /// Filled in by Phase 4; always false in Phase 2.
    pub is_removed_worktree: bool,
}

// ---------------------------------------------------------------------------
// Per-tab state
// ---------------------------------------------------------------------------

/// One tab — corresponds to one git worktree.
pub struct WorktreeTab {
    pub worktree: Worktree,
    /// Live PTY session; `None` until the tab is first activated.
    pub session: Option<PtySession>,
    /// VT screen model for this tab's panel.
    pub vt: VtScreen,
    /// Inner panel dimensions from the last `ui::draw` call for this tab.
    pub last_inner_size: (u16, u16),
    /// Present when the child process has exited.
    pub exited_modal: Option<ExitedModal>,
}

impl WorktreeTab {
    fn new(worktree: Worktree, session: Option<PtySession>) -> Self {
        WorktreeTab {
            worktree,
            session,
            vt: VtScreen::new(INITIAL_SIZE.cols, INITIAL_SIZE.rows),
            last_inner_size: (INITIAL_SIZE.cols, INITIAL_SIZE.rows),
            exited_modal: None,
        }
    }

    /// Drain all pending PTY bytes and feed them into the VT screen.
    pub fn drain_pty(&mut self) {
        if let Some(ref s) = self.session {
            let mut all: Vec<u8> = Vec::new();
            while let Some(chunk) = s.try_recv_chunk() {
                all.extend_from_slice(&chunk);
            }
            if !all.is_empty() {
                self.vt.feed(&all);
            }
        }
    }

    /// Poll for child process exit; set `exited_modal` if it has exited.
    pub fn poll_exit(&mut self) {
        if self.exited_modal.is_none() {
            if let Some(ref mut s) = self.session {
                if let Some(status) = s.try_wait() {
                    let is_removed_worktree = self.worktree.is_removed;
                    self.exited_modal = Some(ExitedModal {
                        status,
                        is_removed_worktree,
                    });
                }
            }
        }
    }

    /// Resize this tab's VT + PTY to `(cols, rows)` if they differ from the
    /// current dimensions.
    pub fn maybe_resize(&mut self, cols: u16, rows: u16) -> anyhow::Result<()> {
        let (cur_cols, cur_rows) = self.vt.dimensions();
        if cur_cols != cols || cur_rows != rows {
            self.vt.resize(cols, rows);
            if let Some(ref mut s) = self.session {
                s.resize(PtySize {
                    rows,
                    cols,
                    pixel_width: 0,
                    pixel_height: 0,
                })?;
            }
        }
        Ok(())
    }
}

// ---------------------------------------------------------------------------
// Top-level app state
// ---------------------------------------------------------------------------

/// Top-level application state (Phase 2+).
pub struct App {
    pub tabs: Vec<WorktreeTab>,
    pub active_tab: usize,
    pub should_quit: bool,
    pub last_worktree_poll: Instant,
}

impl App {
    /// Build the initial app state: discover worktrees, create tabs, and
    /// auto-spawn PTYs for active ws-framework worktrees.
    pub fn new() -> anyhow::Result<Self> {
        let worktrees = discover_worktrees();

        // Fall back to cwd if no git repo or empty list.
        let worktrees = if worktrees.is_empty() {
            let cwd = std::env::current_dir().unwrap_or_default();
            let name = cwd
                .file_name()
                .map(|n| n.to_string_lossy().into_owned())
                .unwrap_or_else(|| "main".into());
            vec![Worktree {
                path: cwd,
                name,
                is_active_ws: false,
                is_removed: false,
            }]
        } else {
            worktrees
        };

        let mut tabs: Vec<WorktreeTab> = Vec::new();
        for wt in worktrees {
            let session = if wt.is_active_ws {
                match PtySession::spawn(&wt.path, INITIAL_SIZE) {
                    Ok(s) => Some(s),
                    Err(_) => None,
                }
            } else {
                None
            };
            tabs.push(WorktreeTab::new(wt, session));
        }

        // If no tab was auto-spawned (no active_ws tabs), spawn the first one.
        if tabs.iter().all(|t| t.session.is_none()) {
            if let Some(tab) = tabs.first_mut() {
                match PtySession::spawn(&tab.worktree.path, INITIAL_SIZE) {
                    Ok(s) => tab.session = Some(s),
                    Err(_) => {}
                }
            }
        }

        Ok(App {
            tabs,
            active_tab: 0,
            should_quit: false,
            last_worktree_poll: Instant::now(),
        })
    }

    /// Return the active tab (immutable).
    pub fn active(&self) -> Option<&WorktreeTab> {
        self.tabs.get(self.active_tab)
    }

    /// Return the active tab (mutable).
    pub fn active_mut(&mut self) -> Option<&mut WorktreeTab> {
        self.tabs.get_mut(self.active_tab)
    }

    /// Switch to `idx` and ensure its PTY is running.
    pub fn activate_tab(&mut self, idx: usize) {
        if idx >= self.tabs.len() {
            return;
        }
        self.active_tab = idx;

        // Spawn PTY on first activation if the tab's session is absent and
        // the worktree hasn't been removed.
        let tab = &mut self.tabs[idx];
        if tab.session.is_none() && !tab.worktree.is_removed {
            let (cols, rows) = tab.last_inner_size;
            let size = PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            };
            match PtySession::spawn(&tab.worktree.path, size) {
                Ok(s) => tab.session = Some(s),
                Err(_) => {}
            }
        }

        // Resize VT + PTY to the current panel size (may have changed while
        // this tab was in the background).
        let (cols, rows) = self.tabs[idx].last_inner_size;
        let _ = self.tabs[idx].maybe_resize(cols, rows);
    }

    /// Reconcile the live tab list against a freshly discovered worktree list.
    ///
    /// - New paths → append tabs; auto-spawn if `is_active_ws`.
    /// - Missing paths with live session → mark removed.
    /// - Missing paths without session → drop immediately.
    /// - Already-removed tabs whose session has exited → drop.
    /// - Re-anchors `active_tab` after any drops.
    pub fn reconcile_worktrees(&mut self, new_list: Vec<Worktree>) {
        // Step 1: add new worktrees.
        for new_wt in &new_list {
            if !self.tabs.iter().any(|t| t.worktree.path == new_wt.path) {
                let session = if new_wt.is_active_ws {
                    PtySession::spawn(&new_wt.path, INITIAL_SIZE).ok()
                } else {
                    None
                };
                let wt = Worktree {
                    path: new_wt.path.clone(),
                    name: new_wt.name.clone(),
                    is_active_ws: new_wt.is_active_ws,
                    is_removed: false,
                };
                self.tabs.push(WorktreeTab::new(wt, session));
            }
        }

        // Step 2: mark/drop tabs whose path is no longer in new_list.
        for tab in &mut self.tabs {
            if !new_list.iter().any(|w| w.path == tab.worktree.path) {
                if tab.session.is_some() {
                    tab.worktree.is_removed = true;
                } else {
                    // Will be dropped below in the retain pass.
                    tab.worktree.is_removed = true;
                }
            }
        }

        // Step 3: drop removed tabs whose process has also exited.
        // We can't call try_wait inside retain cleanly, so collect indices to remove.
        let mut to_remove: Vec<usize> = Vec::new();
        for (i, tab) in self.tabs.iter_mut().enumerate() {
            if tab.worktree.is_removed {
                let dead = if let Some(ref mut s) = tab.session {
                    s.try_wait().is_some()
                } else {
                    true // no session → remove immediately
                };
                if dead {
                    to_remove.push(i);
                }
            }
        }
        // Remove in reverse order to preserve indices.
        for &i in to_remove.iter().rev() {
            self.tabs.remove(i);
            if self.active_tab > i {
                self.active_tab = self.active_tab.saturating_sub(1);
            }
        }

        // Step 4: clamp active_tab to valid range.
        if !self.tabs.is_empty() {
            self.active_tab = self.active_tab.min(self.tabs.len() - 1);
        } else {
            self.active_tab = 0;
        }
    }
}
