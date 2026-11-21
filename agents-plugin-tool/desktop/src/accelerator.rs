#[derive(Clone, Copy)]
pub enum Key {
    R,
    I,
    F12,
    Zero,
    Plus,
    Minus,
}

pub fn menu_id(key: Key, ctrl: bool, shift: bool) -> Option<&'static str> {
    match (key, ctrl, shift) {
        (Key::R, true, false) => Some("retry"),
        (Key::I, true, true) => Some("devtools"),
        (Key::F12, false, false) => Some("devtools_f12"),
        (Key::Zero, true, false) => Some("zoom_reset"),
        (Key::Plus, true, false) => Some("zoom_in"),
        (Key::Plus, true, true) => Some("zoom_in_shifted"),
        (Key::Minus, true, false) => Some("zoom_out"),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{menu_id, Key};
    use crate::zoom::Step;

    #[test]
    fn focused_webview_keys_map_to_retry_and_devtools_menu_items() {
        assert_eq!(menu_id(Key::R, true, false), Some("retry"));
        assert_eq!(menu_id(Key::I, true, true), Some("devtools"));
        assert_eq!(menu_id(Key::F12, false, false), Some("devtools_f12"));
    }

    #[test]
    fn focused_webview_keys_reuse_menu_zoom_steps() {
        for (key, shift, step) in [
            (Key::Zero, false, Step::Reset),
            (Key::Plus, false, Step::In),
            (Key::Plus, true, Step::In),
            (Key::Minus, false, Step::Out),
        ] {
            assert_eq!(
                menu_id(key, true, shift).and_then(Step::from_menu_id),
                Some(step)
            );
        }
    }

    #[test]
    fn typing_and_unassigned_modifier_combinations_have_no_action() {
        for key in [Key::R, Key::I, Key::Zero, Key::Plus, Key::Minus] {
            assert_eq!(menu_id(key, false, false), None);
        }
        assert_eq!(menu_id(Key::R, true, true), None);
        assert_eq!(menu_id(Key::I, true, false), None);
        assert_eq!(menu_id(Key::F12, true, false), None);
        assert_eq!(menu_id(Key::Minus, true, true), None);
    }
}
