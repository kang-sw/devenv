import { describe, expect, test } from 'claude-code/testing'

import { bareSkill, expandPreload, findBlock, PRELOAD_EXCLUDED, type PreloadEdges } from './preload'

const BLOCK = '<ws-preload playbook="lead-discuss">\nCall `ws/playbook.read(name: "lead-discuss")`.\n</ws-preload>'
const SHIM = `# Discuss\n\n${BLOCK}\n`

function edges(over: Partial<PreloadEdges> = {}): PreloadEdges & { reads: [string, string][] } {
  const reads: [string, string][] = []
  return {
    plugin: 'ws',
    key: () => 'lead-key',
    read: async (name, key) => {
      reads.push([name, key])
      return `# ${name}\n\nBody.\n`
    },
    ...over,
    reads,
  }
}

describe('preload', () => {
  test('finds the first block with its playbook name and span', () => {
    expect(findBlock(SHIM)).toEqual({ playbook: 'lead-discuss', start: '# Discuss\n\n'.length, end: '# Discuss\n\n'.length + BLOCK.length })
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
    expect(await expandPreload({ skill: 'lead-discuss', text: `${SHIM}\nARGUMENTS: x\n`, pluginName: 'ws' }, e)).toBe(
      '# Discuss\n\n# lead-discuss\n\nBody.\n\nARGUMENTS: x\n',
    )
    expect(e.reads).toEqual([['lead-discuss', 'lead-key']])
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
