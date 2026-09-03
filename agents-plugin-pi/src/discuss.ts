/**
 * Pure kickoff-string builder for the `/ws-discuss` proof-of-concept command
 * (Phase 4 MVP gate). Split out of index.ts's IO glue so the exact prompt
 * shape is unit-assertable, matching the Phase 1-3 pure-helper/IO-split
 * convention.
 *
 * The returned string is fed to
 * `pi.sendUserMessage(..., { expandPromptTemplates: true })`, which expands the
 * leading `/skill:lead-discuss <topic>` into the discuss skill body, appending
 * everything after it as the skill's `User:` args (docs/skills.md#L76-83). That
 * single message therefore drives all three MVP gate actions in one run:
 *   (a) skills-load    — the `/skill:lead-discuss` expansion loads the skill;
 *   (b) bridge call    — the discuss skill body itself calls the bridged
 *                        `ws__playbook_print` / `ws__workflow_manual` tools;
 *   (c) spawn round-trip — the appended instruction tells the model to dispatch
 *                        one `explore` recon leaf and report its result.
 *
 * (a)+(b) come free from the skill; (c) is NOT inherent to the discuss skill,
 * so the kickoff MUST add it explicitly to make gate (c) deterministic (see the
 * plan's "Risk signal" finding). This keeps the gate a live model-driven run
 * (the Phase 2-3 style), not a unit assertion — the assertion here only pins
 * the kickoff wording that steers that run.
 */

const DEFAULT_TOPIC =
  "the ws-pi-native MVP proof-of-concept: does skills-load + bridge + spawner compose end-to-end on Pi?";

const SPAWN_INSTRUCTION =
  "After loading the discuss procedure, also dispatch one `explore` recon leaf " +
  "to survey how the ws skills are exposed to Pi (the resources_discover skillPaths wiring) " +
  "and report its result — this proves the ws-mcp bridge and the delegation spawner compose.";

/**
 * Builds the kickoff user-message for `/ws-discuss`. `args` is the raw command
 * argument string; when blank, a default PoC topic is used so the command works
 * as a bare `/ws-discuss` gate invocation.
 */
export function buildDiscussKickoff(args: string): string {
  const topic = args.trim().length > 0 ? args.trim() : DEFAULT_TOPIC;
  return `/skill:lead-discuss ${topic}\n\n${SPAWN_INSTRUCTION}`;
}
