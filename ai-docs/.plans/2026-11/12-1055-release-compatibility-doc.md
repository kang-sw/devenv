# Plan: release-compatibility rule in AGENTS.md + new manual

## Outcome

Make the mixed-version deploy compatibility rule a discussion-time obligation in this
repository, not a release-time discovery. The user asked for the rule to be written
"strongly" into AGENTS.md so that, while a change is being discussed or ticketed, the
lead checks whether it interferes with a cross-version surface.

Documentation only. No code, no shipped-surface text (this is this repository's own
release posture: Architecture Rules 4 and 5 keep it out of shipped playbooks).

## Edits

1. **New manual `ai-docs/manuals/release-compatibility.md`** with a one-line
   `summary:` frontmatter like its siblings. Contents:
   - How versions meet today (facts below), the scenario sheet, the T0/T1/T2 tiers,
     the release rules, and the enforcement state.
2. **AGENTS.md `## Workflow`: new `### Compatibility Tiers` subsection** placed right
   after `### Approval Protocol`. Use this text, adjusting wording only for flow:

   ```text
   ### Compatibility Tiers

   A change that touches a cross-version surface - an MCP tool or argument,
   the `/mcp` hello, a shared store's on-disk shape, a tracker event type, or a
   config/rsrc manifest field - states its compatibility tier when it is
   discussed, and the ticket records it in `## Decisions`. It is never first
   discovered at release.

   - T0 transparent: additive only (new tool, argument, optional field;
     playbook or dashboard asset content).
   - T1 reconnect-recoverable: a rename that keeps the old name as an alias for
     one minor; a tool list that refreshes on reconnect.
   - T2 breaking: a removal without an alias, a non-additive store change, a
     protocol or tracker event change. T2 lands only in a minor bump, with the
     store's `schema_version` or the protocol number raised.

   Scenario sheet and enforcement: `ai-docs/manuals/release-compatibility.md`.
   ```
3. **AGENTS.md `### Approval Protocol` "Ask first"**: add "a T2 compatibility change
   (see `### Compatibility Tiers`)" to the list.
4. **AGENTS.md `### Implementation Conventions` table**: add a row
   `| agents-plugin-tool/internal/ | ai-docs/manuals/release-compatibility.md |`.
   Rows in this table are copied into tickets' `## Constraints` by fact population,
   which is what carries the rule to ticket time.
5. Add the manual to the AGENTS.md `## Documentation System` list.

Keep AGENTS.md additions short; the manual carries detail. Avoid defensive/
over-negated prose (see the `ws:lead-audit-doc` posture: say what to do).

## Facts for the manual (verified by code exploration 2026-11-12)

- One daemon per machine; within a minor the newest version wins (`ensure.go`
  keepRunning, `version.go` CompareVersions). An older client never replaces a
  newer daemon. Equal version strings fall back to binary mtime.
- Hosted MCP tools run in the daemon binary, but playbook/skill text comes from the
  connecting plugin's own directory (`WS_RSRC_ROOT` required). The typical in-minor
  skew is therefore old text running against a new binary. Playbook content always
  matches its own package's skills.
- On a minor mismatch the launcher silently (one stderr line) bypasses the daemon
  and runs its own binary per session, while the shared stores stay shared. This is
  the real risk.
- There is effectively no version negotiation: the `/mcp` hello requires
  `protocol: 1` exactly; hello's plugin version is used only for the minor check
  and is not recorded per session; runtime.json per-tool ranges are never
  evaluated; no `tools/list_changed` is sent. (Comment at `version.go:52-54` states
  the wire surface changes only in a minor, but nothing checks it.)
- Stores mostly read tolerantly, but Go struct round-trips can drop fields an older
  writer does not know (suspected; no per-writer audit yet). Only the config manifest
  and rsrc manifest check `schema_version` on read. sqlite has no guard against an
  older binary opening a newer schema. The tracker flags unknown event types.
- Existing mechanisms: intake `accepts:` negotiation for optional event kinds; the
  holder v / v-1 window; Pi's version pin (daemon `serverInfo` >= bundled version,
  same minor) and one `tools/list` at startup; the Claude mod reconnect watchdog,
  which fires only when tools disappear.

### Scenario sheet

| # | Situation | What happens | Tier |
|---|---|---|---|
| 1 | Pi updated a patch, Claude still on the older patch (same minor) | The daemon is replaced by Pi's newer binary; Claude sessions reconnect and run old skill text on the new binary. Fine if tools/args were only added; a removed or renamed arg fails loudly as unknown | added: T0; removed: T2 (today possible even in a patch) |
| 2 | Claude updated, Pi older (same minor) | Symmetric to 1; Pi's pin (daemon >= bundled) passes | same as 1 |
| 3 | Another harness replaces the daemon mid-session | In-flight calls get `-32000`, launcher reconnects; the tool list is not refreshed, so new tools stay invisible and removed tools fail on call | T1 |
| 4 | Minor mismatch (Pi 0.49 / Claude 0.48) | Claude runs the 0.48 binary directly while the daemon is 0.49; both write the same stores (session parts, mailbox, inbox, sqlite, tracker): new fields lost, old binary opens new DB, tracker rejects events; nobody is told | T2 |
| 5 | Same version string, different build (release vs dev) | mtime decides; a newer file wins even with older code; total order, no flapping, but a wasted replace attempt per session start | T0 (cost only) |
| 6 | Local dev: source newer than binary | Asset-only changes not picked up (idea `261112-bug-ws-launcher-fingerprint-skips-embedded-assets`) | dev-only bug |
| 7 | Tracker ledger synced across machines on different versions | The older side flags new event types as errors | T2 (cross-machine; daemon rules cannot prevent it) |

### Tiers (detail for the manual)

- T0 transparent: playbook/skill content changes; dashboard assets; adding tools,
  arguments, optional fields; adding optional store fields provided read-modify-write
  preserves unknown fields.
- T1 reconnect-recoverable: tool list refresh after a daemon replacement; tool/arg
  renames only when the old name stays as an alias for one minor.
- T2 breaking: non-additive store changes (field removal or meaning change,
  destructive sqlite migration); the hello protocol; new tracker event types; strict
  config manifest fields; tool/arg removal without an alias.

### Confirmed release rules (user decisions 2026-11-12)

1. A patch release carries only T0/T1. Within a minor, tools and arguments are never
   removed; renames keep an alias. Enforced by a tool-surface monotonicity test
   against the previous release's runtime.json surface.
2. T2 lands only in a minor bump, raising the affected store's `schema_version` or
   the protocol number. A writer that sees a newer `schema_version` than its own
   refuses to write (fails loud) instead of silently overwriting.
3. The daemon judges skew: it records each session's hello version; on a minor
   mismatch it injects a strong notice into that session's tool results / workflow
   manual ("plugin compatibility is broken; tell the user to update") plus a
   dashboard badge, and the session keeps working. A patch difference shows only a
   quiet dashboard badge. A session that fell back to direct execution makes the
   same judgment from the daemon's `/health` version.
4. After a daemon replacement the tool list is refreshed: `tools/list_changed`
   first; where a host ignores it, a forced reconnect (Claude mod watchdog widened
   to version/content-hash changes; Pi re-fetches `tools/list` on reconnect).

Rules 1-4's mechanisms (monotonicity test, newer-schema write refusal, daemon skew
notice, tool-list refresh) are confirmed but not implemented yet; the manual states
them as the rule and marks the enforcement as pending, in one short section, so the
pending note is easy to remove later.

## Verification

- `git diff` reads cleanly; AGENTS.md section order intact; the manual has
  `summary:` frontmatter.
- `rg -n 'release-compatibility' AGENTS.md` shows the subsection pointer, the
  Implementation Conventions row, and the Documentation System entry.
- No Go build/test needed (docs only). If any test asserts AGENTS.md or manual
  inventory, run it via `~/.local/bin/ws-rgo` only (never local `go test`).

## Boundary

Commit on this branch (`delegate/release-compat-doc`, worktree /home/swkang/.ws-worktrees/devenv/aim-hydration-granite) with the plan
file, one commit, repository commit rules (`## AI Context`). Do not merge or push.
Return to the lead if a rule above contradicts existing AGENTS.md text.
