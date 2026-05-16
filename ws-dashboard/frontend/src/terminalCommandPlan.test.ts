import {
  terminalCommandPlanForPlatform,
  type TerminalCommandPlan,
  type TerminalShellProfile,
} from "./terminalCommandPlan.js";

function assertType<T>(_value: T) {}
function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) throw new Error(`${label}: expected ${expected}, got ${actual}`);
}
function assertIncludes(actual: string, expected: string, label: string) {
  if (!actual.includes(expected)) throw new Error(`${label}: expected ${actual} to include ${expected}`);
}

// CONTRACT: Route/browser tests should cover Unix shell, cmd.exe, and
// PowerShell command plans for echo, ANSI, scroll, alternate-screen, clear,
// interrupt/long-running, EOF, resize, and paste-visible behavior.
// HINT: These are skeleton contract targets; implementation should replace
// comment-only coverage with executable assertions.
assertType<
  (
    platform?: string,
    shellHint?: string,
  ) => TerminalCommandPlan
>(terminalCommandPlanForPlatform);
assertType<TerminalShellProfile>("unix-sh");
assertType<TerminalShellProfile>("cmd-exe");
assertType<TerminalShellProfile>("powershell");

const unix = terminalCommandPlanForPlatform("linux", "/bin/sh");
assertEqual(unix.profile, "unix-sh", "Unix profile selected");
assertIncludes(unix.echo("UNIX-MARKER"), "UNIX-MARKER", "Unix echo includes marker");
assertIncludes(unix.scrollLines("LINE-", 3), "LINE-", "Unix scroll includes prefix");
assertIncludes(unix.clearAndEcho("CLEAR-OK"), "clear", "Unix clear command clears first");

const cmd = terminalCommandPlanForPlatform("win32", "cmd.exe");
assertEqual(cmd.profile, "cmd-exe", "cmd profile selected");
assertIncludes(cmd.echo("CMD-MARKER"), "CMD-MARKER", "cmd echo includes marker");
assertIncludes(cmd.scrollLines("LINE-", 3), "for /L", "cmd scroll uses built-in loop");
assertIncludes(cmd.longRunningCommand(), "ping", "cmd long-running command uses built-in command");

const powershell = terminalCommandPlanForPlatform("win32", "PowerShell");
assertEqual(powershell.profile, "powershell", "PowerShell profile selected");
assertIncludes(powershell.echo("PS-MARKER"), "PS-MARKER", "PowerShell echo includes marker");
assertIncludes(powershell.scrollLines("LINE-", 3), "ForEach-Object", "PowerShell scroll uses pipeline loop");
assertIncludes(powershell.longRunningCommand(), "Start-Sleep", "PowerShell long-running command uses built-in command");
