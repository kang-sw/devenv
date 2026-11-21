---
title: "dashboard 픽스처가 실제 HOME을 상속하고 plan-usage 처리 중 실제 Codex 자식을 실행하는 문제"
related:
  261115-research-ws-effects-browser-unexpected-codex-child: "adjacent tracked process receipt and fixture-isolation evidence"
  261113-feat-ws-dashboard-rail-usage-gutter: "the landed dashboard plan-usage source chain"
---

# Dashboard fixture inherits real HOME and launches a real Codex child during plan-usage handling

## Background

The `viewed002` fixture receipt supplied for this capture reports that the
baseline inherited the real `HOME` and a real `codex app-server` child was
observed during daemon plan-usage handling. The receipt itself is not present
in this checkout; the adjacent tracked evidence ticket
[`261115-research-ws-effects-browser-unexpected-codex-child`](261115-research-ws-effects-browser-unexpected-codex-child.md)
preserves a related isolated-browser process receipt and explicitly leaves
provider activity unknown.

Real `HOME` can expose the user's Codex configuration to plan-usage detection:
`internal/wsdashboard/plan_usage.go:134-159` resolves the home/config paths and
reads Codex login presence. The source chain can start `codex app-server`
(`internal/wsdashboard/plan_usage_cmd_unix.go:10-12`) and send its plan-usage
handshake (`internal/wsdashboard/plan_usage_probe.go:93-165`), but those source
paths do not prove what happened in the `viewed002` run.

The observed child does **not** establish successful authentication, any
specific RPC or reply, model activity, or network activity. Those facts remain
unknown. No usage suppression, source repair, provider invocation, daemon
change, or other remedy is selected.

## Evidence

- `viewed002` baseline receipt: real `HOME` was inherited and a real installed
  `codex app-server` child was observed during the fixture run; the receipt is
  lead-provided and not tracked in this checkout.
- The related research ticket preserves a bounded process ancestry receipt and
  the stop condition against attributing provider activity from process
  presence alone.
- The tracked source chain only establishes capability: home/config detection
  in `plan_usage.go:134-159`, command construction in
  `plan_usage_cmd_unix.go:10-12`, and the initialize / `account/rateLimits/read`
  exchange in `plan_usage_probe.go:93-165`.

## Constraints

- Keep this as an underspecified fixture-isolation bug/idea; do not invent a
  remediation contract or weaken acceptance.
- Do not claim authentication, RPC contents, model invocation, or network
  activity from the child-process observation.
- Do not suppress plan usage, disable the source chain, invoke a provider, or
  repair the fixture/source in this capture.

## Phases

### Phase 1: Identify the fixture HOME and plan-usage boundary

Trace the `viewed002` receipt against its fixture environment and the tracked
plan-usage source chain. Determine how the real `HOME` became visible and why a
real Codex child was reachable, while keeping authentication, RPC/model, and
network behavior explicitly unresolved. Any later fixture boundary or source
repair requires a separate owner-approved contract.
