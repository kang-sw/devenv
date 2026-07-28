---
title: Residual UTF-8 multibyte corruption in terminal output after read-boundary carry fix
related:
  260724-bug-dashboard-terminal-utf8-multibyte-read-boundary-corruption: prior fix this residual survives past
related-mental-model:
  - ws-web-dashboard/terminal
---

# Residual UTF-8 multibyte corruption in terminal output after read-boundary carry fix

## Background

`260724-bug-dashboard-terminal-utf8-multibyte-read-boundary-corruption` (`.done/`,
fix commit `0741e94f`) added a bounded (<=3 byte) `carry: Vec<u8>` in
`spawn_reader_thread` (`ws-dashboard/crates/daemon/src/terminal_helper_process.rs`)
that prepends an incomplete trailing UTF-8 sequence to the next `read()` before
decoding. The fix is intact at HEAD and demonstrably reduced the frequency of
Korean/CJK corruption.

A residual corruption still occurs at low frequency. Captured live during
dogfooding on a **fresh rebuild launched the same afternoon as this ticket**
(2026-07-25), i.e. the running daemon/terminal-helper binary already contained
the `0741e94f` carry fix. This rules out the "stale long-lived daemon binary"
explanation and indicates a genuine leak in the current decode path.

## Observed signature

- Rendered as U+FFFD replacement char (mojibake glyph `?`-in-diamond), **not**
  wrong-but-legible glyphs. This places it in the split-multibyte family, not a
  locale/code-page (CP949/EUC-KR) mis-encoding.
- Appeared as **three consecutive** replacement chars (`... 렌더 <FFFD><FFFD><FFFD> 서 ...`)
  replacing roughly one Korean syllable's worth of text — i.e. more than a clean
  single-codepoint split; looks like a lead byte was separated from its
  continuation bytes, or the decode stream desynced across several bytes.
- Position of the corruption varies from frame to frame; it is not a static
  stored corruption. Triggered while scrolling a **TUI app's internal buffer**
  (Claude Code TUI, in-place screen repaint — each scroll emits a fresh
  full-screen repaint burst through the PTY, so each frame is a new decode pass
  splitting at a different byte offset).
- Low frequency: roughly one occurrence in ~1 hour of active TUI use. Not at a
  disruptive level yet.

## Candidate hypotheses (unverified)

- The `Utf8Error::error_len() == Some(n)` branch (genuinely-malformed span ->
  emit one U+FFFD, resume decoding past it in the same iteration) may
  mis-classify a legitimate cross-`read()` split as malformed under some carry
  state, emitting replacement chars instead of carrying, and possibly desyncing
  subsequent bytes into a run of replacement chars.
- A path where `carry` is not preserved/applied between reads (e.g. after the
  malformed-span branch, or interaction with chunk emission/flush), leaving
  orphan continuation bytes at the head of the next read.

## Not this bug

- Locale/code-page mis-encoding at shell spawn (no `LANG`/`LC_ALL`/`chcp 65001`
  forcing in `spawn_shell`) is a separate latent concern but is ruled out for
  this sighting because the symptom is U+FFFD, not legible-wrong-glyph mojibake.
- Frontend front-trim UTF-16 surrogate splitting affects only surrogate-pair
  codepoints (emoji), not single-code-unit Hangul.

## Next steps when picked up

1. Reproduce deterministically by feeding the reader thread a byte stream that
   splits a 3-byte Hangul syllable at adversarial offsets across successive
   `read()` calls, including immediately after a malformed-span branch.
2. Capture the raw PTY byte stream at the moment of corruption (add a debug tap
   in `spawn_reader_thread`) to confirm whether the daemon receives valid UTF-8
   that the decoder mangles, vs. already-invalid bytes from the shell.
3. Add a regression unit test alongside `reader_thread_utf8_tests` for the
   confirmed residual case.
