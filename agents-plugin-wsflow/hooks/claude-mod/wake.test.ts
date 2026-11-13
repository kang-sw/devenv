import { describe, expect, test } from 'claude-code/testing'

import { createTasks, PERSON_ORIGINS, wakePrompt } from './wake'

const task = (type: string, status = 'completed', body = '', id = 't1', summary = 'Finished') => `<task-notification>
<task-id>${id}</task-id><tool-use-id>u1</tool-use-id><task-type>${type}</task-type>
<status>${status}</status><summary>${summary}</summary>${body}
</task-notification>`
const prompt = (text: string, kind = 'task-notification') => ({ text, origin: { kind } })

describe('observable wake classification', () => {
  test('each supported cause keeps full content and a one-line caption', () => {
    const cases = [
      [prompt(task('local_agent', 'completed', 'The full\nreport')), 'subagent_report', 't1'],
      [prompt(task('local_agent')), 'subagent_done', 't1'],
      [prompt(task('local_agent', 'failed', 'failure details')), 'subagent_failed', 't1'],
      [prompt(task('local_bash', 'completed', '', 'b1', 'Background command "build" completed (exit code 0)')), 'background_exit', 'b1'],
      [prompt('<task-notification><task-id>m1</task-id><summary>Monitor event: "build"</summary><event>New\noutput</event></task-notification>'), 'monitor', 'm1'],
      [prompt('Mail content', 'mail'), 'mail', 'mailbox'],
      [prompt('<teammate-message teammate_id="reviewer">Full\nmessage</teammate-message>', 'peer'), 'message', 'reviewer'],
      [prompt('Time to check the build', 'scheduled-trigger'), 'scheduled', 'scheduler'],
    ] as const
    for (const [p, cause, source] of cases) {
      const wake = wakePrompt(p, 'ws')!
      expect(wake.cause).toBe(cause)
      expect(wake.source).toBe(source)
      expect(wake.text).toBe(p.text)
      expect(wake.caption.includes('\n')).toBe(false)
    }
    expect(wakePrompt({ text: 'check', origin: { kind: 'task-notification', subkind: 'scheduled-trigger' } }, 'ws')?.cause).toBe('scheduled')
  })

  test('observable task identity fills older notifications without a task-type', () => {
    const p = prompt(task('', 'stopped'))
    expect(wakePrompt(p, 'ws', 'background')?.cause).toBe('background_exit')
    expect(wakePrompt(p, 'ws', 'monitors')?.cause).toBe('monitor')
    expect(wakePrompt(p, 'ws', undefined, true)?.cause).toBe('subagent_failed')
    // Unknown tasks are not silently claimed to be subagents or shells.
    expect(wakePrompt(p, 'ws')?.cause).toBe('message')
  })

  test('person input cannot become a wake even when it contains an exact harness envelope', () => {
    for (const kind of PERSON_ORIGINS) {
      expect(wakePrompt(prompt(task('local_bash'), kind), 'ws', 'background')).toBeUndefined()
      expect(wakePrompt({ ...prompt(task('local_agent'), kind), isMeta: true }, 'ws')).toBeUndefined()
    }
    expect(wakePrompt(prompt('please continue', 'plugin'), 'ws')).toBeUndefined()
    expect(wakePrompt(prompt('arbitrary meta', 'observer'), 'ws')).toBeUndefined()
    expect(wakePrompt(prompt('  '), 'ws')).toBeUndefined()
    expect(wakePrompt({ text: '[ws mailbox] mail waiting', origin: { kind: 'plugin', name: 'ws' } }, 'ws')?.cause).toBe('mail')
    expect(wakePrompt({ text: '[ws mailbox] mail waiting', origin: { kind: 'plugin', name: 'other' } }, 'ws')).toBeUndefined()
  })
})

describe('observable background tasks', () => {
  test('shell launch, identity, notification finish, duplicate finish, and stop', () => {
    const tasks = createTasks()
    tasks.began(undefined, 'u1', 'Bash', { run_in_background: true })
    expect(tasks.counts()).toEqual({ background: 1, monitors: 0 })
    tasks.ended(undefined, 'u1', false, { result: { backgroundTaskId: 'b1' } })
    tasks.abort(undefined) // Successfully detached tasks outlive the turn.
    expect(tasks.kind('b1')).toBe('background')
    tasks.notified(task('local_bash', 'completed', '', 'b1'))
    tasks.notified(task('local_bash', 'completed', '', 'b1'))
    expect(tasks.counts()).toEqual({ background: 0, monitors: 0 })
    tasks.began(undefined, 'u2', 'PowerShell', { run_in_background: true })
    tasks.ended(undefined, 'u2', false, { result: { backgroundTaskId: 'b2' } })
    tasks.began(undefined, 'stop', 'TaskStop', { task_id: 'b2' })
    tasks.ended(undefined, 'stop', true)
    expect(tasks.counts().background).toBe(1) // A refused stop kills nothing.
    tasks.began(undefined, 'stop2', 'KillShell', { shell_id: 'b2' })
    tasks.ended(undefined, 'stop2', false)
    expect(tasks.counts().background).toBe(0)
  })

  test('monitor output leaves it armed; terminal notifications and stop/expiry remove it', () => {
    const tasks = createTasks()
    tasks.began('a1', 'mcall', 'Monitor', { command: 'tail -f log' })
    tasks.ended('a1', 'mcall', false, { result: { taskId: 'm1', timeoutMs: 10000 } })
    expect(tasks.counts()).toEqual({ background: 0, monitors: 1 })
    tasks.notified('<task-notification><task-id>m1</task-id><summary>Monitor event</summary><event>output</event></task-notification>')
    expect(tasks.counts().monitors).toBe(1)
    tasks.notified('<task-notification><task-id>m1</task-id><summary>Monitor event</summary><event>[Monitor timed out]</event></task-notification>')
    expect(tasks.counts().monitors).toBe(0)
    tasks.began(undefined, 'm2', 'Monitor', {})
    tasks.ended(undefined, 'm2', false, { result: { taskId: 'm2' } })
    tasks.notified(task('monitor', 'failed', '', 'm2'))
    expect(tasks.counts().monitors).toBe(0)
  })

  test('failed/aborted launches clear, foreground and agent tools do not count, auto-background results do', () => {
    const tasks = createTasks()
    tasks.began(undefined, 'u1', 'Bash', { run_in_background: true })
    tasks.ended(undefined, 'u1', true)
    tasks.began(undefined, 'u2', 'Monitor', {})
    tasks.abort(undefined)
    tasks.began(undefined, 'u3', 'Agent', { run_in_background: true })
    tasks.began(undefined, 'u4', 'Bash', {})
    tasks.ended(undefined, 'u4', false, { result: { stdout: 'done' } })
    expect(tasks.counts()).toEqual({ background: 0, monitors: 0 })
    tasks.began(undefined, 'u5', 'Bash', {})
    tasks.ended(undefined, 'u5', false, { text: '{"backgroundTaskId":"b5"}' })
    expect(tasks.counts().background).toBe(1)
    tasks.clear()
    expect(tasks.counts()).toEqual({ background: 0, monitors: 0 })
    // A shell detached by an interrupt may return its identity after the
    // turn end; the result, not the abort alone, says whether it survived.
    tasks.began(undefined, 'late', 'Bash', {})
    tasks.abort(undefined)
    tasks.ended(undefined, 'late', false, { result: { backgroundTaskId: 'alive', backgroundedByTurnAbort: true } })
    expect(tasks.counts().background).toBe(1)
  })

  test('a completion can beat the launch result, matched by tool-use-id, without resurrection', () => {
    const tasks = createTasks()
    tasks.began(undefined, 'u1', 'Bash', { run_in_background: true })
    tasks.notified(task('local_bash'))
    tasks.ended(undefined, 'u1', false, { result: { backgroundTaskId: 't1' } })
    expect(tasks.counts().background).toBe(0)
  })
})
