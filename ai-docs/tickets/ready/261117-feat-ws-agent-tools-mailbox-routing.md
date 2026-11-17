---
title: "ws/agent.* delegation tools with unified mailbox routing, relation from lineage, and native subagent removal in hosted sessions (leads and workers)"
parent: 261114-epic-ws-common-session-foundation
blocked-by: 261117-feat-ws-harness-capability-catalog#phase1
related:
  261117-feat-ws-harness-capability-catalog: "steer-capability lookup for parent-to-child delivery reads the catalog's mid-turn steer item; Phase 3 here updates the tool-removal grade it wires and the init check subtracts requested removals (Decision 6)"
  261116-research-ws-hosted-session-first-tier: "evidence and Confirmed Decisions"
  261115-feat-ws-worker-profile-session-spawn: "worker-profile caps apply to agent.spawn; its owner-created worker sessions keep native explore/general-purpose subagents (Decision 6 scope)"
  261115-feat-ws-whiteboard-agent-card: "card sessions have a key parent but no ParentRef; never owned, held or cascaded here (Decision 18)"
  261115-feat-ws-session-hire-approval: "hire stays a separate peer-creation path; lineage authority was rejected there"
  261115-feat-ws-permission-profile: "common permission intent; its Phase 2 admission machinery (Decision 3: recomputed on every launch, including wake-on-mail revives) is a prerequisite of Decision 13's implementation"
  261117-feat-ws-mailbox-memory-first-store: "prerequisite of daemon-owned routing (Phase 2): daemon-owned mail persists through its restore ring with the same history and reply-id semantics (its Decision 6)"
  261117-research-ws-daemon-file-touch-map: "release-gate file-touch constraint; not edited from here"
  261112-epic-ws-mcp-pi-subagents: "done; pi.* is the prior art"
  261115-feat-ws-dashboard-session-four-state-followups: "the fifth state's dashboard display"
  261117-feat-ws-hosted-prompt-assembly: "delivers the playbooks' Never-use {{NativeToolScheme}} steer that complements native subagent removal (Decision 6)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 10051ec5d388ebe3
sage-review-completeness-reviewed: 10051ec5d388ebe3
---

# ws/agent.* tools with unified mailbox routing

## Background

Delegation is always `ws/agent.*` regardless of harness (owner (06)): hosted sessions (leads and workers; (29)=ok extended native subagent removal to workers) no
longer use the harness's own spawn tool, so subagent tracking comes from the
daemon. Today `pi.*` is the daemon-owned spawn path (children, depth cap
`MaxSpawnDepth = 2` in `agents-plugin-tool/internal/wsagent/children.go`), and
hosted Claude leads are launched without a tool whitelist, so the native Agent
tool is available (hosted Claude workers are launched with a profile-derived
`--tools` whitelist, `agents-plugin-tool/internal/wsagent/claude.go#L166-L167`;
the dashboard-internal `tracker-support-claude` profile allows `Agent` alone,
`agents-plugin-tool/internal/wsagent/builtin.go#L34-L38`, asserted at
`agents-plugin-tool/internal/wsagent/claude_test.go#L122-L135`). This ticket introduces the final `agent.*` surface and
unifies parent/child/peer communication on a single `mailbox.send`.

The surface below supersedes an earlier draft that had `agent.transcript` and
`agent.wait`.

## Decisions

`(NN)` are owner board IDs from the 2026-11-17 lead-discuss session (see the
parent epic's Cross-Child Decisions); bare three- and four-digit IDs are from
earlier captures.

1. **Surface (06)=a, (09)=a (delegation is always `ws/agent.*`, with this
   tool family).**
   - `agent.spawn(prompt, tier, system_prompt_path? | system_prompt?,
     authority, cwd? | worktree?, label?) -> {address, status}`. `tier`
     follows Decision 11, `authority` (required) Decision 13, the system prompt source
     Decision 12; `label` is display-only (Decision 15); `address` is the
     child's mailbox address and the control key (Decision 15). At most one
     of `cwd` / `worktree`; the `worktree?` value form and acquisition, and
     the `status` value set, are settled in the Phase 1 design record.
   - `agent.interrupt(address)` interrupts the child's current turn; the
     session stays ((55)=a). Its return shape is settled in the Phase 1
     design record.
   - `agent.list()` returns the caller's direct children only (Decision 15),
     each with its address, label, tier, authority, state (including
     Decision 16's child agent waiting state) and the last report summary (a
     superset of `pi.list`). Exact field names are settled in the Phase 1
     design record.
   - `agent.tiers()` (new) returns `{native, ext, aliases}` with each
     entry's resolution (Decision 11).
   - The agent surface is spawn, interrupt, list, tiers. No `agent.stop`
     ((60)=a): a runaway child is interrupted and then sleeps by TTL
     (Decision 14); permanent close is by a human in the dashboard,
     cascading deepest-first. The one automatic exception is Decision 16:
     a child agent waiting for 15 days without activity is deleted with its
     descendants. Rewind is human-only (dashboard), not an agent
     tool ((55)=a).
   - No `wait` and no `transcript`: completion arrives by push (a system mail
     starts a new turn); humans read transcripts in the dashboard.
   - No `agent.send`: a single `mailbox.send(to: <address>)` serves children,
     peers and other leads.
   - Rejected: (55)b a caller-chosen `delivery?: steer|next_turn` on
     `mailbox.send`; (55)c no interrupt tool; (60)b keeping `agent.stop` as
     "sleep now".
2. **In-memory routing for daemon-owned addresses (12).** The daemon Runner
   routes daemon-owned agent addresses in memory, not through mailbox files.
   Daemon-owned mail (including mail held for a sleeping child up to Decision
   16's 15 days) persists through `261117-feat-ws-mailbox-memory-first-store`'s
   restore ring (one segmented JSONL ring per daemon, that ticket's Decision
   1), with the same history and reply-id semantics as external mail (that
   ticket's Decision 6, batch-review coherence C1); that store lands before
   Phase 2's routing. Rejected: a separate agent-tools mail store with its own
   restore files and history behavior. External sessions and CLI leads keep
   using the same daemon mailbox store by address; once that store lands, no
   sender's mail goes through `mailbox.json`. Daemon-owned addresses differ
   only in that the Runner routes them in memory to the agent it owns.
   Reason: the release gate on daemon file touches.
3. **Relation is computed, not stored (11).** Parent/child/peer is derived from
   lineage (Decision 18) at delivery. Its only uses ((73)=a, owner 22:27) are
   the receiver-visible mail label (whether the sender is its parent, child or
   peer) and the `agent.list` display; it never drives delivery timing
   (Decision 4). Rejected: a stored Envelope relation field, because of
   whole-file loss of a new Envelope field on an old binary (11); dropping
   relation computation (would partly reverse (11)).
4. **Delivery timing ((61)=a, owner 19:25 "mailbox keeps steer only").** Every
   `mailbox.send` delivery, for every relation (parent to child, child to
   parent, peer), is a steer when the target supports mid-turn steer, else
   next turn. Support follows the catalog's runtime rule (catalog Decisions 9
   and 13): for a harness with a catalog profile, the steer grade is not
   `unsupported` AND the instance has a steer handle (`controlsFor`); for a
   harness without a profile (Pi until its port), the handle alone decides. A
   Claude worker is graded `unsupported` for steer today, so mail to it is
   next turn (fact population 2026-11-17, auto-confirmed: direct consequence
   of the catalog's confirmed Decisions 9 and 13). The child's final report
   stays the automatic system mail of Decision 5. Rejected: per-relation
   timing with peers at next turn ((61)b), and a caller-chosen delivery option
   ((55)b).
5. **Report-hold rule kept.** A child's final report is an automatic system mail
   to its parent, held while the child's own children are awake.
6. **Native subagent removal in hosted leads and `ws/agent.spawn` workers
   (06)=a, (11), (29)=ok.** Hosted Claude leads and `ws/agent.spawn` Claude
   workers launch with `--disallowedTools Agent` and `Task` (init reports
   Agent as `Task`); the removal extends to hosted workers (29)=ok, a
   consequence of (06) "delegation is always `ws/agent.*` regardless of
   harness". The existing tests at `claude_lead_plugins_test.go:90` and
   `lead_claude_test.go:1182` carry banned-flag lists (`--tools`, `--allowedTools`,
   `--setting-sources`, and others) that do not name `--disallowedTools`
   (`agents-plugin-tool/internal/wsagent/claude_lead_plugins_test.go#L90`,
   `agents-plugin-tool/internal/wsagent/lead_claude_test.go#L1182`; a `disallowed` search
   under `agents-plugin-tool/internal` finds no launch-args source or test hit), so
   they do not assert the flag's absence and adding it does not fail them. The
   complementary measure is the playbooks' per-harness "Never use
   {{NativeToolScheme}}" steer (owner addition with (29)), delivered by
   `261117-feat-ws-hosted-prompt-assembly`, so harnesses whose native subagent
   tools cannot be removed (Codex/Antigravity removability is unprobed, (08)=c:
   decided per harness at stage 3) are still steered to `ws/agent.*`. Scope:
   the removal applies to hosted leads and to `ws/agent.spawn` children only;
   internal and dashboard profiles that need the native tool keep it (for
   example `tracker-support-claude`, whose only tool is `Agent`, `builtin.go`
   `SupportClaudeTools`), and owner-created worker-profile sessions are out of
   scope too: they keep native `explore` + `general-purpose` subagents
   (`261115-feat-ws-worker-profile-session-spawn` Decisions 1 and 4).
   Rejected: removing it from every hosted Claude launch, which would leave
   that profile with no tool (fact population 2026-11-17, auto-confirmed
   technical; consistent with Decision 13 keeping those profiles internal).
   Catalog interaction (batch review, auto-confirmed technical): the catalog's
   startup init-shape check (`261117-feat-ws-harness-capability-catalog`
   Decision 4, implemented in its Phase 3) demotes default tool removal and
   native subagent removability when an init `tools` name is missing, so a
   launch that removed `Agent`/`Task` on purpose would read as a mismatch. The
   expected init tool set therefore subtracts the removals the launch
   requested. The catalog grades default tool removal for leads `unsupported`
   today (catalog Decision 10: no ws wiring yet); Phase 3 here updates the
   grade it wires.
7. **Spawn tree bounds ((72), owner 22:27, "only as deep as the new workflow
   needs"; (77)=ok, (78)=ok, (79)=ok, 2026-11-17).**
   - No concurrent-agent cap.
   - Depth cap 4 (77). Depth counts as today: a lead is depth 0, and a spawn
     whose child would exceed depth 4 fails with an error. `MaxSpawnDepth`
     is kept and only its value moves from 2 to 4
     (`agents-plugin-tool/internal/wsagent/children.go:65-67`, enforced at
     the `d+1 > MaxSpawnDepth` check in the same file; its error text, which
     spells out today's depth-2 chain, follows the new value).
   - Per-agent width cap 99 (effectively unlimited) (78). It counts the
     caller's awake direct children only; a spawn at the cap fails with an
     error naming the cap; mail that wakes a sleeping child ignores the cap.
   - Both values are code constants, not config keys (79): no new config
     manifest field, so T0.
   - `MaxSpawnDepth` is shared with the still-callable `pi.spawn` path
     (`children.go:280-281`), so `pi.spawn`'s depth follows the new value and
     its error text ("lead -> Pi worker -> Pi subagent") is updated with it;
     the width cap applies to `agent.spawn` only, since `pi.*` is hidden and
     its handlers are deleted next minor (Decision 10) (design review
     2026-11-17, auto-confirmed technical).
   Rejected: no depth cap ((10)=c, reversed by (72)); a per-root-tree
   concurrent-agent cap counting awake agents, with its default chosen by
   measurement (reversed by (72)); a config key for either bound ((79): the
   owner fixed the values); counting sleeping children toward the width cap
   (sleeping and waiting children linger up to 15 days, Decision 16, and
   would pin the cap, the same reason (59) gave for the concurrent cap).
8. **Hire stays separate (11).** `session.request_hire` (peer, no lineage) is not
   merged into `agent.spawn` (child, lineage); the hire ticket rejected lineage
   authority (768=c, 775=a).
9. **Caps by tier rank and authority (46), (52)=a.** A child never exceeds its
   parent's tier or authority (1014). The tier cap compares rank
   (small<medium<large<xlarge) regardless of harness; aliases resolve to
   `harness.tier` first (existing rank check, search `children.go` for the tier
   rank check). The authority cap: a child's `authority` never exceeds the
   parent's.
10. **`pi.*` spawn tools are removed, not aliased** (`pi.spawn`, `pi.send`,
    `pi.list`, `pi.stop`, `pi.wait`, `pi.transcript`; owner 2026-11-17 19:16,
    reversing (11)'s alias decision). (54)=b: hidden from `tools/list` now
    (T0: new sessions never see them, old installed-plugin sessions keep working
    because the handlers still answer) and the handlers are deleted in the next
    minor. Rejected: removing the handlers in the same release with a T3
    boundary/minor bump. `agent.*` replaces them and shipped playbook/skill
    references to `pi.*` move to `agent.*` in the same change.
    The Pi backend remains reachable as `agent.spawn(tier: "pi.small")`.
11. **Tier namespace (38)=d, (45), (47), (48), (49).**
    - `tier` is one string: bare `small|medium|large|xlarge` = the parent's
      harness; `<harness>.<tier>` with harness in claude|codex|agy|pi (for
      example `pi.small`, `codex.large`); or a user alias defined in user
      settings and editable in the dashboard (for example `mockup` ->
      `agy.large`).
    - Resolved server-side to model/effort through per-harness tier tables,
      generalizing today's pi bucket `wsconfig.HarnessTiers`
      (`ResolveAgentTierForHarness`, `config.go`).
    - No model or effort arguments at all (45); pinning a specific model is done
      only through a user alias. Rejected: an override object {harness, model,
      effort} ((38) option a), a separate `agent.spawn_ex` (b), flat optional
      harness/model/effort (c).
    - `agent.tiers()` returns native (bare tiers on the parent harness), ext
      (`<harness>.<tier>` entries usable now: the harness is installed and its
      backend is registered (`BackendRegistration`) for the worker role; the
      catalog has no worker-availability item, fact population 2026-11-17,
      auto-confirmed technical) and aliases with their resolution. A backend
      registered for the lead role only (the first Codex slice,
      `261115-feat-ws-hosted-codex-adapter-contract` Phase 2; Antigravity until
      its role scope is settled, `261117-feat-ws-hosted-antigravity-adapter`)
      does not make its tiers spawnable: they are absent from ext, and a bare
      tier from a lead on such a harness fails cleanly with the list of valid
      tiers rather than launching an unimplemented worker (batch-review
      coherence C2).
      Spawning an unknown or unusable tier fails with the valid list. Rejected:
      a static list (47).
    - The alias map is a new user-scope config key, dashboard-editable (48);
      compatibility expected T0 (old binary ignores the unknown key).
    - Shipped playbooks and `playbook.render` recommendations use bare tiers
      only; aliases and `harness.tier` are for lead/user choice (49,
      Architecture Rule 4).
12. **System prompt source (39), (44).** At most one of `system_prompt_path` or
    `system_prompt`, delivered through the harness's append-system-prompt (not
    the first user message as the Pi spawn path does today, search `spawn.go`; not full
    replacement). Spawn is playbook-unaware (owner 18:56): `playbook.render`
    keeps rendering and returns the path plus the recommended tier, which the
    caller passes as `tier`. Rejected: a `playbook`/`context` argument on spawn.
    Spawn reads `system_prompt_path` once at spawn and keeps the text in memory
    for re-injection when a new harness segment opens (ws/compact) (44).
    Rejected: re-reading the path per segment (render outputs are temporary;
    file-touch rule).
13. **`authority` replaces `profile` (52)=a; (74)=b, (75)=a, (76)=c, (84)=a, (86),
    (85)=c (2026-11-17).**
    - `authority: read-only | workdir-write | full` is a required argument of
      `agent.spawn` (76)=c, owner "make it required": omitting it is an
      error; there is no default.
    - Cap (74)=b, owner "just 'authority at or below the parent'; only block
      the inversion of a read-only spawning a full": a child may be created
      at any level up to its parent's current level, `full` included, with
      no user warning; a child above its parent is refused. This amends
      `261115-feat-ws-permission-profile` Decision 3 (`full` requires the
      user's choice after a warning) for `ws/agent.spawn` children only;
      that ticket records the exception. Admission is recomputed against the
      parent's current level at create and on every wake (Decision 14), using
      that ticket's Phase 2 admission machinery. The parent's current level is
      its computed level whatever its source, so a legacy-derived parent
      (`permission.go` `legacyLeadPermission`: `auto` derives workdir-write,
      `bypassPermissions` derives full) caps its children at that derived
      level. A spawn-supplied level is stored as the child's explicit
      selection (permission-profile Decision 8 sidecar), always lower than or
      equal to the parent's, never a raise. The landed user selection path
      (`authorizePermission`, which requires the user and an acknowledgement
      for `full`, and `setPermissionLevel`, which refuses workers) is not
      reused for this: Phase 1 adds a create-time admission for
      `agent.spawn` children and the worker-side persistence of the explicit
      level (design review 2026-11-17, auto-confirmed technical).
    - Intent only (75)=a: `authority` sets only the common `permission_level`
      intent of `261115-feat-ws-permission-profile` (reused enum). Spawned
      workers use a common tool loadout; the level is enforced through
      per-harness permission translation, graded enforced, partial or
      advisory (for example Pi read-only is advisory). It does not map to ws
      tool sets or native tool lists.
    - Authority parent (84)=a: for an `agent.spawn` child, the authority
      parent is the spawning session named by the daemon-written Runner
      spawn record (`Record.Parent`, a `ParentRef` the Runner sets at spawn
      and persists, `wsagent/record.go:106,234-238`). This is the (1177)=a
      authority-parent record, narrowed to the `agent.spawn` path: the
      authenticated parent writes it at spawn, the ceiling is recomputed
      from that parent's current level after a restart, and the worker key
      format is unchanged. It narrows the CSI rule that a conversation
      `ParentRef` never participates in selection
      (`internal/mcp/session_context.go:9-11`) for this one admission, and
      only for the Runner-written record, never a display or conversation
      grouping. Other worker paths (`pi.spawn`, hire, worker-profile
      sessions) stay pending (1177) in `261115-feat-ws-permission-profile`.
      A top-level spawn records a lead parent as a mailbox reply-id
      (`ParentRef{Kind: lead, ReplyID}`, `internal/mcp/pi_tools.go:157-158`),
      not a record id: admission resolves that reply-id to the hosted lead's
      Runner record (`Record.ReplyID`) and its session dir, then reads the
      sidecar selection or the legacy derivation from them. A nested spawn
      records the spawning child's Runner record. A recorded child parent
      whose Runner record no longer exists follows that ticket's Decision 3
      unresolved-parent rule (admission error, no guessed ceiling).
    - Parent with no computable level (86), lead decision 2026-11-17 while
      the owner was away, shown on the board as assumed for veto: a lead
      parent with no Runner record (an external CLI lead such as an MCP-only
      Codex CLI lead, which delegates through `pi.spawn` today), or one whose
      backend has no common level yet (hosted Codex and Antigravity leads
      until `261115-feat-ws-permission-profile` Phases 3 and 4 land;
      `legacyLeadPermission` returns "" for them, `permission.go:53-68`),
      caps its children at `workdir-write`. This extends the existing legacy
      derivation, which already maps a Pi lead, a harness with no permission
      model, to `workdir-write`; it is today's ceiling for a `pi.spawn`
      worker of such a lead, so neither a regression nor an escalation. The
      parent's level becomes computed, and this fallback stops applying, once
      its backend gains a common level. A lead reply-id is judged by what it
      resolves to now: a hosted lead whose Runner record was pruned while its
      children slept takes this fallback too, not the unresolved-parent
      error, because the reply-id alone cannot tell it from an external lead
      and no new state is stored to tell them apart; the fallback still
      bounds it at `workdir-write` (design review 2026-11-17, auto-confirmed
      technical). Rejected: (a) treating such a parent
      as `full` (an escalation over today's worker posture, which (74)=b
      exists to bound); (c) refusing the spawn and narrowing (06)=a
      "delegation is always `ws/agent.*` regardless of harness" (a
      regression for Codex CLI leads, and it contradicts Decision 1).
    - Common ws tool loadout (85)=c: every spawned child receives one
      worker-role ws tool set: today's `PiMiniLeadWSTools`
      (`wsagent/builtin.go:108-113`) with `agent.*` in place of `pi.*`, plus
      `mailbox.send` (Decision 1: one `mailbox.send` serves children, peers
      and other leads; `PiMiniLeadWSTools` carries no `mailbox.*` tool), and
      no lead-only tools. `mailbox.send` is not a workdir mutation, so a
      read-only child keeps it. The capability-token check that keys on a
      `pi.` prefix in `WSTools` (`wsagent/children.go:98-108`, `mayHoldToken`)
      moves to the `agent.` prefix with the swap, so `agent.spawn` children
      still receive a token. The level does not select tools; it is enforced on
      them: for a read-only child the daemon refuses the mutating ws tools
      in that set (`git.commit`, `tickets.create_empty`, `tickets.acquire`,
      `tickets.release`, `tickets.close`, `tickets.move`) with a permission
      error, so read-only holds for ws tools on every harness, Pi included.
      workdir-write and full children may call all of them. A read-only
      child may still spawn read-only children (the (74)=b cap). Native
      tools keep the per-harness translation grades above.
    - Harness/model come only from tier, instructions only from the system
      prompt. Existing `agent.profile.<name>` profiles stay internal: hosted
      leads (lead-claude/pi), dashboard features (search-answer-*,
      tracker-support-*). Agent-facing builtins are expressible through the
      common loadout: pi-worker = `pi.<tier>` + workdir-write; explore =
      read-only + explore.md as system prompt; pi-mini-lead = `pi.<tier>` +
      workdir-write or full (the loadout already carries its ticket tools;
      `full` is allowed when the parent is `full`, (74)=b).
    Rejected: renaming to `authority_profile` with today's contents
    (conflicts with (38)=d because a profile fixes backend/model, and
    duplicates the system prompt source); keeping `profile`; (74)a `full`
    staying a user-only choice after a warning; (75)b per-level ws tool sets
    and native tool lists (would merge the permission and capability
    dimensions, against permission-profile Decisions 1 and 5); (76)a a
    workdir-write default; (76)b a legacy-derived default (permission-profile
    Decision 4); (84)b a separate (1177) authority-parent ticket blocking this
    one first (serializes a new ticket for a record the Runner already
    writes); (85)a the pi-worker set for every child (ticket-worker playbooks
    need playbook.render, route.resolve_implement and tickets.query to spawn
    reviewers); (85)b the superset with read-only left to harness
    translation (advisory on Pi, so a read-only child could commit).
14. **Lifecycle: sleep, not close ((57)=ok).** Drop the immediate auto-close of
    finished children (commit 539c94d6a,
    `agents-plugin-tool/internal/wsagent/runner_events.go`
    `autoCloseDue`/`finished`, about lines 768-785). Every agent sleeps through
    the existing idle timer (`idle_timeout_seconds`, `DefaultIdleTimeout` =
    600 s, `wsagent/profile.go:46`; today 600 s for workers, 0 for leads). A
    sleeping agent keeps its record and wakes on `mailbox.send` to its address,
    generalizing the `resumePiChild` path that reopens a closed Pi child on the
    same id and session (commit e796705ce, `wsagent/runner.go` around 1022). A
    running turn, a live exec job or an awake descendant prevents sleep
    (existing `idleNow`, `runner_events.go:605`); a sleeping child does not
    keep its parent awake. Waking is a revive: permission admission
    (`261115-feat-ws-permission-profile` Decision 3, current-parent admission
    on every launch) is recomputed on wake as on every launch, using that
    ticket's Phase 2 machinery, a prerequisite of Decision 13's
    implementation. A refused wake (the parent's current level dropped below
    the child's) does not lose mail: the `mailbox.send` succeeds and the mail
    stays queued for the sleeping child, the child stays asleep, and the
    sender receives one system mail saying the wake was refused and why. The
    next mail retries admission; held mail is delivered once a wake is
    admitted, or deleted with the child at Decision 16's expiry (design
    review 2026-11-17, auto-confirmed technical). Rejected: failing the send
    (the sender cannot tell a refused wake from a bad address) and silently
    holding the mail (it could sit unseen for 15 days).
15. **Control key ((58)=ok).** The control key is the mailbox address
    `agent.spawn` returns; labels are display-only and may repeat.
    `agent.interrupt`/`agent.list` (and any agent.* control) on an agent the
    caller did not spawn directly returns an error, the same rule as today's
    `piOwned` (`agents-plugin-tool/internal/mcp/pi_tools.go:266`,
    `*st.Parent == parent`). `mailbox.send` to any address stays allowed.
16. **Child agent waiting state ((62)=ok, (63)=ok).** A fifth session state
    "child agent waiting" beside on | off | archived | ended: a sleeping child
    that still receives mail (mail wakes it) and is deleted 15 days after its
    last activity (config key, default 15 days; owner answer to (57),
    2026-11-17 19:31). Rejected: the earlier 7-day proposal, too short per the
    owner. Expiry is checked on the existing periodic sweep and at daemon
    start, not by per-event timers or file rewrites; any mail or turn resets
    the 15-day clock; deleting a waiting child deletes its descendants
    deepest-first. Rejected: per-agent timers, because of the release-gate
    file-touch rule (no per-event file touches). The dashboard display of the
    fifth state is tracked in
    `261115-feat-ws-dashboard-session-four-state-followups`.
17. **No `agent.spawn` presets ((53)=a, owner 2026-11-17 21:42).** `agent.spawn`
    takes explicit arguments only (tier, authority, system prompt source); the
    agent-facing builtins become "`playbook.render` + spawn arguments" recipes.
    Rejected: a config-defined named `preset?` bundle of {tier, authority,
    system_prompt_path} listed by `agent.tiers` - it would give spawn an
    instruction source of its own, against the owner's 18:56 rule that spawn
    stays playbook-unaware, and with `pi.*` removed no old-name resolution needs
    it.
18. **Lineage source is the Runner spawn lineage (batch review,
    auto-confirmed technical).** agent.* ownership (Decision 15), the
    report-hold rule (Decision 5), the 15-day deletion and every deepest-first
    cascade (Decisions 1 and 16) walk the Runner spawn lineage, the record's
    `Parent *ParentRef` (`agents-plugin-tool/internal/wsagent/record.go:106`,
    set by the Runner spawn path, `children.go:323`), not the session key's
    parent. Whiteboard agent-card sessions have a key parent (the host lead)
    but leave `ParentRef` unset on purpose
    (`261115-feat-ws-whiteboard-agent-card`, its close/lineage section; owner
    1073 forbids host-close cascade), so they are never owned, held, expired or
    cascaded by these rules. Rejected: the key parent as lineage (it would
    cascade card sessions).

## Constraints

- Go test location (owner (68), 2026-11-17 22:10): run Go tests locally on the macOS home machine. The earlier "remote w5090 only" rule belonged to the owner's company Windows setup and does not apply here.
- Read before editing `agents-plugin-tool/`: `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`; for `agents-plugin-tool/internal/mcp/`
  also `ai-docs/manuals/ws-mcp.md`. If playbooks, skills, `agents-plugin/rsrc/` or
  `agents-plugin-wsflow/` text is touched (for example spawn-idiom routing), also
  read `ai-docs/manuals/skill-authoring.md` and `ai-docs/manuals/wsflow-mirroring.md`.
- Release-gate file-touch rule (owner, 2026-11-17): minimize daemon file touches
  at the root; files for backup/restore only; deltas (append) not rewrites; no
  heartbeat rewrites; no per-event O(N) rereads; interactive paths never wait on
  file I/O; judged by measured syscalls/s on the owner's PC. Daemon-owned agent
  routing must not add per-message file writes.
- Shipped playbooks must not name this repository's specifics
  (`ai-docs/manuals/shipped-surface-boundary.md`); route per-tier spawn idiom
  through generic hooks.
- Raising the spawn depth cap from 2 to 4 and adding the width cap of 99
  (Decision 7) and blocking native tools in hosted sessions (leads and
  workers) are observable behavior changes, already owner-confirmed by (72),
  (77), (78), (06), (18), (29). Letting an agent create a child up to its own
  level, `full` included, without a user warning, and requiring `authority`
  on every spawn (Decision 13) are owner-confirmed by (74), (76). Dropping
  child auto-close in favor of sleep (Decision 14) and removing `agent.stop`
  are owner-confirmed by (57), (60).
- `pi.*` spawn tools are hidden now and deleted next minor (Decision 10); the Claude CLI tier is frozen and keeps
  its native spawn.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Compatibility

New `agent.*` tools are additive (T0), including `agent.tiers` (T0). The alias
map is a new user-scope config key (T0 expected: the old binary ignores the
unknown key; verdict with source lines in Phase 1's Result (48)). The
child-waiting expiry (Decision 16) is likewise a new config key (T0
expected for the same reason; verdict with source lines in Phase 1's Result,
the phase that lands the expiry). The authority parent ((84)=a) reuses the
existing persisted `Record.Parent` with no new Record field, and the child's
explicit level goes into permission-profile's Decision 8 sidecar
(`<SessionDir>/permission.json`, unknown members retained): T0 expected,
verdict with source lines in Phase 1's Result. Hiding
`pi.*` from `tools/list` is T0 for this release ((54)=b: handlers still answer,
so old sessions keep working); the handler deletion next minor is judged in that
release. The source-line verdict is recorded in Phase 2's Result (the hiding
lands with mailbox routing). Relation computed at delivery rather than stored is
T0 (no new Envelope field, so no whole-file loss on an old binary). Daemon-owned
mail persistence inherits `261117-feat-ws-mailbox-memory-first-store`'s
downgrade judgment rather than adding a store of its own (Decision 2). The
concrete verdicts cite source lines for which version writes and reads what,
per `ai-docs/manuals/release-compatibility.md`, recorded in the Result of the
phase that lands each surface (alias key: Phase 1; `pi.*` hiding and relation:
Phase 2). The depth and width caps are code constants with no config
manifest field (T0, Decision 7). Raising the depth cap, the width cap and
native-tool blocking in hosted sessions (leads and workers) are observable
behavior changes owner-confirmed as above.

## Prior Decisions

- 539c94d6 (2026-11-17, commit): "Done-only: failed/interrupted children stay open, because a closed row maps to a "done" spawn row (SpawnAgentEvent) and would hide the failure. Report-before-close: finished() requires empty PendingReports, Queued, Mail and Reports" — bearing: constrains
- 261112-feat-ws-mcp-pi-spawn-recursion (2026-11-13, e09e620a): "Daemon-as-router: the daemon already owns every conversation, so the Pi worker's ws-mcp forwards to it; the token, not the session key, authorizes pi.* so the child's key stays a parentless delegate key." — bearing: constrains
- 261116-research-ws-hosted-session-first-tier (2026-11-16, Confirmed Decisions): "(06)=a, (09)=a, (10)=c, (11), (12) Delegation is always `ws/agent.*` regardless of harness: `agent.spawn`, `agent.stop`, `agent.list`, with no `wait`, no `transcript` and no `agent.send`" — bearing: supports
- 261117-feat-ws-hosted-prompt-assembly (2026-11-17, Decisions): "Native subagent tool removal extends from hosted leads to hosted workers (29)=ok, a consequence of (06). The removal itself is delivered in `261117-feat-ws-agent-tools-mailbox-routing` (Decision 6, Phase 2)." — bearing: supports; that removal is Phase 3 here since the 2026-11-17 phase split
- 261113-fix-ws-playbook-spawn-idiom-and-delegate-resolve-agent (2026-11-14, 4f057f53b): "MCP-only row (`piMCPOnlyTerminology`): `SpawnIdiom` = `ws/pi.spawn` (tier: the resolved capability tier, not a model name), `ContinueIdiom` = `ws/pi.send to the same subagent id`." — bearing: constrains
- 1192777c (2026-11-17, commit): "Owner (1297)=a: zero-state-risk read reductions. Runner.Lineage reads the record only; piChanged -> SpawnLead -> Runner.Get relisted the parent inbox per child emit." — bearing: constrains
- 195d21ca (2026-11-17, commit): "Owner TOP priority is daemon file-system load (AV cost per touch); (1297)=a approved read caches and skip-if-unchanged writes." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/children.go, runner.go, runner_events.go, profile.go, claude.go, agents-plugin-tool/internal/mcp/pi_tools.go, agents-plugin-tool/internal/wsconfig/config.go, shipped playbooks and skills |
| scope.surface | cross-module | new agent.* MCP tools, new alias and child-waiting expiry config keys, wsagent runner lifecycle, wsconfig tier resolution, shipped playbook text |
| scope.new_public_symbol | yes | agent.spawn, agent.interrupt, agent.list, agent.tiers tools and the alias and child-waiting expiry config keys |
| scope.new_type_contract | yes | agent.spawn argument contract (tier, required authority, system_prompt_path or system_prompt) and the tier namespace resolution |
| scope.test_surface | new-files | existing children_test.go, idle_children_test.go, pi_tools_test.go cover the pi.* path only; no agent.* handler tests exist (search for "agent.spawn" in agents-plugin-tool/internal Go found none) |
| complexity.reuse_points | confirmed | Runner spawn path, tierRank and piOwned, resumePiChild, idleNow, ResolveAgentTierForHarness read; the steer lookup reuses the catalog (pending 261117-feat-ws-harness-capability-catalog) |
| complexity.side_effect_risk | high | raises the depth cap from 2 to 4 and adds a width cap, replaces child auto-close with sleep and wake, hides pi.* tools and changes mailbox delivery for every relation |
| risk.correctness | high | sleep and wake races, awake-only width-cap counting with wake exempt, ownership and caps inheritance, report-hold under steer |
| risk.fit | high | steer and tier availability read a catalog that has no code yet; Pi has no catalog profile (catalog Decision 13), so its steer lookup follows Decision 4 (the handle alone decides) |
| risk.test | high | needs runner concurrency tests and a live hosted delegation end to end |
| risk.security_or_contract | high | authority caps (Runner-record authority parent, daemon refusal of mutating ws tools for read-only), ownership errors, pi.* tools hidden while handlers still answer, T0 verdicts to be recorded in the Phase 1 (alias key) and Phase 2 (pi.* hiding, relation) Results |

## Phases

### Phase 1: agent.spawn/interrupt/list/tiers, sleep and wake, lifecycle and caps

Add `agent.spawn`, `agent.interrupt`, `agent.list`, `agent.tiers` (shapes per
Decision 1) over the existing Runner spawn path generalized to harness-selected
backends with server-side tier namespace resolution (bare, `<harness>.<tier>`,
user alias via the new user-scope alias config key and per-harness tier tables;
only worker-role backends are spawnable, Decision 11), `authority` per Decision
13 (required, intent only, capped at the parent's current level), and system prompt delivery through the harness's
append-system-prompt (path read once, text kept in memory for re-injection).
`agent.spawn` returns the child's address as the control key (Decision 15);
mail to that address is Phase 2. Drop child auto-close so every agent sleeps
after the idle timer and wakes through a Runner-level delivery seam (the seam
Phase 2's mailbox routing calls; wake recomputes permission admission, Decision
14); add Decision 16's fifth state, child agent waiting, with the 15-day
expiry (config key, default 15 days) checked on the existing periodic sweep and
at daemon start, any turn resetting the clock, and deletion cascading
deepest-first. Keep the report-hold rule under sleep. Ownership, report-hold,
expiry and cascades walk the Runner spawn lineage (Decision 18). Return an
ownership error for agent.* control on an address the caller did not spawn
directly; apply tier-rank and authority caps, with authority admission
recomputed against the parent's current level at create and on wake through
a create-time admission and worker-side persistence of the explicit level
(Decision 13; the authority parent is the Runner spawn record, (84)=a, a
lead parent's reply-id resolved to its Runner record; a parent with no
computable level caps at workdir-write, (86); a refused wake keeps the mail
and notifies the sender, Decision 14). Give every child the common ws tool
loadout including `mailbox.send`, move `mayHoldToken` to the `agent.`
prefix, and refuse the loadout's mutating ws tools for a read-only child
(Decision 13, (85)=c). Apply
Decision 7's spawn tree bounds as code constants: `MaxSpawnDepth` moves from 2
to 4 (kept, not removed), and a per-agent width cap of 99 counts awake direct
children only; a spawn over either bound fails with an error naming the cap,
and a wake through the delivery seam ignores the width cap.

Deferred: mailbox routing, steer delivery and `pi.*` hiding (Phase 2; until
then `pi.*` stays visible as the only follow-up path); exec tools, single
holder, any stage-3 harness backend.

Verification: Go tests covering spawn/list (Decision 1 fields), interrupt
keeping the session, no auto-close on finish, sleep after the idle timeout,
wake of a sleeping child through the Runner delivery seam with permission
admission recomputed, the child agent waiting state, deletion 15 days after
last activity on the periodic sweep and at daemon start with a turn resetting
the clock and descendants deleted deepest-first, a whiteboard card session
(key parent, no `ParentRef`) never cascaded, ownership error on a non-own
address, a spawn that would exceed depth 4 failing with an error, a spawn
at 99 awake direct children failing with an error naming the cap while
sleeping children do not count, a wake through the delivery seam succeeding
with the parent at the width cap, tier resolution (bare,
`harness.tier`, alias, unknown/unusable and lead-role-only backends failing
with the valid list), `agent.tiers` output, system prompt delivery via
append-system-prompt and re-injection from memory on a new segment, the
report-hold rule, tier-rank and authority cap enforcement and caps
inheritance (`authority` omitted is an error; a child at the parent's level,
`full` included, is admitted without a warning; a child above it is
refused; a legacy-derived parent capping at its derived level; the child's
explicit level surviving a daemon restart; a wake refused after the parent
lowered its level keeping the mail queued, leaving the child asleep and
sending the sender one system mail; the ceiling recomputed from the
Runner-recorded parent's current level after a daemon restart, a hosted
lead parent's reply-id resolving to its Runner record's level, a lead parent
with no Runner record and a hosted Codex lead parent each capping at
workdir-write (a `full` child refused), and a recorded child parent whose
record is gone failing admission); a read-only child refused each mutating ws
tool of the loadout while workdir-write and full children may call them, and
every child, read-only included, able to call `mailbox.send`; an
`agent.spawn` child receiving a capability token; `pi.spawn` refusing past depth 4 with the updated error text.

### Phase 2: Mailbox routing to daemon-owned addresses, steer delivery, `pi.*` hiding

Depends on Phase 1, on `261117-feat-ws-mailbox-memory-first-store` Phase 2
(the persistence vehicle, Decision 2) and on the catalog's steer item
(`261117-feat-ws-harness-capability-catalog` Phase 1). Route daemon-owned
addresses in memory via the Runner, persisting daemon-owned mail through the
mailbox store's restore ring (same history and reply-id semantics);
accept `mailbox.send` to a daemon-owned address; deliver every `mailbox.send`
as a steer when the catalog and the instance handle support it, else next turn
(Decision 4), with the report-hold rule, and label each delivered mail with
the relation (parent/child/peer) computed from lineage at delivery (Decision
3); mail to a sleeping child wakes it
through Phase 1's seam and resets its 15-day clock. Hide the `pi.*` spawn tools
from `tools/list` (handlers deleted next minor, (54)=b) and move shipped
playbook/skill `pi.*` references to `agent.*`.

Verification: Go tests covering wake on `mailbox.send` to a sleeping child
(ignoring the width cap), mail resetting the 15-day clock, steer delivery for each relation with
next-turn fallback, the relation label on delivered mail for each relation
(computed at delivery, no new Envelope field), daemon-owned mail surviving a daemon restart through the
memory-first store with recv/history/reply-id parity, no `mailbox.json` write
and no write beyond the store's one ring append per send for daemon-owned
addresses, `pi.*` absent from `tools/list` but still callable,
with no shipped text referencing `pi.*`.

### Phase 3: Native subagent removal in hosted Claude leads and `ws/agent.spawn` workers

Launch hosted Claude leads and `ws/agent.spawn` Claude workers with `--disallowedTools
Agent` and `Task` (internal and dashboard profiles that need the native tool, and
owner-created worker-profile sessions, are exempt, Decision 6); extend the two
tests' banned-flag lists as needed (they do not name `--disallowedTools` today,
Decision 6); make the catalog's expected init tool set subtract the requested
removals and update the tool-removal grade this phase wires (catalog Decision
10 grades lead default tool removal `unsupported` today; if the catalog's
Phase 3 init check has not landed yet, the subtraction is wired where it lands);
confirm via the init
event that the native tool is absent and that delegation works through
`agent.spawn`.

Verification: updated tests, including the init-shape check not demoting
default tool removal or native subagent removability for a launch that
requested the removal; a recorded hosted-lead init stream and a recorded
hosted-worker init stream, each showing the tools removed; a hosted lead
delegating through `agent.spawn` end to end; `tracker-support-claude` still
launching with its `Agent` tool.
