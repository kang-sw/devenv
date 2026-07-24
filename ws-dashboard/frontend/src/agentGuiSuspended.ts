// Agent GUI suspended per user directive 2026-07-25 (see ticket family
// 260713-*). Set to `false` to restore the agent-chat feature. While `true`,
// every spawn entry point (toolbar button, `a n` hotkey, and the underlying
// `registerNewAgentChatPane` primitive) is hidden or no-ops so no agent chat
// pane can be created. The agent modules themselves remain in the tree,
// dormant.
export const AGENT_GUI_SUSPENDED = true as const;
