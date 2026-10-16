// The engine lays connected MCP tools into .claude-plugin/types at load time,
// before session.start registers this mod's own tool; declare it here so the
// tool.call matcher and the tests type-check on the first load too.
export {}
declare module 'claude-code' {
  interface McpToolInputs {
    'mcp__ws-mailbox-push-probe__mailbox_watch': { session_key: string }
  }
}
