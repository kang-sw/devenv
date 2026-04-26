//! VT screen state: parse raw PTY bytes into a termwiz `Surface`, then
//! render the surface cells into a ratatui `Buffer`.
//!
//! # Bridge note
//!
//! termwiz ships separate layers — a VT escape parser and a screen-cell
//! model — but does NOT ship a bridge between them.  `VtScreen::feed` is
//! that bridge: it converts `termwiz::escape::Action` events produced by
//! the parser into `termwiz::surface::Change` values consumed by the
//! surface.  Only the subset of actions produced by `claude` (itself a
//! ratatui TUI) is handled; unknown variants are logged and ignored.
//!
//! # Termwiz update policy
//!
//! New termwiz versions may add `Action` or `Change` variants.  The
//! catch-all arm `_ => {}` in `handle_action` silently drops them, which
//! degrades rendering without panicking.
#![allow(unreachable_patterns)]

use ratatui::{
    buffer::Buffer,
    layout::Rect,
    style::{Color, Modifier, Style},
};
use termwiz::{
    cell::{AttributeChange, CellAttributes, Intensity, Underline},
    color::ColorAttribute,
    escape::{
        csi::{Cursor, Edit, EraseInDisplay, EraseInLine, Sgr, CSI},
        Action, ControlCode,
    },
    surface::{Change, CursorVisibility, Position, Surface},
};

pub struct VtScreen {
    surface: Surface,
    parser: termwiz::escape::parser::Parser,
    cols: u16,
    rows: u16,
}

impl VtScreen {
    pub fn new(cols: u16, rows: u16) -> Self {
        VtScreen {
            surface: Surface::new(cols as usize, rows as usize),
            parser: termwiz::escape::parser::Parser::new(),
            cols,
            rows,
        }
    }

    /// Feed raw PTY bytes into the VT state machine.
    pub fn feed(&mut self, bytes: &[u8]) {
        let actions = self.parser.parse_as_vec(bytes);
        for action in actions {
            self.handle_action(action);
        }
    }

    /// Resize the surface (preserves content within the smaller bounding box).
    pub fn resize(&mut self, cols: u16, rows: u16) {
        self.cols = cols;
        self.rows = rows;
        self.surface.resize(cols as usize, rows as usize);
    }

    /// Render the surface cells into `buf` at the given `area`.
    pub fn render_into(&self, buf: &mut Buffer, area: Rect) {
        for (y, line) in self.surface.screen_lines().iter().enumerate() {
            let y = y as u16;
            if area.y + y >= area.bottom() {
                break;
            }
            for cell in line.visible_cells() {
                let x = cell.cell_index() as u16;
                if area.x + x >= area.right() {
                    continue;
                }
                let style = attrs_to_style(cell.attrs());
                buf[(area.x + x, area.y + y)]
                    .set_symbol(cell.str())
                    .set_style(style);
            }
        }

        // Cursor overlay: draw a REVERSED cell at the cursor position so the
        // user knows where input will land.  We do NOT move the host terminal
        // cursor — that stays managed by ratatui.
        let (cx, cy) = self.surface.cursor_position();
        if matches!(self.surface.cursor_visibility(), CursorVisibility::Visible) {
            let cx = cx as u16;
            let cy = cy as u16;
            if area.x + cx < area.right() && area.y + cy < area.bottom() {
                buf[(area.x + cx, area.y + cy)]
                    .set_style(Style::default().add_modifier(Modifier::REVERSED));
            }
        }
    }

    /// Return current `(cols, rows)`.
    pub fn dimensions(&self) -> (u16, u16) {
        (self.cols, self.rows)
    }

    // -----------------------------------------------------------------------
    // Private: action dispatch
    // -----------------------------------------------------------------------

    fn handle_action(&mut self, action: Action) {
        match action {
            Action::Print(c) => {
                self.surface.add_change(Change::Text(c.to_string()));
            }
            Action::PrintString(s) => {
                self.surface.add_change(Change::Text(s));
            }
            Action::Control(ctrl) => {
                self.handle_control(ctrl);
            }
            Action::CSI(csi) => {
                self.handle_csi(csi);
            }
            // Window title changes and other OSC sequences have no surface
            // representation in this viewer.
            Action::OperatingSystemCommand(_) => {}
            Action::Esc(_) => {}
            Action::DeviceControl(_) => {}
            // Sixel/Kitty images: not supported.
            _ => {}
        }
    }

    fn handle_control(&mut self, ctrl: ControlCode) {
        match ctrl {
            ControlCode::LineFeed | ControlCode::VerticalTab | ControlCode::FormFeed => {
                self.surface.add_change(Change::Text("\n".into()));
            }
            ControlCode::CarriageReturn => {
                self.surface.add_change(Change::Text("\r".into()));
            }
            ControlCode::Backspace => {
                // Move cursor left by 1 without erasing.
                // Position::Relative(0) = no change along an axis.
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(-1),
                    y: Position::Relative(0),
                });
            }
            ControlCode::HorizontalTab => {
                self.surface.add_change(Change::Text("\t".into()));
            }
            _ => {}
        }
    }

    fn handle_csi(&mut self, csi: CSI) {
        match csi {
            CSI::Cursor(cursor) => self.handle_cursor(cursor),
            CSI::Sgr(sgr) => self.handle_sgr(sgr),
            CSI::Edit(edit) => self.handle_edit(edit),
            // Mode (private modes like ?1049 alt-screen, ?25 cursor vis):
            // Phase-1 acceptable to ignore.
            CSI::Mode(_) => {}
            _ => {}
        }
    }

    fn handle_cursor(&mut self, cursor: Cursor) {
        match cursor {
            // CUP — absolute position (termwiz uses OneBased; as_zero_based → 0-indexed)
            Cursor::Position { line, col } => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Absolute(col.as_zero_based() as usize),
                    y: Position::Absolute(line.as_zero_based() as usize),
                });
            }
            // Relative cursor moves
            Cursor::Up(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(0),
                    y: Position::Relative(-(n as isize)),
                });
            }
            Cursor::Down(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(0),
                    y: Position::Relative(n as isize),
                });
            }
            Cursor::Left(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(-(n as isize)),
                    y: Position::Relative(0),
                });
            }
            Cursor::Right(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(n as isize),
                    y: Position::Relative(0),
                });
            }
            // CHA — column absolute (OneBased)
            Cursor::CharacterAbsolute(col) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Absolute(col.as_zero_based() as usize),
                    y: Position::Relative(0),
                });
            }
            // HPA — same as CHA
            Cursor::CharacterPositionAbsolute(col) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Absolute(col.as_zero_based() as usize),
                    y: Position::Relative(0),
                });
            }
            // CNL — next line (move to column 0 and down n)
            Cursor::NextLine(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Absolute(0),
                    y: Position::Relative(n as isize),
                });
            }
            // CPL — preceding line (move to column 0 and up n)
            Cursor::PrecedingLine(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Absolute(0),
                    y: Position::Relative(-(n as isize)),
                });
            }
            // VPA — line position absolute (1-based u32, subtract 1 for 0-based)
            Cursor::LinePositionAbsolute(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(0),
                    y: Position::Absolute(n.saturating_sub(1) as usize),
                });
            }
            // VPR / VPB — relative vertical moves
            Cursor::LinePositionForward(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(0),
                    y: Position::Relative(n as isize),
                });
            }
            Cursor::LinePositionBackward(n) => {
                self.surface.add_change(Change::CursorPosition {
                    x: Position::Relative(0),
                    y: Position::Relative(-(n as isize)),
                });
            }
            // Save / restore cursor position — not modelled in this phase.
            Cursor::SaveCursor | Cursor::RestoreCursor => {}
            _ => {}
        }
    }

    fn handle_sgr(&mut self, sgr: Sgr) {
        match sgr {
            Sgr::Reset => {
                self.surface
                    .add_change(Change::AllAttributes(CellAttributes::blank()));
            }
            Sgr::Intensity(i) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Intensity(i)));
            }
            Sgr::Italic(b) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Italic(b)));
            }
            Sgr::Underline(u) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Underline(u)));
            }
            Sgr::Inverse(b) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Reverse(b)));
            }
            Sgr::Foreground(cs) => {
                let attr = color_spec_to_attribute(cs);
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Foreground(attr)));
            }
            Sgr::Background(cs) => {
                let attr = color_spec_to_attribute(cs);
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Background(attr)));
            }
            Sgr::StrikeThrough(b) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::StrikeThrough(b)));
            }
            Sgr::Invisible(b) => {
                self.surface
                    .add_change(Change::Attribute(AttributeChange::Invisible(b)));
            }
            _ => {}
        }
    }

    fn handle_edit(&mut self, edit: Edit) {
        match edit {
            Edit::EraseInLine(erase) => match erase {
                EraseInLine::EraseToEndOfLine => {
                    self.surface
                        .add_change(Change::ClearToEndOfLine(ColorAttribute::Default));
                }
                EraseInLine::EraseLine => {
                    // Move to column 0, then clear to end of line.
                    self.surface.add_change(Change::CursorPosition {
                        x: Position::Absolute(0),
                        y: Position::Relative(0),
                    });
                    self.surface
                        .add_change(Change::ClearToEndOfLine(ColorAttribute::Default));
                }
                _ => {}
            },
            Edit::EraseInDisplay(erase) => match erase {
                EraseInDisplay::EraseToEndOfDisplay => {
                    self.surface
                        .add_change(Change::ClearToEndOfScreen(ColorAttribute::Default));
                }
                EraseInDisplay::EraseDisplay => {
                    self.surface
                        .add_change(Change::ClearScreen(ColorAttribute::Default));
                }
                _ => {}
            },
            _ => {}
        }
    }
}

// ---------------------------------------------------------------------------
// Colour helpers
// ---------------------------------------------------------------------------

fn color_spec_to_attribute(cs: termwiz::color::ColorSpec) -> ColorAttribute {
    use termwiz::color::ColorSpec;
    match cs {
        ColorSpec::Default => ColorAttribute::Default,
        ColorSpec::PaletteIndex(n) => ColorAttribute::PaletteIndex(n),
        ColorSpec::TrueColor(rgba) => ColorAttribute::TrueColorWithDefaultFallback(rgba),
    }
}

fn color_attribute_to_ratatui(attr: ColorAttribute) -> Option<Color> {
    match attr {
        ColorAttribute::TrueColorWithDefaultFallback(srgb) => {
            let (r, g, b, _) = srgb.to_srgb_u8();
            Some(Color::Rgb(r, g, b))
        }
        ColorAttribute::TrueColorWithPaletteFallback(srgb, _) => {
            let (r, g, b, _) = srgb.to_srgb_u8();
            Some(Color::Rgb(r, g, b))
        }
        ColorAttribute::PaletteIndex(n) => Some(Color::Indexed(n)),
        ColorAttribute::Default => None,
    }
}

/// Convert termwiz `CellAttributes` to a ratatui `Style`.
fn attrs_to_style(attrs: &CellAttributes) -> Style {
    let mut style = Style::default();

    if let Some(fg) = color_attribute_to_ratatui(attrs.foreground()) {
        style = style.fg(fg);
    }
    if let Some(bg) = color_attribute_to_ratatui(attrs.background()) {
        style = style.bg(bg);
    }

    if attrs.intensity() == Intensity::Bold {
        style = style.add_modifier(Modifier::BOLD);
    }
    if attrs.italic() {
        style = style.add_modifier(Modifier::ITALIC);
    }
    if attrs.underline() != Underline::None {
        style = style.add_modifier(Modifier::UNDERLINED);
    }
    if attrs.reverse() {
        style = style.add_modifier(Modifier::REVERSED);
    }

    style
}
