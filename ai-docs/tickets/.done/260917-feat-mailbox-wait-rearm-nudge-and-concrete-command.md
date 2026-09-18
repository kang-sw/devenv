---
title: Make the mailbox wait command actionable at read time and self-reminding at fire time
related:
  260917-feat-ws-pi-mailbox-waiter-slug-wake: related
  260913-research-cross-session-mailbox: context
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6bd7359711258363
sage-review-completeness-reviewed: 6bd7359711258363
completed: 2026-09-18
---

# Make the mailbox wait command actionable at read time and self-reminding at fire time

## Background

A lead who wants to sit idle on cross-session mail arms `ws-mcp mailbox wait` as
a background task. Two gaps make this hard to actually follow through on a
harness where the lead runs the CLI itself (Claude, Codex):

1. **No concrete command at read time.** `lead-use-mailbox` tells the lead to
   launch `ws-mcp mailbox wait --session-key <your key> [--slug <your address>]
   ...` but gives no way to obtain the CLI/launcher path or fill in the actual
   session key and registered address. In this project the binary is not a bare
   `ws-mcp` on PATH — it is reached through a launcher — so the abstract command
   is not directly runnable. The skill cannot hardcode a package-internal
   launcher path (shipped surfaces are host-neutral, downstream-first), so the
   actionable command must be produced at render time from runtime context, not
   written into the template.

2. **No re-arm reminder at fire time.** The skill is read once, but a wait fires
   (or times out) much later; by then the lead may have forgotten that a single
   `wait` covers one wake and must be re-armed to keep listening. The reminder
   needs to arrive in-band when the wait actually returns, not only at the
   original read.

Evidence: `lead-use-mailbox`'s SKILL.md calls `ws/playbook.read(name:
"lead-use-mailbox")` with **no** `session_key` (contrast lead-discuss, which
passes `session_key: <your key, omit if fresh>`), so the render presently cannot
fill a concrete `--session-key` command. The mailbox wait CLI lives in
`agents-plugin-tool/cmd/ws-mcp/mailbox.go`.

## Decisions

- **Render-time injection, host-neutral template.** The concrete wait command is
  produced by the playbook render (which runs in the MCP server process and
  therefore knows the launch/CLI context), filling a new template variable — a
  new key in the render's namespace-variable map (`resolveNamespaceVars` /
  `buildPlaybookVars` in `agents-plugin-tool/internal/mcp/playbook_tools.go`,
  alongside the existing `McpNamespace`/`SkillNamespace` keys). (This is the
  playbook variable-substitution path; it is a different mechanism from the
  route resolvers' binding-anchor injection, which does not run in the playbook
  render.) The playbook body stays generic; no package-internal path is written
  into shipped text.
- **session_key: pass it, soft-degrade without it.** SKILL.md is changed to pass
  `session_key: <your key, omit if fresh>` like the house pattern. With a key the
  render emits the concrete command; without one (fresh session) it emits today's
  generic guidance. Rejected: hard-requiring a key — the register/find/send prose
  is useful before a key exists, and forcing a bootstrap just to read guidance is
  heavier than the value.
- **AUTO-safe slug source (shared with `260917-feat-ws-pi-mailbox-waiter-slug-wake`).**
  When filling `--slug`, resolve the caller's **registered** self address (the
  same source `mailbox.lookup_peers` self.address reads), never by re-reading
  `WS_MAILBOX`/`WS_MAILBOX_AUTO` from the environment. Reading `WS_MAILBOX`
  directly is correct only for an explicit `name@scope`; under `WS_MAILBOX_AUTO`
  the env holds no minted stem and re-derivation produces a different address
  than the server registered. No registered address → emit the reply-id-only
  command (`--session-key` alone).
- **Re-arm nudge is a CLI-output concern, not a playbook concern.** The
  `ws-mcp mailbox wait` CLI prints, on return, the raw command to re-arm plus a
  short nudge string, so the reminder is delivered at fire time regardless of
  when the skill was read. The CLI reprints its own invocation (its resolved
  binary + the flags it received), which is naturally host-neutral and needs no
  package path.

## Constraints

- **Shipped-surface, triple-mirrored.** `lead-use-mailbox`'s playbook body
  (`agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md`) is byte-identical
  across the ws, wsflow, and pi packages (confirmed: no diff against
  `agents-plugin-wsflow/rsrc/lead-use-mailbox/lead-use-mailbox.md` or
  `agents-plugin-pi/rsrc/lead-use-mailbox/lead-use-mailbox.md`). Corrected: its
  SKILL.md is **not** byte-identical across all three — only ws and pi match
  byte-for-byte; wsflow's `SKILL.md` differs by deliberate namespace
  substitution per `wsflow-mirroring.md` ("For skills, byte-identical mirroring
  is forbidden"): `agents-plugin/skills/lead-use-mailbox/SKILL.md#L8-L10` reads
  `ws/playbook.read(...)` / `/ws:mcp-server-repair`, while
  `agents-plugin-wsflow/skills/lead-use-mailbox/SKILL.md#L8-L9` reads
  `wsflow/playbook.read(...)` / `/wsflow:mcp-server-repair`;
  `agents-plugin-pi/skills/lead-use-mailbox/SKILL.md` is byte-identical to the ws
  copy. Editing them triggers `ai-docs/manuals/skill-authoring.md` (invariant
  checklist) and `ai-docs/manuals/wsflow-mirroring.md` (mirror resync). Keep body
  edits minimal; prefer a render variable over new prose.
- **Host-neutral / downstream-first.** No package-internal launcher path in
  shipped text; the actionable command comes from render-time runtime context or
  the CLI's own argv (Architecture Rule 4).
- Phase 1 (CLI output) is Go-only and does not touch the mirrored skill surface.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/cmd/ws-mcp/mailbox.go, agents-plugin-tool/internal/wsmailbox/wait.go, agents-plugin-tool/internal/mcp/playbook_tools.go, agents-plugin/skills/lead-use-mailbox/SKILL.md, agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md, and their agents-plugin-wsflow and agents-plugin-pi mirrors |
| scope.surface | public-interface | mailbox wait CLI `--format json` output gains a new field (agents-plugin-tool/cmd/ws-mcp/mailbox.go#L137-L178) and the shipped lead-use-mailbox SKILL.md/playbook text changes (agents-plugin/skills/lead-use-mailbox/SKILL.md, agents-plugin/rsrc/lead-use-mailbox/lead-use-mailbox.md) |
| scope.new_public_symbol | no | the playbook render context is a map[string]string (agents-plugin-tool/internal/mcp/playbook_tools.go#L178-L179, e.g. "McpNamespace"/"SkillNamespace" keys); a new template variable is a new map key, not a new exported Go symbol |
| scope.new_type_contract | yes | the `mailbox wait --format json` output gains a re-arm field, extending the existing declared JSON contract (agents-plugin-tool/cmd/ws-mcp/mailbox.go#L137-L178, emitMailboxWaitResult) |
| scope.test_surface | existing | agents-plugin-tool/cmd/ws-mcp/mailbox_test.go, agents-plugin-tool/internal/wsmailbox/wait_test.go, agents-plugin-wsflow/tests/test_wsflow_skill_bundle.py, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go |
| complexity.reuse_points | confirmed | existing playbook render context construction (agents-plugin-tool/internal/mcp/playbook_tools.go), existing mailbox.lookup_peers self.address resolution (agents-plugin-tool/internal/mcp/mailbox_tools.go#L232-L320), and the existing McpNamespace/SkillNamespace namespace-variable keys as the pattern for a new render variable (agents-plugin-tool/internal/mcp/playbook_tools.go#L176-L180) |
| complexity.side_effect_risk | low | both phases are additive (a new CLI output field/line, a new template variable with a documented degrade-to-generic path); no change to wait/peek semantics, storage, or existing fields |
| risk.correctness | moderate | Phase 2 must resolve the right CLI/launcher invocation per harness and the right AUTO-safe slug across four render scenarios (explicit WS_MAILBOX, WS_MAILBOX_AUTO, env-less, no session key); a wrong resolution emits a broken or misleading command |
| risk.fit | low | follows an established pattern (a new render namespace-variable key mirroring the existing McpNamespace/SkillNamespace map; session_key threading mirroring lead-discuss's `session_key: <your key, omit if fresh>`, agents-plugin/skills/lead-discuss/SKILL.md) |
| risk.test | moderate | Phase 2 verification enumerates four render-context scenarios plus wsflow/pi resync and skill-shim drift tests; Phase 1 extends existing CLI tests for two exit paths (mail, timeout) and the JSON shape |
| risk.security_or_contract | low | no change to the mailbox Envelope, owner-gate, or session_key auth model; the CLI already requires `--session-key`, and the JSON field addition is additive |

## Phases

### Phase 1: Re-arm nudge in the `mailbox wait` CLI output

Behavior: when `ws-mcp mailbox wait` returns (mail found or timeout), append to
its output the raw command to re-arm the wait — its own resolved binary plus the
`--session-key`/`--slug`/`--timeout` flags it was invoked with — and a short
plain-language nudge that one wait covers a single wake and must be re-armed to
keep listening. Applies to the human/text output path; the `--format json` path
carries the same via a new `rearm` field (holding the re-arm command and the
nudge string; exact key name is the implementer's to confirm) rather than prose
so machine callers are unaffected. No change to wait/peek semantics.

Verification: extend the mailbox wait CLI tests in
`agents-plugin-tool/cmd/ws-mcp` (and/or `internal/wsmailbox`) to assert the
re-arm command and nudge appear on the mail and timeout exits, and that the JSON
path stays structurally stable. Independent of Phase 2.

### Result (03e09b3) - 2026-09-18

Landed in `agents-plugin-tool/cmd/ws-mcp/mailbox.go` (+ tests in
`mailbox_test.go`). `mailboxWait` now builds a re-arm command via
`buildMailboxWaitRearmCommand(os.Args[0], sessionKey, slug, timeout)` — the
resolved binary (whatever argv[0] the harness/launcher invoked) plus only the
resolved wait-scoping flags: `--session-key` always, `--slug`/`--timeout` when
set. This is host-neutral by construction (no package-internal launcher path
enters the output, per shipped-surface-boundary.md), satisfying the "CLI reprints
its own invocation" decision without a Phase 2 render dependency.

`emitMailboxWaitResult` gained a `rearmCmd` parameter and now emits the reminder
on **both** return paths (mail found and timeout):
- Text path: a `re-arm: <command>` line plus the `mailboxRearmNudge` string
  ("one mailbox wait covers a single wake; re-run the re-arm command to keep
  listening."), printed to stdout after the result so it rides the same stream
  the harness re-injects into the woken agent.
- JSON path (`--format json`): an additive nested `rearm` object
  (`{"command","nudge"}`). Existing fields (`timed_out`/`unread`/`named`/`reply`)
  and the nil-slice→`[]` normalization are untouched, so machine callers see one
  new field, not a changed shape.

Verification (read in full):
- `go build ./...` and `go vet ./cmd/ws-mcp/`: clean.
- `go test ./cmd/ws-mcp/`: ok (full package). New tests
  `TestMailboxWaitRearmReminderOnMailExit`,
  `TestMailboxWaitRearmReminderOnTimeoutExit`,
  `TestMailboxWaitJSONCarriesRearmField`,
  `TestMailboxWaitJSONTimeoutCarriesRearmField` cover all four
  {text,JSON}×{mail,timeout} combinations and assert existing-field stability.

Review: partitioned correctness + test, round 1, no Critical/Major findings.
Minor items were taken as cheap accuracy fixes (comment mislabel, nudge wording,
tightened assertions). Two Minor items deliberately deferred: no shell-quoting on
the re-arm command (inputs are validated slug-form/`name@scope`, space-free), and
the `--timeout 0` omission branch is not CLI-testable without hanging the test.

Decision recorded: JSON `rearm` is a nested `{command,nudge}` object (not two flat
fields), keeping the machine contract additive under a single key.

### Phase 2: Concrete wait command in the `lead-use-mailbox` render + session_key wiring

Behavior: SKILL.md passes `session_key: <your key, omit if fresh>` to
`ws/playbook.read`. The render fills a new template variable (e.g.
`MailboxWaitCommand`, a new key in the `resolveNamespaceVars`/`buildPlaybookVars`
namespace-variable map alongside `McpNamespace`/`SkillNamespace`; exact name is
the implementer's to confirm) with the concrete, runnable `mailbox wait`
invocation: the resolved CLI/launcher form for the current harness,
`--session-key <key>`, and `--slug <registered self address>`
when one resolves (AUTO-safe per Decisions). Without a session key, or without a
registered address, or on a harness that cannot expose the CLI, degrade to
today's generic guidance rather than emitting a broken command. Keep the
mirrored playbook body edits minimal — the actionable value rides the render
variable, not new prose.

Verification: render `lead-use-mailbox` with a key + explicit `WS_MAILBOX`, with
a key + `WS_MAILBOX_AUTO` (assert the emitted slug matches the registered
address, not the env-derived one), with a key + env-less (reply-id-only command),
and with no key (generic degrade). Re-sync the wsflow/pi mirrors and satisfy the
skill-shim drift tests.

Deferred: no change to the mailbox Envelope/contract, discovery, or reply-id
lifetime; the pi adapter's own auto-arm slug-wake is tracked separately in
`260917-feat-ws-pi-mailbox-waiter-slug-wake`.

### Result (d155958) - 2026-09-18

Landed. `lead-use-mailbox`'s SKILL.md (ws + pi byte-identical; wsflow hand-edited
per its curated-shim rule) now passes `session_key: <your key, omit if fresh>`
to `playbook.read`, and the shared playbook body swaps the literal
`ws-mcp mailbox wait ...` command for `{{.MailboxWaitCommand}}` (pure token swap,
no new prose).

`MailboxWaitCommand` is a new reserved implicit render variable
(`wsrsrc.ImplicitVariableNames`), resolved by `mailboxWaitCommandVar`
(`agents-plugin-tool/internal/mcp/playbook_tools.go`) to a concrete
`<os.Args[0]> mailbox wait --session-key <key> [--slug <registered address>]`:
- `os.Args[0]` is the same host-neutral binary the harness/launcher invoked
  (mirroring Phase 1's re-arm command); no package-internal launcher path enters
  shipped text (shipped-surface-boundary.md).
- `--slug` is the owner-gated registered self address (`s.mailboxOwnerCheck`), the
  same AUTO-safe source `mailbox.lookup_peers` self.address and the workflow_manual
  ambient block read, never an env re-derivation of WS_MAILBOX/WS_MAILBOX_AUTO.
- Degrades to today's generic guidance (a byte-for-byte reproduction of the prior
  abstract command, modulo the old two-line wrap) when the key is absent/
  unresolvable or no address is registered (env-less, or a non-owner session).

Decision (structural deviation, adapted — recorded): the ticket named
`resolveNamespaceVars`/`buildPlaybookVars` as the fill site, but that no-arg
namespace resolver has no session context. Threading `session_key` + `*Server`
through `printPlaybook`/`renderPlaybook`/`renderPlaybookBody` would churn ~130
call sites (surgical-change violation). Instead the value is resolved in the
`playbook.read` and `playbook.render` dispatches (which already hold session_key
+ `*Server`) via `s.injectMailboxWaitCommand`, gated on the one stem that
substitutes it (`mailboxWaitPlaybookName`, mirroring the existing
`workflowManualPlaybookName` special-case). Same variable-substitution path, same
"tool-injected wins over caller context" anti-spoof property, identical behavior.

Decision (risk-accepted, recorded): `os.Args[0]` is emitted unquoted, matching
Phase 1's `buildMailboxWaitRearmCommand`; keeping both command forms identical
outweighs speculative quoting for a space-bearing binary path.

Verification (read in full):
- `go build ./...`, `go vet ./internal/mcp/`: clean.
- `go test ./...` (agents-plugin-tool): ok. New
  `TestMailboxWaitCommandRenderVariable` covers all four ticket scenarios
  end-to-end through `playbook.read` substitution (explicit WS_MAILBOX,
  WS_MAILBOX_AUTO asserting the registered-not-env-derived slug, env-less
  reply-id-only, no-key generic degrade), plus a non-owner delegate isOwner-gate
  case, an anti-spoof case, and a `playbook.render` regression case.
- `python3 -m unittest discover agents-plugin-wsflow/tests`: 12 ok. The
  single-call-shim guard now requires the `session_key` clause for
  lead-use-mailbox and forbids it on the other nine (per-skill, not an optional
  group), so it fails on both a revert and a leak.
- Mirrors resynced: ws rsrc manifest, wsflow rsrc mirror (byte-identical), pi
  rsrc (byte-identical), skills manifest; all drift guards green.

Review: partitioned correctness + test, two rounds. Round 1 raised one Important
per partition (playbook.render ErrUnprovidedVar; a wsflow guard that pinned
nothing) plus test-coverage gaps; all fixed in d155958. Round 2 confirmed every
finding fixed with no regression and no new Critical. The unquoted-`os.Args[0]`
Minor was risk-accepted as above.
