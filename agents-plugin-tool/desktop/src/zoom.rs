// Page zoom for the dashboard webview.
//
// WKWebView has no built-in Cmd+=/Cmd+-/Cmd+0 handling (Safari implements
// those in its own app menu), and Tauri's `zoom_hotkeys_enabled` on macOS
// injects a JS polyfill that calls the `set_webview_zoom` IPC command. The
// remote dashboard deliberately holds no native permissions, so that polyfill
// would be denied. Native menu accelerators drive `set_zoom` instead.

/// Browser-like zoom ladder (Safari/Chrome steps), 1.0 is actual size.
const LEVELS: [f64; 13] = [
    0.5, 0.67, 0.75, 0.8, 0.9, 1.0, 1.1, 1.25, 1.5, 1.75, 2.0, 2.5, 3.0,
];

#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Step {
    In,
    Out,
    Reset,
}

impl Step {
    /// Map a menu item id to a zoom step.
    pub fn from_menu_id(id: &str) -> Option<Step> {
        match id {
            "zoom_in" | "zoom_in_shifted" => Some(Step::In),
            "zoom_out" => Some(Step::Out),
            "zoom_reset" => Some(Step::Reset),
            _ => None,
        }
    }
}

/// The next ladder level from `current`, clamped at both ends.
pub fn next(current: f64, step: Step) -> f64 {
    match step {
        Step::Reset => 1.0,
        Step::In => LEVELS
            .iter()
            .copied()
            .find(|level| *level > current + 1e-6)
            .unwrap_or(LEVELS[LEVELS.len() - 1]),
        Step::Out => LEVELS
            .iter()
            .rev()
            .copied()
            .find(|level| *level < current - 1e-6)
            .unwrap_or(LEVELS[0]),
    }
}

#[cfg(test)]
mod tests {
    use super::{next, Step};

    #[test]
    fn steps_walk_the_ladder_and_clamp() {
        assert_eq!(next(1.0, Step::In), 1.1);
        assert_eq!(next(1.0, Step::Out), 0.9);
        assert_eq!(next(3.0, Step::In), 3.0);
        assert_eq!(next(0.5, Step::Out), 0.5);
        assert_eq!(next(2.5, Step::Reset), 1.0);
        // An off-ladder value snaps to the neighbouring level.
        assert_eq!(next(1.2, Step::In), 1.25);
        assert_eq!(next(1.2, Step::Out), 1.1);
    }

    #[test]
    fn menu_ids_map_to_steps() {
        assert_eq!(Step::from_menu_id("zoom_in"), Some(Step::In));
        assert_eq!(Step::from_menu_id("zoom_in_shifted"), Some(Step::In));
        assert_eq!(Step::from_menu_id("zoom_out"), Some(Step::Out));
        assert_eq!(Step::from_menu_id("zoom_reset"), Some(Step::Reset));
        assert_eq!(Step::from_menu_id("retry"), None);
    }
}
