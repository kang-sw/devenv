/**
 * Playbook preload, a feature of the Claude mod: a lead skill shim marks the
 * reads its model would make with a `<ws-preload playbook="<name>">` block
 * that wraps the fallback prose, and the mod's `skill.prompt` hook replaces
 * the block with that playbook's body, so the skill reaches the model with
 * its procedure already in it and no `playbook_read` round trip. A harness
 * that does not expand (no mod, no key, any failure) leaves the block, and
 * the model follows the prose inside it.
 *
 * `skill.prompt` carries no agentId and also fires for a skill preloaded into
 * a subagent, so a lead load and a subagent preload cannot be told apart.
 * The expansion therefore inlines only what that skill's model could read
 * itself: the playbook body, read with the conversation's lead key so its
 * prompt overrides and language apply, never the session state or the key.
 * The main thread gets its state from the system prompt's manual section and
 * its state pointer (manual.ts). A body that would carry a key
 * (`lead-use-mailbox` substitutes a key-specific wait command) is never
 * expanded.
 *
 * The engine calls arrive as edges bound in register.ts, so the expansion is
 * driven and tested without an engine.
 */

/** Skills whose block always passes through: their body is specific to the reading key. */
export const PRELOAD_EXCLUDED: ReadonlySet<string> = new Set(['lead-use-mailbox'])

/** One block: the open tag with its playbook name, the fallback prose, the close tag. */
const BLOCK = /<ws-preload playbook="([^"\n]+)">[\s\S]*?<\/ws-preload>/

export type PreloadBlock = { playbook: string; start: number; end: number }

/** The first preload block of `text`, or undefined; a block with any other open tag is none. */
export function findBlock(text: string): PreloadBlock | undefined {
  const m = BLOCK.exec(text)
  if (!m) return undefined
  return { playbook: m[1] as string, start: m.index, end: m.index + m[0].length }
}

/** The skill's name without its plugin namespace (`ws:lead-discuss` -> `lead-discuss`). */
export function bareSkill(skill: string, plugin: string): string {
  return skill.startsWith(`${plugin}:`) ? skill.slice(plugin.length + 1) : skill
}

export type PreloadInput = { skill: string; text: string; pluginName?: string }

export type PreloadEdges = {
  /** The engine-reported plugin name: only this plugin's skills expand. */
  plugin: string
  /** The conversation's lead key, or undefined while the mod holds none. */
  key: () => string | undefined
  /** `playbook_read(name, session_key: key)`'s text; rejects on a failed call or an error result. */
  read: (name: string, key: string) => Promise<string>
}

/**
 * The text the model should read in place of `input.text`, or undefined to
 * keep it: the skill is this plugin's and not excluded, its block names the
 * skill's own playbook, the mod holds a key, and the read answered a body
 * that does not carry that key. Rejects when the read does; the hook passes
 * the text through then.
 */
export async function expandPreload(input: PreloadInput, edges: PreloadEdges): Promise<string | undefined> {
  if (input.pluginName !== edges.plugin) return undefined
  const skill = bareSkill(input.skill, edges.plugin)
  if (PRELOAD_EXCLUDED.has(skill)) return undefined
  const block = findBlock(input.text)
  if (!block || block.playbook !== skill) return undefined
  const key = edges.key()
  if (key === undefined) return undefined
  const body = (await edges.read(block.playbook, key)).trim()
  if (!body || body.includes(key)) return undefined
  return input.text.slice(0, block.start) + body + input.text.slice(block.end)
}
