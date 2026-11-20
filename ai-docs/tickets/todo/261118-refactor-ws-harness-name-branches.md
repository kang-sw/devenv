---
title: "공통 레이어의 하네스 이름 분기를 하네스별 등록 데이터로 이전"
parent: 261114-epic-ws-common-session-foundation
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "(143)=a introduces the harness descriptor on BackendRegistration that this cleanup extends"
  261118-research-ws-pi-hosted-connector: "stage 2 targets full common-interface support by Pi; this leak list is its baseline"
---

# Move harness-name branches out of the common layer into per-harness registration data

## Background

The harness common interface (epic `261114-epic-ws-common-session-foundation`)
is only partly isolated from the per-harness implementations. A code read of
`track/agent-v2` on 2026-11-18 (after merge `733201615`; paths relative to
`agents-plugin-tool/`) found:

- **Isolated:** process launch goes through the `wsagent.Backend` interface
  (`internal/wsagent/backend.go` L24-30; Claude, Pi and agy implementations);
  `StartSpec` and `Process` are neutral; the capability catalog
  (`internal/wsagent/catalog.go`) is a data table pinned by tests; backends
  are registered in one place (`cmd/ws-mcp/dashboard.go` L299-305) through
  `BackendRegistration` (`internal/wsagent/registration.go` L14-24).
- **Not isolated:** about 75 non-test `BackendClaude|BackendPi|BackendAgy`
  references sit outside `claude.go`, `pi.go` and `agy.go`, about 55 of them
  in harness-neutral `internal/wsagent` code. The code was written when only
  Claude and Pi existed; agy was added as a lead-only third value.

Significant sites:

| Site | Note |
|---|---|
| `internal/wsagent/runner_process.go` L53, 67, 78, 84, 151, 165, 172, 469 | Pi holder and attach logic inline in the runner |
| `internal/wsagent/runner_process.go` L159, 251, 353, 363, 375 | Claude and agy lead-launch branches (key file, HTTP vs stdio fallback, agy start-unfinished restart rule) |
| `internal/wsagent/runner.go` L422, 627, 768, 799, 1043, 1078 | Pi children, cwd, steering; L768 uses `!= BackendPi` as a proxy for "tier allowed" |
| `internal/wsagent/children.go` L366, 594, 821 | Pi gates on child state and parent kind |
| `internal/wsagent/runner_conv.go` L946 | Pi holder scan check |
| `internal/wsagent/runner_summary.go` L33 | `Backend != BackendPi` summary source |
| `internal/wsagent/permission.go` L39, 54, 57, 72, 95, 180, 274 | Claude/Pi permission logic; L95 hardcodes `(claude \|\| pi)` and so excludes agy by name |
| `internal/wsagent/handoff.go` L93, 153, 160, 299-304 | per-backend handoff and resume commands (`switch rec.Backend`) |
| `internal/wsagent/registration.go` L77-86 | `permissionModes` fallback switch in common code |
| `internal/wsagent/builtin.go` L48, 171-244 | hand-built per-backend tables (search tools, support and lead profiles) |
| `internal/wsagent/agent_tools.go` L249 | Pi-only `ChildCheck` |
| `internal/mcp/playbook_tools.go` L102, 163, 171, 1000 | `harness == "claude"` spawn idiom, Claude/Pi record checks, `harness == "pi"` daemon agents |
| `internal/mcp/summary_owner.go` L18 | `currentHarness() == "pi"` |
| `internal/mcp/server_harness.go` L89, 101-107, 121-129 | closed harness list for clientInfo detection |

Sites the agy ticket replaces under (143)=a (`agentWorkerBackends`,
`renderLeadManual`'s Claude `if`, `advertisedToolName`) are out of this
ticket's list once that ticket lands.

## Decisions

- **(144)=a, 2026-11-18: capture only.** The cleanup is captured in `todo/`
  under the epic and executed later; Claude usage is scarce this week.
  Rejected: no ticket, moving sites only as feature work touches them.

## Constraints

- Manuals for the touched paths: `ai-docs/manuals/performance.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/ws-mcp.md`.
- The approach (which descriptor fields, which sites move, order) is settled
  at promotion; the harness descriptor from
  `261118-feat-ws-agy-tool-names-and-worker-role` is the existing seam.
- Go tests run on remote w5090 only.

## Phases

### Phase 1: Plan at promotion

The phase plan is written when the ticket is promoted to `ready/`.
