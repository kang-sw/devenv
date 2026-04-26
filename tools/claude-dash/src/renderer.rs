// Copied verbatim from tools/claude-watch/src/renderer.rs.
// A future refactor may extract a shared `claude-jsonl` library; for now
// the copy-then-share-later convention is intentional.

use pulldown_cmark::{Event, HeadingLevel, Parser, Tag, TagEnd};
use ratatui::{
    style::{Color, Modifier, Style},
    text::{Line, Span},
};

use crate::parser::{ContentItem, Turn};

pub struct RenderOptions {
    pub show_thinking: bool,
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

/// Render a sequence of Turns to a flat list of ratatui Lines suitable for
/// display in a scrollable Paragraph.
pub fn render_turns(turns: &[Turn], opts: &RenderOptions) -> Vec<Line<'static>> {
    let mut lines: Vec<Line<'static>> = Vec::new();

    for turn in turns {
        render_turn(turn, opts, &mut lines);
    }

    lines
}

// ---------------------------------------------------------------------------
// Per-turn rendering
// ---------------------------------------------------------------------------

fn separator(label: &str, style: Style) -> Line<'static> {
    Line::from(vec![Span::styled(format!("─── {} ", label), style)])
}

fn render_turn(turn: &Turn, opts: &RenderOptions, out: &mut Vec<Line<'static>>) {
    match turn {
        Turn::User(text) => {
            out.push(separator(
                "USER",
                Style::default()
                    .fg(Color::DarkGray)
                    .add_modifier(Modifier::DIM),
            ));
            for line in text.lines() {
                out.push(Line::from(vec![Span::styled(
                    line.to_string(),
                    Style::default().fg(Color::DarkGray),
                )]));
            }
            out.push(Line::default());
        }

        Turn::Assistant(items) => {
            out.push(separator(
                "ASSISTANT",
                Style::default()
                    .fg(Color::Cyan)
                    .add_modifier(Modifier::BOLD),
            ));
            for item in items {
                render_content_item(item, opts, out);
            }
            out.push(Line::default());
        }

        Turn::ToolResult(text) => {
            out.push(Line::from(vec![
                Span::styled(
                    "← result  ".to_string(),
                    Style::default().fg(Color::Magenta),
                ),
                Span::styled(text.clone(), Style::default().fg(Color::DarkGray)),
            ]));
        }
    }
}

fn render_content_item(item: &ContentItem, opts: &RenderOptions, out: &mut Vec<Line<'static>>) {
    match item {
        ContentItem::Text(text) => {
            out.extend(render_markdown(text));
            out.push(Line::default());
        }

        ContentItem::Thinking(thinking) => {
            if opts.show_thinking {
                out.push(Line::from(vec![Span::styled(
                    "┌─ thinking ──────────────".to_string(),
                    Style::default()
                        .fg(Color::DarkGray)
                        .add_modifier(Modifier::DIM),
                )]));
                for line in thinking.lines() {
                    out.push(Line::from(vec![Span::styled(
                        format!("│ {}", line),
                        Style::default()
                            .fg(Color::DarkGray)
                            .add_modifier(Modifier::DIM),
                    )]));
                }
                out.push(Line::from(vec![Span::styled(
                    "└──────────────────────────".to_string(),
                    Style::default()
                        .fg(Color::DarkGray)
                        .add_modifier(Modifier::DIM),
                )]));
            } else {
                out.push(Line::from(vec![Span::styled(
                    "[thinking block hidden — press t to show]".to_string(),
                    Style::default()
                        .fg(Color::DarkGray)
                        .add_modifier(Modifier::DIM),
                )]));
            }
        }

        ContentItem::ToolUse {
            name,
            input_preview,
        } => {
            out.push(Line::from(vec![
                Span::styled(
                    format!("⚙ {}  ", name),
                    Style::default()
                        .fg(Color::Yellow)
                        .add_modifier(Modifier::BOLD),
                ),
                Span::styled(input_preview.clone(), Style::default().fg(Color::DarkGray)),
            ]));
        }
    }
}

// ---------------------------------------------------------------------------
// Markdown renderer (pulldown-cmark → ratatui Lines)
// ---------------------------------------------------------------------------

fn render_markdown(text: &str) -> Vec<Line<'static>> {
    let parser = Parser::new(text);

    let mut output: Vec<Line<'static>> = Vec::new();
    let mut current: Vec<Span<'static>> = Vec::new();

    let mut bold = false;
    let mut italic = false;
    let mut heading: Option<HeadingLevel> = None;
    let mut in_code_block = false;
    let mut code_buf: Vec<String> = Vec::new();

    for event in parser {
        match event {
            Event::Start(Tag::Heading { level, .. }) => {
                heading = Some(level);
            }
            Event::End(TagEnd::Heading(_)) => {
                flush(&mut current, &mut output);
                heading = None;
            }

            Event::Start(Tag::Strong) => bold = true,
            Event::End(TagEnd::Strong) => bold = false,
            Event::Start(Tag::Emphasis) => italic = true,
            Event::End(TagEnd::Emphasis) => italic = false,

            Event::Code(text) => {
                current.push(Span::styled(
                    format!("`{}`", text.into_string()),
                    Style::default().fg(Color::Yellow),
                ));
            }

            Event::Start(Tag::CodeBlock(_)) => {
                in_code_block = true;
                code_buf.clear();
            }
            Event::End(TagEnd::CodeBlock) => {
                in_code_block = false;
                for line in &code_buf {
                    output.push(Line::from(vec![Span::styled(
                        line.clone(),
                        Style::default().fg(Color::Yellow),
                    )]));
                }
                code_buf.clear();
            }

            Event::Text(text) => {
                let text = text.into_string();
                if in_code_block {
                    for (i, line) in text.split('\n').enumerate() {
                        if i == 0 {
                            if let Some(last) = code_buf.last_mut() {
                                last.push_str(line);
                            } else {
                                code_buf.push(line.to_string());
                            }
                        } else {
                            code_buf.push(line.to_string());
                        }
                    }
                } else {
                    let style = heading_or_inline_style(heading, bold, italic);
                    let mut first = true;
                    for segment in text.split('\n') {
                        if !first {
                            flush(&mut current, &mut output);
                        }
                        first = false;
                        if !segment.is_empty() {
                            current.push(Span::styled(segment.to_string(), style));
                        }
                    }
                }
            }

            Event::Start(Tag::Item) => {
                current.push(Span::raw("• ".to_string()));
            }
            Event::End(TagEnd::Item) => {
                flush(&mut current, &mut output);
            }

            Event::SoftBreak | Event::HardBreak => {
                flush(&mut current, &mut output);
            }

            Event::End(TagEnd::Paragraph) => {
                flush(&mut current, &mut output);
                output.push(Line::default());
            }

            _ => {}
        }
    }

    if !current.is_empty() {
        output.push(Line::from(current));
    }

    output
}

#[inline]
fn flush(current: &mut Vec<Span<'static>>, output: &mut Vec<Line<'static>>) {
    output.push(Line::from(std::mem::take(current)));
}

fn heading_or_inline_style(heading: Option<HeadingLevel>, bold: bool, italic: bool) -> Style {
    let base = Style::default();
    if let Some(level) = heading {
        return match level {
            HeadingLevel::H1 => base.fg(Color::Cyan).add_modifier(Modifier::BOLD),
            HeadingLevel::H2 => base.fg(Color::Blue).add_modifier(Modifier::BOLD),
            HeadingLevel::H3 => base.add_modifier(Modifier::BOLD),
            _ => base.add_modifier(Modifier::BOLD),
        };
    }
    let mut style = base;
    if bold {
        style = style.add_modifier(Modifier::BOLD);
    }
    if italic {
        style = style.add_modifier(Modifier::ITALIC);
    }
    style
}
