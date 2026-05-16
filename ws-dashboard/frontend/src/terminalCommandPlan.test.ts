import {
  terminalCommandPlanForPlatform,
  type TerminalCommandPlan,
  type TerminalShellProfile,
} from "./terminalCommandPlan.js";

function assertType<T>(_value: T) {}

// CONTRACT: Route/browser tests should cover Unix shell, cmd.exe, and
// PowerShell command plans for echo, ANSI, scroll, alternate-screen, clear,
// interrupt/long-running, EOF, resize, and paste-visible behavior.
// HINT: These are skeleton contract targets; implementation should replace
// comment-only coverage with executable assertions.
assertType<
  (
    platform?: NodeJS.Platform,
    shellHint?: string,
  ) => TerminalCommandPlan
>(terminalCommandPlanForPlatform);
assertType<TerminalShellProfile>("unix-sh");
assertType<TerminalShellProfile>("cmd-exe");
assertType<TerminalShellProfile>("powershell");
