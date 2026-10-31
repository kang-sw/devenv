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
 * The key, the mailbox line and the state reach the main conversation in the
 * manual feature's epoch row instead (manual.ts `deliver`), appended after an
 * expansion and never inlined here. A body that would carry a key
 * (`lead-use-mailbox` substitutes a key-specific wait command) is never
 * expanded, and a block that passes through appends no row: its fallback
 * prose makes its own `workflow_manual` call where it has one.
 *
 * A playbook is one dedupe unit with the main-thread `playbook_read` reads
 * (manual.ts `preloaded`): while the identical body is still visible in the
 * main conversation, from an earlier expansion or read, the block is replaced
 * by a pointer line followed by its own fallback prose instead of the body.
 * The prose stays because the conversation judged is the main one, and a
 * subagent preload that cannot see that copy must still read the playbook.
 *
 * The engine calls arrive as edges bound in register.ts, so the expansion is
 * driven and tested without an engine.
 */

import { preloadPointerLine } from './manual'

/** Skills whose block always passes through: their body is specific to the reading key. */
export const PRELOAD_EXCLUDED: ReadonlySet<string> = new Set(['lead-use-mailbox'])

/** One block: the open tag with its playbook name, the fallback prose, the close tag. */
const BLOCK = /<ws-preload playbook="([^"\n]+)">([\s\S]*?)<\/ws-preload>/

/** A block's playbook name, its span in the text, and its fallback prose (trimmed). */
export type PreloadBlock = { playbook: string; start: number; end: number; fallback: string }

/** The first preload block of `text`, or undefined; a block with any other open tag is none. */
export function findBlock(text: string): PreloadBlock | undefined {
  const m = BLOCK.exec(text)
  if (!m) return undefined
  return { playbook: m[1] as string, start: m.index, end: m.index + m[0].length, fallback: (m[2] as string).trim() }
}

/** The skill's name without its plugin namespace (`ws:lead-discuss` -> `lead-discuss`). */
export function bareSkill(skill: string, plugin: string): string {
  return skill.startsWith(`${plugin}:`) ? skill.slice(plugin.length + 1) : skill
}

export type PreloadInput = { skill: string; text: string; pluginName?: string }

export type PreloadEdges = {
  /** The engine-reported plugin name: only this plugin's skills expand. */
  plugin: string
  /** The conversation's lead key, waited on a bounded time (manual.ts `ensureKey`), or undefined while the mod holds none; never rejects. */
  key: () => Promise<string | undefined>
  /** `playbook_read(name, session_key: key)`'s text; rejects on a failed call or an error result. */
  read: (name: string, key: string) => Promise<string>
  /** Whether `body` of `playbook` repeats a copy still visible in the main conversation (manual.ts `preloaded`); never rejects. */
  repeat: (playbook: string, body: string) => Promise<boolean>
  /** Called once the block is replaced, before the text returns: the epoch row (manual.ts `deliver`); never rejects. */
  deliver: () => Promise<void>
}

/**
 * The text the model should read in place of `input.text`, or undefined to
 * keep it: the skill is this plugin's and not excluded, its block names the
 * skill's own playbook, the mod holds a key, and the read answered a body
 * that does not carry that key. The block becomes that body, or, when the
 * body repeats a visible copy, the pointer line and the block's fallback
 * prose; `deliver` runs before it returns, so the row lands ahead of the
 * skill's own rows. Rejects when the read does; the hook passes the text
 * through then, and nothing is delivered.
 */
export async function expandPreload(input: PreloadInput, edges: PreloadEdges): Promise<string | undefined> {
  if (input.pluginName !== edges.plugin) return undefined
  const skill = bareSkill(input.skill, edges.plugin)
  if (PRELOAD_EXCLUDED.has(skill)) return undefined
  const block = findBlock(input.text)
  if (!block || block.playbook !== skill) return undefined
  const key = await edges.key()
  if (key === undefined) return undefined
  const body = (await edges.read(block.playbook, key)).trim()
  if (!body || body.includes(key)) return undefined
  const inlined = (await edges.repeat(block.playbook, body)) ? `${preloadPointerLine(block.playbook)}\n\n${block.fallback}` : body
  await edges.deliver()
  return input.text.slice(0, block.start) + inlined + input.text.slice(block.end)
}
