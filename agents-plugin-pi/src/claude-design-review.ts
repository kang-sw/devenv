import { readFileSync, realpathSync, statSync } from "node:fs";
import { isAbsolute, relative, resolve, sep } from "node:path";
import { parse as parseYaml } from "yaml";

interface ToolContent { type: string; text?: string }
interface ToolResult { content: ToolContent[]; isError?: boolean }
export interface ClaudeDesignReviewAccess { callTool(name: string, args: Record<string, unknown>): Promise<ToolResult>; }
export interface ClaudeDesignReviewContext { taskFrame: string; request: string; }
export type ClaudeDesignReviewContextProvider = (input: { cwd: string; request: string; paths?: readonly string[]; signal?: AbortSignal }) => Promise<ClaudeDesignReviewContext>;

interface DesignRequest { ticketPath: string; relations: string; }
interface TicketRecord { path?: unknown; }
const MAX_CONTEXT_CHARS = 500_000;

function firstText(result: ToolResult): string {
  const text = result.content.find(item => item.type === "text")?.text;
  if (result.isError || typeof text !== "string" || text.length === 0) throw new Error("missing design-review lookup context");
  return text;
}

export function parseClaudeDesignReviewRequest(request: string): DesignRequest {
  const match = request.trim().match(/^Ticket:[ \t]*([^\r\n]+)\r?\nRelations:[ \t]*\r?\n([\s\S]+)$/);
  if (!match?.[1]?.trim() || !match[2]?.trim()) throw new Error("design-review request must contain Ticket and explicit Relations context");
  return { ticketPath: match[1].trim().replace(/^@/, ""), relations: match[2].trim() };
}

function readArtifact(root: string, requestedPath: string): { path: string; body: string } {
  const raw = requestedPath.replace(/^@/, "");
  const candidate = resolve(root, raw);
  let canonical: string;
  try { canonical = realpathSync(candidate); } catch { throw new Error(`missing design-review artifact: ${raw}`); }
  const fromRoot = relative(root, canonical);
  if (isAbsolute(fromRoot) || fromRoot === ".." || fromRoot.startsWith(`..${sep}`) || !statSync(canonical).isFile()) throw new Error(`design-review artifact is outside the worktree: ${raw}`);
  return { path: fromRoot.split(sep).join("/"), body: readFileSync(canonical, "utf8") };
}

function ticketParent(body: string): string | undefined {
  const header = body.match(/^---\r?\n([\s\S]*?)\r?\n---/);
  if (!header?.[1]) return undefined;
  const value = parseYaml(header[1]) as { parent?: unknown } | undefined;
  return typeof value?.parent === "string" && value.parent.trim() ? value.parent.trim() : undefined;
}

function ticketRecords(text: string): TicketRecord[] {
  const value = JSON.parse(text) as unknown;
  if (Array.isArray(value)) return value as TicketRecord[];
  return value && typeof value === "object" ? [value as TicketRecord] : [];
}

function framePlaybook(canonical: string): string {
  return `${canonical.trim()}\n\n## Pi adapter context\n\nThe parent adapter already resolved the ticket, current ready inventory, named parent epic, Relations context, and any explicitly supplied artifacts into the user message. Use only that bundle for repository context. Do not call ws tools or spawn explorers; those canonical-playbook capabilities are unavailable in this closed Claude child. If the supplied bundle is missing evidence required by the canonical criteria, report the incomplete check in omitted and do not claim a complete verdict. Preserve the canonical design criteria, severity rules, and exact verdict format.`;
}

function bundle(parsed: DesignRequest, artifacts: readonly { path: string; body: string }[]): string {
  const sections = artifacts.map(artifact => `--- artifact: ${artifact.path} ---\n${artifact.body.trimEnd()}`).join("\n\n");
  const value = `Review the supplied ticket using the canonical design-review contract.\n\nTicket: ${parsed.ticketPath}\nRelations:\n${parsed.relations}\n\nRead-only context bundle:\n${sections}`;
  if (value.length > MAX_CONTEXT_CHARS) throw new Error("design-review context is too large");
  return value;
}

export function createClaudeDesignReviewContextProvider(access: ClaudeDesignReviewAccess): ClaudeDesignReviewContextProvider {
  return async ({ cwd, request, paths = [], signal }) => {
    signal?.throwIfAborted();
    const parsed = parseClaudeDesignReviewRequest(request);
    const root = realpathSync(resolve(cwd));
    if (!statSync(root).isDirectory()) throw new Error("design-review worktree is unavailable");

    const playbook = firstText(await access.callTool("playbook.read", { name: "ticket-reviewer-design" }));
    signal?.throwIfAborted();
    const target = readArtifact(root, parsed.ticketPath);
    const ready = ticketRecords(firstText(await access.callTool("tickets.query", { statuses: ["ready"], format: "json" })));
    signal?.throwIfAborted();

    const ordered = new Map<string, { path: string; body: string }>([[target.path, target]]);
    for (const record of ready) {
      if (typeof record.path !== "string") throw new Error("ready inventory omitted a ticket path");
      const artifact = readArtifact(root, record.path);
      if (artifact.path !== target.path) ordered.set(artifact.path, artifact);
    }

    const parent = ticketParent(target.body);
    if (parent) {
      const found = ticketRecords(firstText(await access.callTool("tickets.query", { ticket_stem: parent, include_done: true, include_dropped: true, format: "json" })));
      if (found.length !== 1 || typeof found[0]?.path !== "string") throw new Error("named parent epic could not be resolved");
      const artifact = readArtifact(root, found[0].path);
      ordered.set(artifact.path, artifact);
    }
    for (const path of paths) {
      const artifact = readArtifact(root, path);
      ordered.set(artifact.path, artifact);
    }
    signal?.throwIfAborted();
    return { taskFrame: framePlaybook(playbook), request: bundle({ ...parsed, ticketPath: target.path }, [...ordered.values()]) };
  };
}
