# Plan: Pi worker/explore children inherit a stale shell WS_MCP_BOOTSTRAP_BINARY and force-reinstall the wrong ws-mcp — Phase 1: Neutralize stale bootstrap overrides on every child launch

## Relevant Ticket Contract

- Sanitize a copied child environment for `WS_MCP_BOOTSTRAP_BINARY` and `WS_MCP_BOOTSTRAP_URL` on one-shot exploration, persistent worker/explore, fork, and dormant-resume launches. Never mutate `process.env`; retain every unrelated inherited value and the lead's deliberately injected local-devenv bootstrap choice.
- At the RPC boundary, supply explicit empty values because `RpcClient` merges its option environment over the parent environment. At a direct `spawn` boundary, remove the two keys from the copied full environment.
- A child may still use its own valid local-devenv marker and the normal compatible-cache/release path. This Pi-track change does not repair the separate released tool-contract mismatch or modify launcher/shared ws-mcp code.
- Update `pi-adapter-runtime` to state the child-launch invariant. Verify actual direct-child and effective RPC merged environments with stale overrides plus an unrelated sentinel, including dormant resume and compatible-install reuse.

## Out of Scope

- `agents-plugin-pi/bin/ws-mcp-launcher.py`, shared `agents-plugin*/` launcher copies, `runtime.json`, ws-mcp Go code, release publishing, and the independently recorded v0.45.2 tool-contract mismatch.
- Changing lead/fork local-devenv selection in `agents-plugin-pi/src/bridge.ts` or adding a new environment/configuration knob.
- Real Pi dogfood acceptance beyond recording it as the ticket's post-build check.

## Codebase Findings

- `agents-plugin-pi/src/spawner.ts#L151-L157` and `#L653-L659` — `buildChildProcessEnv` already makes a copied direct-spawn environment for the terminal one-shot collection/explore leaf; it preserves inherited variables, clears deep mode, and is the direct-path sanitization seam.
- `agents-plugin-pi/src/spawner.ts#L1952-L1989` — `buildRpcClientOptions` centrally builds options for persistent worker, fork, persistent explore, and dormant resume. Its existing explicit-empty role markers document the exact merge-over-parent pattern needed for both bootstrap keys.
- `agents-plugin-pi/src/spawner.ts#L2599-L2621` and `#L2727-L2745` — both initial RPC creation and `sendToAgent`'s dormant-resume branch consume `buildRpcClientOptions`, so a single policy there reaches fresh workers/forks/explores and resume without per-call-site drift.
- `agents-plugin-pi/src/mcp-stdio-client.ts#L167-L197` — bridge-launcher environment merging is a separate, lead-side mechanism and is outside this phase; it must remain untouched so intentional local-devenv bootstrap injection continues to work.
- `agents-plugin-pi/test/spawner.test.ts#L3150-L3245` — existing pure-helper assertions cover RPC option marker placement and direct child environment preservation, providing the focused regression-test homes without spawning Pi.
- `ai-docs/spec/pi-adapter-runtime.md#L343-L349` — current runtime text documents only the lead/fork launcher-child bootstrap injection and needs the complementary child-sanitization invariant.
- Risk signal: the direct and RPC launch APIs have different environment semantics. Deleting keys from the RPC options object would leave stale parent values effective because `RpcClient` merges options over `process.env`; tests must assert the effective merged result, not only key presence in the options fragment.

## Implementation Plan

1. In `agents-plugin-pi/src/spawner.ts`, centralize the two bootstrap-override names with the existing child-environment policy and make `buildChildProcessEnv(baseEnv)` clone then delete them before adding the explore role marker. Preserve all other entries and the existing deep-mode clearing.
2. In `agents-plugin-pi/src/spawner.ts`, add both bootstrap keys as explicit empty strings in `buildRpcClientOptions` alongside the existing empty inherited-marker overrides. Keep this helper as the sole policy point so fresh worker, fork, persistent explore, and dormant resume receive identical effective environment sanitization.
3. Extend `agents-plugin-pi/test/spawner.test.ts` around the existing pure helper suites. Seed stale binary and URL values plus an unrelated sentinel; assert direct child output removes only the stale keys and leaves the input object unchanged. Assert RPC options contain empty overrides and model the SDK's parent-plus-options merge to prove the launched process sees ineffective values, the sentinel survives, and the parent object remains unchanged. Cover worker, fork, persistent explore, and dormant resume through the shared builder/call-site harness where its role selection differs.
4. Add a fixture-launcher regression in the relevant `agents-plugin-pi/test/` launcher/client test seam to begin with stale parent overrides, invoke a child-style sanitized environment, and assert the selected compatible install is reused rather than replaced by the stale binary. Keep it isolated from the unresolved public-release tool mismatch.
5. Update `ai-docs/spec/pi-adapter-runtime.md` with the precise child-launch rule: parent-shell binary/URL force-install overrides are neutralized for child direct and RPC launches, while a child's valid marker discovery and the lead's scoped local-devenv injection remain intact.

## Verification Plan

- Run the focused `agents-plugin-pi/test/spawner.test.ts` and the fixture-launcher/client test added for stale bootstrap reuse through `npm test` from `agents-plugin-pi/`; read complete output.
- Run `npm test` from `agents-plugin-pi/` after the focused tests pass.
- Manually inspect the direct-spawn copied environment and the effective RPC merged environment for both stale keys, an unrelated sentinel, fresh worker/fork/persistent-explore construction, one-shot collection, and dormant resume; confirm `process.env` and intentional lead bootstrap selection are unchanged.
- Treat real Pi dogfood with stale shell overrides as the separate post-build acceptance check; record the public-release mismatch independently if the env-less release path still cannot satisfy the launcher tool check.

## Escalations

- None.
