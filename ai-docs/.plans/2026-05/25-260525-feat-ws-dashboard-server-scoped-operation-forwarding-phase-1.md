# Survey: 25-260525-feat-ws-dashboard-server-scoped-operation-forwarding-phase-1

## Reusable Components
- `ws-dashboard/frontend/src/resourceModel.ts#L20-L25` — `ResourcePath`: existing server/workspace/workRoot/instance identity tuple; it is the brief-required type for helpers that can accept full resource identity.
- `ws-dashboard/frontend/src/routeBasis.ts#L1-L33` — `serverRoutePath`/`normalizeServerRouteLocation`: current frontend server-route encoding and browser-route normalization pattern for `/servers/:serverId` chrome.
- `ws-dashboard/frontend/src/resourceRefresh.ts#L14-L29` — `requestDashboardResources`: already defaults local resources to `/api/dashboard/resources` and uses `/api/dashboard/servers/{serverId}/resources` for non-local servers.
- `ws-dashboard/frontend/src/rootPicker.ts#L46-L58` — root-picker endpoint constants plus `rootPickerListEndpoint`: local compatibility route constants and query encoding for authenticated host-path request data.
- `ws-dashboard/frontend/src/openWorkRoot.ts#L4-L33` — `openWorkRootEndpoint`/`requestOpenWorkRoot`: current open-workRoot POST wrapper, response JSON parsing, and daemon-opened-id header extraction.
- `ws-dashboard/frontend/src/workRootFiles.ts#L162-L177` — `workRootFilesEndpoint`/`workRootFileReadEndpoint`: workRoot route helper pattern with `encodeURIComponent` and `URLSearchParams` for relative paths.
- `ws-dashboard/frontend/src/workRootFiles.ts#L215-L264` — document events/write helpers: current document SSE/write route construction and fetch wrapper shape.
- `ws-dashboard/frontend/src/workRootActivity.ts#L139-L150` — `workRootActivityEventsEndpoint`: Activity SSE endpoint helper with optional `after` cursor query.
- `ws-dashboard/frontend/src/workRootActivity.ts#L271-L302` — Activity snapshot/transcript helpers: current workRoot/activity id route construction plus optional cursor/before/limit query handling.
- `ws-dashboard/frontend/src/terminals.ts#L91-L127` — terminal HTTP/WebSocket endpoint helpers: create/list/output/input/resize/socket/close route strings plus WebSocket path helper.
- `ws-dashboard/frontend/src/gitToolbar.ts#L57-L109` — Git toolbar route wrapper pattern: one `gitBase(workRootId)` helper feeds status/branches/switch/fetch/push/pull fetches.
- `ws-dashboard/frontend/src/gitWorktreeAdd.ts#L49-L131` — Git worktree add helpers: workspace-scoped base helper for options/preview/submit and custom target path confined to request body.
- `ws-dashboard/frontend/src/commands.ts#L45-L90` — `DashboardCommandPayload`: centralized payload contract for workspace/workRoot/activity/document/terminal command identity.
- `ws-dashboard/frontend/src/commands.ts#L115-L240` — command builders: current builders intentionally omit root-picker/open host paths while carrying opaque workspace/workRoot ids.
- `ws-dashboard/frontend/src/commands.ts#L242-L360` — file/activity/document/terminal command builders: useful single place to extend affected command payloads without adding inline component payloads.

## Existing Patterns
- Server-scoped resource route compatibility: see `ws-dashboard/frontend/src/resourceRefresh.ts#L14-L29` and `ws-dashboard/frontend/src/resourceRefresh.test.ts#L79-L103` — local `server-local` keeps the legacy route while linked server ids use encoded canonical server routes.
- Browser route helper tests are pure TypeScript assertion scripts: see `ws-dashboard/frontend/src/routeBasis.test.ts#L13-L37` — route helpers are tested by direct function calls and exact string assertions.
- Root picker host-path privacy tests: see `ws-dashboard/frontend/src/rootPicker.test.ts#L51-L60` and `ws-dashboard/frontend/src/commands.test.ts#L258-L270` — URLs may carry picker paths as authenticated request queries, but command payload/loggable JSON omits host paths.
- Open-workRoot selection authority test: see `ws-dashboard/frontend/src/openWorkRoot.test.ts#L75-L108` — the wrapper uses the opened workRoot id response header rather than deriving selection from labels or row order.
- File pane identity tests: see `ws-dashboard/frontend/src/workRootFiles.test.ts#L120-L151` — pinned files are `editor/<workRootId>/<path>`, preview is `editor-preview/<workRootId>`, and pane ids encode workRoot/path.
- File pane persistence tests: see `ws-dashboard/frontend/src/workRootFiles.test.ts#L278-L390` — restore state stores descriptors/order only, reloads as loading panes, and rejects absolute/traversing paths.
- Activity stale-response guards: see `ws-dashboard/frontend/src/workRootActivity.test.ts#L408-L423` and `ws-dashboard/frontend/src/workRootActivity.test.ts#L826-L868` — stream/transcript application already checks workRoot/activity/request tuples.
- Terminal identity and restore tests: see `ws-dashboard/frontend/src/terminals.test.ts#L56-L66` and `ws-dashboard/frontend/src/terminals.test.ts#L140-L204` — terminal panes/restores are keyed by workRoot plus terminal/tab context, with malformed storage degrading to empty.
- Git route privacy tests: see `ws-dashboard/frontend/src/gitToolbar.test.ts#L43-L64` and `ws-dashboard/frontend/src/gitWorktreeAdd.test.ts#L52-L74` — fetch wrappers are validated for route shape and absence of private host paths in URLs.
- Command dispatch coverage: see `ws-dashboard/frontend/src/commands.test.ts#L64-L103`, `ws-dashboard/frontend/src/commands.test.ts#L215-L255`, and `ws-dashboard/frontend/src/commands.test.ts#L300-L350` — a migrated-command list, handler dispatch, and privacy checks cover payload shape changes.

## Relevant Interfaces
- `ai-docs/spec/ws-web-dashboard/index.md#L212-L245` — server-scoped operation forwarding contract: canonical route families, local compatibility aliases, serverId in pane/source/stream/terminal/command/persisted identities, and no simple proxying for SSE/WebSocket.
- `ai-docs/mental-model/ws-web-dashboard.md#L40-L45` — ownership map for route helper modules, routeBasis, resourceRefresh, and workbench placement helpers.
- `ai-docs/mental-model/ws-web-dashboard.md#L61-L67` — vocabulary and open/root-picker/Git worktree invariants, including host-path privacy and daemon-returned opened ids.
- `ai-docs/mental-model/ws-web-dashboard.md#L71-L82` — file, document, Activity, and terminal route constraints that Phase 1 helper/identity changes must preserve.
- `ws-dashboard/frontend/src/resourceModel.ts#L58-L70` — `WorkRootView.resourcePath`: every workRoot already carries the full `ResourcePath` available to pass into server-scoped helpers.
- `ws-dashboard/frontend/src/resourceModel.ts#L72-L90` — `InstanceView.resourcePath`: instance-like surfaces also carry full server/workspace/workRoot/instance identity.
- `ws-dashboard/frontend/src/App.tsx#L286-L315` — inline activation/workspace-removal API calls in `App.tsx`; these are route-helper candidates because they build server-sensitive URLs inside a component.
- `ws-dashboard/frontend/src/App.tsx#L328-L377` — selected server state and resource refresh coordinator; route helpers called from App can use `selectedServerIdRef.current` or resource paths already in selected WorkRoot data.
- `ws-dashboard/frontend/src/App.tsx#L426-L465` — open-workRoot response reconciliation updates selected server from returned resources and normalizes browser route.
- `ws-dashboard/frontend/src/App.tsx#L600-L690` — read-only file pane open path uses workRoot id for pane logical keys and placement state.
- `ws-dashboard/frontend/src/App.tsx#L2348-L2376` — file explorer snapshots are stored by bare `workRoot.id` and fetch files with bare workRoot id.
- `ws-dashboard/frontend/src/App.tsx#L2731-L2786` — workbench terminal/activity state maps are currently keyed by bare pane/workRoot/activity identity.
- `ws-dashboard/frontend/src/App.tsx#L3018-L3070` — Activity EventSource and snapshot fallback use `workRootActivityEventsEndpoint(rootId)` and bare workRoot/request identity.
- `ws-dashboard/frontend/src/App.tsx#L3254-L3273` — document EventSource applies document events by bare workRoot id before refreshing open documents.
- `ws-dashboard/frontend/src/App.tsx#L3290-L3335` — terminal output polling records terminal id/logical key/sequence without server id.
- `ws-dashboard/frontend/src/App.tsx#L3436-L3452` — terminal restore persistence is updated per bare workRoot id.
- `ws-dashboard/frontend/src/App.tsx#L4720-L4780` — terminal workbench placement logical keys are derived from `persistentTerminal/workRootId/terminalId`.
- `ws-dashboard/frontend/src/App.tsx#L5294-L5318` — read-only file placement reconstructs workbench logical keys by splitting pane logical keys.
- `ws-dashboard/frontend/src/App.tsx#L6225-L6237` — browser route normalization wrapper delegates to `normalizeServerRouteLocation`.

## Constraints
- `server-local` local compatibility is explicit: non-server-scoped operation routes must remain usable as aliases while new browser helper APIs prefer canonical `/api/dashboard/servers/{serverId}/...` routes (`ai-docs/spec/ws-web-dashboard/index.md#L233-L237`).
- Phase 1 must not treat SSE or terminal WebSocket routes as backend-forwarded behavior; the spec calls out that streams/upgrades need separate proxy plans (`ai-docs/spec/ws-web-dashboard/index.md#L239-L241`).
- Host paths are allowed only as authenticated request data for picker/open/custom-target flows; command payloads, URLs beyond request bodies, logical keys, and persisted records should not use host paths (`ai-docs/mental-model/ws-web-dashboard.md#L66-L67`).
- Existing persisted file pane descriptors and terminal restore intents are versioned localStorage records that currently lack server id, so any identity-format change needs a bounded local-to-`server-local` compatibility decision (`ws-dashboard/frontend/src/workRootFiles.ts#L391-L462`, `ws-dashboard/frontend/src/terminals.ts#L253-L329`).
- Route tests are wired as npm scripts that compile TypeScript helpers into `node_modules/.tmp/route-tests`; affected scripts are listed in `ws-dashboard/frontend/package.json#L8-L22`.

## Risk Signals
- `ws-dashboard/frontend/src/resourceModel.ts#L179-L215` — Possible identity risk: flattened server/workspace/workRoot entity ids are bare daemon ids; if two selected server resource views reuse a bare id, selection and nav reconciliation can collapse without a server-scoped entity id strategy.
- `ws-dashboard/frontend/src/App.tsx#L286-L315` — Possible shortcut risk: activation and workspace-removal URLs are constructed inline in `App.tsx`, bypassing the helper-module pattern the brief wants for server-sensitive operations.
- `ws-dashboard/frontend/src/workRootFiles.ts#L93-L125` — Possible collision risk: file source keys and content/error fan-out match only bare `workRootId + path`, so same workRoot id/path on different servers would share refresh state.
- `ws-dashboard/frontend/src/workRootFiles.ts#L289-L310` — Possible collision risk: read-only pane logical keys and pane ids contain workRootId/path but not serverId; previews are especially one-per-bare-workRoot.
- `ws-dashboard/frontend/src/workRootFiles.ts#L432-L457` — Possible persisted-state risk: saved pane descriptors include workRootId/path/mode/title only, so server-scoped panes would need a compatibility path for existing local-only records.
- `ws-dashboard/frontend/src/terminals.ts#L200-L218` — Possible collision risk: terminal logical keys and pane ids use bare workRootId/terminalId or terminalId only, so same terminalId across servers can share pane identity.
- `ws-dashboard/frontend/src/terminals.ts#L221-L329` — Possible persisted-state risk: terminal restore intents store bare workRootId/title/cwdHint only; multi-server restore needs server identity while keeping old local records readable.
- `ws-dashboard/frontend/src/workRootActivity.ts#L134-L150` — Possible stream risk: Activity stream requests carry only workRootId/requestId, so a same workRoot id on another server can pass stale-request guards unless server id becomes part of stream identity.
- `ws-dashboard/frontend/src/workRootActivity.ts#L380-L423` — Possible activity collision risk: acknowledgements, revision ordering, and selection helpers operate on item ids without server/workRoot namespace in the type.
- `ws-dashboard/frontend/src/commands.ts#L45-L90` — Possible command contract risk: workspace, workRoot, activity, file, document, and terminal commands carry bare ids only; command logs/handlers cannot disambiguate same ids across servers.
- `ws-dashboard/frontend/src/App.tsx#L2348-L2376` — Possible explorer-state risk: file explorer snapshots are keyed by bare workRoot id, so expanded/loaded directory state can mix across servers.
- `ws-dashboard/frontend/src/App.tsx#L2731-L2786` — Possible workbench-state risk: terminal panes, activity pane open state, workbench groups/order, and closed-agent pane state are keyed by bare workRoot or pane ids.
- `ws-dashboard/frontend/src/App.tsx#L3018-L3070` — Possible SSE identity risk: Activity EventSource and fallback snapshot fetches derive from bare root id and compare `view.workRootId !== rootId`, not a server-scoped resource identity.
- `ws-dashboard/frontend/src/App.tsx#L3254-L3273` — Possible document-event risk: document EventSource handling filters only by event workRootId before refreshing panes, which can refresh a matching remote/local bare id path incorrectly.
- `ws-dashboard/frontend/src/App.tsx#L4720-L4780` — Possible workbench placement risk: terminal surface logical keys are built from `persistentTerminal/workRootId/terminalId`; same ids on two servers can point to the same Dockview attachment identity.

## Opinion
- The code already has good helper/test seams for route strings, but server identity is uneven: resource fetching has a `server-local` compatibility pattern, while most operation helpers and state maps still accept bare ids.
- The highest-survey uncertainty is how broadly to introduce server-scoped identity helpers versus threading full `ResourcePath` through App-owned state; the brief gives the contract, but implementer/planner judgment is still needed for the smallest compatible surface.
