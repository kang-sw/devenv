---
title: "Research unified ws-mcp agent tools versus per-harness tool adaptation"
parent: 261114-epic-ws-common-session-foundation
related:
  261114-refactor-ws-common-session-interface: "preserve distinct entry modes, native integrations, authority and parity boundaries; independent research, not a new prerequisite"
---

# Unified agent tool surface

## Research Intent

MAIN relayed owner (853)=a on 2026-11-15 at 01:16:53 UTC: capture a
comparison of a complete ws-mcp agent toolset (read/grep/find/edit/exec,
with harness-native tools disabled) against per-harness tool-surface adaptation.
The owner needs evidence and decision-ready options, not a selected architecture.

Cover Codex, agy, Claude and Pi, recording exact harness and model versions.
Preserve the owner's name `agy`; relate it to Antigravity only if identity is
evidenced. The expectation that Pi may mostly work already is unverified and
should receive a cheap confirmation check before deeper investigation.

This idea captures future research only. TASK3 remains top-priority independent
active work; neither this capture nor future research adds a foundation
implementation prerequisite or delays TASK3. No research execution, experiment,
source/runtime workload, tool restriction or implementation is authorized here.

## Foundation and Interface Boundaries

- Parent `261114-epic-ws-common-session-foundation`, Scope and Cross-Child
  Decisions: minimum interface -> current behavior/state parity -> extensions;
  distinct CLI-plugin and MCP-only hosted entry, existing Runner/adapter owners,
  preserved identity/authority/lineage and one input/mail delivery owner.
  Common access does not make MCP attachment native integration or process control.
- Related `261114-refactor-ws-common-session-interface`, Decisions 1–5 and
  Constraints: preserve native integrations, approvals, delegation ownership,
  recovery, existing keys/stores and backend capability differences. Profile
  labels and mailbox reachability are not permission proofs.
- That interface's Phase 1 owns minimum shared access and backend
  admission/registration/unsupported-operation contracts; Phase 2 owns current
  context/state parity adoption. Research may inform later choices but neither
  changes those contracts nor asserts their completion. Audit still consumes
  interface Phase 2; its UI still consumes audit-backend Phase 2.
- No new MCP API, serialized field, store/protocol shape, universal runtime,
  approval power or root-key issuance restriction is approved. The existing
  foundation targets additive T0; any later cross-version change needs its own
  compatibility-tier settlement, and T2 needs explicit owner approval.

## Expected Research Deliverables

Acceptance of the future investigation requires:

- An exact-version evidence matrix for each harness/tool surface, separating
  official documented support, source evidence and runtime verification. Give
  citations, commands/configuration, conditions and explicit unverified gaps;
  absence of evidence is not proof of impossibility.
- A reproducible paired experiment design, not fabricated results, with task
  fixtures, grading, controls, repeatability, safety bounds and unresolved
  model/tooling differences. Execution and thresholds remain unapproved.
- A safety/UX and trust-boundary comparison, an existing-profile mapping, and
  enforceability/bypass analysis addressing all five questions below.
- Decision-ready options for the owner: trade-offs, evidence strength,
  limitations and further authorization needed. Findings do not themselves
  select a surface or authorize implementation.

## Outcome Ledger

### Verified Findings

- VERIFIED [parent, Scope / Non-Scope / Cross-Child Decisions]: the foundation
  ticket preserves native behavior and state, distinguishes CLI and hosted
  entry, and does not authorize a replacement universal runtime or new powers.
- VERIFIED [related interface, Decisions / Constraints / Phases 1–2]: the
  interface separates minimum access/admission contracts from parity adoption
  and preserves native capability differences and existing authority. These
  are documentary boundaries, not runtime capability verification.
- No harness tool-substitution, profile-enforcement or policy-enforcement
  capability has been verified by this capture.

### Confirmed Decisions

- Owner (853)=a, as relayed by MAIN: capture this comparison as independent
  research alongside TASK3 without delaying it. This approves the research
  ticket, not a surface choice, experiment execution or implementation.

### Proposals

- Compare native and ws-mcp toolsets on identical small, isolated repository
  tasks per model: inspection/search, a localized edit and a command-assisted
  repair. Specify fixtures and machine-checkable expected diffs/tests, plus
  human grading where semantics cannot be checked automatically.
- Pair runs by exact model/version and harness/version; hold prompts, starting
  tree, context, task budget, permissions and environment constant where
  possible. Reset between runs, counterbalance order, repeat runs with recorded
  sampling settings, and retain failures as well as successes. Report residual
  tool-schema, model availability, prompt, caching and mediation differences
  rather than treating them as controlled.
- Measure edit success (correctness and unintended changes), tool-choice
  errors (invalid, unavailable or inappropriate calls), tokens and latency.
  Specify token-accounting sources, missing telemetry, timing boundaries and
  overhead separately. Propose sample sizes, grading rules and any thresholds
  for owner review; none is settled here.
- Bound any later authorized experiment to disposable fixtures, least
  privilege, no secrets, no external mutation/network by default and explicit
  stop/cleanup criteria. Separate safe negative-policy probes from normal
  accuracy tasks. Obtain execution approval before running either.

### Open Questions

1. **Native-tool substitution, per exact harness/version.** Can native tools
   be disabled or allowlisted and MCP tools substituted for read/grep/find/edit/
   exec? Identify official docs, CLI flags and config for each mode, then
   separately label source evidence and runtime verification. Distinguish
   advertised tool visibility from enforced denial. What remains unsupported
   or unverified? First cheaply check the owner's Pi expectation; it is not a
   finding. Establish agy's identity before borrowing Antigravity evidence.
2. **Paired accuracy experiment.** How should the same tasks compare native
   versus ws-mcp surfaces per model using edit success, tool-choice errors,
   tokens and latency? Refine the proposed controls, repeatability, grading,
   safety bounds and unresolved differences into a reproducible design.
   Which comparisons are invalid or confounded? No particular thresholds or
   execution are approved by this capture.
3. **Safety and UX.** Which native protections and affordances are retained or
   lost, including Codex sandboxing, approval prompts and diff rendering?
   Trace execution location, credentials, filesystem/network authority,
   approval mediation and result presentation. Distinguish MCP server
   authority from harness execution mediation: an attached MCP server does
   not automatically inherit a native execution sandbox. What alternative
   mediation or UX could cover gaps, and what would remain unverified?
4. **Existing profile restrictions.** What do wsagent `Profile` `tools` and
   `ws_tools` actually restrict today for each candidate surface? Ground the
   map in existing definitions, validation, launch wiring, dispatch checks
   and relevant tests, distinguishing intended configuration from actual
   trust/enforcement boundaries. Do not invent profile semantics or treat a
   profile label, hidden tool or advertised allowlist as an authority proof.
5. **Enforceable policy and bypasses.** Could ws-owned exec reject an
   out-of-pool `git worktree add`, and under what exact trust assumptions?
   Enumerate alternate execution and mutation paths: remaining native tools,
   other MCP servers, subprocesses/interpreters, delegated agents and direct
   filesystem/Git metadata mutation. Identify which are mediated, denied or
   outside the boundary, and whether path/argument inspection is sufficient.
   Owning exec alone does not establish arbitrary policy enforcement. Which
   options are enforceable, advisory or unverifiable under each configuration?

### Rejected Alternatives

No tool-surface alternative has been rejected. Surface selection and any new
mechanism remain open pending evidence and owner decisions.

## Desk Evidence (2026-11-15)

Scope: owner (946)=a, documentation and source research only. No experiment,
no model-accuracy run, no install, no tool restriction was executed. The only
local commands run were read-only introspection (`codex --version`,
`codex features list`, `agy --version/--help/changelog/mcp --help`,
`claude --version`, `strings` on the installed `agy` binary, `gh api` reads of
the public `openai/codex` tree). Labels: **Confirmed** = cited doc or source;
**Unverified** = not read, inferred, or only secondhand. Absence of evidence is
not proof of impossibility. Findings select no surface and authorize nothing.

Versions observed locally: Codex CLI 0.160.0; agy 1.5.9; Claude Code 2.1.325;
pi 0.84.4 (repo-pinned, `agents-plugin-pi/node_modules`) and pi 1.5.2
(global `/opt/homebrew/lib/node_modules`, changelog dated 2026-11-10).
Codex source read at `openai/codex` main, head `b016cd1847` (2026-11-15).

### Identity of `agy`

Confirmed: the local `agy` binary (v1.5.9, `~/.local/bin/agy`) is the
Antigravity CLI. Evidence: `agy --help` lists `acp`, `mcp`, `plugin`,
`remote-control` subcommands; the Antigravity docs page
`https://antigravity.google/docs/features?tab=cli` says "Run `agy` to work with
the agent in an interactive terminal UI" with `agy mic-serve`,
`agy remote-control start`, `agy plugin install`; settings live at
`~/.gemini/antigravity-cli/settings.json`; the in-repo ticket
`261114-feat-ws-hosted-antigravity-acp-adapter` already reports `agy acp`.
Interpretation used here: agy = Antigravity CLI. The IDE/desktop app and SDK
are separate surfaces and are not covered.

### Per-harness findings

#### Codex CLI (0.160.0)

1. Disabling native tools.
   - Shell: `[features] shell_tool = false` removes command execution.
     Confirmed: config reference
     `https://learn.chatgpt.com/docs/config-file/config-reference`
     ("Enable command execution tools (stable; on by default)") and source
     `codex-rs/core/src/tools/spec_plan.rs` `add_shell_tools` (returns early
     when `!features.enabled(Feature::ShellTool)` or model
     `shell_type == Disabled`). `features.unified_exec` user opt-out is
     ignored (docs: "User opt-outs are ignored"); only managed requirements can
     degrade it to one-shot `exec_command`. First-version of `shell_tool`
     toggle: Unverified.
   - `apply_patch`: no user toggle found. `codex features list` shows
     `apply_patch_freeform  removed  false`. Source `spec_plan.rs`
     (`add_core_utility_tools`): `ApplyPatchHandler` is registered iff
     `model_info.apply_patch_tool_type.is_some()`, i.e. it is decided by the
     model catalog entry (`ModelInfo.apply_patch_tool_type`,
     `codex-rs/protocol/src/openai_models.rs`), not by a feature flag.
     A custom model catalog (`model_catalog_json` exists in
     `codex-rs/core/src/config/mod.rs`) could in principle set
     `apply_patch_tool_type: null` and `shell_type: "disabled"`; whether that
     path is honored for a built-in model slug at runtime is Unverified.
   - `features.view_image = false` removes `view_image`. Confirmed (config
     reference). `update_plan`, `request_user_input`, MCP-resource list/read
     tools, collaboration/multi-agent tools are added by other branches of
     `spec_plan.rs`; per-tool toggles for each: Unverified.
2. MCP-only visibility. Partially Confirmed. MCP per-server `enabled_tools`
   (allow list) / `disabled_tools` (deny list) exist (config reference), so
   the MCP side is fully controllable. A pure MCP-only surface needs shell off
   (`features.shell_tool=false`, Confirmed) plus `apply_patch` gone (only via
   model-catalog override, Unverified) and the residual utility tools
   (`update_plan`, `view_image`, MCP resource tools, `request_user_input`)
   remaining unless individually toggled (Unverified). Whether the tool list
   can be empty of native tools without an API/harness error: Unverified.
3. Features tied to native tools.
   - Sandbox: `sandbox_mode` / `permissions.<name>` / `default_permissions`
     govern "model-generated commands" (docs
     `https://learn.chatgpt.com/docs/agent-approvals-security.md`; network
     proxy filters "scripts, programs, and child processes that run inside the
     local command sandbox"). The docs do not state that MCP tool calls run in
     the sandbox; a mirrored README copy says trusted MCP calls run outside it
     (non-official mirror; Unverified). Treat an MCP server as unsandboxed by
     Codex: any ws-owned exec would need its own confinement.
   - Approvals for MCP: `mcp_servers.<id>.default_tools_approval_mode` and
     `tools.<tool>.approval_mode` (config reference); "Destructive app/MCP tool
     calls always require approval when the tool advertises a destructive
     annotation ... unless the tool advertises a read annotation, which takes
     priority" (agent-approvals-security). So MCP annotations (`readOnlyHint`,
     `destructiveHint`) drive approval; ws tools would have to advertise them
     accurately. Confirmed.
   - Parallel tool calls: MCP tools support parallel calls only with a
     read-only hint or a server opt-in (`codex-rs/core/src/tools/handlers/mcp.rs`
     tests `mcp_parallel_calls_require_read_only_hint_or_server_opt_in`).
     Confirmed from source/test names; exact opt-in key name: Unverified.
   - Diff display / turn diff: native `apply_patch` produces patch events used
     by the UI (not verified in source); a ws edit tool returning a unified
     diff in its MCP result would not feed the native diff UI: Unverified.
   - Checkpoints/undo: Codex-side file-state tracking or undo tied to
     `apply_patch`: Unverified.
4. Edit shape: freeform (non-JSON) `apply_patch` custom tool with a Lark
   grammar (`codex-rs/core/assets/tools/apply_patch.lark`,
   `apply_patch_spec.rs`, "This is a FREEFORM tool, so do not wrap the patch
   in JSON"). Grammar (Confirmed):
   `*** Begin Patch` / hunks / `*** End Patch`; hunks `*** Add File: p` with
   `+` lines, `*** Delete File: p`, `*** Update File: p` optionally
   `*** Move to: p`, then chunks introduced by `@@` or `@@ <context line>`,
   lines prefixed ` `, `-`, `+`, optional `*** End of File`. Locating a chunk
   uses `seek_sequence` with decreasing strictness: exact, then ignoring
   trailing whitespace, then ignoring leading+trailing whitespace; `eof`
   chunks try end-of-file first (`codex-rs/apply-patch/src/seek_sequence.rs`).
   No read-before-edit rule observed in what was read; atomicity across
   multi-file patches: Unverified (`lib.rs` not read in full). A multi-env
   variant adds `*** Environment ID:`. Models are trained/tuned for it per the
   code comment ("Well-suited for GPT-5 models"); training-set claims: see
   Owner hypothesis.

#### Claude Code (2.1.325)

1. Disabling native tools. Confirmed, CLI reference
   `https://code.claude.com/docs/en/cli-reference.md`:
   - `--tools ""` disables all built-ins, `"default"` the default set, or a
     name list (`"Bash,Edit,Read"`); "The flag doesn't affect MCP tools; to
     deny those too, use `--disallowedTools "mcp__*"`".
   - `--disallowedTools`: "A bare tool name removes the matching tools from
     Claude's context: `"Edit"` removes Edit, `"*"` removes every tool ... A
     scoped rule such as `Bash(rm *)` leaves the tool available and denies
     only calls that match". Same via `permissions.deny` in settings
     (`https://code.claude.com/docs/en/settings.md`). Introducing version of
     these flags: Unverified. Managed settings can pin policy
     (`managed-settings.json`).
   - `--strict-mcp-config --mcp-config ./mcp.json` loads only the given MCP
     servers. `--bare` skips auto-discovery and keeps Bash/Read/Edit.
2. MCP-only: Confirmed achievable except `EndConversation`:
   tools-reference
   (`https://code.claude.com/docs/en/tools-reference.md`): "neither
   `--disallowedTools` nor a `--tools` list can remove it" while any other tool
   remains; harmless (ends the session, no file/data access). Other built-ins
   (Agent, Skill, ToolSearch, Task*, Monitor, LSP, ...) are removable via
   deny/`--tools`; MCP-resource tools (`ListMcpResourcesTool`,
   `ReadMcpResourceTool`) exist as built-ins. A live "tools visible to the
   model" listing for an MCP-only config was not obtained (Unverified).
   Glob/Grep are absent by default on macOS/Linux/WSL (docs), so the default
   native read/search path is Read plus Bash (grep/rg).
3. Features tied to native tools:
   - Checkpoints/rewind: Confirmed, checkpointing docs
     (`https://code.claude.com/docs/en/checkpointing.md`): "Claude Code tracks
     all changes made by its file editing tools"; "A file that only Bash
     commands changed isn't tracked and stays as it is"; once a file has been
     edited via the editing tools, later Bash changes to it are undone by
     `/rewind`. Subagent edits are usually not captured. MCP-tool edits are not
     mentioned: they would not be tracked (inference; Unverified). The verify
     request is answered: the "bash-modified files are not checkpointed" claim
     is Confirmed with the nuance above.
   - Read-before-edit: Confirmed (tools reference, Edit behavior): Edit
     requires the file to have been read in-conversation (PARTIAL view does not
     count; Opus 4.6, Haiku 4.5 and older always require it, newer models may
     edit an unread file when reading would need no permission prompt);
     Bash `cat`/`grep`/`rg`/`sed -n` on one file also satisfy it. Stale files
     can still be edited if `old_string` matches current content. None of this
     state is available to an MCP edit tool; ws would need its own mtime/hash
     tracking if it wants this guard.
   - Permissions: path-scoped `Read(...)`/`Edit(...)` rules and Bash rules do
     not cover MCP tools; MCP is addressed as `mcp__<server>__<tool>` rules
     (tools reference / CLI reference `"mcp__*"`). `Read` deny rules also block
     Edit/Write on the path ("including creating a new file there") and apply
     to listed Bash read commands; none of that protects paths a ws exec tool
     touches. Confirmed (tools reference). Sandbox for Bash: Claude's
     sandbox settings were not fetched (Unverified); it applies to Bash, not to
     MCP server processes (inference; Unverified).
   - Diff display, PreToolUse hooks: hooks match tool names including MCP
     (hook matchers use tool names per tools reference); diff rendering for
     MCP results: Unverified.
   - Parallel tool calls: Claude Code concurrency for MCP versus native:
     Unverified.
4. Edit shape: Confirmed (tools reference, Edit tool behavior): "exact string
   replacement. It takes an `old_string` and a `new_string` ... doesn't use
   regex or fuzzy matching"; `old_string` must match exactly (whitespace
   included) and appear exactly once, else supply more context or
   `replace_all: true`; read-before-edit as above; refuses non-UTF-8 files
   (UTF-16LE BOM excepted); a `Read` deny rule refuses the edit. Write is a
   full-file overwrite. Anthropic's SWE-bench post
   (`https://www.anthropic.com/engineering/swe-bench-sonnet`): string
   replacement had "the highest reliability", edit only if "exactly one
   match", tool made to always require absolute paths.

#### agy / Antigravity CLI (1.5.9)

1. Disabling native tools. Official docs
   (`https://antigravity.google/docs/permissions?tab=cli`) give a permission
   system, not a tool allowlist: `permissions.{allow,ask,deny}` arrays in
   `~/.gemini/antigravity-cli/settings.json` with rule forms
   `read_file(path)`, `write_file(path)`, `command(prefix|regex:..|*)`,
   `read_url`, `execute_url`, `unsandboxed(...)` (CLI, business/enterprise
   only), `mcp(server/tool|server/*|*)`; precedence Deny > Ask > Allow; "Deny:
   The action is blocked immediately." Whether a deny hides the tool from the
   model or only blocks the call is not stated (Unverified). `--settings` takes
   per-session JSON of settings keys; `--sandbox` / `enableTerminalSandbox`
   (off by default per
   `https://antigravity.google/docs/features?tab=cli`); `--mode`
   (accept-edits, auto, plan).
   - Binary evidence (Unverified semantics): `strings` on the 1.5.9 binary
     shows ACP new-session parameter `disableBuiltInTools` /
     `disable_built_in_tools` (`acp.NewSessionParams.disableBuiltInTools`,
     `acp.applyDisabledBuiltinTools`), and MCP server fields `enabledTools` /
     `disabledTools` in `mcp_config.json` (changelog 1.5.x mentions
     `enabledTools`, `timeoutSeconds`, `url`, `tools.eager`). So an ACP-hosted
     agy may be able to run MCP-only; exact semantics and version of
     introduction were not verified. No documented CLI flag found.
2. MCP-only: Unverified. `agy mcp add|remove|list|enable|disable` exists
   (`agy mcp --help`). Which built-ins remain (e.g. `run_workflow`, browser
   subagent, plan/ask tools) was not established.
3. Tied features: terminal sandbox covers commands only ("commands run in the
   Terminal sandbox", default workspace + temp, no network, permissions page);
   `/diff` interactive diff viewer and per-turn changes (features page);
   `/rewind` is named in the session commands but not described, so what it
   restores and whether it covers non-native edits is Unverified;
   `run_workflow` fans out subagents. MCP calls under sandbox: Unverified.
   Parallel calls: Unverified. `agy acp` reports edit diffs as
   `oldText`/`newText` (changelog 1.5.9), a UX channel an MCP edit tool would
   not use.
4. Edit shape: Official docs do not define it. Source evidence is limited to
   embedded tool descriptions in the 1.5.9 binary (`strings`; Unverified as
   authoritative): an `edit_file`-family tool: "Use this tool to edit an
   existing file ... For one change, set Target and Replacement. To change
   several separate places in the same file, set Edits to a list of
   {Target, Replacement} pairs ... applied in order and all-or-nothing ...
   Target must exactly match existing file text, including whitespace ...
   Target must appear exactly once ... or set AllowMultiple to true to replace
   every occurrence." Related tool names in the binary: `view_file`,
   `view_file_outline`, `list_dir`, `grep_search`, `run_command`,
   `write_to_file`. Struct names `ReplacementChunks{TargetContent,
   ReplacementContent}` and `AllowMultiple` appear, i.e. a str_replace-like
   shape with multi-edit.

#### pi (pi-coding-agent)

Two relevant versions: repo-pinned 0.84.4 and global 1.5.2. They differ on MCP.

1. Disabling native tools. Confirmed (0.84.4 `README.md`, `docs/usage.md`,
   `docs/settings.md`, `CHANGELOG.md`; 1.5.2 `docs/cli.md`):
   `--tools/-t <list>` allowlist across built-in/extension/custom;
   `--exclude-tools/-xt` (added 0.77.0); `--no-builtin-tools/-nbt` (correct
   semantics since 0.70.0; `--no-tools` disables all, since 0.39.0 for
   built-ins and all tools since 0.68.0); `defaultTools` setting
   (0.84.2); `createAgentSession({tools, excludeTools, noTools})` SDK. Built-ins:
   read, bash, edit, write, grep, find, ls (0.84.3 adds optional `powershell`);
   defaults are read, bash, edit, write. Extensions may also call
   `pi.setActiveTools()` at runtime.
2. MCP-only.
   - 0.84.4: pi has no native MCP ("No MCP", README). The repo plugin supplies
     it through an extension: `agents-plugin-pi/src/index.ts` spawns the ws-mcp
     launcher, `src/mcp-stdio-client.ts` + `bridge.ts` re-register every
     ws-mcp tool via `pi.registerTool`. Confirmed (source read).
   - 1.5.2: native MCP (changelog 0.99.0, 2026-09-29, "Codemode and MCP";
     `pi mcp add|remove|list|login|logout`, `/mcp`, `--no-mcp`); `--tools`
     "does not remove MCP tools" unless an entry starts with `mcp__`;
     `--no-tools` disables everything including MCP; `--exclude-tools 'mcp__*'`.
     Confirmed (`docs/cli.md`, `docs/mcp.md`).
   - So "only MCP tools visible": `pi --no-builtin-tools` plus bridge/MCP
     extension tools is a Confirmed documented mode (docs/extensions.md:
     "use `--no-builtin-tools` to start without any built-in tools while keeping
     extension tools enabled"). No tool is documented as non-removable.
   - The owner's cheap check ("Pi may mostly work already"): partially
     Confirmed by source: the repo's lead sessions already call
     `pi.setActiveTools(computeLeadActiveTools(...))`
     (`agents-plugin-pi/src/execute-gateway.ts`, `index.ts`) which removes
     native `bash` and `read` and adds gated `ws-execute`/`ws-approve` plus the
     "ugly-named" read/exec tools; `edit`, `write`, `grep`, `find` stay native.
     A full read/grep/find/edit/exec replacement over ws-mcp is not
     implemented; not exercised at runtime here.
3. Tied features: pi has no sandbox ("No Built-in Sandbox",
   `docs/security.md`: isolation comes from containers/VMs), no permission
   popups ("No permission popups. Run in a container, or build your own
   confirmation flow with extensions", README), so moving edit/shell to MCP
   loses little harness protection; the loss is pi-specific UX: edit diff
   rendering (`renderDiff` on native edit results, `edit.js`), per-file
   mutation queue (`dist/core/tools/file-mutation-queue.js`, extensions
   docs: custom file-mutating tools must use `withFileMutationQueue()` because
   "tool calls run in parallel by default"), bash output truncation and
   timeout handling, image reads. Parallel: default parallel tool execution
   mode; siblings are preflighted sequentially then run concurrently
   (`docs/extensions.md`). Checkpoint/rewind of files: none found in 0.84.4
   docs read (session tree `/tree` is conversation-level); file-state tracking
   such as read-before-edit: none observed in `edit.js` (Confirmed absence in
   that file only).
4. Edit shape: Confirmed, `dist/core/tools/edit.js` and `edit-diff.js`
   (0.84.4): `edit({path, edits:[{oldText,newText}, ...]})`. Each `oldText`
   must be unique in the ORIGINAL file; edits are matched against the original,
   not incrementally; overlapping edits are rejected; "no change" errors;
   exact match first, then fuzzy match on normalized text (NFKC, trailing
   whitespace stripped, smart quotes, Unicode dashes and spaces normalized);
   BOM and line endings preserved; legacy single `oldText/newText` args and
   JSON-string `edits` are coerced ("Some models (Opus 4.6, GLM-5.1) send edits
   as a JSON string"). Duplicate error: "Found N occurrences ... must be
   unique. Please provide more context". `read({path, offset?, limit?})`
   1-indexed, truncated at default lines/bytes with a continue-offset hint;
   `write({path, content})` creates parents, overwrites; `bash({command,
   timeout?})` seconds with a maximum.

### Mirrorable edit shapes (summary)

| Harness | Tool | Input | Match rule | Multi-edit | Fuzzy | Read-before-edit |
|---|---|---|---|---|---|---|
| Claude Code | `Edit` | `file_path, old_string, new_string, replace_all?` | exact, unique unless `replace_all` | one per call | none | required (model-dependent relaxation) |
| Codex | `apply_patch` (freeform) | `*** Begin Patch` grammar | context lines, exact then whitespace-relaxed `seek_sequence` | multi-file, multi-hunk, add/delete/move | whitespace only | none observed |
| pi | `edit` | `path, edits[{oldText,newText}]` | exact then normalized-fuzzy, unique in original, no overlap | yes, disjoint | yes (Unicode/whitespace) | none observed |
| agy | `edit_file`-family (binary strings) | `Target/Replacement` or `Edits[]`, `AllowMultiple` | exact, unique unless `AllowMultiple` | yes, ordered, all-or-nothing | not stated | not stated |

A ws edit tool mirroring Claude's shape plus pi's `edits[]` array (atomic,
original-file-relative) covers three of the four; Codex's patch grammar is the
outlier and would be a separate tool or a translating wrapper.

### Evidence and gap table

| Question | Codex 0.160.0 | Claude Code 2.1.325 | agy 1.5.9 | pi 0.84.4 / 1.5.2 |
|---|---|---|---|---|
| Per-tool disable | shell: yes (`features.shell_tool`); apply_patch: model-catalog only (source), no flag | yes (`--tools`, `--disallowedTools`, `permissions.deny`) | permission deny rules documented; `disableBuiltInTools` ACP param in binary (Unverified) | yes (`--tools`, `-xt`, `-nbt`, `-nt`, `defaultTools`) |
| MCP-only possible | likely, residual utility tools; Unverified live | yes, except `EndConversation` | Unverified | yes; MCP native only from 0.99.0 (bridge ext. in 0.84.4) |
| Non-removable | none found; apply_patch/utility tools unproven | `EndConversation` (harmless) | Unverified | none documented |
| Sandbox tied to native | yes (commands); MCP outside (official silent) | Bash sandbox; MCP outside (Unverified) | terminal sandbox (commands) | none exists |
| Approval prompts | MCP approval via annotations/`approval_mode` | `mcp__server__tool` rules, hooks | `mcp(server/tool)` rules | none by design |
| Diff UI | native patch events (Unverified); MCP not | Edit diff UI; MCP Unverified | `/diff` viewer; ACP edit diffs | `renderDiff` for native edit |
| File-state tracking | none observed | read-before-edit + checkpoints (native edit only) | Unverified | none observed; mutation queue |
| Checkpoint/rewind | Unverified | confirmed: Bash-only changes untracked | `/rewind` named, undefined | none found |
| Parallel calls | MCP only with read-only hint/opt-in | Unverified | Unverified | default parallel; queue needed for file mutation |
| Edit shape | `apply_patch` grammar (Confirmed) | exact `str_replace` (Confirmed) | binary strings only | `edits[]` exact+fuzzy (Confirmed) |

### Verdict per harness (desk only)

- Codex: shell can be removed by config; `apply_patch` and some utility tools
  have no documented toggle, so a pure MCP-only surface is not shown. Main
  blocker: `apply_patch` presence is catalog-driven. Sandbox does not cover MCP.
- Claude Code: yes, documented and enforceable (`--tools ""` plus
  `--disallowedTools` plus `--strict-mcp-config`); only `EndConversation`
  stays. Cost: checkpoints/rewind, read-before-edit, path-scoped Read/Edit
  rules and diff UI do not follow edits that go through MCP.
- agy: unknown. Docs offer allow/ask/deny only; a binary-level
  `disableBuiltInTools` ACP parameter is a lead, not evidence. Needs runtime
  check of an `agy acp` session.
- pi: yes, closest already. `--no-builtin-tools` is documented; this repo
  already strips `bash`/`read` from leads and bridges ws-mcp. No sandbox or
  approvals exist to lose; the loss is native diff rendering and the
  per-file mutation queue.

## Owner Hypothesis

Hypothesis: "tool-use skill matters more than familiarity with a specific tool
shape." Verdict: Not supported as stated; weakly contradicted for edit-format
effects in published data, though the size of the effect is contested and
shrinks for current models in mature harnesses. Evidence:

- Aider (`https://aider.chat/docs/unified-diffs.html`): GPT-4 Turbo scored 20%
  with SEARCH/REPLACE versus 61% with unified diff on a laziness benchmark of
  89 tasks (about 3x less lazy); GPT-4 0613 26% to 59%. Disabling flexible
  patching raised editing errors about 9x. Old models, but format moved results
  a lot. Confirmed (cited).
- Aider edit leaderboard
  (`https://aider.chat/docs/leaderboards/edit.html`, dated 2025-04-12): same
  model, different format, e.g. gemini-exp-1206 whole 80.5% vs diff 69.2%;
  gpt-4-turbo udiff 63.9% vs diff 57.6%. The page says aider is "configured to
  use the best edit format for the popular OpenAI and Anthropic models", i.e.
  best format is model-specific. Confirmed (cited).
- SWE-agent ACI ablation (`https://arxiv.org/html/2405.15793v3`, GPT-4 Turbo,
  SWE-bench Lite): full ACI 18.0%; edit without linting 15.0%; no edit command
  10.3%; shell-only 11.0% (7.33% without demonstration). Tool design shifts
  resolve rate by several points to a 64% relative gain. Confirmed (cited).
- Anthropic (`https://www.anthropic.com/engineering/swe-bench-sonnet`):
  string replacement "had the highest reliability"; tools were "error-proofed";
  no isolated ablation number. Confirmed (cited), effect size Unverified.
- Harness-problem post (`https://stencil.so/blog/the-harness-problem`, redirected
  from blog.can.ac, Feb 2026): 16 models, 3 runs x 180 synthetic React bug-fix
  tasks, patch vs replace vs hashline; hashline beat patch for 14 of 16 models,
  about +15 points on average; one model 6.7% to 68.3%. Vendor-published, not
  independently replicated, synthetic tasks, no variance reported. Cited as
  reported; treat as indicative only.
- Counter-evidence (`https://dev.to/nwyin/hashline-vs-replace-does-the-edit-format-matter-15n2`):
  20 tasks per language, 3 models; "Hashline vs replace is not a clear winner
  either way"; model gap (about 55-65% vs 90%+) exceeds format gap; fuzzy
  matching never triggered for replace mode. Small, "not statistically
  rigorous".
- Reading: published evidence supports that format/tool shape can be a first
  order effect for weak or older models, and a second-order effect for strong
  recent models, where the model gap dominates. It does not show that
  familiarity (training on a specific shape, e.g. `apply_patch` for GPT-5/Codex
  models, str_replace for Claude) is irrelevant: Codex's source comment says
  the freeform tool is "Well-suited for GPT-5 models", and pi's source coerces
  malformed `edits` from specific models (Opus 4.6, GLM-5.1), which are
  harness-side evidence that shape mismatches occur. Whether any vendor
  documents training on its edit tool shape (model cards): Unverified, not
  searched beyond the above. The hypothesis is also testable only by the
  paired experiment in Open Question 2; this desk pass supplies priors, not a
  verdict on ws-mcp tools.

## Unverified Items

1. Codex: existence/behavior of a supported way to remove `apply_patch`
   (model-catalog override at runtime), and whether any other tool-list
   toggles exist for `update_plan`, `request_user_input`, MCP resource tools;
   whether an empty native tool list errors; version where `shell_tool`
   toggle appeared; whether MCP calls run outside the sandbox (official docs
   silent); MCP parallel opt-in key name; native diff/undo coupling to
   `apply_patch`; multi-file atomicity of `apply_patch`.
2. Claude Code: introduction versions of `--tools`/`--disallowedTools`; a live
   listing of tools visible in an MCP-only config; MCP edits and checkpoint
   coverage; Bash sandbox scope versus MCP; MCP vs native parallel execution;
   diff rendering for MCP results.
3. agy: whether a deny rule hides a tool from the model; the built-in tool
   list from official docs; `disableBuiltInTools` ACP param semantics and
   version; what remains in an MCP-only agy; `/rewind` semantics; sandbox and
   parallel behavior for MCP; edit-tool semantics beyond binary strings;
   fuzzy matching and read-before-edit rules.
4. pi: runtime confirmation that a pure ws-mcp surface (no native
   read/grep/find/edit/bash) is complete in 0.84.4 and 1.5.2; whether the
   repo should migrate from the bridge extension to native MCP in 1.5.x;
   existence of file checkpoint features outside the docs read.
5. Owner hypothesis: vendor model-card statements on tool-shape training;
   independent replication of the harness-problem numbers; SWE-bench harness
   ablations other than SWE-agent's.

## Runtime checks (2026-11-15)

Owner approved runtime checks (board (960)=a). Scope: can each harness run with
all native tools disabled so only MCP tools remain. Versions: `codex-cli
0.160.0` (model `gpt-6-sol`), `agy 1.5.9` (default model of the logged-in
account). Every probe ran in a fresh `/tmp` dir with `< /dev/null`; no global
config was edited (`~/.codex/config.toml` and `~/.gemini` untouched; `agy mcp
list` is still empty). Pi and Claude were not probed here.

### Codex

Commands (all `codex exec --ephemeral --skip-git-repo-check -s read-only`):

- Model catalog override: `codex debug models` dumps the catalog JSON; copy it
  and, for every model, set `shell_type: "disabled"`, `apply_patch_tool_type:
  null`, `experimental_supported_tools: []`, `tool_mode:
  "non_code_mode_only"`, `multi_agent_version: null`, `supports_search_tool:
  false`; point at it with `-c model_catalog_json="/tmp/.../cat.json"`.
- Flags: `-c web_search="disabled" --disable view_image --disable goals
  --disable multi_agent --disable image_generation --disable apps --disable
  tool_suggest --disable skill_search --disable browser_use --disable
  in_app_browser --disable shell_tool --disable unified_exec`.
- Probe MCP: `-c 'mcp_servers.probe.command="python3"' -c
  'mcp_servers.probe.args=["/tmp/mcpsrv/srv.py"]'` (a 30-line stdio echo
  server with one tool `probe_echo`); `-c
  'mcp_servers.probe.default_tools_approval_mode="approve"'` was needed for
  the call to run (without it: "MCP tool call requires approval, but approval
  policy is never").

Observed tool lists (model self-report, prompt: list every tool verbatim):

| step | tools the model reported |
|---|---|
| baseline | `functions.exec/wait/request_user_input(+_async)`, `clock.sleep`, 6 `collaboration.*`, `tools.apply_patch/exec_command/write_stdin/view_image/web__run/image_gen__imagegen/clock__curr_time`, goal tools, MCP resource tools, `request_plugin_install` |
| catalog: shell off, apply_patch null | `exec_command`, `write_stdin`, `apply_patch`, `curr_time`, `request_user_input_async` gone; JS `functions.exec`/`wait` (code mode) remain |
| + `tool_mode: non_code_mode_only` | code-mode `exec`/`wait` gone; tools become direct `functions.*`; `tool_search` appears |
| + flags above | view_image, goals, web, image_gen, `request_plugin_install` gone |
| + `multi_agent_version: null` | `collaboration.*` gone |
| + `supports_search_tool: false` | MCP tools listed directly instead of deferred behind `tool_search` |

Final list with probe + ws plugin MCP attached: `functions.list_mcp_resources`,
`functions.list_mcp_resource_templates`, `functions.read_mcp_resource`,
`functions.request_user_input`, `mcp__probe.probe_echo`, and 156 listed
entries for the ws plugin (`mcp__ws.*`; the plugin MCP loaded in the temp
dir). The `probe_echo` call returned `echo:hi`.

Findings:

- `apply_patch` CAN be removed, but only through the model catalog
  (`model_catalog_json`, a supported config key); there is no feature flag for
  it. The same catalog route removes the shell (`shell_type: "disabled"`).
  The `features.shell_tool` / `features.unified_exec` toggles were only tried
  together with the catalog override, so their solo effect is not isolated;
  the catalog route is the verified one.
- Not removable by anything tried: `request_user_input` (the
  `collaboration_modes` feature is removed; `default_mode_request_user_input`
  is the opposite switch) and the three `*_mcp_resource*` tools (generic MCP
  plumbing, harmless for a MCP-only design).
- Without `supports_search_tool: false`, the catalog default defers MCP tools
  behind `tool_search`, which errored `unsupported call: tool_searchtool_search_tool`
  in the non-code-mode configuration: MCP tools were unusable until the
  catalog flag was cleared.
- Caveats: the catalog override replaces the shipped model catalog file for
  the process (it must be regenerated per Codex version; format is internal,
  unversioned); `codex debug prompt-input` renders messages, not the tool
  schema, so no schema dump was obtained; the lists above are model
  self-reports (weaker evidence than a schema dump), though the removed tools
  also stopped being callable. A sandbox/approval path still applies to MCP
  calls (annotation-less tools need the approval-mode key in `exec`).

Verdict: Codex - full replacement possible, with residual
`request_user_input` and MCP-resource tools, via an internal-format catalog
override (fragile across versions).

### agy

- `agy --help` has no tool flag. Built-in tool names (baseline, self-report):
  `read_file`, `run_command`, `manage_task`, `send_message`, `schedule`,
  `invoke_subagent`, `define_subagent`, `manage_subagents`, `write_file`,
  `edit_file`, `search_web`, `run_workflow`, `swarm`, `ask_question`.
- The binary string `Session asked to disable built-in tools; not supported by
  the default agent yet, keeping them` shows the ACP `disableBuiltInTools`
  request is ignored for the default agent. `--settings` rejects unknown keys
  (`mcpServers`, arbitrary keys) and has no tool key found.
- Working route: a project custom agent, `.agents/agents/bare.md`, with
  frontmatter `excludeDefaultComponents: true`, `tools: []` (or an explicit
  list), `enable_mcp_tools: true`, run as `agy --agent bare -p ...`. The
  changelog documents `excludeDefaultComponents` ("opt out of default prompt
  sections and built-in tools") and `tools` lists.
- Observed with `tools: []`: `manage_task` only (cannot be removed). With
  `enable_mcp_tools: true`: `call_mcp_tool`, `list_resources`,
  `read_resource`, `manage_task`. With `tools: [read_file]`, `read_file` is
  added, so the `tools` list is honored.
- MCP: `agy mcp add` writes the user-level `~/.gemini/config/mcp_config.json`
  (not touched). A fake `HOME` loses auth (login is not copyable), so the
  probe server was attached as a project plugin instead:
  `.agents/plugins/probe/{plugin.json,mcp_config.json}`. Plugin servers are
  namespaced `<plugin>_<server>`: `call_mcp_tool(ServerName="probe_probe",
  ToolName="probe_echo")` returned `echo:hi` with `--settings
  '{"permissions":{"allow":["mcp(*)"]}}'`. A wrong server name gives "tool
  probe_echo is not enabled for server ...".
- MCP tools are not separate model-visible tools: they go through the generic
  `call_mcp_tool` dispatcher (server name + tool name + args), unlike Codex.
  The model needs the server/tool names from the prompt or `list_resources`.
- Headless `-p` auto-denies anything not pre-allowed
  (`permissions.allow` in `--settings`); `--dangerously-skip-permissions` was
  not used.
- Caveats: self-report only, no schema dump; `agy` agent-file frontmatter
  keys were learned from changelog text, not a spec; project agents may need
  folder trust; the `manage_task` residual and the dispatcher shape remain.

Verdict: agy - partial: all file/shell/web tools can be removed via a custom
agent, but `manage_task` remains and MCP tools sit behind `call_mcp_tool`.

## Resolution
Triage 2026-11-15 (owner (1057)): DROP - 도구 표면 통합 대 하네스별 적응 비교 연구. 구체적 다음 단계 없음.
