import { useContext, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SerializeAddon } from "@xterm/addon-serialize";
import { WebglAddon } from "@xterm/addon-webgl";
import { CanvasAddon } from "@xterm/addon-canvas";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { LigaturesAddon } from "@xterm/addon-ligatures";
import {
  clampTerminalSize,
  terminalWebSocketCursor,
  terminalWebSocketUrl,
  type TerminalPaneState,
  type TerminalWebSocketServerMessage,
} from "./terminals.js";
import {
  buildEffectiveTerminalFontFamily,
  terminalFontFamilyReapplySequence,
  TerminalPrefsContext,
} from "./terminalPrefs.js";
import {
  resolveTerminalDeltaWrite,
  resolveTerminalMountWrite,
  terminalVisualRestoreDebounceMs,
  terminalVisualRestoreScrollbackLines,
  type TerminalVisualRestoreEntry,
} from "./workbench/terminalVisualRestore.js";
import { shouldRefocusTerminal } from "./terminalRefocusGuard.js";

export type TerminalPaneActions = {
  onSendData: (pane: TerminalPaneState, data: string) => void;
  onClose: (pane: TerminalPaneState) => void;
  onResize: (
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ) => Promise<void>;
  onSocketStatus: (
    pane: TerminalPaneState,
    socketStatus: TerminalPaneState["socketStatus"],
    error?: string | null,
  ) => void;
  onVisibilityGated: (pane: TerminalPaneState, visibilityGated: boolean) => void;
  onSocketMessage: (
    pane: TerminalPaneState,
    message: TerminalWebSocketServerMessage,
  ) => void;
  onSocketResize: (
    pane: TerminalPaneState,
    columns: number,
    rows: number,
  ) => void;
  onFocusInput: (pane: TerminalPaneState) => void;
  isActivePane: (pane: TerminalPaneState) => boolean;
  // Root-switch auto-focus (per-workRoot last-focused-pane UX fix): true for
  // exactly one pane per render - the pane App-level `lastFocusedPaneByRootRef` recorded
  // as last-focused within the NOW-selected root - so the effect below can
  // restore real keyboard focus the instant a work root becomes active
  // again, without requiring an extra click on the pane itself.
  shouldAutoFocus: (pane: TerminalPaneState) => boolean;
  // Looked up once at TerminalPaneBody mount, by `pane.logicalKey`, against
  // the session-lifetime `terminalVisualRestoreRef` snapshot. Returns
  // `undefined` for a brand-new session (restore-intent fallback or a
  // logicalKey never captured before) so the mount effect falls back to the
  // existing plain-text `pane.output` replay.
  onVisualRestoreEntryFor: (
    pane: TerminalPaneState,
  ) => TerminalVisualRestoreEntry | undefined;
  // Returns pane.nextSequence "as of right now", including any output
  // frame cursor advance still batched (not yet applied to `pane` itself)
  // in the App-level pending-cursor accumulator (260723 Phase 1). Needed
  // because the debounced visual-capture effect below reads a cursor value
  // synchronously off its own liveRef, independent of App's rAF-batched
  // setTerminalPanes flush - reading straight off `pane.nextSequence` there
  // could observe a stale, not-yet-flushed cursor.
  getPendingNextSequence: (pane: TerminalPaneState) => number;
  // Fired from the debounced capture effect once per ~900ms of quiet PTY
  // output. Persists (or replaces) this pane's entry in the browser-local
  // visual-restore snapshot, keyed by `pane.logicalKey`.
  onVisualCapture: (
    pane: TerminalPaneState,
    capture: { serialized: string; viewportY: number; nextSequence: number },
  ) => void;
};

// Statuses a pane visually retires for (260724 Phase 2): the underlying
// shell process is gone, so the pane switches to a gray-out treatment; the
// tab's own close (x) button still closes it the same way as a live pane.
// Mirrors the `pane.session.status === "running"` precedent in
// `terminalRestoreIntentsFromPanes` (terminals.ts) as the complementary
// non-running set.
const terminalRetiredStatuses: ReadonlySet<string> = new Set([
  "exited",
  "terminated",
  "error",
]);

// Mounted per terminal pane and stays mounted (with its wrapper
// `display:none`'d by Dockview) while the owning root is not the active
// root - a mere selection/visibility flip does NOT unmount this component.
// It IS unmounted when the terminal is genuinely removed from the
// daemon-reported session list feeding `placeTerminalSessions`/
// `terminalPanes`. This means React's own mount lifetime of
// `TerminalPaneBody` already IS "logical terminal presence" (260714 Phase 2
// Prong 2) - the socket-lifecycle effect below keys its teardown on that
// mount lifetime (deps `[terminalId]`, not `paneVisible`) rather than
// inventing a new boolean/flag for the same concept.
export function TerminalPaneBody({
  pane,
  actions,
}: {
  pane: TerminalPaneState;
  actions: TerminalPaneActions;
}) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  // Absolute stream position (not a raw string length - see
  // `resolveTerminalDeltaWrite`) of the last character already written to
  // this pane's emulator. Stays in the same absolute coordinate space as
  // `pane.outputTrimOffset` so it remains directly comparable across any
  // number of front-trims (260723 Phase 1).
  const writtenAbsoluteRef = useRef(0);
  const lastForwardedSizeRef = useRef<
    { columns: number; rows: number; transport: "socket" | "http" } | null
  >(null);
  const socketRef = useRef<WebSocket | null>(null);
  // Latest fitNow/forwardSize closures from the mount effect below, exposed so
  // the paneVisible-gated socket effect can trigger a corrective refit (on a
  // false -> true visibility transition, or a terminalId change while
  // already visible) without duplicating the fit logic. Nulled on
  // mount-effect cleanup so a stray call can never reach a disposed terminal.
  const fitNowRef = useRef<(() => void) | null>(null);
  const forwardSizeRef = useRef<(() => void) | null>(null);
  // Serialize addon instance for this pane's terminal, loaded once at mount
  // so the debounced visual-buffer capture effect below can call
  // `.serialize()` without re-creating it on every output frame. Nulled on
  // mount-effect cleanup so a stray fire cannot reach a disposed terminal.
  const serializeAddonRef = useRef<SerializeAddon | null>(null);
  // Active GPU renderer addon (WebGL preferred, 2D canvas as fallback) for
  // this pane's terminal. Held so it can be disposed on unmount and swapped
  // out on a WebGL context-loss event. `null` when only xterm's built-in DOM
  // renderer is active (no GPU acceleration available). See the mount effect's
  // load-after-open()/fallback chain below.
  const rendererAddonRef = useRef<WebglAddon | CanvasAddon | null>(null);
  // Unicode v11 width-table provider for this pane's terminal. Held so it can
  // be disposed on unmount (which unregisters the provider). `null` when the
  // provider could not be constructed and xterm's built-in v6 tables stay
  // active. This is a character-width lookup change only; the output/data path
  // is untouched.
  const unicodeAddonRef = useRef<Unicode11Addon | null>(null);
  // Programming-ligature shaper for this pane's terminal. Held so it can be
  // disposed on unmount. `null` when construction/activation failed, in
  // which case the terminal renders unchanged with no ligatures. Loaded
  // immediately after `terminal.open()` and before the GPU renderer chain
  // below so a WebGL texture atlas (if one loads) already sees ligatures
  // active at construction time, per the addon's own ordering guidance.
  const ligaturesAddonRef = useRef<LigaturesAddon | null>(null);
  const visualCaptureTimerRef = useRef<number | null>(null);
  const keepTerminalFocusRef = useRef(false);
  // Set by the compositionstart/compositionend listeners below (mount
  // effect) and read at fire time by `refocusActiveTerminal`'s deferred
  // callback, `keydownFallback`, and the guard's `composing` field. A
  // component-level ref (not an effect-local variable) so it stays visible
  // to `refocusActiveTerminal`, which is defined outside every effect
  // (260727 Phase 1 - IME composition focus-steal fix).
  const composingRef = useRef(false);
  // Optimistic default matches current always-connect behavior for the
  // common case of a newly mounted, actually-visible pane; a pane mounted
  // while already hidden briefly opens then closes on the first watchdog
  // tick, an accepted minor inefficiency, not a correctness issue.
  const [paneVisible, setPaneVisible] = useState(true);
  // Daemon-owned PTY logical size as last established by THIS pane: seeded
  // from the session view at mount (the creation size, or the daemon's
  // current size for a reattach after reload) and advanced by `forwardSize`
  // below on each accepted forward, over either transport.
  //
  // Component-local state rather than a read of `pane.session.{columns,rows}`
  // in the JSX, which would be permanently stale: for a CONNECTED
  // `persistentTerminal`, `shouldUpdateDockviewWorkbenchPanelParams`
  // (workbench/dockviewLayoutModel.ts) deliberately suppresses Dockview
  // `updateParameters` pushes, so the `pane` prop reaching this component -
  // and with it `pane.session` - stops advancing entirely once the socket is
  // up. Everything else here is prop-independent (output streams straight
  // into the emulator from the socket listener), which is why that freeze is
  // otherwise invisible. Verified empirically: rendering the prop's columns
  // held at the 80-column creation size across a 1440px -> 480px viewport
  // narrowing that visibly refit the emulator.
  const [forwardedPtySize, setForwardedPtySize] = useState(() => ({
    columns: pane.session.columns,
    rows: pane.session.rows,
  }));
  // Live terminal-style prefs (font family/size, background) for the mount
  // effect's construction-time read below and the post-mount subscription
  // effect further down (see that effect for the live-apply path).
  const terminalPrefs = useContext(TerminalPrefsContext);
  // Latest pane/actions/terminalPrefs for emulator callbacks registered once
  // at mount.
  const liveRef = useRef({ pane, actions, terminalPrefs });
  liveRef.current = { pane, actions, terminalPrefs };

  const terminalId = pane.session.terminalId;
  // Single choke point for all three refocus call sites (sendInputBytes,
  // focusWatchdog, the WS "output" handler). The guard state is built
  // inside the deferred callback - read at fire time, not at call time - so
  // a composition that starts after the timeout was scheduled but before it
  // fires is still caught (260727 Phase 1: gate on active IME composition
  // to stop refocus from stealing focus mid-composition and corrupting
  // input).
  const refocusActiveTerminal = () => {
    window.setTimeout(() => {
      // Cheap short-circuit before the layout-triggering `offsetParent` read
      // and the `isActivePane` call below: most deferred callbacks fire while
      // composing or without keep-focus intent, and both are checked here
      // for free. `shouldRefocusTerminal` stays the single source of truth
      // for the actual boolean logic - this is a fast-path skip, not a
      // duplicate of its semantics (260727 correctness review: restore the
      // pre-extraction short-circuit so a streaming active pane with
      // `keepFocus === false` doesn't pay a forced reflow per output chunk).
      if (composingRef.current || !keepTerminalFocusRef.current) {
        return;
      }
      const shouldRefocus = shouldRefocusTerminal({
        composing: composingRef.current,
        keepFocus: keepTerminalFocusRef.current,
        visible: Boolean(containerRef.current?.offsetParent),
        active: liveRef.current.actions.isActivePane(liveRef.current.pane),
      });
      if (shouldRefocus) {
        terminalRef.current?.focus();
        containerRef.current
          ?.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea")
          ?.focus();
      }
    }, 0);
  };

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const terminal = new Terminal({
      cursorBlink: true,
      // Prefer Powerline/Nerd Font capable families so prompt glyphs render
      // correctly, falling back to plain monospace when none are installed;
      // an empty prefs override reproduces this exact fallback stack
      // unchanged (see `buildEffectiveTerminalFontFamily`).
      fontFamily: buildEffectiveTerminalFontFamily(
        liveRef.current.terminalPrefs.fontFamilyOverride,
      ),
      fontSize: liveRef.current.terminalPrefs.fontSize,
      theme: { background: liveRef.current.terminalPrefs.themeBackground },
      // LigaturesAddon's activate() calls the proposed/experimental
      // `registerCharacterJoiner` API, which xterm.js guards behind this
      // flag and throws without it - a throw the ligature-loading try/catch
      // below silently swallows with no console output, making ligatures
      // permanently inert whenever this is unset. Harmless to set
      // unconditionally (it only relaxes an API guard) rather than gating on
      // `terminalPrefs.ligaturesEnabled`, which isn't computed until after
      // this constructor call.
      allowProposedApi: true,
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    const serializeAddon = new SerializeAddon();
    terminal.loadAddon(serializeAddon);
    serializeAddonRef.current = serializeAddon;
    terminal.open(container);
    terminalRef.current = terminal;

    // Renderer/addon selection happens once here, at construction time, and
    // is not live-swapped later - `terminalPrefs.gpuAcceleration` /
    // `.ligaturesEnabled` only take effect for terminals constructed after a
    // settings change (matches the "Applies to newly opened terminal panes"
    // settings-UI note).
    const useLigatures = liveRef.current.terminalPrefs.ligaturesEnabled;
    // WebGL/Canvas renderers draw glyphs via `CanvasRenderingContext2D`/GL
    // texture atlases, which have no mechanism to honor
    // `font-feature-settings`/`calt` ligature substitution - only xterm's
    // built-in DOM renderer (real text nodes, CSS cascade) can shape
    // ligatures. This is an architectural limitation of the GPU renderers,
    // not a bug (matches microsoft/vscode#274296; VS Code documents
    // disabling GPU acceleration as the official workaround for the same
    // limitation). So ligatures force the DOM renderer by skipping the GPU
    // renderer addons entirely, at the cost of DOM-renderer performance.
    const useGpuRenderer =
      liveRef.current.terminalPrefs.gpuAcceleration && !useLigatures;

    // Activate programming-ligature shaping (`->`, `=>`, `!=`, etc.) right
    // after open() and before the GPU renderer chain below, so a WebGL
    // texture atlas - if one loads - already reflects ligatures at
    // construction time instead of needing a reactivation step.
    // Use the addon's default constructor (default fallback-ligature list)
    // rather than overriding `fallbackLigatures`: font-based GSUB detection
    // needs the Local Font Access API (`navigator.fonts.query()` /
    // `window.queryLocalFonts()`), which is Chromium-only and requires a
    // secure context. This dashboard is dogfooded over plain HTTP on a LAN
    // address, so that API is never available here, font-driven detection
    // silently resolves to nothing, and the built-in fallback list is the
    // only way ligatures ever render in this environment. Wrapped in
    // try/catch so a construction/activation failure leaves the terminal
    // working unchanged with no ligatures.
    if (useLigatures) {
      try {
        const ligaturesAddon = new LigaturesAddon();
        terminal.loadAddon(ligaturesAddon);
        ligaturesAddonRef.current = ligaturesAddon;
      } catch (error) {
        // Logged rather than silently swallowed: a prior silent failure here
        // (missing `allowProposedApi`, see the Terminal constructor above)
        // made ligatures look inert with zero observable signal anywhere.
        console.error("Failed to activate terminal ligatures addon", error);
        ligaturesAddonRef.current = null;
      }
    }

    // Attach a GPU renderer AFTER open() so a canvas/WebGL context exists;
    // without one xterm 5.x falls back to its slow DOM renderer, which
    // dominates throughput when a full-screen TUI repaints many frames per
    // second. Prefer WebGL, degrade to the 2D canvas renderer, and finally to
    // the built-in DOM renderer, so an environment without GPU acceleration -
    // or one that loses its GL context at runtime - still renders output
    // unchanged. This only swaps the render backend; the output/data path is
    // untouched. Skipped entirely when GPU acceleration is off (either by
    // preference or because ligatures forced it off above) - the DOM
    // renderer stays active, which is required for ligature glyphs to
    // render at all.
    if (useGpuRenderer) {
      const loadCanvasRenderer = () => {
        try {
          const canvasAddon = new CanvasAddon();
          terminal.loadAddon(canvasAddon);
          rendererAddonRef.current = canvasAddon;
        } catch {
          // No 2D canvas renderer either; leave the DOM renderer in place.
          rendererAddonRef.current = null;
        }
      };
      try {
        const webglAddon = new WebglAddon();
        // A lost GPU context would otherwise blank the terminal permanently;
        // dispose the WebGL addon and drop to the canvas renderer so output
        // keeps rendering.
        webglAddon.onContextLoss(() => {
          webglAddon.dispose();
          if (rendererAddonRef.current === webglAddon) {
            rendererAddonRef.current = null;
          }
          loadCanvasRenderer();
        });
        terminal.loadAddon(webglAddon);
        rendererAddonRef.current = webglAddon;
      } catch {
        // WebGL unavailable in this environment; try 2D canvas, then DOM.
        loadCanvasRenderer();
      }
    }

    // Swap xterm's default (Unicode v6) character-width tables for the v11
    // tables so wide glyphs - notably emoji - occupy their correct two cells
    // instead of one. This only changes width lookups used for cursor
    // advancement/layout; the output/data path is untouched. If the provider
    // fails to construct, xterm keeps its built-in v6 tables and rendering
    // continues unchanged.
    try {
      const unicode11Addon = new Unicode11Addon();
      terminal.loadAddon(unicode11Addon);
      terminal.unicode.activeVersion = "11";
      unicodeAddonRef.current = unicode11Addon;
    } catch {
      unicodeAddonRef.current = null;
    }
    // `pane.outputTrimOffset` is always 0 at genuine mount time in practice
    // (this component only unmounts/remounts on a real terminal close/
    // reopen, never a mere visibility toggle - see the type-level comment on
    // `TerminalPaneBody` above - and `terminalPaneFromSession` always seeds a
    // fresh pane at offset 0), but seeding from the live value here keeps
    // `writtenAbsoluteRef`'s meaning correct by construction rather than by
    // that invariant holding elsewhere.
    writtenAbsoluteRef.current = liveRef.current.pane.outputTrimOffset;

    // A reattached pane with a matching persisted visual-restore snapshot
    // (id-reattach to a still-alive daemon terminal) writes that serialized
    // buffer - scrollback, cursor position, styles - plus its scroll
    // viewport offset, instead of the plain-text `pane.output` replay below.
    // `writtenAbsoluteRef` stays at `pane.outputTrimOffset` in both branches:
    // the delta-write effect tracks `pane.output` independent of whichever
    // initial write happened here, so a restored snapshot's own
    // escape-sequence text is never diffed against `pane.output` (which
    // starts at "" for a freshly reattached pane either way). New sessions
    // spawned via the restore-intent fallback have no matching entry and
    // fall through to the existing replay path unchanged.
    //
    // The three-way branch selection itself (restore vs. replay vs. no-op)
    // is pure and lives in `resolveTerminalMountWrite` (`workbench/terminalVisualRestore.ts`)
    // so it is unit testable independent of xterm/DOM; only the actual
    // `terminal.write`/`scrollToLine`/`writtenAbsoluteRef` side effects stay here.
    const restoreEntry = liveRef.current.actions.onVisualRestoreEntryFor(
      liveRef.current.pane,
    );
    const mountWrite = resolveTerminalMountWrite(
      liveRef.current.pane,
      restoreEntry,
    );
    if (mountWrite.kind === "restore") {
      // `terminal.write()` is asynchronous (parsed on a later tick via the
      // internal write buffer), so `scrollToLine` must run in the write's
      // completion callback - calling it immediately after `write()` would
      // clamp the scroll target against the then-still-empty buffer.
      terminal.write(mountWrite.serialized, () => {
        terminal.scrollToLine(mountWrite.viewportY);
      });
    } else if (mountWrite.kind === "replay") {
      // Replay PTY output buffered before this surface mounted so reselecting
      // a terminal tab restores its emulator contents.
      terminal.write(mountWrite.text);
      writtenAbsoluteRef.current =
        liveRef.current.pane.outputTrimOffset + mountWrite.text.length;
    }

    // Keyboard input originates from the focused emulator surface and reaches
    // the daemon terminal session.
    const sendInputBytes = (data: string) => {
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify({ type: "input", data }));
        refocusActiveTerminal();
        return;
      }
      liveRef.current.actions.onSendData(liveRef.current.pane, data);
      refocusActiveTerminal();
    };

    const inputDisposable = terminal.onData(sendInputBytes);
    const markComposing = () => {
      composingRef.current = true;
    };
    const clearComposing = () => {
      composingRef.current = false;
      // Trailing edge: restore focus immediately once composition finishes,
      // rather than waiting on the next per-keystroke/per-chunk call site or
      // the 100ms watchdog.
      refocusActiveTerminal();
    };
    const markFocusedTerminal = () => {
      keepTerminalFocusRef.current = true;
      liveRef.current.actions.onFocusInput(liveRef.current.pane);
      terminal.focus();
    };
    const clearFocusedTerminal = (event: PointerEvent) => {
      const target = event.target as Node | null;
      if (target && !container.contains(target)) {
        keepTerminalFocusRef.current = false;
      }
    };
    const clearFocusedTerminalOnOutsideFocus = (event: FocusEvent) => {
      const target = event.target as Node | null;
      if (target && !container.contains(target)) {
        keepTerminalFocusRef.current = false;
      }
    };
    container.addEventListener("compositionstart", markComposing);
    container.addEventListener("compositionend", clearComposing);
    container.addEventListener("focusin", markFocusedTerminal);
    container.addEventListener("pointerdown", markFocusedTerminal);
    window.addEventListener("pointerdown", clearFocusedTerminal, true);
    window.addEventListener(
      "focusin",
      clearFocusedTerminalOnOutsideFocus,
      true,
    );

    const keydownFallback = (event: KeyboardEvent) => {
      if (!container.offsetParent) {
        return;
      }
      if (!liveRef.current.actions.isActivePane(liveRef.current.pane)) {
        return;
      }
      if (event.isComposing || event.key === "Process" || composingRef.current) {
        return;
      }
      const isMetaLineStart = event.metaKey && event.key.toLowerCase() === "a";
      if (container.contains(document.activeElement) && !isMetaLineStart) {
        return;
      }
      const target = event.target as HTMLElement | null;
      const tagName = target?.tagName.toLowerCase();
      if (
        target?.isContentEditable ||
        tagName === "input" ||
        tagName === "textarea" ||
        tagName === "select"
      ) {
        return;
      }
      let data: string | null = null;
      if (event.ctrlKey || event.metaKey) {
        const key = event.key.toLowerCase();
        if (key === "c") data = "\x03";
        if (key === "l") data = "\x0c";
        if (key === "a") data = "\x01";
        // Native shell line-editing controls: ctrl-u clears the current line
        // and ctrl-w deletes the previous word. Dockview does not reliably keep
        // the xterm helper textarea focused, so this fallback forwards the same
        // raw control bytes xterm's onData path would send when it is focused.
        if (key === "u") data = "\x15";
        if (key === "w") data = "\x17";
      } else if (event.key.length === 1) {
        data = event.key;
      } else if (event.key === "Enter") {
        data = "\r";
      } else if (event.key === "Backspace") {
        data = "\x7f";
      } else if (event.key === "ArrowLeft") {
        data = "\x1b[D";
      } else if (event.key === "ArrowRight") {
        data = "\x1b[C";
      } else if (event.key === "ArrowUp") {
        data = "\x1b[A";
      } else if (event.key === "ArrowDown") {
        data = "\x1b[B";
      }
      if (data !== null) {
        event.preventDefault();
        liveRef.current.actions.onFocusInput(liveRef.current.pane);
        sendInputBytes(data);
      }
    };
    window.addEventListener("keydown", keydownFallback);

    const fitNow = () => {
      // Guard against a degenerate short-container collapse: a *visible* pane
      // whose usable height momentarily measures too small to host a real
      // grid (e.g. during dockview relayout on a tab/session switch, or an
      // actually-short window/split) makes both `fitAddon.fit()` and the
      // shrink loop below drive `terminal.rows` down to the vendor floor
      // (`1`), which also clears the rendered screen. `proposeDimensions()`
      // reports what `fit()` would apply without applying it; when it is
      // unmeasurable or proposes the degenerate floor, skip the fit/shrink
      // entirely and preserve the last-good emulator size instead. This is
      // the fit-relevant *measured* signal, unlike `offsetParent`, which
      // stays non-null (pane visible) throughout this collapse.
      const proposed = fitAddon.proposeDimensions();
      if (!proposed || proposed.rows <= 1 || proposed.cols <= 1) {
        return;
      }
      // A workRoot switch's Dockview relayout (see the `paneVisible` effect
      // below) can transiently propose a size collapsed on BOTH axes at
      // once, well below the terminal's last-good size, before Dockview's
      // own layout engine settles - confirmed empirically
      // (`proposeDimensions()` briefly returning e.g. 10x3 immediately
      // after a root switch, corrected roughly one frame later). A
      // deliberate user resize essentially never shrinks both axes to
      // under a quarter of their prior size in a single measurement, so
      // treat that specific pattern as a transient mismeasurement and skip
      // rather than apply it - the next ResizeObserver/visibility-triggered
      // fit picks up the real size once layout has settled.
      if (
        terminal.cols > 4 &&
        terminal.rows > 4 &&
        proposed.cols < terminal.cols / 4 &&
        proposed.rows < terminal.rows / 4
      ) {
        return;
      }
      try {
        fitAddon.fit();
      } catch {
        /* container not measurable yet */
        return;
      }
      // Cap the emulator grid to the PTY size contract so the emulator and the
      // daemon-owned logical PTY size never disagree on very wide/tall panes.
      const capped = clampTerminalSize(terminal.cols, terminal.rows);
      if (capped.columns !== terminal.cols || capped.rows !== terminal.rows) {
        terminal.resize(capped.columns, capped.rows);
      }
      while (terminal.rows > 1 && !terminalScreenFitsVisibleBox(container)) {
        terminal.resize(terminal.cols, terminal.rows - 1);
      }
    };
    fitNowRef.current = fitNow;

    const forwardSize = () => {
      // The emulator grid is already capped to the PTY bounds by fitNow, so
      // this size is always inside the daemon resize contract. When Dockview is
      // stacked below the fold in a narrow viewport, its internal cached width
      // may lag the viewport; still bound the PTY columns to the viewport so
      // the daemon-visible logical size follows responsive relayout.
      const viewportColumns = Math.max(
        1,
        Math.floor((window.innerWidth - 32) / 8),
      );
      const next = clampTerminalSize(
        Math.min(terminal.cols, viewportColumns),
        terminal.rows,
      );
      if (next.columns !== terminal.cols || next.rows !== terminal.rows) {
        terminal.resize(next.columns, next.rows);
      }
      const socketOpen = socketRef.current?.readyState === WebSocket.OPEN;
      const prev = lastForwardedSizeRef.current;
      if (
        prev &&
        prev.columns === next.columns &&
        prev.rows === next.rows &&
        prev.transport === (socketOpen ? "socket" : "http")
      ) {
        return;
      }
      // Keeps the rendered `data-terminal-{columns,rows}` projection in step
      // with what was actually forwarded. Functional + equality-guarded so a
      // repeat forward of an unchanged size does not re-render the pane.
      const recordForwardedPtySize = () => {
        setForwardedPtySize((current) =>
          current.columns === next.columns && current.rows === next.rows
            ? current
            : { columns: next.columns, rows: next.rows },
        );
      };
      // Record the forwarded size only after the daemon accepts it; a rejected
      // resize must stay retryable rather than being suppressed as a no-op.
      const socket = socketRef.current;
      if (socket?.readyState === WebSocket.OPEN) {
        socket.send(
          JSON.stringify({
            type: "resize",
            columns: next.columns,
            rows: next.rows,
          }),
        );
        liveRef.current.actions.onSocketResize(
          liveRef.current.pane,
          next.columns,
          next.rows,
        );
        lastForwardedSizeRef.current = { ...next, transport: "socket" };
        recordForwardedPtySize();
        return;
      }
      void liveRef.current.actions
        .onResize(liveRef.current.pane, next.columns, next.rows)
        .then(() => {
          lastForwardedSizeRef.current = { ...next, transport: "http" };
          recordForwardedPtySize();
        })
        .catch(() => {
          /* leave lastForwardedSizeRef unchanged so the next fit retries */
        });
    };
    forwardSizeRef.current = forwardSize;

    fitNow();

    // ResizeObserver keeps the emulator fitted to the pane; resize forwarding
    // to the daemon is debounced so visual split drag does not continuously
    // rewrite logical PTY dimensions.
    let resizeTimer: number | null = null;
    const scheduleResizeForward = () => {
      fitNow();
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      resizeTimer = window.setTimeout(() => {
        resizeTimer = null;
        forwardSize();
      }, 250);
    };
    const observer = new ResizeObserver(scheduleResizeForward);
    observer.observe(container);
    window.addEventListener("resize", scheduleResizeForward);
    const focusWatchdog = window.setInterval(() => {
      const nowVisible = Boolean(container.offsetParent);
      setPaneVisible((current) => (current === nowVisible ? current : nowVisible));
      if (!keepTerminalFocusRef.current) {
        return;
      }
      if (!container.offsetParent) {
        return;
      }
      if (!liveRef.current.actions.isActivePane(liveRef.current.pane)) {
        return;
      }
      if (container.contains(document.activeElement)) {
        return;
      }
      // Focus has genuinely left the container. Browsers force-commit or
      // cancel IME composition on blur, so no composition can legitimately
      // still be active here - reset explicitly because `compositionend`
      // delivery is not guaranteed when the composing element is hidden,
      // re-parented, or the window loses OS focus mid-composition (e.g.
      // alt-tab). Without this, a dropped `compositionend` would leave
      // `composingRef` latched `true` forever, permanently disabling both
      // this watchdog's refocus and `keydownFallback` for the pane's
      // remaining lifetime (260727 Phase 1 correctness review: restores the
      // watchdog's unconditional-safety-net property).
      composingRef.current = false;
      refocusActiveTerminal();
    }, 100);

    // A previously-downloaded custom webfont (e.g. Fira Code) doesn't
    // survive a page reload in `document.fonts` state, so `App.tsx`'s own
    // mount effect independently re-fetches it via `reregisterDownloadedFonts`
    // (see `downloadableFonts.ts`), racing with this effect. If the
    // `fontFamily` read above resolves to such a font before its bytes are
    // registered, the browser silently substitutes the fallback for the
    // initial glyph measurement/paint, and the GPU renderer caches that
    // measurement - nothing else re-triggers a re-measure once the real font
    // lands. A one-shot `document.fonts.ready` read here would be unreliable:
    // React fires child mount effects (this one) before parent mount effects
    // (App.tsx's), so at this point `reregisterDownloadedFonts` may not have
    // even started its network fetch yet, and `.ready` could resolve before
    // that fetch registers a pending load. Listening for `loadingdone`
    // instead catches every load batch that completes for the life of this
    // mount, regardless of when it started relative to this effect.
    //
    // The re-apply itself goes through `terminalFontFamilyReapplySequence`
    // rather than a bare `terminal.options.fontFamily = ...` assignment: in
    // this exact race the effective family string is UNCHANGED across the
    // download, and xterm's option setter is equality-guarded, so a bare
    // assignment fires no option-change event and therefore re-measures
    // nothing - leaving in place the very stale fallback cell metrics this
    // listener exists to correct (and which the alt-screen `fitNow` branch
    // below would then read). See that helper for the mechanism.
    const onFontsLoadingDone = () => {
      if (terminalRef.current !== terminal) {
        // Pane unmounted/remounted since this listener was registered.
        return;
      }
      const effectiveFontFamily = buildEffectiveTerminalFontFamily(
        liveRef.current.terminalPrefs.fontFamilyOverride,
      );
      for (const value of terminalFontFamilyReapplySequence(
        terminal.options.fontFamily ?? "",
        effectiveFontFamily,
      )) {
        terminal.options.fontFamily = value;
      }
      // A font swap can change glyph cell width, so fit() may resize with a
      // changed column count, which reflows the buffer. On a normal buffer
      // with real scrollback that remaps ydisp/ybase and visibly jumps the
      // viewport to the top - only safe to do on an alt-screen session
      // (same carve-out as the shrink/restore trick below).
      if (terminal.buffer.active.type === "alternate") {
        fitNowRef.current?.();
      }
    };
    document.fonts.addEventListener("loadingdone", onFontsLoadingDone);

    return () => {
      observer.disconnect();
      window.removeEventListener("resize", scheduleResizeForward);
      window.clearInterval(focusWatchdog);
      document.fonts.removeEventListener("loadingdone", onFontsLoadingDone);
      if (resizeTimer !== null) {
        window.clearTimeout(resizeTimer);
      }
      window.removeEventListener("keydown", keydownFallback);
      container.removeEventListener("compositionstart", markComposing);
      container.removeEventListener("compositionend", clearComposing);
      container.removeEventListener("focusin", markFocusedTerminal);
      container.removeEventListener("pointerdown", markFocusedTerminal);
      window.removeEventListener("pointerdown", clearFocusedTerminal, true);
      window.removeEventListener(
        "focusin",
        clearFocusedTerminalOnOutsideFocus,
        true,
      );
      inputDisposable.dispose();
      // Belt-and-suspenders alongside the debounced capture effect's own
      // cleanup: guarantees no pending serialize callback can ever fire
      // against a disposed terminal, even if effect cleanup ordering changed.
      if (visualCaptureTimerRef.current !== null) {
        window.clearTimeout(visualCaptureTimerRef.current);
        visualCaptureTimerRef.current = null;
      }
      // Release the GPU renderer's GL/2D context and buffers before disposing
      // the terminal; a no-op when only the DOM renderer was active.
      rendererAddonRef.current?.dispose();
      rendererAddonRef.current = null;
      // Unregister the v11 Unicode provider before disposing the terminal; a
      // no-op when the built-in v6 tables were left active.
      unicodeAddonRef.current?.dispose();
      unicodeAddonRef.current = null;
      // Unload the ligature shaper before disposing the terminal; a no-op
      // when construction/activation failed at mount.
      ligaturesAddonRef.current?.dispose();
      ligaturesAddonRef.current = null;
      terminal.dispose();
      terminalRef.current = null;
      serializeAddonRef.current = null;
      fitNowRef.current = null;
      forwardSizeRef.current = null;
    };
  }, []);

  // Effect A - visibility bookkeeping only (260714 Phase 2 Prong 2). Deps
  // `[paneVisible]` ONLY: this effect never creates or tears down the
  // socket (see Effect B below) - a `paneVisible` flip (root deselected, or
  // a different Dockview tab brought to front within the same root) must
  // not touch the socket at all, since the terminal is still logically
  // open the whole time (see the mount-lifetime note on `TerminalPaneBody`
  // above). `onSocketStatus` is intentionally NOT called here on hide:
  // per Prong 2 the socket stays connected while hidden, so its status
  // should keep reading "connected", not a fabricated "disconnected" -
  // `onVisibilityGated` alone is what the HTTP output-poll fallback needs
  // to stay suppressed while hidden (see `shouldPollTerminalOutput` in
  // `terminals.ts`, whose `socketStatus !== "connecting"/"connected"`
  // clause already returns `false` once the socket stays connected, making
  // `visibilityGated` a secondary/redundant-but-harmless guard for this
  // case and still the primary one for a genuine post-error fallback).
  useEffect(() => {
    if (!paneVisible) {
      liveRef.current.actions.onVisibilityGated(liveRef.current.pane, true);
      return;
    }
    liveRef.current.actions.onVisibilityGated(liveRef.current.pane, false);
    // Deterministic corrective refit on a false -> true visibility
    // transition (pane shown again after a tab/session/workRoot switch):
    // the pane may have been measured short-but-visible for a frame while
    // still transitioning (see fitNow's degenerate-container guard above),
    // or measured while briefly hidden/detached (no ResizeObserver
    // correction). Explicitly re-fit now rather than relying solely on the
    // next incidental ResizeObserver callback, and forward the size only if
    // it actually changed, reusing the existing fitNow/forwardSize
    // closures.
    //
    // A workRoot switch toggles `display:none` on Dockview's entire
    // per-root layout subtree, which forces Dockview's own internal layout
    // engine through a real hide -> show cycle that needs its own tick(s)
    // to settle real group/panel sizes - unlike an intra-root tab switch,
    // which never hides that Dockview instance at all. Reading the
    // container's box on the very same tick this effect runs can therefore
    // observe a genuinely-but-transiently tiny measurement (confirmed
    // empirically: proposeDimensions briefly returning single-digit
    // columns immediately after a root switch, then the correct size ~one
    // frame later), which fitNow's degenerate guard does not catch since
    // it only rejects `rows <= 1`. Deferring through two rAFs lets both
    // Dockview's relayout and the browser's own layout/paint settle first.
    let rafA = 0;
    let rafB = 0;
    rafA = window.requestAnimationFrame(() => {
      rafB = window.requestAnimationFrame(() => {
        const beforeFit = terminalRef.current
          ? { columns: terminalRef.current.cols, rows: terminalRef.current.rows }
          : null;
        fitNowRef.current?.();
        if (
          beforeFit &&
          terminalRef.current &&
          (terminalRef.current.cols !== beforeFit.columns ||
            terminalRef.current.rows !== beforeFit.rows)
        ) {
          forwardSizeRef.current?.();
        } else if (
          beforeFit &&
          terminalRef.current &&
          terminalRef.current.buffer.active.type === "alternate"
        ) {
          // Size didn't actually change while hidden (the common case - e.g.
          // switching dashboard tabs without resizing the browser window), so a
          // same-size resize would be silently dropped both by the frontend
          // dedupe (`lastForwardedSizeRef` in forwardSize) and by the kernel
          // (Linux only emits SIGWINCH when ws_row/ws_col actually differ). A
          // full-screen TUI app (htop/vim/tmux) that under-repainted while its
          // alt-screen scrollback was replayed client-side would then stay
          // visually stale with no redraw trigger. Force two genuinely
          // different sizes through - a one-row shrink then restore - so each
          // one forwards and triggers a real SIGWINCH, guaranteeing a full
          // redraw.
          //
          // Gated to the alternate screen buffer only: a normal-buffer session
          // (plain shell, no full-screen app) never had the under-repaint
          // symptom in the first place, and resize-driven reflow of a normal
          // buffer's real scrollback can jump the viewport to the top - a
          // regression with no corresponding benefit there.
          const terminal = terminalRef.current;
          const shrunkRows = Math.max(1, terminal.rows - 1);
          if (shrunkRows !== terminal.rows) {
            terminal.resize(terminal.cols, shrunkRows);
            forwardSizeRef.current?.();
            terminal.resize(beforeFit.columns, beforeFit.rows);
            forwardSizeRef.current?.();
          }
        }
      });
    });
    return () => {
      window.cancelAnimationFrame(rafA);
      window.cancelAnimationFrame(rafB);
    };
  }, [paneVisible]);

  // Effect B - socket lifecycle only (260714 Phase 2 Prong 2). Deps
  // `[terminalId]` ONLY, deliberately excluding `paneVisible`: the socket
  // now connects once per `terminalId` (mount, or a genuine terminal-id
  // change e.g. a reattach to a new daemon session) and is torn down only
  // on a `terminalId` change or this component's own unmount - never on a
  // mere visibility flip. `TerminalPaneBody` stays mounted (wrapper
  // `display:none`'d) while its owning root is not the active root and is
  // only unmounted when the terminal is genuinely removed from the
  // daemon-reported session list, so React's own mount lifetime already IS
  // "logical terminal presence" (see the type-level comment on
  // `TerminalPaneBody` above) - keying teardown on that lifetime, instead
  // of on `paneVisible`, is the whole fix: a hidden-but-still-open terminal
  // (root deselected, or a different Dockview tab in front) no longer has
  // its OPEN socket closed and immediately reopened on the next visibility
  // flip. The only teardown path for an OPEN socket after this split is
  // this cleanup running for a real `terminalId` change or a genuine
  // unmount - no leaked-connection path is introduced, since unmount always
  // runs this same cleanup, which already correctly closes any
  // non-CONNECTING socket.
  useEffect(() => {
    let disposed = false;
    const socket = new WebSocket(
      terminalWebSocketUrl(
        terminalId,
        terminalWebSocketCursor(liveRef.current.pane),
        window.location,
        liveRef.current.pane.session.serverRoute,
      ),
    );
    socketRef.current = socket;
    liveRef.current.actions.onSocketStatus(
      liveRef.current.pane,
      "connecting",
      null,
    );

    socket.addEventListener("open", () => {
      if (disposed) {
        // Cleanup ran while this socket was still CONNECTING, so it was
        // intentionally left open (see cleanup below) to avoid aborting the
        // handshake. Now that it has finished connecting, close it here
        // instead so it does not leak as an orphaned live connection.
        socket.close();
        return;
      }
      liveRef.current.actions.onSocketStatus(
        liveRef.current.pane,
        "connected",
        null,
      );
      // Catch-up resize forward now that the socket is open: an earlier
      // HTTP-fallback forward (e.g. while the socket was still connecting)
      // may have already latched the daemon to these dimensions, but the
      // transport-aware gate in forwardSize() lets this through so the
      // now-live socket also carries a resize frame, matching the spec's
      // "carries PTY output, input, resize, ping/pong, and close frames"
      // contract for the socket transport itself.
      forwardSizeRef.current?.();
    });
    socket.addEventListener("message", (event) => {
      if (disposed || typeof event.data !== "string") return;
      try {
        const message = JSON.parse(
          event.data,
        ) as TerminalWebSocketServerMessage;
        if (message.type === "output") {
          terminalRef.current?.write(message.chunk.data);
          // The live socket path never trims `pane.output`, so this stays a
          // plain running total in the same absolute coordinate space as
          // `writtenAbsoluteRef`'s other writers - no offset math needed here.
          writtenAbsoluteRef.current += message.chunk.data.length;
          if (liveRef.current.actions.isActivePane(liveRef.current.pane)) {
            refocusActiveTerminal();
          }
        }
        liveRef.current.actions.onSocketMessage(liveRef.current.pane, message);
      } catch {
        // Ignore malformed daemon frames and allow the socket close/fallback path to recover.
      }
    });
    socket.addEventListener("error", () => {
      if (!disposed) {
        liveRef.current.actions.onSocketStatus(
          liveRef.current.pane,
          "fallback",
          "terminal WebSocket failed",
        );
      }
    });
    socket.addEventListener("close", () => {
      if (!disposed)
        liveRef.current.actions.onSocketStatus(
          liveRef.current.pane,
          "fallback",
          null,
        );
    });

    return () => {
      disposed = true;
      if (socketRef.current === socket) socketRef.current = null;
      // Do not abort a still-CONNECTING handshake: closing a WebSocket
      // before it reaches OPEN throws "WebSocket is closed before the
      // connection is established" and, for linked/remote-server terminals
      // whose handshake crosses an extra browser -> gateway -> WSL hop, a
      // transient unmount/remount race (or, before this Prong 2 split, a
      // `paneVisible` flip) could land mid-handshake and kill the socket
      // before the daemon relay ever runs. Leave CONNECTING sockets alone
      // here; the "open" listener above closes them itself once `disposed`
      // is observed at open time. close() on OPEN/CLOSING/CLOSED remains a
      // safe no-op/normal close.
      if (socket.readyState !== WebSocket.CONNECTING) socket.close();
    };
  }, [terminalId]);

  // Live fan-out of terminal-style prefs (Settings > Terminal): applies
  // `terminal.options.*` post-construction on every `terminalPrefs` change,
  // so an open pane's font/size/background updates without a remount -
  // deliberately declared after the mount effect (deps `[]`, restore/
  // reattach logic) so on first mount it runs once `terminalRef.current` is
  // already set. This is the only place `terminalPrefs` drives the emulator;
  // the mount effect above only reads it once, at construction time, via
  // `liveRef`.
  useEffect(() => {
    const terminal = terminalRef.current;
    if (!terminal) {
      return;
    }
    terminal.options.fontFamily = buildEffectiveTerminalFontFamily(
      terminalPrefs.fontFamilyOverride,
    );
    terminal.options.fontSize = terminalPrefs.fontSize;
    terminal.options.theme = { background: terminalPrefs.themeBackground };
    // A font metric change (family or size) recomputes the emulator's cell
    // size but not its col/row count, so without a re-fit a larger font
    // overflows/clips the pane and the daemon PTY keeps the stale geometry
    // until the next container resize. Re-run the mount effect's own fit +
    // size-forward closures (same path as a ResizeObserver tick). Both refs are
    // nulled on the mount effect's cleanup, so the optional-chaining guard makes
    // this a no-op against a disposed terminal.
    fitNowRef.current?.();
    forwardSizeRef.current?.();
  }, [terminalPrefs]);

  // Stream PTY output deltas into the emulator so ANSI color and control
  // sequences render as terminal behavior rather than raw text. The
  // branch/offset decision is pure (`resolveTerminalDeltaWrite`, 260723 Phase
  // 1 load-bearing fix for `appendTerminalOutput`'s front-trim bound) - this
  // effect only performs the resulting `terminal.clear()`/`terminal.write()`/
  // ref-update side effects.
  useEffect(() => {
    const terminal = terminalRef.current;
    if (!terminal) {
      return;
    }
    const resolved = resolveTerminalDeltaWrite(pane, writtenAbsoluteRef.current);
    if (resolved.kind === "noop") {
      return;
    }
    if (resolved.kind === "reset") {
      terminal.clear();
    }
    terminal.write(resolved.text);
    writtenAbsoluteRef.current = resolved.nextWrittenAbsolute;
  }, [pane.output]);

  // Debounced capture of this pane's serialized visual buffer (scrollback,
  // cursor, styles) plus scroll viewport offset, persisted browser-locally
  // so a page reload can restore this pane's appearance for an id-reattached
  // terminal (see the mount effect's restore branch above), rather than only
  // the plain-text `pane.output` history. Triggered on the same `pane.output`
  // changes as the delta-write effect above - any new PTY output is a reason
  // to refresh the snapshot - but coalesced behind an idle timer per the
  // ticket's "writes debounced" constraint, since output can arrive many
  // times per second. The timer is owned by this effect's own cleanup, which
  // React runs on every dependency change and on unmount, so a disposed
  // terminal can never have a pending serialize callback fire against it.
  useEffect(() => {
    visualCaptureTimerRef.current = window.setTimeout(() => {
      visualCaptureTimerRef.current = null;
      const serializeAddon = serializeAddonRef.current;
      const terminal = terminalRef.current;
      if (!serializeAddon || !terminal) {
        return;
      }
      const serialized = serializeAddon.serialize({
        scrollback: terminalVisualRestoreScrollbackLines,
      });
      liveRef.current.actions.onVisualCapture(liveRef.current.pane, {
        serialized,
        viewportY: terminal.buffer.active.viewportY,
        nextSequence: liveRef.current.actions.getPendingNextSequence(
          liveRef.current.pane,
        ),
      });
    }, terminalVisualRestoreDebounceMs);
    return () => {
      if (visualCaptureTimerRef.current !== null) {
        window.clearTimeout(visualCaptureTimerRef.current);
        visualCaptureTimerRef.current = null;
      }
    };
  }, [pane.output]);

  // Root-switch auto-focus: when `actions.shouldAutoFocus(pane)` flips to
  // `true` (this pane is the one the App-level `lastFocusedPaneByRootRef`
  // recorded as last-focused within the root that just became selected),
  // grab real keyboard focus the same way a user click into the terminal
  // would - `terminal.focus()` fires this container's own `focusin`
  // listener (`markFocusedTerminal`, mount effect above), which sets
  // `keepTerminalFocusRef.current = true` and lets the existing 100ms
  // watchdog defend the focus from there, so no separate keep-focus
  // bookkeeping is duplicated here. Deps `[shouldAutoFocus]` only: this must
  // fire once per false->true transition, not on every render while it
  // stays `true` (which would fight the user's own subsequent clicks
  // elsewhere in the same still-selected root). Deferred one tick, mirroring
  // `refocusActiveTerminal`'s idiom, so this runs after the sidebar click's
  // own window-level `focusin`/`pointerdown` listeners have already cleared
  // any stale `keepTerminalFocusRef` state from the previously focused pane.
  //
  // Also re-fits/forwards size proactively here rather than leaning on
  // Effect A's `paneVisible` correction below: that correction only starts
  // once the 100ms `focusWatchdog` poll notices `container.offsetParent`
  // went non-null, so it can lag a root switch by up to ~100ms. A manual
  // click into the terminal happens well after that window in practice
  // (human reaction time), masking the race - but auto-focus lands on the
  // very next tick, so without this, typing immediately after a root switch
  // could reach the daemon before the PTY's dimensions were corrected for
  // the newly visible pane's actual size. `fitNow`/`forwardSize` are both
  // cheap no-ops when the size already matches (fit) or was already
  // forwarded (`lastForwardedSizeRef` dedupe), so calling them here in
  // addition to Effect A's own later correction is harmless.
  //
  // The fit/forward half is deferred through two rAFs (same reasoning as
  // Effect A below): reading the container's box on the very same tick a
  // root switch happens can observe Dockview's own relayout mid-flight, not
  // yet settled. The focus half stays on an immediate `setTimeout(0)` -
  // unlike the size measurement, grabbing focus has no correctness
  // dependency on layout being settled, and delaying it would reintroduce
  // perceptible input lag.
  const shouldAutoFocus = actions.shouldAutoFocus(pane);
  useEffect(() => {
    if (!shouldAutoFocus) {
      return;
    }
    window.setTimeout(() => {
      terminalRef.current?.focus();
      containerRef.current
        ?.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea")
        ?.focus();
    }, 0);
    let rafA = 0;
    let rafB = 0;
    rafA = window.requestAnimationFrame(() => {
      rafB = window.requestAnimationFrame(() => {
        fitNowRef.current?.();
        forwardSizeRef.current?.();
      });
    });
    return () => {
      window.cancelAnimationFrame(rafA);
      window.cancelAnimationFrame(rafB);
    };
  }, [shouldAutoFocus]);

  // Gated on `pane.session.status` (the parent-owned session view), which
  // observes both the live WebSocket "message" listener and HTTP
  // fallback-poll status updates (see `appendTerminalOutput` in
  // terminals.ts) - the retirement treatment must cover both transports
  // (260724 Phase 2). Retain-with-clear, not auto-remove, per the ticket
  // contract - the pane and its scrollback stay visible until the user
  // explicitly clears it.
  const isRetired = terminalRetiredStatuses.has(pane.session.status);

  return (
    <div
      className={
        isRetired ? "terminal-pane terminal-pane-retired" : "terminal-pane"
      }
      data-terminal-id={terminalId}
      // CONTRACT (260725 Phase 2, browser spawn profile): browser-visible
      // provenance hook, mirroring `data-terminal-id` - lets the acceptance
      // suite assert which registry profile (if any) produced this pane
      // straight from rendered DOM rather than only from the network
      // response. Empty string (not omitted) for the unchanged
      // default-shell path, matching `TerminalSessionView.profileId`'s
      // `null`.
      data-profile-id={pane.session.profileId ?? ""}
      // CONTRACT (260728, replaces the `<cols>x<rows>` half of the removed
      // `.terminal-status-line`): browser-visible projection of the PTY
      // logical size this pane last forwarded to the daemon, same
      // provenance-hook pattern as `data-terminal-id`/`data-profile-id`
      // above. The acceptance gate's PTY-resize assertions read this;
      // without a DOM projection they have no non-emulator source for the
      // forwarded size at all (`.xterm-rows > div` only measures the
      // emulator's own grid, and is deliberately kept as the independent
      // second signal). Sourced from `forwardedPtySize`, NOT from
      // `pane.session` - see that state's declaration for why the prop is
      // frozen for a connected terminal. Unlike the removed status bar's
      // `displaySession` mirror, which only observed the socket path, this
      // covers both resize transports.
      data-terminal-columns={forwardedPtySize.columns}
      data-terminal-rows={forwardedPtySize.rows}
    >
      <div
        className="terminal-surface"
        data-command-id="terminal.input"
        ref={containerRef}
      />
      {pane.error ? <div className="terminal-error">{pane.error}</div> : null}
    </div>
  );
}

function terminalScreenFitsVisibleBox(container: HTMLElement) {
  const screen = container.querySelector<HTMLElement>(".xterm-screen");
  if (!screen) {
    return true;
  }
  const containerBox = container.getBoundingClientRect();
  const screenBox = screen.getBoundingClientRect();
  return screenBox.bottom <= containerBox.bottom + 0.5;
}
