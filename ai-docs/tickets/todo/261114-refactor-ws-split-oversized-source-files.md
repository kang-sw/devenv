---
title: "Split oversized source files flagged by the git.merge size guard"
---

# Split oversized source files flagged by the git.merge size guard

## Background

Owner answer (560), 2026-11-13, relayed by MAIN: "아젠다에 "이 파일들은 당분간
경고 무시"로 기록해두기로 합시다. 그렇게 기준선 올려놓고 롤백 잊어버리면 오히려
낭패라, 자세한 계획은 없이 그냥 todo에 refactor 해야함, 정도로만 잡아두고
populate는 적당한 타이밍에 하기로 해요."

The `git.merge` size advisory flagged these files on 2026-11-13 merges:

- `agents-plugin-tool/internal/mcp/server.go` (6307 lines)
- `agents-plugin-tool/internal/mcp/playbook_tools_test.go` (2949 lines)
- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` (7290 lines)

Added on 2026-11-14 under the same policy (flagged by the
`261113-feat-ws-compaction-before-queued-followup` merge):

- `agents-plugin-tool/internal/wsagent/runner.go` (3169 lines)
- `agents-plugin/hooks/claude-mod/watcher.test.ts` (2881 lines)
- `agents-plugin-wsflow/hooks/claude-mod/watcher.test.ts` (2881 lines, the
  wsflow mirror)

Until this ticket is populated, the advisories for these files are ignored;
the per-path guards in `.gitattributes` are deliberately not raised, so the
warning stays visible. This is a placeholder: no plan, decisions, or fact
population yet; populate it at a suitable time (for `dashboard.js`, after the
in-flight dashboard tickets that edit it have landed).
