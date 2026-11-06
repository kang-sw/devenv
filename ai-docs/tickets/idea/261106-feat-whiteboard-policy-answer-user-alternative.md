---
title: Whiteboard policy answer that proposes the user's own alternative
---

# Whiteboard policy answer that proposes the user's own alternative

## Background

Dogfood surprise (2026-11-06, devenv lead session): the user answered a
policy item with an alternative none of its options named ("(202) note:
store each ring entry as its own file ..."). `whiteboard.update` refused
the answer: "a policy answer starts with its option letter (a-c)". The
lead had to patch the item's `options` to append the user's alternative as
option d in a separate update (since `answer` applies before `patch` in one
batch), and then record `d` in a second update, breaking the one-update-
per-turn rule.

The `whiteboard` playbook's `#### Answers` covers letters, notes, and
`assumed`, but not an answer that is a new alternative. Users do this often
in discussion, so every lead hits the same two-update workaround.

Open for design: accept a free-text policy answer (for example `answer:
"other, <text>"` or no letter), apply `patch` before `answer` within a batch
so one update can add and select the option, or document the two-step
workaround in the playbook.
