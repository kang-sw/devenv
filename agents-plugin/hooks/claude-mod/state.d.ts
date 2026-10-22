// The Claude mod's $.state contract, named by `types` in each package's
// plugin.json. Only the tool-call caption feature keeps $.state values, and it
// runs in the ws package alone, so the values sit under `ws` in every package
// that ships this byte-identical copy.

export type ToolCaption = {
  /** The call's concrete target, drawn after the tool name; "" when none. */
  subtitle: string
  /** What the call was for. */
  intention: string
  /** What came back. */
  result: string
  /** Characters of the raw call (input and output) the lead's context holds. */
  bytes: number
}

export type RunCaption = {
  /** One target per call, in the run's order; "" when a call has none. */
  subtitles: string[]
  /** What the run set out to do and what came of it. */
  summary: string
  /** Characters of the run's raw calls the lead's context holds. */
  bytes: number
}

declare module 'claude-code' {
  interface PluginState {
    ws: {
      /** A lone call's caption, keyed by its tool_use_id. */
      toolCaptions: Record<string, ToolCaption>
      /** A folded run's caption, keyed by its tool_use_ids joined with ','. */
      runCaptions: Record<string, RunCaption>
    }
  }
}
