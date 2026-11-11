/**
 * Hosted-session mode: a session the dashboard daemon spawned and feeds. The
 * daemon writes every input (dashboard messages, mailbox mail) as the
 * session's user turns and reads its output itself, so under the flag the mod
 * runs no mailbox wait (and with it no board-inbox drain and no interrupt
 * handling) and sends no dashboard feed, and the manual feature adopts the
 * daemon-minted key in place of minting one. Everything else (manual
 * delivery, preload, the compaction lever and advisory, the band) runs as
 * usual. The same two variables gate the Pi extension and ws-mcp.
 */

/** Set to a truthy value (`1`, `true`, `yes`, `on`) by the daemon on a session it hosts. */
export const HOSTED_FLAG_ENV = 'WS_HOSTED_SESSION'

/** The lead session key the daemon minted for the session it hosts. */
export const HOSTED_KEY_ENV = 'WS_HOSTED_SESSION_KEY'

/**
 * `off`: no flag, the mod runs as everywhere else. `hosted`: the flag and a
 * key. `missing-key`: the flag without a key, which gates nothing (the mod
 * mints and waits as today) and only warns.
 */
export type HostedMode = { mode: 'off' } | { mode: 'hosted'; key: string } | { mode: 'missing-key' }

/** The truthy spellings ws-mcp's own env flags accept. */
export function flagOn(value: string | undefined): boolean {
  return ['1', 'true', 'yes', 'on'].includes((value ?? '').trim().toLowerCase())
}

/** The mode the two variables' values select. */
export function hostedMode(flag: string | undefined, key: string | undefined): HostedMode {
  if (!flagOn(flag)) return { mode: 'off' }
  const trimmed = (key ?? '').trim()
  return trimmed ? { mode: 'hosted', key: trimmed } : { mode: 'missing-key' }
}

/** The transcript warning for a flag set without a key; `plugin` is the engine-reported name. */
export function missingKeyWarning(plugin: string): string {
  return `[${plugin}] ${HOSTED_FLAG_ENV} is set but ${HOSTED_KEY_ENV} is missing or empty: running as an ordinary session (own key, mailbox wait, dashboard feed).`
}
