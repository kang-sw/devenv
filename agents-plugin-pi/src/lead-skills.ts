/**
 * 260906 Phase 1 (`260906-bug-ws-pi-lead-cannot-see-or-load-skills`): the
 * lead/fork skill surface — an `<available_skills>` block in the ws
 * system-prompt block, and a `ws-skill(name, args?)` tool to load one.
 *
 * Root cause this phase fixes: `computeLeadActiveTools` (execute-gateway.ts)
 * removes native `read`/`bash` from the lead's (and a fork's) active-tools
 * surface, but Pi's own skill-loading path (`/skill:<name>` expansion,
 * `formatSkillsForPrompt`'s `<available_skills>` block) both assume a model
 * that can call `read` on the SKILL.md path it names — a block the reshaped
 * lead surface can no longer act on, and Pi's own block generation happens
 * upstream of this extension's reshape anyway, so it is never rendered for a
 * ws lead/fork at all (confirmed: `formatSkillsForPrompt` is only reachable
 * from Pi's own system-prompt assembly, which this extension's
 * `before_agent_start` override runs alongside, not instead of — the ws
 * block is additive, so a would-be Pi skills block is simply absent, not
 * overwritten). The fix is adapter-owned, mirroring the block/tool split
 * already used for the workflow manual (`lead-bootstrap.ts`) and the
 * approval gateway (`execute-gateway.ts`): render our own
 * `<available_skills>` block (pointing at `ws-skill`, never `read`) and
 * register `ws-skill` itself as a lead/fork tool.
 *
 * Skill source of truth is `pi.getCommands()` — the SAME list backing Pi's
 * own `/skill:<name>` slash commands — filtered to `source: "skill"`
 * entries, never a ws-tree directory scan. This means the block and the tool
 * cover every installed skill (ws skills and any other skill pack alike),
 * not just `agents-plugin/skills/`.
 *
 * Dogfood bug fixed here: `pi.getCommands()` is a LIVE read (confirmed
 * against the installed `agent-session.js`'s `getCommands` closure, which
 * reads `this._resourceLoader.getSkills()` fresh on every call), but Pi's
 * own startup sequence runs `session_start` FIRST and only afterwards calls
 * `extendResourcesFromExtensions` (which merges THIS adapter's own
 * `resources_discover` skill path into the resource loader). A ws-skill
 * lookup or `<available_skills>` block built from a `pi.getCommands()`
 * snapshot taken AT `session_start` therefore predates the ws skills
 * entirely, on the affected first-session-of-a-process path — the model
 * only ever saw `imagegen` (or whatever other extension registered its
 * skills before ws did) and `ws-skill lead-drain-ready-queue` answered
 * "Unknown skill". The fix: `resolveSkillEntries(pi.getCommands())` is
 * called LIVE at ws-skill's own `execute()` time (this file) and again at
 * each `before_agent_start` firing for the `<available_skills>` block
 * (`lead-bootstrap.ts`'s `computeSkillsBlockCached`) — never captured into a
 * `session_start`-scoped ref. `sourceInfo.path` (the SKILL.md path) is
 * therefore re-resolved on every read too; only the file body is cached (by
 * `computeSkillsBlockCached`, once), for the reverse reason the
 * workflow-manual snapshot caches its body but not its per-call state.
 *
 * `parseFrontmatter`/`SkillFrontmatter` are imported from
 * `@earendil-works/pi-coding-agent` rather than hand-rolled: it is the exact
 * function Pi's own `/skill:<name>` expansion uses
 * (`agent-session.js#_expandSkillCommand`) to strip frontmatter and read
 * `disable-model-invocation`, so `ws-skill`'s stripping behavior is
 * byte-identical to Pi's own slash command for the same file.
 *
 * Gate shape: `isLeadOrFork(role)` only — never conditioned on whether
 * `read`/`bash` are currently active. Gating on tool-set membership would
 * silently reproduce this exact bug for a different reason (evaluated at the
 * wrong point relative to `index.ts`'s reshape sequencing), so the ticket
 * calls this out explicitly. `addSkillToolIfLeadOrFork` therefore uses
 * `isLeadOrFork` (lead AND fork), not the narrower `role !== undefined`
 * lead-only gate `addForkToolIfLead`/`addAskToolsIfLead` use — this tool's
 * applies-to-both semantics matches `computeLeadActiveTools`'s, not those
 * two lead-only functions'.
 *
 * Neither `loadSkillFile` nor `computeWsSkillResult` ever throws: a
 * missing/unreadable SKILL.md is reported back to the model as a
 * `ws-skill` result naming the path, and `buildSkillsBlock` silently skips
 * that entry from the block rather than surfacing an error there — matching
 * the ticket's "neither path throws" constraint.
 */

import { readFileSync } from "node:fs";
import type { ExtensionAPI, SlashCommandInfo } from "@earendil-works/pi-coding-agent";
import { parseFrontmatter, type SkillFrontmatter } from "@earendil-works/pi-coding-agent";
import { isLeadOrFork, type SpawnRole } from "./process-role.ts";

/** Lead-facing tool name (pi-lead-guide.md), registered below. */
export const WS_SKILL_TOOL_NAME = "ws-skill";

/** One entry from `pi.getCommands()`'s `source: "skill"` subset. */
export interface SkillEntry {
  name: string;
  description: string;
  path: string;
}

export type LoadedSkill = { ok: true; body: string; disableModelInvocation: boolean } | { ok: false; error: string };

/**
 * `pi.getCommands()` -> `SkillEntry[]`: filters to `source: "skill"` and
 * strips the `"skill:"` name prefix `pi.getCommands()` prepends (matching
 * `formatSkillsForPrompt`'s own bare-name convention and the ticket's
 * `ws-skill <name>` calling shape).
 */
export function resolveSkillEntries(commands: readonly SlashCommandInfo[]): SkillEntry[] {
  return commands
    .filter((c) => c.source === "skill")
    .map((c) => ({
      name: c.name.startsWith("skill:") ? c.name.slice("skill:".length) : c.name,
      description: c.description ?? "",
      path: c.sourceInfo.path,
    }));
}

/**
 * Reads and parses one SKILL.md, via the caller-injected `readFile` (default
 * `readFileSync`) — the same pure-logic/injected-IO split `execute-gateway.ts`
 * and `fork.ts` already use, so this needs no fake `ExtensionAPI`/filesystem
 * for its own unit tests, only a fake `readFile`. Never throws: a read
 * failure is returned as `{ ok: false, error }`, with `path` folded into the
 * message so `computeWsSkillResult`'s error text names it without having to
 * carry `path` separately. `parseFrontmatter` is wrapped in its OWN `try`
 * (review cycle 1, Important #1): it calls the `yaml` package's `parse()` on
 * the frontmatter block and throws `YAMLParseError` on malformed YAML (Pi's
 * own resource loader tolerates this by dropping the skill at load time —
 * `dist/core/skills.js`'s `loadSkillFromFile` returns `{skill: null}` on the
 * same failure — but a live-edited SKILL.md with a YAML typo, or one edited
 * in the window between resource load and this session's `session_start`,
 * reaches `loadSkillFile` directly). Left unguarded, that throw would
 * propagate out of `buildSkillsBlock` and `computeSessionBootstrap`,
 * aborting `session_start` before `pi.setActiveTools()` ran — losing the
 * WHOLE lead/fork tool reshape and ws block, not just the one skill — and
 * out of `ws-skill`'s own `execute()` instead of returning the documented
 * error string. A parse failure is therefore treated exactly like a read
 * failure: `{ ok: false, error }`, which `buildSkillsBlock` already skips
 * silently and `computeWsSkillResult` already reports via `ws-skill`.
 */
export function loadSkillFile(path: string, readFile: (p: string) => string = (p) => readFileSync(p, "utf8")): LoadedSkill {
  let raw: string;
  try {
    raw = readFile(path);
  } catch (err) {
    return { ok: false, error: `could not read "${path}": ${err instanceof Error ? err.message : String(err)}` };
  }
  try {
    const { frontmatter, body } = parseFrontmatter<SkillFrontmatter>(raw);
    return { ok: true, body, disableModelInvocation: frontmatter["disable-model-invocation"] === true };
  } catch (err) {
    return { ok: false, error: `could not parse frontmatter in "${path}": ${err instanceof Error ? err.message : String(err)}` };
  }
}

function escapeXml(str: string): string {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&apos;");
}

/**
 * Renders the `<available_skills>` block: one `<skill>` per entry whose
 * SKILL.md loaded successfully AND is not `disable-model-invocation:true`
 * (a disabled-from-block skill stays loadable by exact name — see
 * `computeWsSkillResult` — it is only hidden from this listing, mirroring
 * Pi's own `formatSkillsForPrompt` exclusion). An unreadable SKILL.md is
 * silently skipped here (never surfaced as an error in the block itself —
 * `ws-skill` is where a load failure is reported). Returns `""` when the
 * visible set is empty, so `buildWsBlock`'s caller can join it in
 * unconditionally.
 *
 * Deliberately NOT `formatSkillsForPrompt` (mirrors its XML shape and
 * private `escapeXml`, reimplemented locally — that function's preamble is
 * hard-coded to instruct `read`, which is absent from the reshaped lead/fork
 * surface, and it lacks Pi's own "resolve relative paths against the skill
 * directory" line on purpose, since `ws-skill` takes a name, not a path).
 */
export function buildSkillsBlock(entries: readonly SkillEntry[], loadFile: (path: string) => LoadedSkill): string {
  const visible = entries.filter((e) => {
    const loaded = loadFile(e.path);
    return loaded.ok && !loaded.disableModelInvocation;
  });
  if (visible.length === 0) {
    return "";
  }
  const lines = [
    "The following skills provide specialized instructions for specific tasks.",
    `Load a skill's file with \`${WS_SKILL_TOOL_NAME} <name>\` when the task matches its description — never with \`read\` (it is not on your tool surface).`,
    "",
    "<available_skills>",
  ];
  for (const entry of visible) {
    lines.push("  <skill>");
    lines.push(`    <name>${escapeXml(entry.name)}</name>`);
    lines.push(`    <description>${escapeXml(entry.description)}</description>`);
    lines.push(`    <location>${escapeXml(entry.path)}</location>`);
    lines.push("  </skill>");
  }
  lines.push("</available_skills>");
  return lines.join("\n");
}

/**
 * Pure `ws-skill(name, args?)` body. Looks `name` up against ALL `entries`
 * (not just the visible/block-listed subset — a `disable-model-invocation:
 * true` skill must still be loadable by exact name, same as Pi's own
 * `/skill:<name>` expansion). Frontmatter is stripped from the returned
 * body (via `loadFile`); `args`, when given, is appended as a trailing
 * `User: <args>` line.
 */
export function computeWsSkillResult(name: string, args: string | undefined, entries: readonly SkillEntry[], loadFile: (path: string) => LoadedSkill): string {
  const entry = entries.find((e) => e.name === name);
  if (!entry) {
    const names = entries.map((e) => e.name).sort();
    return `Unknown skill "${name}". Available skills: ${names.length > 0 ? names.join(", ") : "(none)"}`;
  }
  const loaded = loadFile(entry.path);
  if (!loaded.ok) {
    return `Error loading skill "${name}": ${loaded.error}`;
  }
  return args ? `${loaded.body}\n\nUser: ${args}` : loaded.body;
}

/**
 * Pure, lead-AND-fork-gated `ws-skill` active-tools addition. Uses
 * `isLeadOrFork` (not the narrower lead-only `role !== undefined` gate
 * `addForkToolIfLead`/`addAskToolsIfLead` use in fork.ts/ask.ts) — see this
 * file's header comment for why the gate shape differs from those two.
 */
export function addSkillToolIfLeadOrFork(activeTools: readonly string[], role: SpawnRole | undefined): string[] {
  if (!isLeadOrFork(role) || activeTools.includes(WS_SKILL_TOOL_NAME)) {
    return [...activeTools];
  }
  return [...activeTools, WS_SKILL_TOOL_NAME];
}

/**
 * Registers `ws-skill` declaratively (factory scope, same placement as
 * `registerFork`/`registerAsk`) so a fork child's own re-run of
 * `session_start` has it present to activate too. Resolves
 * `pi.getCommands()` LIVE inside `execute()` (never a ref captured at
 * `session_start`) — see this file's header comment for the dogfood bug this
 * fixes: Pi merges an extension's skills into `pi.getCommands()` AFTER
 * `session_start` returns, so a snapshot taken at registration or at
 * `session_start` time can miss every ws skill on the affected path. Whether
 * `ws-skill` is ever ACTIVE for a given session is `addSkillToolIfLeadOrFork`'s
 * job, not this function's.
 */
export function registerWsSkillTool(pi: ExtensionAPI): void {
  pi.registerTool({
    name: WS_SKILL_TOOL_NAME,
    label: WS_SKILL_TOOL_NAME,
    description:
      "Load a skill's instructions by name (from the <available_skills> list in your system prompt, or any other installed skill) — the replacement for `read`-ing a SKILL.md directly, which is not on your tool surface. Returns the skill body with its frontmatter stripped; an unknown name returns the list of available names instead of erroring.",
    parameters: {
      type: "object",
      properties: {
        name: { type: "string", description: "Skill name, exactly as listed in <available_skills> (no `skill:` prefix)." },
        args: { type: "string", description: "Optional free-text arguments to pass the skill — appended to the loaded body as a trailing `User: <args>` line." },
      },
      required: ["name"],
    } as never,
    async execute(_toolCallId, params) {
      const p = params as { name: string; args?: string };
      const entries = resolveSkillEntries(pi.getCommands());
      const text = computeWsSkillResult(p.name, p.args, entries, (path) => loadSkillFile(path));
      return { content: [{ type: "text", text }] };
    },
  });
}
