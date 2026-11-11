import { expect, test } from 'claude-code/testing'

import { flagOn, hostedMode, missingKeyWarning } from './hosted'

test('the flag takes the truthy spellings ws-mcp accepts, case and whitespace aside', () => {
  for (const value of ['1', 'true', 'TRUE', ' yes ', 'on']) expect(flagOn(value)).toBe(true)
  for (const value of [undefined, '', '0', 'false', 'off', 'no', 'hosted']) expect(flagOn(value)).toBe(false)
})

test('hosted only with the flag and a non-empty key; the flag without one is its own mode; no flag is off whatever the key', () => {
  expect(hostedMode('1', ' hosted-key ')).toEqual({ mode: 'hosted', key: 'hosted-key' })
  expect(hostedMode('1', undefined)).toEqual({ mode: 'missing-key' })
  expect(hostedMode('on', '  ')).toEqual({ mode: 'missing-key' })
  expect(hostedMode(undefined, 'hosted-key')).toEqual({ mode: 'off' })
  expect(hostedMode('0', 'hosted-key')).toEqual({ mode: 'off' })
})

test('the missing-key warning names both variables and the plugin', () => {
  const text = missingKeyWarning('ws')
  expect(text.startsWith('[ws] ')).toBe(true)
  expect(text).toContain('WS_HOSTED_SESSION ')
  expect(text).toContain('WS_HOSTED_SESSION_KEY')
})
