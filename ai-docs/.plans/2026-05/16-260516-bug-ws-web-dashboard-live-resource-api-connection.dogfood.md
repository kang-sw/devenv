# Dogfood Verification: 260516-bug-ws-web-dashboard-live-resource-api-connection

Date: 2026-05-16

## Production-Served Daemon Smoke

Command:

```sh
cd ws-dashboard
./dev.sh run --port 8787
```

Result: production frontend build succeeded and the daemon served
`frontend/dist` from `http://127.0.0.1:8787` behind owner auth. The served
production JS bundle contains the `workRoot.open` command id, the
`/api/dashboard/work-roots/open` POST, and the canonical
`/api/dashboard/resources` fetch — the browser open affordance and refresh
path ship in the production build.

## Primary Resource Endpoint Evidence (before / after)

The verification starts from the dashboard's normal resource load and proves
the canonical endpoint is live, not mock-backed.

- `GET /api/dashboard/resources` unauthenticated → `HTTP 401` (owner-auth gate).
- Paired through `/pair?token=...` → `303 /` with an HttpOnly owner cookie.
- `GET /api/dashboard/resources` **before opening any workRoot** →
  `200`, `server.id = server-local`, `workspaces: []`. This is the honest
  empty live view — the static mock `workspace-devenv` / `workspace-notes`
  fixture workspaces are **absent**.
- `POST /api/dashboard/work-roots/open` with `{"path":"/Users/kang-sw/devenv"}`
  → `200`; opaque workRoot id `root-local-d2c26f826a7df7a0`
  (workspace `workspace-local-675b806158e6a537`, kind `gitPrimaryRoot`,
  status `online`). The open response is the aggregated live view.
- `GET /api/dashboard/resources` **after opening** → `200`; the response is
  byte-equivalent to the open response: one workspace, workRoot
  `root-local-d2c26f826a7df7a0`. The mock `workspace-devenv` workspace is
  **absent**. The canonical endpoint now reflects live opened-workRoot state.

## Real WorkRoot File / Read / Terminal Flow

All exercised against the real opened workRoot `root-local-d2c26f826a7df7a0`,
not a mock fixture id:

- `GET /api/dashboard/work-roots/{workRootId}/files` → `status: ok`, 20 real
  entries from `/Users/kang-sw/devenv` (`.git`, `.github`, `agents-plugin`, …).
- `GET /api/dashboard/work-roots/{workRootId}/files/read?path=AGENTS.md` →
  `status: ok`, `readOnly: true`, `sizeBytes: 8944`, `languageHint: markdown`,
  real file content (`# AGENTS.md - devenv …`).
- `POST …/work-roots/{workRootId}/terminals` → terminal
  `term_N1GSNPuAI6GiNS1rv8` created (`running`, 80x24).
- `POST …/terminals/{terminalId}/input` (`printf ws-dashboard-live-resource-check`)
  → `204`; `GET …/output?after=0` contained the marker.
- `GET …/work-roots/{workRootId}/terminals` listed the live terminal.
- `DELETE …/terminals/{terminalId}` → `204`; subsequent live-session listing
  returned `[]` (close terminates and reaps the PTY child).
- A final `GET /api/dashboard/resources` refresh still returned the opened
  workRoot and no mock workspace — the canonical endpoint stays authoritative
  for refresh / re-entry.

## Blocker - 2026-05-16

- Step: Browser visual/manual checks of the left-nav "Open workRoot" control
  and the resource tree turning live in an interactive browser.
- Expected: Type a path into the new opener, submit, and watch the nav tree
  switch from the empty live state to the real opened workRoot.
- Actual: This execution environment exposes shell/curl verification but no
  interactive browser or screenshot automation tool for the daemon-served UI.
- Evidence: HTTP/curl evidence above covers every primary-flow step; the
  production JS bundle was confirmed to contain the open affordance and
  canonical-endpoint wiring. The pure-function transition is additionally
  covered by `frontend/src/resourceModel.test.ts`
  (`npm run test:resource-model`), which proves the resource tree reconciles
  from a mock-style view to the live opened workRoot.
- Impact: Dogfood partial. Backend live-resource authority, the open flow, and
  the file/read/terminal flow against a real workRoot were verified end to end;
  the browser-rendered tree transition is verified by build + pure-function
  test rather than an interactive screenshot.
- Follow-up: Run a manual browser dogfood with the same `./dev.sh run --port
  8787` flow in an environment with browser access if visual evidence is
  required.
