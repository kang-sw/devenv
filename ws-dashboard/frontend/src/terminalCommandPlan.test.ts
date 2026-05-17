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
function assertThrows(fn: () => unknown, label: string) {
  try {
    fn();
  } catch {
    return;
  }
  throw new Error(`${label}: expected throw`);
}

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
assertEqual(unix.limitations.length, 0, "Unix profile has no known command limitations");
assertIncludes(unix.echo("UNIX-MARKER"), "UNIX-MARKER", "Unix echo includes marker");
assertIncludes(unix.echo("quote'&marker"), "'quote'\\''&marker'", "Unix single quote is escaped");
assertIncludes(unix.ansiGreen("GREEN"), "\\033[32m", "Unix ANSI emits SGR");
assertIncludes(unix.scrollLines("LINE-", 3), "LINE-", "Unix scroll includes prefix");
assertIncludes(unix.scrollLines("LINE-", 3), "-le 3", "Unix scroll bounds count");
assertIncludes(unix.alternateScreenBottomRow("BOTTOM"), "BOTTOM", "Unix alternate fixture includes marker");
assertIncludes(unix.clearAndEcho("CLEAR-OK"), "clear", "Unix clear command clears first");
assertIncludes(unix.trailingWordEcho("WORD-OK"), "WORD-OK", "Unix trailing word echo includes marker");
assertThrows(() => unix.trailingWordEcho("bad word"), "Unix trailing word rejects spaces");
assertIncludes(unix.longRunningCommand(), "sleep", "Unix long-running command uses built-in sleep");
assertThrows(() => unix.scrollLines("BAD", 0), "Unix rejects zero count");

const cmd = terminalCommandPlanForPlatform("win32", "cmd.exe");
assertEqual(cmd.profile, "cmd-exe", "cmd profile selected");
assertIncludes(cmd.limitations.join("\n"), "ANSI", "cmd profile records ANSI limitation");
assertIncludes(cmd.echo("CMD-MARKER"), "CMD-MARKER", "cmd echo includes marker");
assertIncludes(cmd.echo("A&B|C%D^E"), "A^&B^|C^%D^^E", "cmd metacharacters are escaped");
assertIncludes(cmd.scrollLines("LINE-", 3), "for /L", "cmd scroll uses built-in loop");
assertIncludes(cmd.scrollLines("LINE-", 3), "(1,1,3)", "cmd scroll bounds count");
assertIncludes(cmd.clearAndEcho("CLEAR-OK"), "cls", "cmd clear command clears first");
assertIncludes(cmd.trailingWordEcho("WORD-OK"), "WORD-OK", "cmd trailing word echo includes marker");
assertIncludes(cmd.longRunningCommand(), "ping", "cmd long-running command uses built-in command");
assertThrows(() => cmd.scrollLines("BAD", 501), "cmd rejects excessive count");

const defaultWindows = terminalCommandPlanForPlatform("win32");
assertEqual(defaultWindows.profile, "powershell", "Windows profile defaults to PowerShell");

const powershell = terminalCommandPlanForPlatform("win32", "PowerShell");
assertEqual(powershell.profile, "powershell", "PowerShell profile selected");
assertIncludes(powershell.echo("PS-MARKER"), "PS-MARKER", "PowerShell echo includes marker");
assertIncludes(powershell.echo("quote'&marker"), "'quote''&marker'", "PowerShell single quote is escaped");
assertIncludes(powershell.ansiGreen("PS-GREEN"), "`e[32m", "PowerShell ANSI emits SGR prefix");
assertIncludes(powershell.ansiGreen("PS-GREEN"), "PS-GREEN", "PowerShell ANSI includes marker");
assertIncludes(powershell.ansiGreen("dollar$marker;ok"), "'dollar$marker;ok'", "PowerShell ANSI single-quotes shell metacharacters");
assertIncludes(powershell.scrollLines("LINE-", 3), "ForEach-Object", "PowerShell scroll uses pipeline loop");
assertIncludes(powershell.trailingWordEcho("WORD-OK"), "WORD-OK", "PowerShell trailing word echo includes marker");
assertIncludes(powershell.longRunningCommand(), "Start-Sleep", "PowerShell long-running command uses built-in command");
