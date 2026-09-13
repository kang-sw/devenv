import { lstatSync, realpathSync, statSync } from "node:fs";
import { basename, dirname, isAbsolute, join, posix, relative, resolve, sep } from "node:path";
import { homedir } from "node:os";
import { fileURLToPath } from "node:url";
import type { ExtensionAPI, EditToolInput, WriteToolInput } from "@earendil-works/pi-coding-agent";
import { createEditToolDefinition, createWriteToolDefinition } from "@earendil-works/pi-coding-agent";

export type WriteScope =
  | { path: string; kind: "file" }
  | { path: string; kind: "tree"; include?: string[] };

export type NormalizedWriteScope =
  | { readonly path: string; readonly kind: "file" }
  | { readonly path: string; readonly kind: "tree"; readonly include?: readonly string[] };

export type EffectiveWriteCapability =
  | { readonly mode: "none" }
  | { readonly mode: "unrestricted" }
  | { readonly mode: "scoped"; readonly scopes: readonly NormalizedWriteScope[] };

const NONE: EffectiveWriteCapability = Object.freeze({ mode: "none" });
const UNRESTRICTED: EffectiveWriteCapability = Object.freeze({ mode: "unrestricted" });

function scopeError(detail: string): Error {
  return new Error(`ws-pi-agent: invalid write scope: ${detail}`);
}

function patternError(): Error {
  return new Error("ws-pi-agent: invalid write scope include pattern");
}

function unescapedAt(value: string, index: number): boolean {
  let slashes = 0;
  for (let cursor = index - 1; cursor >= 0 && value[cursor] === "\\"; cursor -= 1) slashes += 1;
  return slashes % 2 === 0;
}

function validateBracketRange(body: string): void {
  const tokens: Array<{ char: string; escaped: boolean }> = [];
  for (let index = 0; index < body.length; index += 1) {
    let escaped = false;
    if (body[index] === "\\") {
      if (index === body.length - 1) throw patternError();
      escaped = true;
      index += 1;
    }
    tokens.push({ char: body[index]!, escaped });
  }
  for (let index = 1; index < tokens.length - 1; index += 1) {
    if (tokens[index]!.char === "-" && !tokens[index]!.escaped && tokens[index - 1]!.char.codePointAt(0)! > tokens[index + 1]!.char.codePointAt(0)!) throw patternError();
  }
}

/** Accept only the positive path.matchesGlob subset promised by the public contract. */
export function validateWriteScopePattern(pattern: string): void {
  if (!pattern || pattern.includes("\0") || pattern.startsWith("/") || pattern.endsWith("/")) throw patternError();
  const segments = pattern.split("/");
  if (segments.some(segment => !segment || segment === "." || segment === "..")) throw patternError();
  if (pattern[0] === "!") throw patternError();

  for (const segment of segments) {
    let inClass = false;
    let classStart = -1;
    let hasDoubleStar = false;
    for (let index = 0; index < segment.length; index += 1) {
      const char = segment[index]!;
      if (char === "\\") {
        if (index === segment.length - 1) throw patternError();
        index += 1;
        continue;
      }
      if (!inClass && (char === "{" || char === "}")) throw patternError();
      if (!inClass && char === "(" && index > 0 && unescapedAt(segment, index - 1) && ["?", "*", "+", "@", "!"].includes(segment[index - 1]!)) throw patternError();
      if (char === "[") {
        if (inClass) throw patternError();
        inClass = true;
        classStart = index;
        continue;
      }
      if (char === "]") {
        if (!inClass || index === classStart + 1) throw patternError();
        const first = segment[classStart + 1];
        if (first === "!" || first === "^") throw patternError();
        validateBracketRange(segment.slice(classStart + 1, index));
        inClass = false;
        continue;
      }
      if (!inClass && char === "*" && segment[index + 1] === "*" && unescapedAt(segment, index + 1)) hasDoubleStar = true;
    }
    if (inClass) throw patternError();
    if (hasDoubleStar && segment !== "**") throw patternError();
  }

  try {
    posix.matchesGlob("write-scope-validation", pattern);
  } catch {
    throw patternError();
  }
}

function frozenScope(scope: NormalizedWriteScope): NormalizedWriteScope {
  if (scope.kind === "tree" && scope.include) Object.freeze(scope.include);
  return Object.freeze(scope);
}

function hasOnlyKeys(value: object, allowed: readonly string[]): boolean {
  return Object.keys(value).every(key => allowed.includes(key));
}

function parseNormalizedScope(value: unknown): NormalizedWriteScope {
  const scope = value as { path?: unknown; kind?: unknown; include?: unknown } | null;
  if (!scope || typeof scope !== "object" || Array.isArray(scope) || typeof scope.path !== "string" || !isAbsolute(scope.path) || resolve(scope.path) !== scope.path) {
    throw scopeError("path must be normalized and absolute");
  }
  if (scope.kind === "file") {
    if (!hasOnlyKeys(scope, ["path", "kind"])) throw scopeError("file scopes do not accept include patterns");
    return frozenScope({ path: scope.path, kind: "file" });
  }
  if (scope.kind !== "tree" || !hasOnlyKeys(scope, ["path", "kind", "include"])) throw scopeError("kind must be file or tree");
  if (scope.include !== undefined && (!Array.isArray(scope.include) || scope.include.length === 0 || scope.include.some(pattern => typeof pattern !== "string"))) {
    throw scopeError("tree include must be a nonempty string array");
  }
  const include = scope.include as string[] | undefined;
  include?.forEach(validateWriteScopePattern);
  return frozenScope({ path: scope.path, kind: "tree", ...(include ? { include: [...new Set(include)] } : {}) });
}

/** Parse a persisted binding without re-resolving it against mutable filesystem state. */
export function parseEffectiveWriteCapability(value: unknown): EffectiveWriteCapability {
  const capability = value as { mode?: unknown; scopes?: unknown } | null;
  if (!capability || typeof capability !== "object" || Array.isArray(capability)) throw scopeError("malformed persisted capability");
  if (capability.mode === "none" && hasOnlyKeys(capability, ["mode"])) return NONE;
  if (capability.mode === "unrestricted" && hasOnlyKeys(capability, ["mode"])) return UNRESTRICTED;
  if (capability.mode !== "scoped" || !hasOnlyKeys(capability, ["mode", "scopes"]) || !Array.isArray(capability.scopes) || capability.scopes.length === 0) {
    throw scopeError("malformed persisted capability");
  }
  return Object.freeze({ mode: "scoped", scopes: Object.freeze(capability.scopes.map(parseNormalizedScope)) });
}

const UNICODE_SPACES = /[\u00A0\u2000-\u200A\u202F\u205F\u3000]/g;

/** Mirror Pi's native edit/write input conveniences, then pass only the checked absolute result back to Pi. */
function resolveNativeWriteInput(path: string, cwd: string): string {
  let normalized = path.replace(UNICODE_SPACES, " ");
  if (normalized.startsWith("@")) normalized = normalized.slice(1);
  if (process.platform === "win32" && normalized.startsWith("/") && !normalized.startsWith("//") && !normalized.includes("\\")) {
    const match = normalized.match(/^\/(?:mnt\/|cygdrive\/)?([a-z])(?:\/(.*))?$/i);
    if (match) normalized = `${match[1]!.toUpperCase()}:\\${match[2]?.replaceAll("/", "\\") ?? ""}`;
  }
  if (normalized === "~") normalized = homedir();
  else if (normalized.startsWith("~/") || (process.platform === "win32" && normalized.startsWith("~\\"))) normalized = join(homedir(), normalized.slice(2));
  if (/^file:\/\//.test(normalized)) normalized = fileURLToPath(normalized);
  return resolve(cwd, normalized);
}

function lstat(path: string): ReturnType<typeof lstatSync> | undefined {
  try { return lstatSync(path); } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return undefined;
    throw error;
  }
}

/** Canonicalize an existing target or its nearest existing ancestor without following a dangling link. */
function canonicalPotentialPath(path: string): string {
  let cursor = resolve(path);
  const tail: string[] = [];
  for (;;) {
    const entry = lstat(cursor);
    if (entry) {
      const canonical = realpathSync(cursor);
      return tail.length ? resolve(canonical, ...tail.reverse()) : canonical;
    }
    const parent = dirname(cursor);
    if (parent === cursor) throw scopeError("no existing ancestor");
    tail.push(basename(cursor));
    cursor = parent;
  }
}

export function normalizeWriteScopes(scopes: readonly WriteScope[]): EffectiveWriteCapability {
  if (!Array.isArray(scopes) || scopes.length === 0) throw scopeError("write_scopes must contain at least one grant");
  const normalized = scopes.map((scope): NormalizedWriteScope => {
    if (!scope || typeof scope.path !== "string" || !isAbsolute(scope.path)) throw scopeError("path must be absolute");
    const requested = resolve(scope.path);
    if (scope.kind === "file") {
      if (Object.hasOwn(scope, "include")) throw scopeError("file scopes do not accept include patterns");
      const entry = lstat(requested);
      if (entry) {
        const canonical = realpathSync(requested);
        if (!statSync(canonical).isFile()) throw scopeError("file grant must name a file");
        return frozenScope({ path: canonical, kind: "file" });
      }
      const parent = dirname(requested);
      const parentEntry = lstat(parent);
      if (!parentEntry || !statSync(realpathSync(parent)).isDirectory()) throw scopeError("file grant parent must exist");
      return frozenScope({ path: join(realpathSync(parent), basename(requested)), kind: "file" });
    }
    if (scope.kind !== "tree") throw scopeError("kind must be file or tree");
    const entry = lstat(requested);
    if (!entry) throw scopeError("tree root must exist");
    const canonical = realpathSync(requested);
    if (!statSync(canonical).isDirectory()) throw scopeError("tree root must be a directory");
    const include = scope.include;
    if (include !== undefined && (!Array.isArray(include) || include.length === 0 || include.some(pattern => typeof pattern !== "string"))) {
      throw scopeError("tree include must be a nonempty string array");
    }
    include?.forEach(validateWriteScopePattern);
    return frozenScope({ path: canonical, kind: "tree", ...(include ? { include: [...new Set(include)] } : {}) });
  });
  const deduped = normalized.filter((scope, index) => normalized.findIndex(candidate => JSON.stringify(candidate) === JSON.stringify(scope)) === index);
  return Object.freeze({ mode: "scoped", scopes: Object.freeze(deduped) });
}

function relativePosix(root: string, candidate: string): string | undefined {
  const rel = relative(root, candidate);
  if (!rel || rel === ".." || rel.startsWith(`..${sep}`) || isAbsolute(rel)) return undefined;
  return rel.split(sep).join("/");
}

export function matchesWriteScopeInclude(candidate: string, include: readonly string[]): boolean {
  return include.some(pattern => posix.matchesGlob(candidate, pattern));
}

function scopeAuthorizes(scope: NormalizedWriteScope, candidate: string): boolean {
  if (scope.kind === "file") return candidate === scope.path;
  try {
    if (realpathSync(scope.path) !== scope.path || !statSync(scope.path).isDirectory()) return false;
  } catch {
    return false;
  }
  const rel = relativePosix(scope.path, candidate);
  return rel !== undefined && (!scope.include || matchesWriteScopeInclude(rel, scope.include));
}

/** Throws one intentionally inventory-free diagnostic on every denial or canonicalization failure. */
export function authorizeWritePath(capability: EffectiveWriteCapability, candidatePath: string, cwd = process.cwd()): string {
  if (capability.mode === "unrestricted") return resolveNativeWriteInput(candidatePath, cwd);
  try {
    const candidate = canonicalPotentialPath(resolveNativeWriteInput(candidatePath, cwd));
    if (capability.mode === "scoped" && capability.scopes.some(scope => scopeAuthorizes(scope, candidate))) return candidate;
  } catch {
    // Collapse filesystem and containment detail into the same bounded denial.
  }
  throw new Error("ws-pi-agent: write path is outside delegated write scopes");
}

function scopeSubset(parent: NormalizedWriteScope, child: NormalizedWriteScope): boolean {
  if (child.kind === "file") return scopeAuthorizes(parent, child.path);
  if (parent.kind !== "tree") return false;
  const childRootRelative = child.path === parent.path ? "" : relativePosix(parent.path, child.path);
  if (child.path !== parent.path && childRootRelative === undefined) return false;
  if (!parent.include) return true;
  if (!child.include) return false;
  return child.include.every(pattern => {
    const parentRelativePattern = childRootRelative ? `${childRootRelative}/${pattern}` : pattern;
    return parent.include!.includes(parentRelativePattern);
  });
}

export function canDelegateWriteCapability(parent: EffectiveWriteCapability, child: EffectiveWriteCapability): boolean {
  if (child.mode === "none") return true;
  if (parent.mode === "unrestricted") return true;
  if (parent.mode !== "scoped" || child.mode !== "scoped") return false;
  return child.scopes.every(scope => parent.scopes.some(parentScope => scopeSubset(parentScope, scope)));
}

/** Install same-name access-control wrappers while delegating execution and result shapes to Pi's native tools. */
export function registerScopedWriteTools(pi: ExtensionAPI, capability: EffectiveWriteCapability): void {
  if (capability.mode !== "scoped" || capability.scopes.length === 0) throw scopeError("scoped wrappers require a nonempty binding");
  const edit = createEditToolDefinition(process.cwd());
  const write = createWriteToolDefinition(process.cwd());
  if (typeof edit.execute !== "function" || typeof write.execute !== "function") throw new Error("ws-pi-agent: native edit/write delegation is unavailable");

  pi.registerTool({
    ...edit,
    async execute(toolCallId, params: EditToolInput, signal, onUpdate, ctx) {
      const path = authorizeWritePath(capability, params.path, ctx.cwd);
      return createEditToolDefinition(ctx.cwd).execute(toolCallId, { ...params, path }, signal, onUpdate, ctx);
    },
  });
  pi.registerTool({
    ...write,
    async execute(toolCallId, params: WriteToolInput, signal, onUpdate, ctx) {
      const path = authorizeWritePath(capability, params.path, ctx.cwd);
      return createWriteToolDefinition(ctx.cwd).execute(toolCallId, { ...params, path }, signal, onUpdate, ctx);
    },
  });
}
