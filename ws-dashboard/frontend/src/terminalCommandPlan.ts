export type TerminalShellProfile = "unix-sh" | "cmd-exe" | "powershell";

export type TerminalCommandPlan = {
  profile: TerminalShellProfile;
  echo(marker: string): string;
  ansiGreen(marker: string): string;
  scrollLines(prefix: string, count: number): string;
  alternateScreenBottomRow(marker: string): string;
  clearAndEcho(marker: string): string;
  longRunningCommand(): string;
};

export function terminalCommandPlanForPlatform(
  platform: NodeJS.Platform = process.platform,
  shellHint?: string,
): TerminalCommandPlan {
  // CONTRACT: Browser acceptance tests must express terminal intent through
  // this helper rather than embedding shared POSIX-only command strings.
  // HINT: Map Unix shells, cmd.exe, and PowerShell to equivalent observable
  // behaviors where practical.
  // HOLE: Fill command builders and shellHint detection.
  void platform;
  void shellHint;
  throw new Error("HOLE: terminal command plan");
}
