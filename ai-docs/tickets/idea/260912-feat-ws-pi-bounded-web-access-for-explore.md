---
title: "Pi Explore: bounded fetch plus bundled web-search extension"
related:
  260912-refactor-ws-pi-unify-persistent-explore-modes: consumes the common web capability through its web-search intent alias
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: network-read authority must obey descendant capability ceilings
---

# Pi Explore: bounded fetch plus bundled web-search extension

## Background

Codex and Claude harnesses already provide host-native web retrieval, while Pi Explore currently receives only repository read/search tools. Adding provider and secret handling to shared ws-mcp would duplicate host capabilities and create a cross-harness responsibility that only Pi needs.

Provide Pi-local bounded URL fetching directly in `agents-plugin-pi`, and compose a pinned external Pi extension for search. Pi does not auto-activate extension packages listed in npm `dependencies`: package A must explicitly list or load package B, and spawned RPC children do not inherit the parent's extension set because the current launcher uses `--no-extensions` with exact extension paths. The integration must therefore compose the dependency explicitly for Explore children.

`pi-web-access@0.29.0` is the selected initial search dependency. It is MIT licensed, declares Pi core libraries as peer dependencies, supports multiple provider routes including Codex/OpenAI, Brave, SearXNG, and keyless DuckDuckGo, and avoids the duplicate host-library pattern seen in heavier alternatives. Its broad fetch and media surface is not part of this contract.

## Decisions

- **Pi-local only.** Do not add web search/fetch tools or provider configuration to ws-mcp, shared playbooks, or other harness packages.
- **Own bounded fetch.** Register a ws Pi extension tool with a non-conflicting name such as `ws_web_fetch`. It performs public HTTP(S) GET retrieval only and returns bounded text or markdown evidence.
- **Bundle search.** Pin and bundle `pi-web-access@0.29.0` with `agents-plugin-pi`. Do not discover or depend on a separately configured user copy.
- **Load the dependency only where consumed.** Keep the external extension out of the interactive lead's normal package extension list. Add its exact package-local extension path to every persistent Explore RPC child alongside the ws extension. Use the same extension set for every Explore mode so mode remains a tier alias rather than a tool-profile branch.
- **Allow only search from the dependency.** Add the external `web_search` tool to the Explore child `--tools` allowlist. Do not expose its fetch, content-store, media, GitHub-clone, or other tools. Register the ws-owned fetch under a distinct name so first-registration-wins behavior cannot shadow either contract.
- **Fail before dispatch when composition is broken.** Resolve and validate the bundled extension path and required search tool during Explore spawn. A missing/incompatible dependency or unavailable provider reports a precise error rather than silently degrading `web-search` into local-only research.
- **Treat network read as explicit authority.** Extend Pi's adapter-owned capability ceiling with bounded network-search and network-fetch authority. Descendants at depth 2 or greater may receive only network authority already present in their direct parent's ceiling; network read never implies shell, filesystem write, arbitrary headers, or mutating MCP authority.
- **Bound direct fetch.** Permit only `http` and `https`; deny user-supplied credentials, cookies, arbitrary headers, request bodies, local files, loopback, private/link-local/reserved destinations, and non-public redirect targets. Re-resolve and validate each redirect against DNS rebinding, cap redirects/time/bytes, accept a narrow content-type set, remove executable HTML content, and mark returned page text as untrusted evidence.
- **Preserve package isolation.** Keep Pi core libraries as peers rather than bundled physical copies, pin the external extension version, carry its license notice, and test that separately installed copies are neither discovered nor loaded. Package-local loading must work identically for direct lead Explore and worker-owned recursive Explore.
- **No consumer-search scraping contract.** Do not implement Google result-page HTML scraping as a fallback. Search-provider routing belongs to the pinned dependency; ws owns only composition and capability restriction.

## Phases

### Phase 1: Add bounded Pi web retrieval and isolated search composition

Implement and test the ws-owned bounded fetch tool, package and resolve the pinned `pi-web-access` extension, and pass both the ws extension and dependency extension explicitly to Explore RPC children. Extend the common Explore tool allowlist and descendant capability envelope with only `ws_web_fetch` and external `web_search`, keeping all other dependency tools unavailable.

Verify public fetch success, text/HTML extraction, content and deadline caps, redirect validation, DNS rebinding and private-network refusal, malformed URLs, unsupported content, untrusted-content framing, missing usage metadata, and cancellation. Verify package install/update, peer dependency resolution, exact extension ordering, duplicate separately configured copies, missing dependency failure, search-provider failure, all-mode tool-profile identity, depth monotonicity, dormant/restarted child composition, and real direct-lead and nested-worker Explore searches. Confirm Codex/Claude/shared ws-mcp manifests and behavior remain unchanged.
