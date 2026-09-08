/**
 * TUI rendering for the six pushed child-report families
 * (`spawner.ts`'s `PUSH_FAMILIES`).
 *
 * Why this exists at all: Pi's default custom-message component
 * (`modes/interactive/components/custom-message.ts`) prints a bold
 * `[customType]` label of its own and then the message content — and every
 * pushed message's content already OPENS with `[family] agent <id>` (see
 * `buildPushContent`), because that head line is what the lead's model reads.
 * Rendered by the default, each push therefore showed its family twice. The
 * content is deliberately left alone (the model sees only `content`, as a user
 * message; changing it to satisfy the TUI would change what the lead reads),
 * so the duplicate is removed on the RENDER side instead:
 * `pi.registerMessageRenderer(family, ...)` draws the head once in the
 * `customMessageLabel` color, the payload lines under it, and the status line
 * dim.
 *
 * `@earendil-works/pi-tui` is reached through `./pi-tui.ts`'s
 * `loadHostPiTui()` — the one resolution point that resolves the package
 * through the host at runtime (see that file's Addendum doc comment for why:
 * `pi-coding-agent`'s own `npm-shrinkwrap.json` makes a single deduped
 * on-disk copy unattainable, so the live-instance identity guarantee comes
 * from routing through the host, not from `npm ls` reporting one copy).
 * `loadHostPiTui()` always resolves (falling back to this package's own
 * static copy only if the host import ever fails), so `loadPushTuiModules`
 * no longer has an "unavailable" branch; a renderer that cannot make sense
 * of a message still returns `undefined`, which `CustomMessageComponent.rebuild()`
 * treats as "use Pi's default".
 *
 * The theme is NOT imported: Pi hands the live `Theme` to the renderer as its
 * third argument (it is not part of `pi-coding-agent`'s public export surface
 * anyway — only the `Theme` class is), and that instance is the same singleton
 * Pi's own component paints with.
 *
 * `buildPushRenderLines` is the pure half, kept free of every host import so
 * the line split has direct `node --test` coverage.
 */

import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { loadHostPiTui } from "./pi-tui.ts";
import { PUSH_FAMILIES } from "./spawner.ts";

/** The three visual bands of a pushed message, split out of its plain-text content. */
export interface PushRenderLines {
  /** `[family] agent <id>` — drawn once, in the custom-message label color. */
  head: string;
  /** The `key: value` payload lines between the head and the status line. */
  body: string[];
  /** The fan-in line, when the message carries one (absent when nothing is delegated). */
  status: string | undefined;
}

/** The status line's own shape (`computeRunningStatusLine`), used to recognize it positionally. */
const STATUS_LINE_PATTERN = /^\d+ delegated agents? still running$/;

/** Pulls the plain text out of a custom message's `content` (string or text parts). */
function extractText(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((part): part is { type: string; text: string } => {
      const p = part as { type?: unknown; text?: unknown };
      return p?.type === "text" && typeof p.text === "string";
    })
    .map((part) => part.text)
    .join("\n");
}

/**
 * Splits one pushed message into head / payload / status.
 *
 * The status line is identified by `details.status` when present (that is the
 * exact string `sendPush` put there) and otherwise by shape, so a message that
 * legitimately carries no status line — nothing delegated — keeps its last
 * payload line as a payload line. Returns `undefined` for anything this
 * module cannot recognize (empty content, a foreign message shape), which the
 * caller turns into Pi's default rendering rather than an empty box.
 */
export function buildPushRenderLines(message: { content?: unknown; details?: unknown }): PushRenderLines | undefined {
  const lines = extractText(message.content).split("\n");
  if (lines.length === 0 || lines[0].trim() === "") return undefined;

  const head = lines[0];
  const rest = lines.slice(1);
  const declared = (message.details as { status?: unknown } | undefined)?.status;
  const last = rest[rest.length - 1];
  const isStatus =
    last !== undefined && (typeof declared === "string" ? last === declared : STATUS_LINE_PATTERN.test(last));

  return {
    head,
    body: isStatus ? rest.slice(0, -1) : rest,
    status: isStatus ? last : undefined,
  };
}

/** The `pi-tui` surface this module needs, as reached through `./pi-tui.ts`'s `loadHostPiTui()`. */
export interface PushTuiModules {
  Box: new (paddingX?: number, paddingY?: number, bgFn?: (text: string) => string) => {
    addChild(child: unknown): void;
    render(width: number): string[];
  };
  Text: new (text?: string, paddingX?: number, paddingY?: number) => unknown;
}

/** Duck-typed slice of Pi's `Theme` (only the two colors this renderer paints with). */
export interface PushRenderTheme {
  fg?(color: string, text: string): string;
  bold?(text: string): string;
}

/**
 * `./pi-tui.ts`'s `loadHostPiTui()`, narrowed to the slice this module needs.
 * Always resolves — see `pi-tui.ts`'s Addendum doc comment.
 */
export async function loadPushTuiModules(): Promise<PushTuiModules> {
  return (await loadHostPiTui()) as unknown as PushTuiModules;
}

/**
 * Assembles one message's component: a one-column-padded box holding the head
 * line in `customMessageLabel`, the payload lines plain, and the status line
 * dim. Returns `undefined` when the message is unrecognizable, which is Pi's
 * "use the default" signal.
 */
export function buildPushComponent(
  tui: PushTuiModules,
  message: { content?: unknown; details?: unknown },
  theme: PushRenderTheme | undefined,
): unknown {
  const parts = buildPushRenderLines(message);
  if (!parts) return undefined;
  const paint = (color: string, text: string): string => {
    try {
      return theme?.fg?.(color, text) ?? text;
    } catch {
      return text;
    }
  };
  const box = new tui.Box(1, 0);
  box.addChild(new tui.Text(paint("customMessageLabel", parts.head), 0, 0));
  for (const line of parts.body) {
    box.addChild(new tui.Text(paint("customMessageText", line), 0, 0));
  }
  if (parts.status) box.addChild(new tui.Text(paint("dim", parts.status), 0, 0));
  return box;
}

/**
 * Registers the compact renderer for every push family. Call only from a TUI
 * process (`ctx.mode === "tui"`) — there is no component to draw anywhere
 * else. The `Promise<boolean>` return is no longer an "unavailable" signal
 * (`loadPushTuiModules` always resolves — see `pi-tui.ts`'s Addendum doc
 * comment); it stays `Promise<boolean>` only so `index.ts`'s
 * `pushRenderersRegistered` retry-on-teardown guard (a genuine, still-live
 * failure mode: a rejection from e.g. `assertActive()` during teardown) keeps
 * its existing `.then((registered) => ...)` wiring unchanged.
 */
export async function registerPushMessageRenderers(pi: ExtensionAPI, tuiModules?: PushTuiModules): Promise<boolean> {
  const tui = tuiModules ?? (await loadPushTuiModules());
  for (const family of PUSH_FAMILIES) {
    pi.registerMessageRenderer(family, (message, _options, theme) =>
      buildPushComponent(tui, message as { content?: unknown; details?: unknown }, theme as unknown as PushRenderTheme) as never,
    );
  }
  return true;
}
