# Review dispositions — 260907 persistent exploration, cycle 1

Scope: findings over `1083e01a..048a9cb2`; live dogfood remains an owner-run gate.

## Correctness

- **Correctness C1 — [fixed].** `verifyResearchSelection` reads `RpcClient.getState()` after startup/effort application and before every researcher prompt. Simple captures Pi's actual default or clamped thinking level once; deep and every resume require the stored model/effort exactly. Startup/resume mismatch follows common cleanup and sends no prompt. Evidence: `agents-plugin-pi/src/spawner.ts`; `persistent-explore.test.ts` covers empty small effort, clamping, strict deep mismatch, and two sidecar-restored resumes.
- **Correctness C2 — [fixed].** The non-lead leaf selects `recon` for worker/execute-worker process callers and `read-only` only for a deep researcher. Evidence: `src/spawner.ts#registerAgentTools`; registered wrapper tests assert both profiles and the forwarded small effort.
- **Correctness I1 — [fixed].** Sidecar parsing now treats role, mode, and read-only research groups as one strict tuple, rejects invalid/missing mode, wrong role/group, explicit tools, missing frozen selection, unknown roles, and unknown groups. Evidence: `src/agent-sidecar.ts#parseOrphans`; `agent-sidecar.test.ts` corrupt-tuple matrix and two-cycle round trip.
- **Correctness I2 — [fixed].** The runtime spec replaces the contradictory one-shot/recon/inheritance lead/fork text with persistent simple/deep selection, sidecar, lifecycle, collection, effort, and widget behavior. Evidence: `ai-docs/spec/pi-adapter-runtime.md` affected exploration, curation, depth, model-resolution, delegation, and widget anchors; stale one-shot exploration search is empty.

## Fit

- **Fit I1 — [fixed].** Same implementation as Correctness C2; ordinary worker and execute-worker collection retain recon/bash.
- **Fit I2 — [fixed].** Same documentation correction as Correctness I2; all named stale anchors now describe persistent two-mode exploration and forwarded collection effort.

## Test

- **Test I1 — [fixed].** `persistent-explore.test.ts` invokes registered tool wrappers and real `spawnAgent`/dormant `sendToAgent` code with a process-free `RpcClient` transport fake. It covers public schema, lead/fork/worker/deep/simple/invalid-mode registration, immediate result shape, dispatch, profiles, and a Pi-style dynamic-registration/allowlist fixture. A no-model runtime probe independently registered a dynamic tool and confirmed `--tools read,probe-dynamic-tool` activated exactly `['read', 'probe-dynamic-tool']`.
- **Test I2 — [fixed].** Registered simple and deep-collection handlers cover accepted small, no small lookup for deep creation, transport/MCP/parse/non-Pi/unknown/no-auth/empty/malformed/throwing catalog failures, one-resolution refusal ordering, held alias/registry/no-directory artifacts, and no collection leaf on refusal.
- **Test I3 — [fixed].** Sidecar tests cover simple/deep role/mode/group/model/effort preservation through two capture/restore cycles and corrupt rejection. Persistent wrapper coverage resumes the restored simple researcher twice through `sendToAgent` and verifies the frozen effective effort.
- **Test I4 — [fixed].** The package `test` script explicitly strips inherited `WS_PI_SPAWN_ROLE` and `WS_PI_EXPLORE_MODE`. Evidence: clean `npm test` and inherited `WS_PI_SPAWN_ROLE=worker WS_PI_EXPLORE_MODE=deep npm test` each completed 946/946.

## Minor maintenance comments

- **Fit Minor — [fixed alongside required docs].** Updated stale one-shot comments in `spawner.ts`, `agent-sidecar.ts`, `agent-widget.ts`, and `process-role.ts`; no independent relay scope was opened.

## Verification evidence

- `cd agents-plugin-pi && npm test`: **PASS**, 946 tests / 166 suites / 0 failures (full log: `/tmp/pi-test-clean.log`).
- `cd agents-plugin-pi && WS_PI_SPAWN_ROLE=worker WS_PI_EXPLORE_MODE=deep npm test`: **PASS**, 946 tests / 166 suites / 0 failures (full log: `/tmp/pi-test-inherited-worker-deep.log`).
- No-model dynamic allowlist probe: **PASS**, active tools were exactly `read, probe-dynamic-tool`; no provider prompt was issued.
- `cd agents-plugin-pi && npm pack --dry-run`: **PASS**, package includes `explore-guide.md`.
- Owner live dogfood: **pending**. It must exercise simple/deep settle and follow-up, deep no-bash collection, stop/resume, session reload restoration, parent retuning, unavailable small/no-artifact refusal, and worker/execute-worker recon leaves against a real provider.
