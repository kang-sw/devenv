//! Application state for Phase 1 (single interactive PTY session).
//!
//! Later phases expand this into multi-tab, multi-slot state; the type
//! names and field layout are fixed by the plan's cross-phase contracts.

use crate::pty::PtySession;
use crate::vt::VtScreen;

/// Exit-modal state shown when the child process terminates.
pub struct ExitedModal {
    pub status: portable_pty::ExitStatus,
}

/// Top-level application state (Phase 1).
pub struct App {
    /// Live PTY session; `None` while spawning or after close-tab.
    pub session: Option<PtySession>,
    /// VT screen model for the single panel.
    pub vt: VtScreen,
    /// Set to `true` to exit the event loop.
    pub should_quit: bool,
    /// Present when the child process has exited.
    pub exited_modal: Option<ExitedModal>,
    /// Inner panel dimensions from the last `ui::draw` call (cols, rows).
    /// Used by the resize check in the main loop.
    pub last_panel_size: (u16, u16),
}

impl App {
    /// Create a new app and immediately spawn `claude` in `cwd`.
    pub fn new(cwd: &std::path::Path) -> anyhow::Result<Self> {
        let initial_size = portable_pty::PtySize {
            rows: 24,
            cols: 80,
            pixel_width: 0,
            pixel_height: 0,
        };
        let vt = VtScreen::new(initial_size.cols, initial_size.rows);
        let session = Some(PtySession::spawn(cwd, initial_size)?);
        Ok(App {
            session,
            vt,
            should_quit: false,
            exited_modal: None,
            last_panel_size: (initial_size.cols, initial_size.rows),
        })
    }
}
