// Vendor-neutral turn-state wire shape and token comparison for the
// per-terminal turn-state callback route (260725 Phase 4,
// `terminal.rs::post_terminal_turn_state`). Deliberately independent of
// `cli::TurnStateArg` (clap-only, no `Deserialize`, drives the
// `terminal-notify` CLI's own `--state` flag) - this module owns the wire
// (HTTP body) side of the same three-state vocabulary.
//
// CONTRACT: `parse_turn_state` is a plain function, not a `Deserialize` impl
// wired directly into the request body's `Json<...>` extractor - the route
// handler must reject a WRONG TOKEN before it ever reports whether `state`
// was well-formed (never let a caller distinguish "bad token" from "bad
// state" ordering by probing which error comes back first), and axum's
// `Json` extractor runs (and can reject) before handler body code executes
// at all. Parsing `state` from a raw `String` field, deferred to AFTER the
// token check, is what keeps that ordering handler-controlled instead of
// framework-controlled. See `terminal.rs::TerminalTurnStateRequest`.

use serde::Deserialize;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum TurnState {
    Working,
    Ready,
    Idle,
}

/// Parses the POST body's `state` string into `TurnState`, reusing the
/// `Deserialize` impl above (rather than a hand-rolled `match`) so the
/// route's accepted vocabulary and any future `Deserialize`-driven consumer
/// stay byte-for-byte the same list. `None` on any unrecognized value.
pub fn parse_turn_state(raw: &str) -> Option<TurnState> {
    serde_json::from_value(serde_json::Value::String(raw.to_owned())).ok()
}

/// Constant-time equality check over the raw bytes of two tokens - the
/// per-terminal turn-state route's auth check (`terminal.rs`) must not leak,
/// via wall-clock timing, how many leading bytes of a wrong token happened to
/// match the real one. Length is compared first (not a secret worth hiding -
/// the caller already knows the daemon's token length is fixed) and then
/// every byte pair is XOR-accumulated unconditionally: the loop never returns
/// early on a mismatch, so a byte-0 mismatch and a byte-(N-1) mismatch cost
/// the same number of comparisons.
pub fn tokens_match(expected: &str, candidate: &str) -> bool {
    constant_time_eq(expected.as_bytes(), candidate.as_bytes(), |_| {})
}

/// Byte-level core of `tokens_match`, generic over an `on_byte_compared`
/// hook purely so its own dedicated test (below) can prove every index was
/// actually visited, independent of the XOR result - see that test's
/// CONTRACT for why this is the non-vacuity guard the plan's Verification
/// Plan item 6 asks for.
fn constant_time_eq(a: &[u8], b: &[u8], mut on_byte_compared: impl FnMut(usize)) -> bool {
    if a.len() != b.len() {
        return false;
    }
    let mut diff = 0u8;
    for (index, (x, y)) in a.iter().zip(b.iter()).enumerate() {
        on_byte_compared(index);
        diff |= x ^ y;
    }
    diff == 0
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_turn_state_accepts_all_three_pinned_values() {
        assert_eq!(parse_turn_state("working"), Some(TurnState::Working));
        assert_eq!(parse_turn_state("ready"), Some(TurnState::Ready));
        assert_eq!(parse_turn_state("idle"), Some(TurnState::Idle));
    }

    #[test]
    fn parse_turn_state_rejects_an_unrecognized_value() {
        assert_eq!(parse_turn_state("not-a-real-state"), None);
        assert_eq!(parse_turn_state(""), None);
        assert_eq!(parse_turn_state("Working"), None, "case-sensitive, not title-case");
    }

    #[test]
    fn tokens_match_accepts_identical_tokens() {
        assert!(tokens_match("abc123", "abc123"));
    }

    #[test]
    fn tokens_match_rejects_a_different_token_of_the_same_length() {
        assert!(!tokens_match("abc123", "abc124"));
    }

    #[test]
    fn tokens_match_rejects_tokens_of_different_lengths() {
        assert!(!tokens_match("short", "shorter"));
        assert!(!tokens_match("", "nonempty"));
    }

    // CONTRACT (non-vacuity, plan Verification Plan item 6): if
    // `constant_time_eq`'s loop were "fixed" to short-circuit on the first
    // mismatching byte (the classic accidental regression when someone
    // rewrites this for "clarity"), a byte-0 mismatch would only ever visit
    // index 0 before returning, while a byte-(N-1) mismatch would visit
    // every index - this test tells the two cases apart by recording EVERY
    // index the loop actually visits, independent of the boolean result
    // `tokens_match` returns. Mutating the loop to `if diff != 0 { return
    // false; }` right after the XOR (a real, plausible mutation) makes this
    // test fail while `tokens_match`'s own black-box tests above keep
    // passing unchanged - that gap is exactly why this dedicated test
    // exists.
    #[test]
    fn constant_time_eq_visits_every_byte_index_even_after_an_early_mismatch() {
        let a = "aaaaaaaaaa";
        let b_mismatch_at_start = "Xaaaaaaaaa";
        let b_mismatch_at_end = "aaaaaaaaaX";

        let mut visited_start = Vec::new();
        let result_start = constant_time_eq(a.as_bytes(), b_mismatch_at_start.as_bytes(), |index| {
            visited_start.push(index)
        });
        let mut visited_end = Vec::new();
        let result_end = constant_time_eq(a.as_bytes(), b_mismatch_at_end.as_bytes(), |index| {
            visited_end.push(index)
        });

        assert!(!result_start);
        assert!(!result_end);
        let expected: Vec<usize> = (0..a.len()).collect();
        assert_eq!(
            visited_start, expected,
            "a mismatch at index 0 must still visit every index, not stop early"
        );
        assert_eq!(
            visited_end, expected,
            "a mismatch at the last index must visit exactly the same indices as one at index 0"
        );
    }
}
