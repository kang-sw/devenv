# Dogfood Verification: 260516-feat-ws-web-workroot-io-workbench-integration

Date: 2026-05-16

## Production-Served Daemon Smoke

Command:

```sh
cd ws-dashboard
./dev.sh run --port 8787
```

Result: production frontend build succeeded and the daemon served `frontend/dist`
from `http://127.0.0.1:8787` behind owner auth.

API/browser-equivalent steps exercised with the paired owner cookie:

- Paired through `/pair?token=...`; received owner session cookie and `303 /`.
- Loaded `/`; received production `index.html` from the daemon.
- Opened workRoot `/Users/kang-sw/devenv`; received opaque workRoot id
  `root-local-d2c26f826a7df7a0`.
- Listed root files through
  `/api/dashboard/work-roots/{workRootId}/files`; received 20 entries.
- Read `README.md` through the read-only file API; received `readOnly: true`
  and 2362 bytes of text content.
- Created terminal `term_puxSIkq8FXtdJUfxT9` for the opened workRoot.
- Sent `printf ws-dashboard-terminal\n`; output polling contained the marker.
- Listed live terminals; the created terminal appeared.
- Closed the terminal with `DELETE /api/dashboard/terminals/{terminalId}`;
  subsequent live-session listing returned zero sessions.

## Blocker - 2026-05-16

- Step: Browser visual/manual checks at desktop, 960px, and 560px widths.
- Expected: Inspect left-nav identity/file explorer, read-only text pane,
  terminal pane, duplicate focus behavior, and narrow layout in an interactive
  browser.
- Actual: This execution environment exposes shell/curl verification but no
  interactive browser or screenshot automation tool for the daemon-served UI.
- Evidence: Production-served daemon smoke above completed through HTTP/API;
  visual layout was not captured.
- Impact: Dogfood partial. Backend lifecycle and production static serving were
  verified; visual breakpoint acceptance remains weakly verified by build/tests
  only.
- Follow-up: Run manual browser dogfood with the same `./dev.sh run --port 8787`
  flow in an environment with browser access if visual evidence is required.
