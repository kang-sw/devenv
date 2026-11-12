---
title: "Whiteboard runtime rejects wb:recap while its own command help documents it"
related:
  261112-research-ws-hosted-pi-missing-workflow-tools: fallback MCP access exposed this during hosted Pi dogfood
---

# Whiteboard recap command/help mismatch

## Evidence

On 2026-11-12, the bundled launcher MCP connection reported ws-mcp
`0.48.17`. A `whiteboard.update` batch containing `wb:topic`, `wb:recap`,
and `wb:body` was atomically refused with `unknown command` for each
`wb:recap`. The refusal's supported-command list omitted `wb:recap`, but
its appended command documentation explicitly included `wb:recap` and its
syntax and semantics. A subsequent query confirmed nothing was applied.

This occurred through `python3 agents-plugin/bin/ws-mcp-launcher.py serve
--stdio --root /home/swkang/devenv`, used as a fallback because this hosted
Pi lead lacked ws tools. Whether the launcher selected an older runtime or
the executing parser itself omitted a documented command is unverified.
The repository playbook also documents recap support.

## Follow-up

Identify the selected runtime and compare parser registration with the
bundled command help; ensure an agent following that runtime's own help can
use the advertised commands. Do not assume a daemon upgrade alone fixes the
fallback connection. No protocol or compatibility change is authorized.

## Workaround

For this dogfood session, put the short state summary in the topic body
instead of sending `wb:recap`; leave the refused batch unapplied and retry
with supported commands only.
