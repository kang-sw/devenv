import { describe, expect, test } from 'claude-code/testing'

import { preloadPointerLine } from './manual'
import { bareSkill, expandPreload, findBlock, KEY_PLACEHOLDER, type PreloadEdges } from './preload'

const BLOCK = '<ws-preload playbook="lead-discuss">\n# Discuss\n\nCall `ws/playbook.read(name: "lead-discuss")`.\n</ws-preload>'
const SHIM = `${BLOCK}\n`

function edges(over: Partial<PreloadEdges> = {}): PreloadEdges & { reads: [string, string][]; repeats: [string, string][]; delivered: number } {
  const reads: [string, string][] = []
  const repeats: [string, string][] = []
  const e = {
    plugin: 'ws',
    key: async () => 'lead-key' as string | undefined,
    read: async (name: string, key: string) => {
      reads.push([name, key])
      return `# ${name}\n\nBody.\n`
    },
    repeat: async (playbook: string, body: string) => (repeats.push([playbook, body]), false),
    deliver: async () => void (e.delivered += 1),
    ...over,
    reads,
    repeats,
    delivered: 0,
  }
  return e
}

describe('preload', () => {
  test('finds the first block with its playbook name and span', () => {
    expect(findBlock(`Base\n\n${SHIM}`)).toEqual({
      playbook: 'lead-discuss',
      start: 'Base\n\n'.length,
      end: 'Base\n\n'.length + BLOCK.length,
      fallback: '# Discuss\n\nCall `ws/playbook.read(name: "lead-discuss")`.',
    })
    expect(findBlock('# Discuss\n\nno block\n')).toBeUndefined()
    // Only the playbook attribute opens a block; an unclosed block is none.
    expect(findBlock('<ws-preload tool="workflow_state">\nx\n</ws-preload>')).toBeUndefined()
    expect(findBlock('<ws-preload playbook="lead-discuss">\nx\n')).toBeUndefined()
  })

  test('strips only the plugin namespace from the skill name', () => {
    expect(bareSkill('ws:lead-discuss', 'ws')).toBe('lead-discuss')
    expect(bareSkill('lead-discuss', 'ws')).toBe('lead-discuss')
    expect(bareSkill('other:lead-discuss', 'ws')).toBe('other:lead-discuss')
  })

  test('replaces the block with the trimmed body, keeping the text around it', async () => {
    const e = edges()
    expect(await expandPreload({ skill: 'lead-discuss', text: `Base\n\n${SHIM}\nARGUMENTS: x\n`, pluginName: 'ws' }, e)).toBe(
      'Base\n\n# lead-discuss\n\nBody.\n\nARGUMENTS: x\n',
    )
    expect(e.reads).toEqual([['lead-discuss', 'lead-key']])
    // The repeat check sees the trimmed body it would inline.
    expect(e.repeats).toEqual([['lead-discuss', '# lead-discuss\n\nBody.']])
    expect(e.delivered).toBe(1)
  })

  test('delivers before the expanded text returns, and never inlines what it delivers', async () => {
    const order: string[] = []
    const e = edges({
      repeat: async () => (order.push('repeat'), false),
      deliver: async () => void order.push('deliver'),
    })
    const text = await expandPreload({ skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }, e)
    order.push('returned')
    expect(order).toEqual(['repeat', 'deliver', 'returned'])
    expect(text).toBe('# lead-discuss\n\nBody.\n')
    expect(text).not.toContain('lead-key')
  })

  test('a repeat body becomes the pointer line followed by the block fallback prose, heading included', async () => {
    const e = edges({ repeat: async () => true })
    expect(await expandPreload({ skill: 'lead-discuss', text: `Base\n\n${SHIM}\nARGUMENTS: x\n`, pluginName: 'ws' }, e)).toBe(
      `Base\n\n${preloadPointerLine('lead-discuss')}\n\n# Discuss\n\nCall \`ws/playbook.read(name: "lead-discuss")\`.\n\nARGUMENTS: x\n`,
    )
    // The pointer form is an expansion too: the row's scan decides whether to append.
    expect(e.delivered).toBe(1)
  })

  test('passes through without reading or delivering: another plugin, a mismatched or missing block, no key', async () => {
    const cases: [Parameters<typeof expandPreload>[0], Partial<PreloadEdges>][] = [
      [{ skill: 'lead-discuss', text: SHIM, pluginName: 'wsflow' }, {}],
      [{ skill: 'lead-discuss', text: SHIM }, {}],
      [{ skill: 'lead-run', text: SHIM, pluginName: 'ws' }, {}],
      [{ skill: 'lead-discuss', text: '# Discuss\n', pluginName: 'ws' }, {}],
      [{ skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }, { key: async () => undefined }],
      [{ skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }, { key: async () => '' }],
    ]
    for (const [input, over] of cases) {
      const e = edges(over)
      expect(await expandPreload(input, e)).toBeUndefined()
      expect(e.reads).toEqual([])
      expect(e.repeats).toEqual([])
      expect(e.delivered).toBe(0)
    }
  })

  test('every occurrence of the key in the body read becomes the placeholder, the rest of the body unchanged', async () => {
    const e = edges({ read: async () => '# lead-discuss\n\nwait --session-key lead-key --slug lead-key-mail; then lead-key again.\n' })
    const text = await expandPreload({ skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }, e)
    expect(text).toBe(`# lead-discuss\n\nwait --session-key ${KEY_PLACEHOLDER} --slug ${KEY_PLACEHOLDER}-mail; then ${KEY_PLACEHOLDER} again.\n`)
    expect(text).not.toContain('lead-key')
    // The repeat check sees the body as inlined, placeholder and all.
    expect(e.repeats).toEqual([['lead-discuss', `# lead-discuss\n\nwait --session-key ${KEY_PLACEHOLDER} --slug ${KEY_PLACEHOLDER}-mail; then ${KEY_PLACEHOLDER} again.`]])
    expect(e.delivered).toBe(1)
  })

  test('lead-use-mailbox expands like any lead skill: its key-specific wait command inlined with the placeholder, delivered', async () => {
    const shim = SHIM.replaceAll('lead-discuss', 'lead-use-mailbox')
    const e = edges({ read: async (_name, key) => `# Use Mailbox\n\nRun \`ws-mcp mailbox wait --session-key ${key} --slug quiet-otter\`.\n` })
    expect(await expandPreload({ skill: 'ws:lead-use-mailbox', text: shim, pluginName: 'ws' }, e)).toBe(
      `# Use Mailbox\n\nRun \`ws-mcp mailbox wait --session-key ${KEY_PLACEHOLDER} --slug quiet-otter\`.\n`,
    )
    expect(e.delivered).toBe(1)
  })

  test('an empty body, or one still carrying the key after the replacement, passes through undelivered; a failed read rejects undelivered', async () => {
    const input = { skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }
    // The backstop: a key the placeholder itself contains survives the replacement.
    for (const over of [{ read: async () => '  \n' }, { key: async () => 'key', read: async () => 'wait --session-key key' }]) {
      const e = edges(over)
      expect(await expandPreload(input, e)).toBeUndefined()
      expect(e.repeats).toEqual([])
      expect(e.delivered).toBe(0)
    }
    const failed = edges({ read: async () => Promise.reject(new Error('down')) })
    await expect(expandPreload(input, failed)).rejects.toThrow('down')
    expect(failed.delivered).toBe(0)
  })
})
