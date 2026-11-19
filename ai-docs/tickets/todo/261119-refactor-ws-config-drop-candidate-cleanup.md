---
title: "설정 DROP 후보 정리: 사용자 노출 설정 키 축소"
related:
  261119-feat-ws-config-git-sync: "origin: the config-sync triage produced the DROP candidates; there they stay local and unsynced (owner (1334)=a)"
  261113-refactor-ws-dashboard-drop-lan-serving: "precedent for removing a user-facing key: accept-and-warn for one minor, then delete"
  261116-feat-ws-dashboard-config-settings-ui: "a smaller user-facing key set shrinks the settings sheet's catalog"
---

# Config DROP-candidate cleanup: shrink the user-facing config surface

## Background

The config-sync triage (lead-commissioned read-only triage, 2026-11-19,
against `develop`; full table in `261119-feat-ws-config-git-sync` Appendix A)
marked 28 of 68 user-settable elements as **DROP-CANDIDATE**: internal timers
and byte budgets, dead constants, internal playbook splices, and repo-only
coordination flags that are rarely tuned from their defaults. The owner chose
to handle them in a separate cleanup ticket rather than inside config sync
(owner answer (1334)=a, 2026-11-19).

The triage verdict is a candidate list, not a removal decision. Several
candidates are read by more than the Go resolver (Claude mod and Pi extension
startup readers, playbook splices), and a few are repo-level constructs rather
than unused knobs, so each needs re-verification before the owner confirms it.

## Decisions

1. **(1334)=a, owner answer 2026-11-19.** DROP candidates are handled by this
   separate cleanup ticket. The 28 candidates are re-verified key by key, the
   list is posted to the owner for confirmation, and only owner-confirmed keys
   are removed from the user-facing config surface. Code defaults stay.

## Constraints

- **Owner confirmation precedes implementation.** Removing a user-facing key
  removes functionality. No key is removed until the owner has confirmed it by
  name; the confirmed per-key list is then recorded in `## Decisions`. The
  table in `## Unconfirmed Candidates` below carries no authority.
- **Old config files keep working.** A `~/.ws/config.json`, project-scope
  file, or committed repo config that still carries a removed key must load
  and resolve normally: the removed key is ignored, never a parse or
  resolution error. Target tier T0/T1 (`ai-docs/manuals/release-compatibility.md`);
  the grade is settled through (1353) and recorded in `## Decisions` before
  release. Evidence at drafting time: overrides are a free `map[string]string`
  (`agents-plugin-tool/internal/wsconfig/config.go:40`) that every writer
  round-trips whole (`config.go:783-805`), and resolution reads only the key
  it is asked for (`wsconfig/resolver.go:82-131`).
- **Code defaults stay.** Removal takes a key off the tunable surface
  (registry/catalog, manifests, `config.tune`/`config.list`, docs and skill
  text); the behavior keeps running on its built-in default.
- **Manifests stay mirrored.** A key removed from a config manifest is removed
  from every package copy in the same change, and the ws and wsflow manifests
  stay key-identical with only `default` differing
  (`TestShippedClaudeManifestsDifferOnlyInDefaults`,
  `agents-plugin-tool/internal/mcp/config_manifest_parity_test.go:51`;
  `ai-docs/manuals/wsflow-mirroring.md`). Removing a key is not a manifest
  *field* change, so the parser's unknown-field refusal
  (`internal/mcp/config_manifest.go:179`) is not crossed.
- **Not synced meanwhile.** Until removed, DROP candidates stay local and are
  never written to the synced layer of `261119-feat-ws-config-git-sync`.

## Unconfirmed Candidates

The 28 DROP-CANDIDATE rows from the triage, copied unchanged. Evidence lines
are `develop` at triage time. **These are unconfirmed candidates**: each is
re-verified in Phase 1 and removed only after owner confirmation.

| key | what it controls | current default write scope | verdict | one-line reason | evidence file:line |
|---|---|---|---|---|---|
| `bootstrap_alarm` | Gates `AGENTS.md` template-version staleness warning | `global` (global-only) | DROP-CANDIDATE | Rarely tuned from default `on`; syncing adds negligible value | `agents-plugin-tool/internal/wsconfig/scope.go:130,226`, `agents-plugin-tool/internal/mcp/config_registry.go:364` |
| `agent.child_waiting_days` | Days before daemon deletes idle waiting child agents (default `15`) | `global` (global-only) | DROP-CANDIDATE | Internal daemon retention TTL rarely tuned from 15 days | `agents-plugin-tool/internal/wsconfig/scope.go:151,229`, `agents-plugin-tool/internal/mcp/config_registry.go:411` |
| `prompt.TicketHandlerPersistConfirmation.<claude\|codex\|pi\|all>` | Internal override splice for `ticket-handler` confirmation-record rule | `project` | DROP-CANDIDATE | Internal playbook splice driven by `ticket_review_auto_ack`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/ticket-handler/ticket-handler.md:37`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `prompt.TicketHandlerReviewAutoAck.<claude\|codex\|pi\|all>` | Internal override splice for `ticket-handler` auto-confirm rules | `project` | DROP-CANDIDATE | Internal playbook splice driven by `ticket_review_auto_ack`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/ticket-handler/ticket-handler.md:77`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `prompt.TicketQueueWhiteboard.<claude\|codex\|pi\|all>` | Internal override splice for `lead-ticket` whiteboard queue rule | `project` | DROP-CANDIDATE | Internal playbook splice driven by `workflow.whiteboard_enabled`, not a user-preference prompt | `agents-plugin-tool/internal/mcp/config_registry.go:570-597`, `agents-plugin/rsrc/lead-ticket/lead-ticket.md:99`, `agents-plugin-tool/internal/mcp/ticket_review_text_test.go:86-118` |
| `worktree.profile.<name>` (`structured_overrides`) | Worktree pool partition, sparse-checkout, preserve patterns, and lifecycle hooks | `project` | DROP-CANDIDATE | Repo/project construct (belongs in `.ws-workflow/config.json`); rarely tuned globally and hooks are OS/tool-bound | `agents-plugin-tool/internal/mcp/worktree_profile.go:22,288-307`, `agents-plugin-tool/internal/mcp/config_structured.go:116-119`, `agents-plugin-tool/internal/mcp/server_config.go:575` |
| `worktree.nudge_stale_days` | Inactivity days before `worktree.acquire` nudges about a pooled worktree (default `7`) | `project` | DROP-CANDIDATE | Local pool cleanup threshold rarely tuned from default 7 days | `agents-plugin-tool/internal/wsconfig/scope.go:189,249`, `agents-plugin-tool/internal/mcp/worktree_nudge.go:22-30` |
| `worktree.nudge_preserved_gb` | Preserved ignored-artifact GB threshold for detached worktree nudge (default `1`) | `project` | DROP-CANDIDATE | Local disk-usage threshold rarely tuned from default 1 GB | `agents-plugin-tool/internal/wsconfig/scope.go:190,250`, `agents-plugin-tool/internal/mcp/worktree_nudge.go:31-39` |
| `git.merge_size_soft_lines` | Soft line-count limit for `git.merge` advisory file-size warning (default `1000`) | `project` | DROP-CANDIDATE | Per-repo codebase convention (`.ws-workflow/config.json`), rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:197,251`, `agents-plugin-tool/internal/mcp/git_merge_size.go:19-27` |
| `git.merge_size_hard_lines` | Hard line-count limit for `git.merge` advisory file-size warning (default `2000`) | `project` | DROP-CANDIDATE | Per-repo codebase convention (`.ws-workflow/config.json`), rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:198,252`, `agents-plugin-tool/internal/mcp/git_merge_size.go:28-36` |
| `git.merge_size_extensions` | Space-separated file extensions checked by `git.merge` size warning | `project` | DROP-CANDIDATE | Per-repo language/extension set, rarely tuned globally | `agents-plugin-tool/internal/wsconfig/scope.go:199,253`, `agents-plugin-tool/internal/mcp/git_merge_size.go:37-44` |
| `workflow.skeptical_posture` | Prepends evidence-posture block to `workflow_manual` (unlisted in `config.tune`) | `global` (global-only) | DROP-CANDIDATE | Has no `config.tune` writer and defaults `on`; rarely tuned | `agents-plugin-tool/internal/wsconfig/scope.go:123,225`, `agents-plugin-tool/internal/mcp/workflow_manual.go:369-370` |
| `ticket-assignee-aware` | Opt-in project gate for ticket assignee tracking (no `config.tune` writer) | `project` | DROP-CANDIDATE | Committed per-repo team decision in `.ws-workflow/config.json`, not a user-global setting | `agents-plugin-tool/internal/wsconfig/scope.go:165,257`, `agents-plugin-tool/internal/mcp/server_tickets.go:14-26` |
| `sage_review_design_tier` | Registered `scope.go` constant for design reviewer tier (unused/orphaned) | `project` | DROP-CANDIDATE | Dead constant in `scope.go`; never read or written by `config.tune` or runtime | `agents-plugin-tool/internal/wsconfig/scope.go:104,241` |
| `sage_review_completeness_tier` | Registered `scope.go` constant for completeness reviewer tier (unused/orphaned) | `project` | DROP-CANDIDATE | Dead constant in `scope.go`; never read or written by `config.tune` or runtime | `agents-plugin-tool/internal/wsconfig/scope.go:108,242` |
| `claude.manual_injection_enabled` | Enables Claude Code lead session key minting and workflow manual injection | `project` | DROP-CANDIDATE | Core plugin mechanism defaulting `true`; rarely turned off | `agents-plugin/config-manifest.json:19-23`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.display_summary_context_tokens` | Context window size in tokens for `claude.display_summary_model` | `project` | DROP-CANDIDATE | Derived from the summary model (`200000`); rarely tuned independently | `agents-plugin/config-manifest.json:67-72`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.mcp_watchdog_enabled` | Enables periodic `/mcp reconnect` watchdog when Claude Code idles | `project` | DROP-CANDIDATE | Host/network reconnect workaround rarely tuned from package default | `agents-plugin/config-manifest.json:74-78`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `claude.mcp_watchdog_interval_minutes` | Polling interval in minutes for Claude Code MCP watchdog | `project` | DROP-CANDIDATE | Internal timer constant rarely tuned from 10 minutes | `agents-plugin/config-manifest.json:80-85`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `dashboard.mailbox_undelivered_minutes` | Minutes before daemon sends undelivered-mail notice (default `3`) | `global` (global-only) | DROP-CANDIDATE | Internal daemon sweep threshold rarely tuned from 3 minutes | `agents-plugin/dashboard-config-manifest.json:27-33`, `agents-plugin-tool/internal/mcp/dashboard_config.go:23,33` |
| `pi.agent_wait_animation` | Animates Pi live-agent widget while child waits | `global` | DROP-CANDIDATE | Minor visual polish rarely tuned from default `true` | `agents-plugin/native-pi/first-party/config-manifest.json:6-11` |
| `pi.agent_widget_rows` | Tool-batch rows per agent in expanded Pi `/agents` widget | `global` | DROP-CANDIDATE | Minor UI row cap (`3`, auto-shrinks on short terminals); rarely tuned | `agents-plugin/native-pi/first-party/config-manifest.json:13-20` |
| `pi.runaway_threshold` | Consecutive Pi goal-loop reminders with no tool call before force-stop | `project` | DROP-CANDIDATE | Internal safety loop counter rarely tuned from 10 | `agents-plugin/native-pi/first-party/config-manifest.json:36-41`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.settle_delay_ms` | Goal-loop settle wait / wake retry delay in milliseconds | `project` | DROP-CANDIDATE | Internal timing constant rarely tuned from 5000ms | `agents-plugin/native-pi/first-party/config-manifest.json:43-48`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.context_window_override` | Overrides model context window size for Pi percent calculation | `project` | DROP-CANDIDATE | Rarely tuned fallback (`0` uses model's own window) | `agents-plugin/native-pi/first-party/config-manifest.json:64-69`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.compaction_dialog_budget_bytes` | Byte budget for dialog section of Pi lead compaction summary | `project` | DROP-CANDIDATE | Internal summary byte budget rarely tuned from 40960 | `agents-plugin/native-pi/first-party/config-manifest.json:71-76`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.compaction_reread_budget_bytes` | Byte budget for inlined required re-reads in Pi lead compaction summary | `project` | DROP-CANDIDATE | Internal summary byte budget rarely tuned from 40960 | `agents-plugin/native-pi/first-party/config-manifest.json:78-83`, `agents-plugin-tool/internal/mcp/config_manifest.go:273-274` |
| `pi.child_retention_ttl_days` | Days before Pi prunes stored child homes at lead startup | `global` | DROP-CANDIDATE | Local disk-cleanup TTL rarely tuned from 30 days | `agents-plugin/native-pi/first-party/config-manifest.json:85-91` |

## Open Decision Queue

Last ID used: (1359)

(1353) [open] [announced] Compatibility tier T0/T1. A stored value of a removed key is ignored by the new binary (resolution never asks for it; writers keep the map entry, `config.go:40,783-805`), so the file still loads (T1 for the user whose tuned value stops applying, no data change). An older binary on the same machine serves its own embedded manifests and registry and keeps honoring the value (T0 for the old side). Settled in `## Decisions` once confirmed together with (1354).

(1354) [open] [policy] What `config.tune`, `config.unset`, and `config.get` do for a removed key from an older caller (old skill or playbook text, Claude mod, Pi extension).
- Context: today `config.tune` refuses a key it does not know (`internal/mcp/server_dispatch.go:422`), while `config.get` reports an unknown key as unset (`internal/mcp/config_manifest.go:507,566-569`); so plain removal turns an older caller's tune into an error, and a removed call or argument without an alias is T3 (`release-compatibility.md` Tools and Protocol Mapping). Precedent: `261113-refactor-ws-dashboard-drop-lan-serving` accepts and warns for one minor, then deletes.
- Alternative a: for one minor, accept the key with a deprecation warning (tune writes nothing; get returns the code default), then refuse it from the next minor with owner approval.
- Alternative b: refuse immediately on removal.
> a: keeps the release patch-safe and matches the LAN-serving precedent; b is a T3 boundary for every removed key.

(1355) [open] [policy] Outcome vocabulary for the per-key confirmation.
- Context: three candidates are repo-level constructs rather than unused knobs (`worktree.profile.<name>`, `git.merge_size_*`, `ticket-assignee-aware`); the triage reason is that they belong in the committed repo config, not that they are unused. `worktree.profile.<name>` carries live worktree-pool behavior and hooks.
- Alternative a: each key gets one of remove / restrict to repo scope / keep, chosen by the owner.
- Alternative b: remove or keep only; restriction becomes a separate ticket.
> a: the triage reason for those three is scope, not disuse; removal would delete working repo configuration.

(1356) [open] [technical] Dead constants `sage_review_design_tier` and `sage_review_completeness_tier` (registered in `wsconfig/scope.go`, never read or written by the triage's account) are deleted outright after Phase 1 re-confirms there is no reader, since no code default exists to keep.

(1357) [open] [technical] Phase 1 re-verification records per key: every reader on current `develop` (Go, Claude mod, Pi extension, playbook/rsrc splices, tests), the fallback each reader takes on an unset value, every doc or skill line that tells a user to tune the key, and whether its removal touches a manifest copy; the owner receives that table.

(1358) [open] [technical] Removed keys still present in a loaded config file produce one warning in `config.list` output naming the key and file, so a user whose tune stopped applying can see why; no rewrite of the file.

(1359) [open] [technical] The internal splice points `prompt.TicketHandlerPersistConfirmation.*`, `prompt.TicketHandlerReviewAutoAck.*`, and `prompt.TicketQueueWhiteboard.*` leave the user-facing `prompt.<pointId>` override surface but keep their playbook defaults, which remain driven by `ticket_review_auto_ack` and `workflow.whiteboard_enabled`.

## Phases

### Phase 1: Re-verify the 28 candidates and obtain owner confirmation

No code change.

- Re-verify every candidate in `## Unconfirmed Candidates` against current
  `develop` per (1357).
- Post the per-key list with a recommended outcome per key ((1355)) to the
  owner, and record the confirmed list in `## Decisions`.
- Verification: every candidate has a recorded reader set and an owner
  outcome; keys the owner did not confirm stay unchanged.

### Phase 2: Remove confirmed keys from the user-facing surface

Depends on Phase 1 and on (1353)/(1354) being settled.

- For each confirmed key: take it off the registry/catalog, `config.tune` and
  `config.list`, every manifest copy, and user-facing docs and skill text;
  keep the code default (Decision 1); apply the older-caller behavior from
  (1354).
- Verification: a fixture config file carrying every removed key loads and
  resolves without error and with the code defaults in effect; manifest parity
  tests pass for ws and wsflow; older-caller `config.tune`/`config.get` of a
  removed key behaves per (1354); remaining keys resolve unchanged.
