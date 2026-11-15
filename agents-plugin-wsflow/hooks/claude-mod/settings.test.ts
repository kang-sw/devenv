import { expect, test } from 'claude-code/testing'

import { createStartupSettings } from './settings'

const success = (key: string, value: unknown = key) => ({ key, ok: true, value, scope: 'project' })
const envelope = (results: unknown[]) => JSON.stringify({ mode: 'resolved-v1', results })

test('concurrent startup reads share an ordered bulk request and preserve all value types', async () => {
  const calls: Record<string, unknown>[] = []
  const values = [false, null, { backend: 'pi' }, 3, 'text']
  const read = createStartupSettings(async (args) => {
    calls.push(args)
    return envelope((args.keys as string[]).map((key, i) => success(key, values[i])))
  })
  const got = await Promise.all(['bool', 'unset', 'object', 'number', 'string'].map(read))
  expect(got).toEqual(values)
  expect(calls).toEqual([{ keys: ['bool', 'unset', 'object', 'number', 'string'], format: 'json' }])
})

test('a complete bulk error row rejects only its key without falling back', async () => {
  const calls: Record<string, unknown>[] = []
  const read = createStartupSettings(async (args) => {
    calls.push(args)
    return envelope([success('on', true), { key: 'bad', ok: false, error: { message: 'config.get: broken' } }])
  })
  const got = await Promise.allSettled([read('on'), read('bad')])
  expect(got[0]).toEqual({ status: 'fulfilled', value: true })
  expect(got[1].status).toBe('rejected')
  expect(String((got[1] as PromiseRejectedResult).reason)).toContain('config.get: broken')
  expect(calls).toHaveLength(1)
})

test('old-server errors and every malformed envelope fall back to precisely the single reads', async () => {
  for (const answer of [
    'throw', 'not json', '{}', JSON.stringify({ results: [success('a'), success('b')] }),
    envelope([success('a')]), envelope([success('a'), success('wrong')]),
    envelope([success('b'), success('a')]), envelope([success('a'), success('a')]),
    envelope([success('a'), { key: 'b', ok: true, scope: 'project' }]),
    envelope([success('a'), { key: 'b', ok: true, value: 2, scope: 5 }]),
    envelope([success('a'), { key: 'b', ok: false, error: {} }]),
    envelope([success('a'), { ...success('b'), warnings: [3] }]),
  ]) {
    const calls: Record<string, unknown>[] = []
    const read = createStartupSettings(async (args) => {
      calls.push(args)
      if ('keys' in args) {
        if (answer === 'throw') throw new Error('unknown keys')
        return answer
      }
      return JSON.stringify({ key: args.key, value: `single:${args.key}`, scope: 'project' })
    })
    expect(await Promise.all([read('a'), read('b')])).toEqual(['single:a', 'single:b'])
    expect(calls).toEqual([{ keys: ['a', 'b'], format: 'json' }, { key: 'a', format: 'json' }, { key: 'b', format: 'json' }])
  }
})

test('fallback single failures remain rejected, and later reads do not reuse startup values', async () => {
  const calls: Record<string, unknown>[] = []
  const read = createStartupSettings(async (args) => {
    calls.push(args)
    if ('keys' in args) throw new Error('older server')
    if (args.key === 'bad') throw new Error('single failure')
    return JSON.stringify({ value: calls.length })
  })
  const got = await Promise.allSettled([read('ok'), read('bad')])
  expect(got[0].status).toBe('fulfilled')
  expect(got[1].status).toBe('rejected')
  expect(await read('ok')).toBe(5)
  expect(calls.every((args) => !('key' in args && 'keys' in args))).toBe(true)
})
