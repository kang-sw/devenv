//! Application state — multi-tab, multi-slot (Phases 2+).
//!
//! Each tab owns one worktree + one PTY session + one VT screen.
//! Named-agent slots are added in Phase 3.

use std::path::PathBuf;
use std::sync::mpsc;
use std::time::Instant;

use portable_pty::PtySize;

use crate::agent::{discover_named_agents, file_mtime, NamedAgent};
use crate::parser::parse_turns;
use crate::pty::PtySession;
use crate::renderer::{render_turns, RenderOptions};
use crate::session::{find_git_root, parse_session_metadata};
use crate::vt::VtScreen;
use crate::worktree::{discover_worktrees, Worktree};

/// Background token-parse result: `(uuid, total_tokens)`.
type TokenResult = (String, u64);

/// How often worktree/agent lists are refreshed (seconds).
pub const WORKTREE_POLL_SECS: u64 = 5;

/// Number of scroll lines per Up/Down keypress in the agent viewer.
pub const SCROLL_STEP: usize = 3;

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
    pub is_removed_worktree: bool,
}

// ---------------------------------------------------------------------------
// Named-agent slot
// ---------------------------------------------------------------------------

/// Which content the main panel shows for a tab.
pub enum SlotKind {
    /// The interactive PTY terminal.
    Main,
    /// Read-only JSONL viewer for named_agents[idx].
    Named(usize),
}

/// State for the JSONL viewer shown in a Named slot.
pub struct AgentView {
    pub uuid: String,
    pub rendered_lines: Vec<ratatui::text::Line<'static>>,
    pub scroll_offset: usize,
    pub loaded_mtime: Option<std::time::SystemTime>,
    /// Cached `(total_visual_rows, panel_width)` for the scrollbar.
    pub cached_visual_rows: Option<(usize, u16)>,
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
    /// Named agents discovered for this tab's worktree.
    pub named_agents: Vec<NamedAgent>,
    /// Which slot (Main PTY or Named agent) is currently shown.
    pub active_slot: SlotKind,
    /// State for the currently open agent viewer (if active_slot == Named).
    pub agent_view: Option<AgentView>,
}

impl WorktreeTab {
    fn new(worktree: Worktree, session: Option<PtySession>) -> Self {
        WorktreeTab {
            worktree,
            session,
            vt: VtScreen::new(INITIAL_SIZE.cols, INITIAL_SIZE.rows),
            last_inner_size: (INITIAL_SIZE.cols, INITIAL_SIZE.rows),
            exited_modal: None,
            named_agents: Vec::new(),
            active_slot: SlotKind::Main,
            agent_view: None,
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

    /// Refresh named-agent list for this tab.
    ///
    /// For agents whose `token_total` is not yet known and whose session file
    /// mtime changed (or is new), spawns a background thread that calls
    /// `parse_session_metadata` and sends the result via `token_tx`.
    pub fn refresh_named_agents(
        &mut self,
        git_root: &PathBuf,
        token_tx: &mpsc::SyncSender<TokenResult>,
    ) {
        let new_agents = discover_named_agents(git_root);
        // Reconcile: preserve token_total/parse_queued when uuid + mtime match.
        let reconciled: Vec<NamedAgent> = new_agents
            .into_iter()
            .map(|mut new| {
                if let Some(old) = self.named_agents.iter().find(|a| a.uuid == new.uuid) {
                    if old.mtime == new.mtime {
                        new.token_total = old.token_total;
                        new.parse_queued = old.parse_queued;
                    }
                }
                // Spawn background parse if needed.
                if new.token_total.is_none() && !new.parse_queued {
                    let path = new.session_path.clone();
                    let uuid = new.uuid.clone();
                    let tx = token_tx.clone();
                    std::thread::spawn(move || {
                        if let Some((tokens, _)) = parse_session_metadata(&path) {
                            let _ = tx.send((uuid, tokens));
                        }
                    });
                    new.parse_queued = true;
                }
                new
            })
            .collect();
        self.named_agents = reconciled;
    }

    /// Activate the Named(idx) slot and create or refresh the AgentView.
    pub fn open_agent_view(&mut self, idx: usize) {
        let agent = match self.named_agents.get(idx) {
            Some(a) => a,
            None => return,
        };
        let session_path = agent.session_path.clone();
        let uuid = agent.uuid.clone();

        // Load (or reload) the session content.
        let mtime = file_mtime(&session_path);
        let turns = parse_turns(&session_path);
        let opts = RenderOptions {
            show_thinking: false,
        };
        let rendered_lines = render_turns(&turns, &opts);

        self.agent_view = Some(AgentView {
            uuid,
            rendered_lines,
            scroll_offset: 0,
            loaded_mtime: mtime,
            cached_visual_rows: None,
        });
        self.active_slot = SlotKind::Named(idx);
    }

    /// Check whether the open AgentView's mtime has changed and reload if so.
    pub fn maybe_reload_agent_view(&mut self) {
        let idx = match self.active_slot {
            SlotKind::Named(i) => i,
            _ => return,
        };
        let session_path = match self.named_agents.get(idx) {
            Some(a) => a.session_path.clone(),
            None => return,
        };
        let current_mtime = file_mtime(&session_path);
        let last_mtime = self.agent_view.as_ref().and_then(|v| v.loaded_mtime);

        if current_mtime != last_mtime {
            let turns = parse_turns(&session_path);
            let opts = RenderOptions {
                show_thinking: false,
            };
            let rendered_lines = render_turns(&turns, &opts);
            if let Some(ref mut view) = self.agent_view {
                view.rendered_lines = rendered_lines;
                view.loaded_mtime = current_mtime;
                view.cached_visual_rows = None;
                // scroll_offset is intentionally preserved — the render path
                // already clamps the displayed scroll to max_scroll, so the
                // view stays consistent even if content shrinks.  Resetting
                // here would scroll the user back to the top on every live
                // refresh, which is the bug we are fixing.
            }
        }
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
    /// Git root for the current repo (used for named-agent discovery).
    pub git_root: Option<PathBuf>,
    /// Sender used by background token-parse threads to report results.
    token_tx: mpsc::SyncSender<TokenResult>,
    /// Receiver drained each frame to update `NamedAgent::token_total`.
    token_rx: mpsc::Receiver<TokenResult>,
    /// Whether all spawned `claude` subprocesses should receive
    /// `--dangerously-skip-permissions` (set from the CLI flag at startup).
    pub skip_permissions: bool,
    /// True while the prefix key (Ctrl+B) has been pressed and we are
    /// waiting for the next keystroke to dispatch a command.
    pub prefix_active: bool,
}

/// Spawn a `claude` PTY, optionally appending `--dangerously-skip-permissions`.
///
/// Used at every spawn site so the `skip_permissions` flag propagates
/// uniformly — both during initial construction (before `App` exists) and
/// inside `impl App` methods where a mutable borrow of `self.tabs` may
/// already be active, making it unsafe to call `self.spawn_session()`.
fn spawn_claude(
    cwd: &std::path::Path,
    size: PtySize,
    skip: bool,
) -> anyhow::Result<PtySession> {
    if skip {
        PtySession::spawn_with_args(cwd, size, &["--dangerously-skip-permissions"])
    } else {
        PtySession::spawn(cwd, size)
    }
}

impl App {
    /// Build the initial app state: discover worktrees, create tabs, and
    /// auto-spawn PTYs for active ws-framework worktrees.
    ///
    /// `skip_permissions` controls whether every spawned `claude` process
    /// receives `--dangerously-skip-permissions` (mirrors the CLI flag).
    pub fn new(skip_permissions: bool) -> anyhow::Result<Self> {
        let git_root = find_git_root();

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
                spawn_claude(&wt.path, INITIAL_SIZE, skip_permissions).ok()
            } else {
                None
            };
            tabs.push(WorktreeTab::new(wt, session));
        }

        // If no tab was auto-spawned, spawn the first one.
        if tabs.iter().all(|t| t.session.is_none()) {
            if let Some(tab) = tabs.first_mut() {
                match spawn_claude(&tab.worktree.path, INITIAL_SIZE, skip_permissions) {
                    Ok(s) => tab.session = Some(s),
                    Err(_) => {}
                }
            }
        }

        // Bounded channel for background token-parse results.
        // Capacity 64 is ample for ≤8 agents × number of tabs.
        let (token_tx, token_rx) = mpsc::sync_channel::<TokenResult>(64);

        Ok(App {
            tabs,
            active_tab: 0,
            should_quit: false,
            last_worktree_poll: Instant::now(),
            git_root,
            token_tx,
            token_rx,
            skip_permissions,
            prefix_active: false,
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

        // Read the fields we need before taking any mutable borrow, so that
        // spawn_claude (which needs self.skip_permissions) does not conflict.
        let needs_spawn = {
            let tab = &self.tabs[idx];
            tab.session.is_none() && !tab.worktree.is_removed
        };
        if needs_spawn {
            let (cols, rows) = self.tabs[idx].last_inner_size;
            let size = PtySize {
                rows,
                cols,
                pixel_width: 0,
                pixel_height: 0,
            };
            let cwd = self.tabs[idx].worktree.path.clone();
            let skip = self.skip_permissions;
            match spawn_claude(&cwd, size, skip) {
                Ok(s) => self.tabs[idx].session = Some(s),
                Err(_) => {}
            }
        }

        let (cols, rows) = self.tabs[idx].last_inner_size;
        let _ = self.tabs[idx].maybe_resize(cols, rows);
    }

    /// Reconcile the live tab list against a freshly discovered worktree list.
    pub fn reconcile_worktrees(&mut self, new_list: Vec<Worktree>) {
        // Add new worktrees.
        for new_wt in &new_list {
            if !self.tabs.iter().any(|t| t.worktree.path == new_wt.path) {
                let session = if new_wt.is_active_ws {
                    spawn_claude(&new_wt.path, INITIAL_SIZE, self.skip_permissions).ok()
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

        // Mark/drop tabs whose path is no longer in new_list.
        for tab in &mut self.tabs {
            if !new_list.iter().any(|w| w.path == tab.worktree.path) {
                if !tab.worktree.is_removed {
                    // Newly removed: kill the session so the process does not
                    // run indefinitely waiting for user input.  The tab stays
                    // alive until try_wait() confirms exit (handled below).
                    tab.worktree.is_removed = true;
                    if let Some(ref mut s) = tab.session {
                        s.kill();
                    }
                }
            }
        }

        // Drop removed tabs whose process has also exited.
        let mut to_remove: Vec<usize> = Vec::new();
        for (i, tab) in self.tabs.iter_mut().enumerate() {
            if tab.worktree.is_removed {
                let dead = if let Some(ref mut s) = tab.session {
                    s.try_wait().is_some()
                } else {
                    true
                };
                if dead {
                    to_remove.push(i);
                }
            }
        }
        for &i in to_remove.iter().rev() {
            self.tabs.remove(i);
            if self.active_tab > i {
                self.active_tab = self.active_tab.saturating_sub(1);
            }
        }

        if !self.tabs.is_empty() {
            self.active_tab = self.active_tab.min(self.tabs.len() - 1);
        } else {
            self.active_tab = 0;
        }
    }

    /// Spawn a `claude` PTY in `cwd` with the app's `skip_permissions` setting.
    ///
    /// Intended for call sites in `main.rs` (e.g. the exit-modal restart path)
    /// that cannot call the private free function `spawn_claude`.
    pub fn spawn_session(
        &self,
        cwd: &std::path::Path,
        size: PtySize,
    ) -> anyhow::Result<PtySession> {
        spawn_claude(cwd, size, self.skip_permissions)
    }

    /// Spawn a new `claude` process in the current tab's worktree directory.
    ///
    /// Always force-respawns: if a session is already running it is killed
    /// (via `PtySession::drop`) before the new one starts.  The exit modal
    /// is cleared on every call.
    ///
    /// Retained for potential future callers; prefix+n now calls `open_new_tab`
    /// instead (which opens a new tab rather than restarting the current one).
    #[allow(dead_code)]
    pub fn spawn_new_claude_in_tab(&mut self) -> anyhow::Result<()> {
        let idx = self.active_tab;
        if idx >= self.tabs.len() {
            return Ok(());
        }
        let cwd = self.tabs[idx].worktree.path.clone();
        let (cols, rows) = self.tabs[idx].last_inner_size;
        let size = PtySize {
            rows,
            cols,
            pixel_width: 0,
            pixel_height: 0,
        };
        let skip = self.skip_permissions;
        // Drop any existing session before spawning — PtySession::drop kills
        // the child process and joins the reader thread.
        let _ = self.tabs[idx].session.take();
        self.tabs[idx].exited_modal = None;
        let session = spawn_claude(&cwd, size, skip)?;
        self.tabs[idx].session = Some(session);
        Ok(())
    }

    /// Open a new tab running `claude` in the project root (or current tab's
    /// worktree path when no git root is detected).
    ///
    /// The new tab is appended to the end and immediately made active.
    pub fn open_new_tab(&mut self) -> anyhow::Result<()> {
        // Determine working directory: prefer git_root; fall back to current tab
        // or cwd if tabs is empty.
        let cwd = if let Some(ref root) = self.git_root {
            root.clone()
        } else if !self.tabs.is_empty() {
            self.tabs[self.active_tab].worktree.path.clone()
        } else {
            std::env::current_dir().unwrap_or_default()
        };

        // Derive tab label from the last path component, matching App::new().
        let name = cwd
            .file_name()
            .map(|n| n.to_string_lossy().into_owned())
            .unwrap_or_else(|| "new".into());

        // Spawn before constructing Worktree so cwd can be moved (not cloned)
        // into the struct literal.  Swallow the error — if `claude` is missing
        // or PTY allocation fails we simply do not open the tab (matching the
        // `activate_tab` / `reconcile_worktrees` error-handling pattern).
        let session = match spawn_claude(&cwd, INITIAL_SIZE, self.skip_permissions) {
            Ok(s) => s,
            Err(_) => return Ok(()),
        };
        let worktree = Worktree {
            path: cwd,
            name,
            is_active_ws: true,
            is_removed: false,
        };
        self.tabs.push(WorktreeTab::new(worktree, Some(session)));
        self.active_tab = self.tabs.len() - 1;
        Ok(())
    }

    /// Refresh named agents for all tabs, spawning background token parses.
    pub fn refresh_named_agents(&mut self) {
        if let Some(ref root) = self.git_root.clone() {
            let tx = self.token_tx.clone();
            for tab in &mut self.tabs {
                tab.refresh_named_agents(root, &tx);
            }
        }
    }

    /// Drain completed token-parse results and update matching agents.
    ///
    /// Call once per frame in the main loop so token counts appear within
    /// a frame of the background thread finishing.
    pub fn drain_token_results(&mut self) {
        while let Ok((uuid, tokens)) = self.token_rx.try_recv() {
            for tab in &mut self.tabs {
                if let Some(agent) = tab.named_agents.iter_mut().find(|a| a.uuid == uuid) {
                    agent.token_total = Some(tokens);
                    agent.parse_queued = false;
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use crate::agent::NamedAgent;

    fn make_worktree(name: &str) -> Worktree {
        Worktree {
            path: std::path::PathBuf::from(format!("/tmp/wt/{}", name)),
            name: name.to_string(),
            is_active_ws: false,
            is_removed: false,
        }
    }

    fn make_app(names: &[&str]) -> App {
        let tabs = names
            .iter()
            .map(|name| WorktreeTab::new(make_worktree(name), None))
            .collect();
        let (token_tx, token_rx) = mpsc::sync_channel(64);
        App {
            tabs,
            active_tab: 0,
            should_quit: false,
            last_worktree_poll: Instant::now(),
            git_root: None,
            token_tx,
            token_rx,
            skip_permissions: false,
            prefix_active: false,
        }
    }

    fn make_agent(uuid: &str) -> NamedAgent {
        NamedAgent {
            name: uuid.to_string(),
            uuid: uuid.to_string(),
            session_path: std::path::PathBuf::from("/tmp/fake.jsonl"),
            mtime: chrono::Local::now(),
            token_total: None,
            parse_queued: false,
        }
    }

    // --- reconcile_worktrees ---

    #[test]
    fn new_worktree_path_appears_in_tabs_after_reconcile() {
        let mut app = make_app(&["main"]);
        let new = vec![make_worktree("main"), make_worktree("feature")];
        app.reconcile_worktrees(new);
        assert_eq!(app.tabs.len(), 2);
        assert!(app.tabs.iter().any(|t| t.worktree.name == "feature"));
    }

    #[test]
    fn sessionless_removed_tab_drops_immediately() {
        // A tab with no session that disappears from the list is dropped immediately.
        let mut app = make_app(&["main", "gone"]);
        let new = vec![make_worktree("main")];
        app.reconcile_worktrees(new);
        // "gone" had no session → dropped entirely.
        assert_eq!(app.tabs.len(), 1);
        assert_eq!(app.tabs[0].worktree.name, "main");
    }

    #[test]
    fn active_tab_clamps_to_last_when_final_tab_removed() {
        let mut app = make_app(&["main", "feat"]);
        app.active_tab = 1; // active = "feat"
        let new = vec![make_worktree("main")]; // "feat" removed, no session
        app.reconcile_worktrees(new);
        assert_eq!(app.tabs.len(), 1);
        assert_eq!(app.active_tab, 0);
    }

    #[test]
    fn active_tab_shifts_down_when_earlier_tab_removed() {
        let mut app = make_app(&["a", "b", "c"]);
        app.active_tab = 2; // active = "c"
        // Remove "a" (index 0, before active).
        let new = vec![make_worktree("b"), make_worktree("c")];
        app.reconcile_worktrees(new);
        assert_eq!(app.tabs.len(), 2);
        // active_tab should have shifted down by 1 to still point at "c".
        assert_eq!(app.tabs[app.active_tab].worktree.name, "c");
    }

    #[test]
    fn active_tab_resets_to_zero_when_all_tabs_removed() {
        // Start with active_tab = 1 so the clamping logic is exercised.
        // Both tabs have no session → dropped immediately.
        let mut app = make_app(&["a", "b"]);
        app.active_tab = 1;
        app.reconcile_worktrees(vec![]);
        // All tabs gone; active_tab must be clamped to 0.
        assert_eq!(app.active_tab, 0);
    }

    // --- drain_token_results ---

    #[test]
    fn drain_token_results_updates_matching_agent() {
        let mut app = make_app(&["main"]);
        let mut agent = make_agent("uuid-001");
        // Simulate the in-flight state set by refresh_named_agents.
        agent.parse_queued = true;
        app.tabs[0].named_agents.push(agent);
        app.token_tx.send(("uuid-001".to_string(), 42_000)).unwrap();
        app.drain_token_results();
        let agent = &app.tabs[0].named_agents[0];
        assert_eq!(agent.token_total, Some(42_000));
        // parse_queued must be cleared after the result arrives.
        assert!(!agent.parse_queued);
    }

    #[test]
    fn drain_token_results_ignores_unknown_uuid() {
        let mut app = make_app(&["main"]);
        app.tabs[0].named_agents.push(make_agent("uuid-001"));
        // Result for a different UUID must not update our agent.
        app.token_tx.send(("unknown-uuid".to_string(), 99)).unwrap();
        app.drain_token_results();
        assert_eq!(app.tabs[0].named_agents[0].token_total, None);
    }

    #[test]
    fn drain_token_results_applies_result_to_correct_tab() {
        // Agent lives in tabs[1] — drain must iterate all tabs, not only tabs[0].
        let mut app = make_app(&["main", "feat"]);
        let mut agent = make_agent("uuid-tab1");
        agent.parse_queued = true;
        app.tabs[1].named_agents.push(agent);
        app.token_tx.send(("uuid-tab1".to_string(), 7_777)).unwrap();
        app.drain_token_results();
        let agent = &app.tabs[1].named_agents[0];
        assert_eq!(agent.token_total, Some(7_777));
        assert!(!agent.parse_queued);
        // tabs[0] must be unaffected.
        assert!(app.tabs[0].named_agents.is_empty());
    }
}
