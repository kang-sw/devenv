import { describe, expect, test } from 'claude-code/testing'

import { preloadPointerLine } from './manual'
import { bareSkill, expandPreload, findBlock, PRELOAD_EXCLUDED, type PreloadEdges } from './preload'

const BLOCK = '<ws-preload playbook="lead-discuss">\n# Discuss\n\nCall `ws/playbook.read(name: "lead-discuss")`.\n</ws-preload>'
const SHIM = `${BLOCK}\n`

function edges(over: Partial<PreloadEdges> = {}): PreloadEdges & { reads: [string, string][]; repeats: [string, string][] } {
  const reads: [string, string][] = []
  const repeats: [string, string][] = []
  return {
    plugin: 'ws',
    key: () => 'lead-key',
    read: async (name, key) => {
      reads.push([name, key])
      return `# ${name}\n\nBody.\n`
    },
    repeat: async (playbook, body) => (repeats.push([playbook, body]), false),
    ...over,
    reads,
    repeats,
  }
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
  })

  test('a repeat body becomes the pointer line followed by the block fallback prose, heading included', async () => {
    const e = edges({ repeat: async () => true })
    expect(await expandPreload({ skill: 'lead-discuss', text: `Base\n\n${SHIM}\nARGUMENTS: x\n`, pluginName: 'ws' }, e)).toBe(
      `Base\n\n${preloadPointerLine('lead-discuss')}\n\n# Discuss\n\nCall \`ws/playbook.read(name: "lead-discuss")\`.\n\nARGUMENTS: x\n`,
    )
  })

  test('passes through without reading: another plugin, an excluded skill, a mismatched or missing block, no key', async () => {
    const cases: [Parameters<typeof expandPreload>[0], Partial<PreloadEdges>][] = [
      [{ skill: 'lead-discuss', text: SHIM, pluginName: 'wsflow' }, {}],
      [{ skill: 'lead-discuss', text: SHIM }, {}],
      [{ skill: 'lead-use-mailbox', text: SHIM.replaceAll('lead-discuss', 'lead-use-mailbox'), pluginName: 'ws' }, {}],
      [{ skill: 'lead-run', text: SHIM, pluginName: 'ws' }, {}],
      [{ skill: 'lead-discuss', text: '# Discuss\n', pluginName: 'ws' }, {}],
      [{ skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }, { key: () => undefined }],
    ]
    for (const [input, over] of cases) {
      const e = edges(over)
      expect(await expandPreload(input, e)).toBeUndefined()
      expect(e.reads).toEqual([])
      expect(e.repeats).toEqual([])
    }
    expect(PRELOAD_EXCLUDED.has('lead-use-mailbox')).toBe(true)
  })

  test('an empty body or one carrying the key passes through; a failed read rejects', async () => {
    const input = { skill: 'lead-discuss', text: SHIM, pluginName: 'ws' }
    expect(await expandPreload(input, edges({ read: async () => '  \n' }))).toBeUndefined()
    expect(await expandPreload(input, edges({ read: async () => 'wait --session-key lead-key' }))).toBeUndefined()
    await expect(expandPreload(input, edges({ read: async () => Promise.reject(new Error('down')) }))).rejects.toThrow('down')
  })
})
