# Plan: ws-agent-list shows each agent's model, and a revived orphan keeps its last-report time — Phase 1: List model and keep last-report time across revival

## Relevant Ticket Contract

- **D4 fix (model field).** `ws-agent-list` gains a `model` field per entry:
  `modelBase` plus `/<effort>` when `modelEffort` is set; bare `modelBase` when
  no effort; omitted only when the record has no `modelBase` at all (pre-field
  sidecar). It reports the *effective resolved* model — an inheriting child
  shows the parent's concrete model by name, not an absent field.
- **Staleness note (ticket text vs. current code).** The ticket names
  `resolveModelForAlias` and a "catalog"; that resolver is now
  `resolveModelForAliasViaWsMcp` (`src/spawner.ts:382-406`), which resolves
  through ws-mcp's `config.resolve_agent` tool and falls back to
  `ctx.inheritModel` (the parent's concrete model) on any non-hit. The
  Decision's substance is unchanged — only the name and mechanism moved. The
  tool description text must say "the model the agent runs on; an inheriting
  child shows its parent's model", with no mention of a catalog.
- **H fix (last-report fidelity across revival).** `RpcAgentRecord` gains an
  optional `lastReportAtOverride` (ISO string) that `rehydrateOrphanRecord`
  fills from the sidecar's already-persisted `PersistedOrphan.lastReportAt`.
  `ws-agent-list`'s `last_report_at` derivation prefers the newest `reportLog`
  entry and falls back to the override. The registry-cap eviction activity
  score (`max(lastLeadPromptAt, newest reportLog.at)`) consumes the same
  fallback, so a revived orphan is scored by its real last activity instead of
  as never-active.
- **No sidecar shape change.** `modelBase`, `modelEffort`, `lastReportAt` are
  already persisted; only the read side (`rehydrateOrphanRecord`) and the two
  listing/eviction consumers change.
- **Constraints.** Listing's existing fields/ordering are unchanged (additive
  only). A record that has reported since revival must show the real newest
  report time, never the stale override.
- **Six required tests (ticket phase text, verbatim intent):**
  1. a record with `modelBase` and `modelEffort` lists `model: "<base>/<effort>"`
  2. a record with `modelBase` only lists the bare base
  3. a record with neither has no `model` key
  4. a rehydrated orphan lists the sidecar's `last_report_at`
  5. a rehydrated orphan that reports afterwards lists the new time
  6. eviction prefers to drop a never-active record over a revived orphan
     whose override is newer
- **Spec impact.** Amend `ai-docs/spec/pi-adapter-runtime.md`'s
  delegation-spawner anchor's `ws-agent-list` entry (add `model`) and the
  shutdown-sidecar paragraph (revived record keeps its last-report time in
  both the listing and the eviction score).

## Out of Scope

- Rejected-alternative provenance flag (inherited-vs-explicit) — ticket
  explicitly rejects this.
- Injecting a fake `reportLog` entry for a revived orphan — ticket explicitly
  rejects this.
- Any sidecar file-shape change (`PersistedOrphan`/`serializeOrphans`/
  `parseOrphans`) — the sidecar already carries everything needed; only the
  read side changes.
- `agents-plugin-tool/` and `agents-plugin/skills/` — this is an adapter-only
  change under `agents-plugin-pi/` plus the one named spec file.
- The ticket's related sibling
  (`260905-feat-ws-pi-agent-alias-park-and-registry-cap`) and any other phase —
  not touched here.
- Live checks (no-catalog dual-spawn comparison, catalog-configured
  comparison, `/reload` last-report survival) — owner-run, out of band.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts:382-406` — `resolveModelForAliasViaWsMcp`
  (the renamed resolver): calls `config.resolve_agent` via ws-mcp,
  `{ model: inheritModel }` on any miss/error/non-`"pi"` `resolved_from`. This
  is what makes an inheriting child's `modelBase` equal to the parent's
  concrete model string — no separate provenance signal needed for `model` to
  read correctly.
- `agents-plugin-pi/src/spawner.ts:728-963` — `RpcAgentRecord` interface.
  `modelBase?: string` at line 757, `modelEffort?: string` at line 759,
  `reportLog: AgentReportLogEntry[]` at line 875 (preceded by its doc comment
  from 867). Add `lastReportAtOverride?: string;` right after line 875,
  documented as sidecar-revival-only, and noting `listAgents`/
  `evictForCapacity` prefer the newest `reportLog` entry over it.
- `agents-plugin-pi/src/spawner.ts:2416-2450` — `listAgents`. Current body
  (2438-2449):
  ```
  export function listAgents(
    registry: RpcAgentRegistry,
    opts?: { includePrompt?: boolean },
  ): Array<{ agent_id: string; status: AgentStatus; alias?: string; title?: string; last_report_at?: string; prompt?: string }> {
    return [...registry.entries()].map(([agentId, record]) => {
      const lastReport = record.reportLog[record.reportLog.length - 1];
      return {
        agent_id: agentId,
        status: ...,
        ...(record.alias ? { alias: record.alias } : {}),
        ...(record.title ? { title: record.title } : {}),
        ...(lastReport ? { last_report_at: new Date(lastReport.at).toISOString() } : {}),
        ...(opts?.includePrompt && record.prompt ? { prompt: record.prompt } : {}),
      };
    });
  }
  ```
  Needs: (a) a `model` field added to the return-type object literal and to
  the mapped object, computed from `record.modelBase`/`record.modelEffort`;
  (b) the `last_report_at` derivation falling back to
  `record.lastReportAtOverride` when `reportLog` is empty.
- `agents-plugin-pi/src/spawner.ts:2734-2753` — the `ws-agent-list`
  `pi.registerTool` call. Current description (line 2738, one string):
  `"List every tracked agent_id, its alias/title (when set), status
  (running/idle/dormant — most agents park to dormant shortly after settling,
  so idle is transient), and last_report_at (ISO, absent if it has never
  reported). Use it to check on a quiet agent — there is no wait tool; every
  report, question, approval request and completion is pushed to you as a
  ws-agent-* message on its own."` Needs a `model` clause inserted, phrased
  exactly per the Decision: "the model the agent runs on; an inheriting child
  shows its parent's model" — no mention of "catalog".
- `agents-plugin-pi/src/spawner.ts:2079-2113` — `evictForCapacity`. The
  activity score is line 2097:
  `const activity = Math.max(record.lastLeadPromptAt ?? 0, record.reportLog.at(-1)?.at ?? 0);`
  Needs the `reportLog.at(-1)?.at` half to fall back to
  `Date.parse(record.lastReportAtOverride)` when `reportLog` is empty and an
  override is present, before feeding into the same `Math.max`.
- `agents-plugin-pi/src/agent-sidecar.ts:51-80` — `PersistedOrphan`. Already
  carries `modelBase?`, `modelEffort?` (60-61) and `lastReportAt?: string`
  (78-79, "ISO time of the newest `reportLog` entry at shutdown; omitted when
  the child never reported"). No shape change needed here.
- `agents-plugin-pi/src/agent-sidecar.ts:199-218` — `rehydrateOrphanRecord`.
  Current body constructs the dormant `RpcAgentRecord` without any
  `lastReportAtOverride` field. Add
  `lastReportAtOverride: orphan.lastReportAt,` to the returned object
  (straight passthrough — both are already ISO strings, no conversion).
- `agents-plugin-pi/test/spawner.test.ts:601-613` — `freshRpcRecord` test
  helper (plain `Partial<RpcAgentRecord>` overrides merged over sane
  defaults); once `lastReportAtOverride` exists on the interface, tests can
  set it directly via `freshRpcRecord({ lastReportAtOverride: "..." })` with
  no helper change needed.
- `agents-plugin-pi/test/spawner.test.ts:2051-2121` — `describe("listAgents")`.
  Existing pattern at 2080-2091 (`last_report_at` omitted/present) and
  2099-2108 (`alias`/`title` inclusion) is the template for the three
  `model`-field tests (1-3 above) and the "override survives without any
  report" happy path (test 4, if placed here instead of agent-sidecar.test.ts
  — see Implementation Plan step 5 for the recommended split).
- `agents-plugin-pi/test/spawner.test.ts:2561-2643` — `describe("evictForCapacity")`.
  Existing pattern at 2586-2595 (`last-activity is max(...)`) is the direct
  template for test 6 — build two `freshRpcRecord`s purely with
  `freshRpcRecord` (no need to go through `rehydrateOrphanRecord`): one
  never-active (`{}`, activity 0) and one with only
  `lastReportAtOverride` set to a later ISO time, then assert
  `evictForCapacity` evicts the never-active one.
- `agents-plugin-pi/test/agent-sidecar.test.ts:36-42` — module's own import of
  `RpcAgentRecord`/`RpcAgentRegistry`/`applyRpcEvent` from `../src/spawner.ts`;
  add `listAgents` to this import for the cross-module rehydrate→list tests.
- `agents-plugin-pi/test/agent-sidecar.test.ts:281-311` —
  `describe("rehydrateOrphanRecord")`. Existing "rebuilds a DORMANT record"
  test (282-297) is the template: extend or add tests here for #4 (rehydrate
  with `lastReportAt` set on the orphan, then `listAgents([...])` shows that
  ISO as `last_report_at`) and #5 (same rehydrated record, then push a new
  entry onto `revived.reportLog` — e.g. `revived.reportLog.push({ at: Date.now() })`
  — and assert `listAgents` now shows the new time, not the override).
- `ai-docs/spec/pi-adapter-runtime.md:276-283` — the `ws-agent-list` bullet in
  the delegation-spawner anchor (`{#260903-pi-delegation-spawner-tools}`):
  "enumerate registry members with their status, alias and title. Status
  vocabulary is `running`/`idle`/`dormant`, ...". Needs `model` added to the
  enumerated field list with the same "effective resolved model; inheriting
  child shows the parent's" framing as the tool description.
- `ai-docs/spec/pi-adapter-runtime.md:310-338` — the shutdown-sidecar
  paragraph in the same anchor. Line 320-321 already says the sidecar
  captures "its state at shutdown (...) and its last-report time"; the
  paragraph needs one added sentence saying a revived record's last-report
  time is not lost — it surfaces again through `ws-agent-list`'s
  `last_report_at` and the registry-cap eviction score, until the record
  reports again for real.

## Implementation Plan

1. `agents-plugin-pi/src/spawner.ts` — add `lastReportAtOverride?: string;` to
   `RpcAgentRecord` (after line 875's `reportLog` field), with a doc comment
   naming this ticket, stating it is sidecar-revival-only and that
   `listAgents`/`evictForCapacity` prefer a real `reportLog` entry over it.
2. `agents-plugin-pi/src/spawner.ts` — in `listAgents` (2435-2450): compute
   `model` from `record.modelBase`/`record.modelEffort`
   (`modelBase` alone, or `` `${modelBase}/${modelEffort}` `` when
   `modelEffort` is set; `undefined` when `modelBase` is absent) and splice it
   into the returned object (immediately after `title`, before
   `last_report_at`, matching the ticket's field-order framing); compute
   `last_report_at` from the newest `reportLog` entry, falling back to
   `record.lastReportAtOverride` when `reportLog` is empty. Add `model?:
   string` to the function's return-type annotation.
3. `agents-plugin-pi/src/spawner.ts` — update the `ws-agent-list`
   `registerTool` description (line ~2738) to mention `model`: "the model the
   agent runs on; an inheriting child shows its parent's model" — no
   "catalog" wording.
4. `agents-plugin-pi/src/spawner.ts` — in `evictForCapacity` (2090-2113),
   change the activity computation (line 2097) so the `reportLog` half falls
   back to `Date.parse(record.lastReportAtOverride)` (treated as `0` when
   `lastReportAtOverride` is absent) when `reportLog` is empty, before the
   `Math.max` with `lastLeadPromptAt`.
5. `agents-plugin-pi/src/agent-sidecar.ts` — in `rehydrateOrphanRecord`
   (199-218), add `lastReportAtOverride: orphan.lastReportAt,` to the returned
   record (direct passthrough of the already-ISO `PersistedOrphan.lastReportAt`).
6. Tests in `agents-plugin-pi/test/spawner.test.ts`, inside
   `describe("listAgents")` (2051-2121): add three tests for the `model`
   field (ticket tests 1-3): `modelBase`+`modelEffort` → `"<base>/<effort>"`;
   `modelBase` only → bare base; neither → no `model` key.
7. Test in `agents-plugin-pi/test/spawner.test.ts`, inside
   `describe("evictForCapacity")` (2561-2643): add ticket test 6 — a
   never-active `freshRpcRecord({ agentId: "never" })` (activity 0) versus a
   `freshRpcRecord({ agentId: "revived", lastReportAtOverride: <later ISO> })`
   (no `reportLog`, no `lastLeadPromptAt`); `evictForCapacity(registry, 2)`
   must evict `"never"`.
8. Tests in `agents-plugin-pi/test/agent-sidecar.test.ts`: add `listAgents` to
   the existing spawner.ts import (line ~41); inside or beside
   `describe("rehydrateOrphanRecord")` (281-311), add ticket tests 4-5:
   (4) `rehydrateOrphanRecord({ ..., lastReportAt: <ISO> })` then
   `listAgents(new Map([["a", revived]]))` shows `last_report_at: <ISO>`;
   (5) same revived record, push a new entry onto `revived.reportLog`
   (`{ at: <later epoch ms> }`), then `listAgents` shows the new ISO time
   instead of the override.
9. `ai-docs/spec/pi-adapter-runtime.md` — amend the `ws-agent-list` bullet
   (276-283) to list `model` alongside status/alias/title, phrased as the
   effective resolved model with parent-inheritance display.
10. `ai-docs/spec/pi-adapter-runtime.md` — amend the shutdown-sidecar
    paragraph (310-338) with a sentence that a revived record's last-report
    time survives into the listing and the eviction score, not just into the
    sidecar file itself.

## Verification Plan

- `cd agents-plugin-pi && npm test`
- Live checks (owner-run, out of band, per ticket): no-catalog dual spawn
  (`complex:true` vs. default `ws-execute` worker) both list the lead's own
  concrete `model`; a configured catalog entry for the complex alias then
  makes the two `model` values differ; `/reload` and confirm `last_report_at`
  survives for a revived record.

## Escalations

- None.
