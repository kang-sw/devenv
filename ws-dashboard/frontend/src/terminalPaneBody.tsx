import { useContext, useEffect, useRef, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SerializeAddon } from "@xterm/addon-serialize";
import {
  clampTerminalSize,
  terminalWebSocketCursor,
  terminalWebSocketUrl,
  type TerminalPaneState,
  type TerminalWebSocketServerMessage,
} from "./terminals.js";
import {
  buildEffectiveTerminalFontFamily,
  TerminalPrefsContext,
} from "./terminalPrefs.js";
import {
  resolveTerminalMountWrite,
  terminalVisualRestoreDebounceMs,
  terminalVisualRestoreScrollbackLines,
  type TerminalVisualRestoreEntry,
} from "./workbench/terminalVisualRestore.js";

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
  const writtenLengthRef = useRef(0);
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
  const visualCaptureTimerRef = useRef<number | null>(null);
  const keepTerminalFocusRef = useRef(false);
  const [displaySession, setDisplaySession] = useState(() => pane.session);
  // Optimistic default matches current always-connect behavior for the
  // common case of a newly mounted, actually-visible pane; a pane mounted
  // while already hidden briefly opens then closes on the first watchdog
  // tick, an accepted minor inefficiency, not a correctness issue.
  const [paneVisible, setPaneVisible] = useState(true);
  // Live terminal-style prefs (font family/size, background) for the mount
  // effect's construction-time read below and the post-mount subscription
  // effect further down (see that effect for the live-apply path).
  const terminalPrefs = useContext(TerminalPrefsContext);
  // Latest pane/actions/terminalPrefs for emulator callbacks registered once
  // at mount.
  const liveRef = useRef({ pane, actions, terminalPrefs });
  liveRef.current = { pane, actions, terminalPrefs };

  const terminalId = pane.session.terminalId;
  const refocusActiveTerminal = () => {
    window.setTimeout(() => {
      if (keepTerminalFocusRef.current && containerRef.current?.offsetParent) {
        terminalRef.current?.focus();
        containerRef.current
          ?.querySelector<HTMLTextAreaElement>(".xterm-helper-textarea")
          ?.focus();
      }
    }, 0);
  };

  useEffect(() => {
    setDisplaySession(pane.session);
  }, [pane.session]);

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
    });
    const fitAddon = new FitAddon();
    terminal.loadAddon(fitAddon);
    const serializeAddon = new SerializeAddon();
    terminal.loadAddon(serializeAddon);
    serializeAddonRef.current = serializeAddon;
    terminal.open(container);
    terminalRef.current = terminal;
    writtenLengthRef.current = 0;

    // A reattached pane with a matching persisted visual-restore snapshot
    // (id-reattach to a still-alive daemon terminal) writes that serialized
    // buffer - scrollback, cursor position, styles - plus its scroll
    // viewport offset, instead of the plain-text `pane.output` replay below.
    // `writtenLengthRef` stays at 0 in both branches: the delta-write effect
    // tracks `pane.output` length independent of whichever initial write
    // happened here, so a restored snapshot's own escape-sequence text is
    // never diffed against `pane.output` (which starts at "" for a freshly
    // reattached pane either way). New sessions spawned via the
    // restore-intent fallback have no matching entry and fall through to the
    // existing replay path unchanged.
    //
    // The three-way branch selection itself (restore vs. replay vs. no-op)
    // is pure and lives in `resolveTerminalMountWrite` (`workbench/terminalVisualRestore.ts`)
    // so it is unit testable independent of xterm/DOM; only the actual
    // `terminal.write`/`scrollToLine`/`writtenLengthRef` side effects stay here.
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
      writtenLengthRef.current = mountWrite.text.length;
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
    let composingInput = false;
    const markComposing = () => {
      composingInput = true;
    };
    const clearComposing = () => {
      composingInput = false;
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
      if (event.isComposing || event.key === "Process" || composingInput) {
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
      if (!proposed || proposed.rows <= 1) {
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
        setDisplaySession((current) => ({
          ...current,
          columns: next.columns,
          rows: next.rows,
        }));
        liveRef.current.actions.onSocketResize(
          liveRef.current.pane,
          next.columns,
          next.rows,
        );
        lastForwardedSizeRef.current = { ...next, transport: "socket" };
        return;
      }
      void liveRef.current.actions
        .onResize(liveRef.current.pane, next.columns, next.rows)
        .then(() => {
          lastForwardedSizeRef.current = { ...next, transport: "http" };
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
      refocusActiveTerminal();
    }, 100);

    return () => {
      observer.disconnect();
      window.removeEventListener("resize", scheduleResizeForward);
      window.clearInterval(focusWatchdog);
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
    }
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
          writtenLengthRef.current += message.chunk.data.length;
          if (liveRef.current.actions.isActivePane(liveRef.current.pane)) {
            refocusActiveTerminal();
          }
        } else {
          setDisplaySession((current) => ({
            ...current,
            status: message.status,
          }));
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
  // sequences render as terminal behavior rather than raw text.
  useEffect(() => {
    const terminal = terminalRef.current;
    if (!terminal) {
      return;
    }
    if (pane.output.length > writtenLengthRef.current) {
      terminal.write(pane.output.slice(writtenLengthRef.current));
      writtenLengthRef.current = pane.output.length;
    } else if (pane.output.length < writtenLengthRef.current) {
      terminal.clear();
      terminal.write(pane.output);
      writtenLengthRef.current = pane.output.length;
    }
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

  return (
    <div className="terminal-pane" data-terminal-id={terminalId}>
      <div
        className="terminal-surface"
        data-command-id="terminal.input"
        ref={containerRef}
      />
      {pane.error ? <div className="terminal-error">{pane.error}</div> : null}
      <div className="terminal-controls">
        <span className="terminal-status-line">
          {displaySession.status} · {displaySession.columns}x
          {displaySession.rows}
        </span>
        <button
          className="action-button"
          data-command-id="terminal.close"
          type="button"
          onClick={() => actions.onClose(pane)}
        >
          Terminate
        </button>
      </div>
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
