export type TerminalShellProfile = "unix-sh" | "cmd-exe" | "powershell";

export type TerminalCommandPlatform = "win32" | "darwin" | "linux" | string;

export type TerminalCommandPlan = {
  profile: TerminalShellProfile;
  limitations: string[];
  echo(marker: string): string;
  ansiGreen(marker: string): string;
  scrollLines(prefix: string, count: number): string;
  alternateScreenBottomRow(marker: string): string;
  clearAndEcho(marker: string): string;
  longRunningCommand(): string;
};

function quoteSingle(value: string): string {
  return `'${value.replace(/'/g, "'\\''")}'`;
}

function quotePowerShellSingle(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

function cmdLiteral(value: string): string {
  return value.replace(/[&<>|^%]/g, "^$&");
}

function boundedCount(count: number): number {
  if (!Number.isInteger(count) || count < 1 || count > 500) {
    throw new Error(`terminal command count must be an integer from 1 to 500, got ${count}`);
  }
  return count;
}

function runtimePlatform(): TerminalCommandPlatform {
  if (typeof navigator !== "undefined" && /Windows/i.test(navigator.userAgent)) {
    return "win32";
  }
  return "linux";
}

function profileFor(platform: TerminalCommandPlatform, shellHint?: string): TerminalShellProfile {
  const normalized = shellHint?.toLowerCase() ?? "";
  if (normalized.includes("powershell") || normalized.includes("pwsh")) {
    return "powershell";
  }
  if (normalized.includes("cmd.exe") || normalized === "cmd" || normalized === "cmd-exe") {
    return "cmd-exe";
  }
  return platform === "win32" ? "cmd-exe" : "unix-sh";
}

export function terminalCommandPlanForPlatform(
  platform: TerminalCommandPlatform = runtimePlatform(),
  shellHint?: string,
): TerminalCommandPlan {
  // CONTRACT: Browser acceptance tests must express terminal intent through
  // this helper rather than embedding shared POSIX-only command strings.
  // HINT: Map Unix shells, cmd.exe, and PowerShell to equivalent observable
  // behaviors where practical.
  const profile = profileFor(platform, shellHint);

  if (profile === "powershell") {
    return {
      profile,
      limitations: ["PowerShell alternate-screen fixture uses a deterministic line output substitute."],
      echo: (marker) => `Write-Output ${quotePowerShellSingle(marker)}`,
      ansiGreen: (marker) => `Write-Host ("` + "`e" + `[32m" + ${quotePowerShellSingle(marker)} + "` + "`e" + `[0m")`,
      scrollLines: (prefix, count) => `1..${boundedCount(count)} | ForEach-Object { Write-Output ${quotePowerShellSingle(prefix)}$_ }`,
      alternateScreenBottomRow: (marker) => `Write-Output ${quotePowerShellSingle(marker)}`,
      clearAndEcho: (marker) => `Clear-Host; Write-Output ${quotePowerShellSingle(marker)}`,
      longRunningCommand: () => "Start-Sleep -Seconds 30",
    };
  }

  if (profile === "cmd-exe") {
    return {
      profile,
      limitations: [
        "cmd.exe ANSI fixture asserts visible text only because SGR color support depends on host console settings.",
        "cmd.exe alternate-screen fixture uses a deterministic line output substitute.",
      ],
      echo: (marker) => `echo ${cmdLiteral(marker)}`,
      ansiGreen: (marker) => `echo ${cmdLiteral(marker)}`,
      scrollLines: (prefix, count) => `for /L %i in (1,1,${boundedCount(count)}) do @echo ${cmdLiteral(prefix)}%i`,
      alternateScreenBottomRow: (marker) => `echo ${cmdLiteral(marker)}`,
      clearAndEcho: (marker) => `cls & echo ${cmdLiteral(marker)}`,
      longRunningCommand: () => "ping -n 30 127.0.0.1 > nul",
    };
  }

  return {
    profile,
    limitations: [],
    echo: (marker) => `printf '%s\\n' ${quoteSingle(marker)}`,
    ansiGreen: (marker) => `printf '\\033[32m%s\\033[0m\\n' ${quoteSingle(marker)}`,
    scrollLines: (prefix, count) => `i=1; while [ $i -le ${boundedCount(count)} ]; do printf '%s%s\\n' ${quoteSingle(prefix)} "$i"; i=$((i+1)); done`,
    alternateScreenBottomRow: (marker) => `printf '%s\\n' ${quoteSingle(marker)}`,
    clearAndEcho: (marker) => `clear; printf '%s\\n' ${quoteSingle(marker)}`,
    longRunningCommand: () => "sleep 30",
  };
}
