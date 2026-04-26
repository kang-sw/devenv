// Copied verbatim from tools/claude-watch/src/parser.rs.
// TODO(shared-crate): deduplicate with `claude-watch` once a shared
// `claude-jsonl` utility crate is extracted from this workspace.

use serde_json::Value;
use std::fs;
use std::path::Path;

/// A single content item within an assistant turn.
#[derive(Debug, Clone)]
pub enum ContentItem {
    Text(String),
    Thinking(String),
    ToolUse {
        name: String,
        /// JSON-serialised input, truncated to ≤200 chars.
        input_preview: String,
    },
}

/// A parsed turn from a JSONL session file.
#[derive(Debug, Clone)]
pub enum Turn {
    User(String),
    Assistant(Vec<ContentItem>),
    ToolResult(String),
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Extract a text string from `content` which may be a plain string or an
/// array of content objects.  Unknown shapes produce an empty string.
fn content_to_text(content: &Value) -> String {
    match content {
        Value::String(s) => s.clone(),
        Value::Array(arr) => arr
            .iter()
            .filter_map(|item| {
                let obj = item.as_object()?;
                let ty = obj.get("type")?.as_str()?;
                match ty {
                    "text" => obj.get("text").and_then(|v| v.as_str()).map(str::to_string),
                    _ => None,
                }
            })
            .collect::<Vec<_>>()
            .join("\n"),
        _ => String::new(),
    }
}

/// Truncate `s` to at most `max` bytes, retreating to the previous UTF-8
/// character boundary and appending "…" when truncated.
fn truncate_str(s: &str, max: usize) -> String {
    if s.len() <= max {
        s.to_string()
    } else {
        let mut end = max;
        while !s.is_char_boundary(end) {
            end -= 1;
        }
        format!("{}…", &s[..end])
    }
}

fn parse_content_items(content: &Value) -> Vec<ContentItem> {
    let arr = match content {
        Value::Array(a) => a,
        Value::String(s) => return vec![ContentItem::Text(s.clone())],
        _ => return vec![],
    };

    arr.iter()
        .filter_map(|item| {
            let obj = item.as_object()?;
            let ty = obj.get("type")?.as_str()?;
            match ty {
                "text" => {
                    let text = obj
                        .get("text")
                        .and_then(|v| v.as_str())
                        .unwrap_or("")
                        .to_string();
                    Some(ContentItem::Text(text))
                }
                "thinking" => {
                    let thinking = obj
                        .get("thinking")
                        .or_else(|| obj.get("text"))
                        .and_then(|v| v.as_str())
                        .unwrap_or("")
                        .to_string();
                    Some(ContentItem::Thinking(thinking))
                }
                "tool_use" => {
                    let name = obj
                        .get("name")
                        .and_then(|v| v.as_str())
                        .unwrap_or("unknown")
                        .to_string();
                    let input = obj.get("input").cloned().unwrap_or(Value::Null);
                    let raw = serde_json::to_string(&input).unwrap_or_default();
                    let input_preview = truncate_str(&raw, 200);
                    Some(ContentItem::ToolUse {
                        name,
                        input_preview,
                    })
                }
                _ => None,
            }
        })
        .collect()
}

fn extract_tool_result(content: &Value) -> String {
    let raw = content_to_text(content);
    truncate_str(&raw, 300)
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Parse a JSONL session file into a sequence of Turns.
///
/// This parser is defensive: malformed lines, unknown types, and missing
/// fields degrade gracefully — they are skipped without panicking.
pub fn parse_turns(path: &Path) -> Vec<Turn> {
    let content = match fs::read_to_string(path) {
        Ok(c) => c,
        Err(_) => return vec![],
    };

    let mut turns: Vec<Turn> = Vec::new();

    for line in content.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }

        let value: Value = match serde_json::from_str(line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        let obj = match value.as_object() {
            Some(o) => o,
            None => continue,
        };

        let ty = match obj.get("type").and_then(|v| v.as_str()) {
            Some(t) => t,
            None => continue,
        };

        match ty {
            "user" => {
                if let Some(msg) = obj.get("message") {
                    let content = msg.get("content").cloned().unwrap_or(Value::Null);
                    parse_user_content(content, &mut turns);
                }
            }
            "assistant" => {
                if let Some(msg) = obj.get("message") {
                    let content = msg.get("content").cloned().unwrap_or(Value::Null);
                    let items = parse_content_items(&content);
                    if !items.is_empty() {
                        turns.push(Turn::Assistant(items));
                    }
                }
            }
            "tool_result" => {
                let content = obj.get("content").cloned().unwrap_or(Value::Null);
                let text = extract_tool_result(&content);
                if !text.is_empty() {
                    turns.push(Turn::ToolResult(text));
                }
            }
            _ => {}
        }
    }

    turns
}

fn parse_user_content(content: Value, turns: &mut Vec<Turn>) {
    match content {
        Value::Array(arr) => {
            for item in &arr {
                let item_obj = match item.as_object() {
                    Some(o) => o,
                    None => continue,
                };
                match item_obj.get("type").and_then(|v| v.as_str()) {
                    Some("tool_result") => {
                        let tr_content = item_obj.get("content").cloned().unwrap_or(Value::Null);
                        let text = extract_tool_result(&tr_content);
                        if !text.is_empty() {
                            turns.push(Turn::ToolResult(text));
                        }
                    }
                    Some("text") => {
                        if let Some(t) = item_obj.get("text").and_then(|v| v.as_str()) {
                            if !t.is_empty() {
                                turns.push(Turn::User(t.to_string()));
                            }
                        }
                    }
                    _ => {}
                }
            }
        }
        other => {
            let text = content_to_text(&other);
            if !text.is_empty() {
                turns.push(Turn::User(text));
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
    use std::sync::atomic::{AtomicUsize, Ordering};

    // RAII wrapper: writes a temp JSONL file and removes it on drop.
    // Guarantees cleanup even when a test panics.
    struct TempFile(std::path::PathBuf);

    impl TempFile {
        fn write(data: &str) -> Self {
            static CTR: AtomicUsize = AtomicUsize::new(0);
            let n = CTR.fetch_add(1, Ordering::Relaxed);
            let path =
                std::env::temp_dir().join(format!("cldash_parser_test_{n}.jsonl"));
            std::fs::write(&path, data).unwrap();
            TempFile(path)
        }
        fn path(&self) -> &std::path::Path {
            &self.0
        }
    }

    impl Drop for TempFile {
        fn drop(&mut self) {
            let _ = std::fs::remove_file(&self.0);
        }
    }

    // --- truncate_str ---

    #[test]
    fn string_within_byte_limit_returns_unchanged() {
        assert_eq!(truncate_str("hello", 10), "hello");
    }

    #[test]
    fn string_at_exact_byte_limit_returns_unchanged() {
        assert_eq!(truncate_str("hello", 5), "hello");
    }

    #[test]
    fn string_over_limit_truncates_with_ellipsis() {
        // "hello world" truncated at 5 bytes → "hello" + "…"
        assert_eq!(truncate_str("hello world", 5), "hello…");
    }

    #[test]
    fn empty_string_returns_empty() {
        assert_eq!(truncate_str("", 0), "");
        assert_eq!(truncate_str("", 10), "");
    }

    #[test]
    fn truncate_retreats_to_codepoint_boundary() {
        // "日本語" — each char is 3 bytes; max=4 lands inside the second codepoint.
        // Must retreat to byte 3 ("日"), producing "日…".
        assert_eq!(truncate_str("日本語", 4), "日…");
    }

    #[test]
    fn truncate_at_zero_returns_ellipsis_only() {
        // Retreats to boundary 0 → empty prefix + "…"
        assert_eq!(truncate_str("abc", 0), "…");
    }

    // --- parse_turns ---

    #[test]
    fn missing_file_returns_no_turns() {
        let turns = parse_turns(std::path::Path::new("/nonexistent/__x__.jsonl"));
        assert!(turns.is_empty());
    }

    #[test]
    fn empty_file_returns_no_turns() {
        let f = TempFile::write("");
        assert!(parse_turns(f.path()).is_empty());
    }

    #[test]
    fn blank_lines_produce_no_turns() {
        let f = TempFile::write("\n\n\n");
        assert!(parse_turns(f.path()).is_empty());
    }

    #[test]
    fn malformed_json_line_is_skipped() {
        let f = TempFile::write("not json\n");
        assert!(parse_turns(f.path()).is_empty());
    }

    #[test]
    fn non_object_json_lines_are_skipped() {
        // Valid JSON but not objects: null, array, number — all must be skipped.
        let f = TempFile::write("null\n[]\n42\n");
        assert!(parse_turns(f.path()).is_empty());
    }

    #[test]
    fn user_string_content_produces_user_turn() {
        let data = r#"{"type":"user","message":{"content":"hello"}}"#;
        let f = TempFile::write(data);
        let turns = parse_turns(f.path());
        assert_eq!(turns.len(), 1);
        assert!(matches!(&turns[0], Turn::User(s) if s == "hello"));
    }

    #[test]
    fn assistant_text_array_produces_text_content_item() {
        let data =
            r#"{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}"#;
        let f = TempFile::write(data);
        let turns = parse_turns(f.path());
        assert_eq!(turns.len(), 1);
        assert!(matches!(
            &turns[0],
            Turn::Assistant(items) if matches!(&items[0], ContentItem::Text(t) if t == "hi")
        ));
    }

    #[test]
    fn interleaved_user_and_assistant_turns_parse_in_order() {
        // Two-line JSONL: user turn followed by assistant turn.
        let data = concat!(
            r#"{"type":"user","message":{"content":"ping"}}"#,
            "\n",
            r#"{"type":"assistant","message":{"content":[{"type":"text","text":"pong"}]}}"#,
        );
        let f = TempFile::write(data);
        let turns = parse_turns(f.path());
        assert_eq!(turns.len(), 2);
        assert!(matches!(&turns[0], Turn::User(s) if s == "ping"));
        assert!(matches!(
            &turns[1],
            Turn::Assistant(items) if matches!(&items[0], ContentItem::Text(t) if t == "pong")
        ));
    }
}
