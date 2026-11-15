type Call = (args: Record<string, unknown>) => Promise<string>
type Row = { key: string; ok: true; value: unknown; scope: string; warnings?: string[] } | { key: string; ok: false; error: { message: string } }

const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value)

/** Validate the entire envelope before accepting any row: older daemons may ignore keys. */
function bulkRows(text: string, keys: string[]): Row[] {
  const data: unknown = JSON.parse(text)
  if (!object(data) || data.mode !== 'resolved-v1' || !Array.isArray(data.results) || data.results.length !== keys.length) throw new Error('invalid bulk config result')
  for (let i = 0; i < keys.length; i += 1) {
    const row: unknown = data.results[i]
    if (!object(row) || row.key !== keys[i]) throw new Error('invalid bulk config key')
    if (row.ok === true) {
      if (!Object.prototype.hasOwnProperty.call(row, 'value') || typeof row.scope !== 'string' || 'error' in row ||
        ('warnings' in row && (!Array.isArray(row.warnings) || !row.warnings.every((warning) => typeof warning === 'string')))) throw new Error('invalid bulk config success')
    } else if (row.ok !== false || !object(row.error) || typeof row.error.message !== 'string' || 'value' in row || 'scope' in row) {
      throw new Error('invalid bulk config error')
    }
  }
  return data.results as Row[]
}

/**
 * A single start generation's keyless reads. Only contemporaneous requests are
 * coalesced; feature gates still decide whether and when each key is read.
 * Key adoption, tuning and manual retries bypass this startup-only reader.
 */
export function createStartupSettings(call: Call): (key: string) => Promise<unknown> {
  type Pending = { key: string; resolve: (value: unknown) => void; reject: (reason: unknown) => void; promise: Promise<unknown> }
  let pending = new Map<string, Pending>()
  const flush = async () => {
    const batch = [...pending.values()]
    pending = new Map()
    let rows: Row[]
    try {
      rows = bulkRows(await call({ keys: batch.map((entry) => entry.key), format: 'json' }), batch.map((entry) => entry.key))
    } catch {
      // T0: no dummy key on the bulk request; older servers get the exact
      // legacy requests, with the same (keyless) session context.
      await Promise.all(batch.map(async (entry) => {
        try {
          entry.resolve((JSON.parse(await call({ key: entry.key, format: 'json' })) as { value?: unknown }).value)
        } catch (err) {
          entry.reject(err)
        }
      }))
      return
    }
    rows.forEach((row, i) => {
      const entry = batch[i]! // bulkRows checked the exact count and order.
      if (row.ok) entry.resolve(row.value)
      else entry.reject(new Error(row.error.message))
    })
  }
  return (key) => {
    const held = pending.get(key)
    if (held) return held.promise
    let resolve!: Pending['resolve']
    let reject!: Pending['reject']
    const promise = new Promise<unknown>((yes, no) => { resolve = yes; reject = no })
    pending.set(key, { key, resolve, reject, promise })
    if (pending.size === 1) void Promise.resolve().then(flush)
    return promise
  }
}
