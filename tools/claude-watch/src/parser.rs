use std::fs;
use std::path::Path;
use serde_json::Value;

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
                    // Use and_then so a missing "text" key yields "" rather than
                    // dropping the item entirely (C-6).
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
                    Some(ContentItem::ToolUse { name, input_preview })
                }
                // Unknown content types are silently skipped.
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

        // Malformed JSON lines are skipped.
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
            // Fallback handler for top-level tool_result entries (older format
            // variants).  In practice, tool results appear inside user-message
            // content arrays; see `parse_user_content`.
            "tool_result" => {
                let content = obj.get("content").cloned().unwrap_or(Value::Null);
                let text = extract_tool_result(&content);
                if !text.is_empty() {
                    turns.push(Turn::ToolResult(text));
                }
            }
            // All unknown top-level types are silently ignored.
            _ => {}
        }
    }

    turns
}

/// Dispatch user-message content.
///
/// Tool results arrive as `{"type":"tool_result", ...}` items inside the
/// user-message content array — not as top-level JSONL entries — per the
/// Claude API/CLI format.  This function extracts them and emits separate
/// Turn::ToolResult entries so they are rendered in the content panel.
fn parse_user_content(content: Value, turns: &mut Vec<Turn>) {
    match content {
        Value::Array(arr) => {
            let mut text_parts: Vec<String> = Vec::new();

            for item in &arr {
                let item_obj = match item.as_object() {
                    Some(o) => o,
                    None => continue,
                };
                match item_obj.get("type").and_then(|v| v.as_str()) {
                    Some("tool_result") => {
                        let tr_content =
                            item_obj.get("content").cloned().unwrap_or(Value::Null);
                        let text = extract_tool_result(&tr_content);
                        if !text.is_empty() {
                            turns.push(Turn::ToolResult(text));
                        }
                    }
                    Some("text") => {
                        if let Some(t) =
                            item_obj.get("text").and_then(|v| v.as_str())
                        {
                            text_parts.push(t.to_string());
                        }
                    }
                    _ => {}
                }
            }

            if !text_parts.is_empty() {
                turns.push(Turn::User(text_parts.join("\n")));
            }
        }
        other => {
            // Plain-string or other content shape — treat as user text.
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
    use super::truncate_str;

    #[test]
    fn truncate_within_limit_is_unchanged() {
        assert_eq!(truncate_str("hello", 10), "hello");
    }

    #[test]
    fn truncate_exact_limit_is_unchanged() {
        assert_eq!(truncate_str("hello", 5), "hello");
    }

    #[test]
    fn truncate_one_byte_over_adds_ellipsis() {
        assert_eq!(truncate_str("hello", 4), "hell…");
    }

    #[test]
    fn truncate_empty_string() {
        assert_eq!(truncate_str("", 0), "");
        assert_eq!(truncate_str("", 5), "");
    }

    #[test]
    fn truncate_multibyte_backs_to_char_boundary() {
        // "é" is 2 bytes; max=2 splits inside the char, must back to index 1 ("h").
        let s = "hé";
        let result = truncate_str(s, 2);
        assert_eq!(result, "h…");
    }

    #[test]
    fn truncate_single_multibyte_char_at_max_1() {
        // "é" is 2 bytes, max=1 → backs to index 0, empty prefix → "…"
        let result = truncate_str("é", 1);
        assert_eq!(result, "…");
    }
}
