# Tauri attach-only readiness documentation

## Authority and outcome
MAIN 2026-11-14 15:11:55 and 15:14:52 relays owner direction to start and dogfood the Tauri shell now, overriding the previous after-release scheduling assumption (701). Owner (797)=a permits tonight's tooling install and implementation, but this assignment is documentation only. MAIN owns decision IDs; mint none. Prepare the existing thin-shell ticket for lead-owned fact population, Sage review, and ready promotion.

## Scope
- ai-docs/tickets/todo/261113-feat-werkschaft-thin-tauri-shell.md
- ai-docs/tickets/todo/261110-epic-ws-dashboard-agent-host.md
- ai-docs/tickets/todo/261110-research-ws-dashboard-agent-host-foundations.md
Read the related shell research and signing/local-only tickets only for decisions that constrain this edit. Read ticket conventions and relevant authoring manuals before editing. The source sizing report is /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/shared/docs/tauri-thin-shell-sizing.md, especially sections 3, 6 and Text that conflicts. Historical shared reports are evidence, not new product scope.

## Settled contract
Use agents-plugin-tool/desktop/ with a nested Cargo module, no root module directories. Carve the first unfinished Phase 1 into Phase 1a attach-only macOS first window: endpoint.json port discovery respecting daemon cache override, existing /health, open exact http://127.0.0.1:<port>/, external links to system browser, retry/error view when no healthy daemon, close leaves daemon running. No daemon start/stop/swap, no new UI credential, no endpoint token in page JS, no daemon API change. Single-instance focus is next, not a first-window blocker. Broader WKWebView acceptance, safe missing-daemon start, Windows/WebView2, zip, signing and updater are later slices; preserve established signing boundaries. (798) JS-memory scoped grant versus (702)=a remains OPEN and explicitly gated OUT of Phase 1a; do not introduce an Open Decision Queue that blocks the already-settled independent Phase 1a. Do not resolve (798), invent another auth model, or assert implementation evidence.
Prefer a pinned prebuilt local Tauri CLI under desktop (npm devDependency is acceptable) over global cargo install. Existing Rust is ~/.cargo/bin 1.95; dependencies may be fetched by the later implementation worker. First cargo build requires MAIN's later heavy slot; no build/install/fetch now. First product deliverable is a runnable .app/dev binary path opening the live dashboard, not Windows/signing/updater completion. Compatibility T0: additive artifact, no protocol/store change.

Correct stale shell-after-dashboard deferral wording in the two named tracked documents, preserving historical decisions with an explicit superseding direction rather than pretending they were never made. Trace scope through Decisions, Constraints, phases and verification so old attach/start+all-platform Phase 1 does not silently remain a blocker. Do not modify completed Result phase text.

## Return boundary
Static documentation edits only. No source implementation, install, fetch, cargo/npm commands, tests, network, daemon operation, new children, ticket status move, Sage stamp, merge or push. Do not commit yet: root will inspect your exact diff, invoke the required promotion tooling, and commit the reviewed body. Return edited paths, concise evidence-backed rationale, unresolved decision gaps if any, and the status of historical shared text. Leave the immutable plan unchanged.
