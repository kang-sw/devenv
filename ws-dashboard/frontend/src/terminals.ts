import { defaultPtyLogicalSize } from "./workbench/policy.js";

export type TerminalSessionView = {
  terminalId: string;
  workRootId: string;
  title: string;
  status: "running" | "exited" | "terminated" | "error" | string;
  columns: number;
  rows: number;
  createdAtMs: number;
};

export type TerminalOutputChunk = {
  sequence: number;
  data: string;
  stream: "pty" | string;
};

export type TerminalOutputView = {
  terminalId: string;
  status: TerminalSessionView["status"];
  nextSequence: number;
  chunks: TerminalOutputChunk[];
};

export type TerminalPaneState = {
  session: TerminalSessionView;
  logicalKey: string;
  paneId: string;
  output: string;
  nextSequence: number;
  inputDraft: string;
  error: string | null;
};

export function workRootTerminalsEndpoint(workRootId: string) {
  return `/api/dashboard/work-roots/${encodeURIComponent(workRootId)}/terminals`;
}

export function terminalOutputEndpoint(terminalId: string, after = 0) {
  const query = new URLSearchParams({ after: String(after) });
  return `/api/dashboard/terminals/${encodeURIComponent(terminalId)}/output?${query.toString()}`;
}

export function terminalInputEndpoint(terminalId: string) {
  return `/api/dashboard/terminals/${encodeURIComponent(terminalId)}/input`;
}

export function terminalResizeEndpoint(terminalId: string) {
  return `/api/dashboard/terminals/${encodeURIComponent(terminalId)}/resize`;
}

export function terminalCloseEndpoint(terminalId: string) {
  return `/api/dashboard/terminals/${encodeURIComponent(terminalId)}`;
}

export async function createTerminal(workRootId: string) {
  const response = await fetch(workRootTerminalsEndpoint(workRootId), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify({
      columns: defaultPtyLogicalSize.columns,
      rows: defaultPtyLogicalSize.rows,
      title: "Terminal",
    }),
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return (await response.json()) as TerminalSessionView;
}

export async function listTerminals(workRootId: string) {
  const response = await fetch(workRootTerminalsEndpoint(workRootId), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return (await response.json()) as TerminalSessionView[];
}

export async function fetchTerminalOutput(terminalId: string, after: number) {
  const response = await fetch(terminalOutputEndpoint(terminalId, after), {
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return (await response.json()) as TerminalOutputView;
}

export async function sendTerminalInput(terminalId: string, data: string) {
  const response = await fetch(terminalInputEndpoint(terminalId), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify({ data }),
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
}

export async function resizeTerminal(terminalId: string, columns: number, rows: number) {
  const size = validateTerminalSize(columns, rows);
  const response = await fetch(terminalResizeEndpoint(terminalId), {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(size),
  });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
  return (await response.json()) as TerminalSessionView;
}

export async function closeTerminal(terminalId: string) {
  const response = await fetch(terminalCloseEndpoint(terminalId), { method: "DELETE" });
  if (!response.ok) {
    throw new Error(await terminalErrorMessage(response));
  }
}

export function terminalPaneLogicalKey(workRootId: string, terminalId: string) {
  return ["persistentTerminal", workRootId, terminalId].join("/");
}

export function terminalPaneId(terminalId: string) {
  return `terminal:${encodeURIComponent(terminalId)}`;
}

export function terminalPaneFromSession(session: TerminalSessionView): TerminalPaneState {
  return {
    session,
    logicalKey: terminalPaneLogicalKey(session.workRootId, session.terminalId),
    paneId: terminalPaneId(session.terminalId),
    output: "",
    nextSequence: 0,
    inputDraft: "",
    error: null,
  };
}

export function mergeListedTerminalSessions(
  current: Record<string, TerminalPaneState>,
  sessions: TerminalSessionView[],
) {
  const next = { ...current };
  for (const session of sessions) {
    const key = terminalPaneLogicalKey(session.workRootId, session.terminalId);
    next[key] = next[key]
      ? { ...next[key], session }
      : terminalPaneFromSession(session);
  }
  return next;
}

export function appendTerminalOutput(pane: TerminalPaneState, output: TerminalOutputView) {
  return {
    ...pane,
    session: { ...pane.session, status: output.status },
    output: pane.output + output.chunks.map((chunk) => chunk.data).join(""),
    nextSequence: output.nextSequence,
    error: null,
  };
}

export function removeClosedTerminalPane(
  current: Record<string, TerminalPaneState>,
  logicalKey: string,
) {
  const next = { ...current };
  delete next[logicalKey];
  return next;
}

export function validateTerminalSize(columns: number, rows: number) {
  if (!Number.isInteger(columns) || !Number.isInteger(rows) || columns <= 0 || rows <= 0 || columns > 300 || rows > 120) {
    throw new Error("invalid terminal size");
  }
  return { columns, rows };
}

async function terminalErrorMessage(response: Response) {
  try {
    const value = (await response.json()) as { error?: unknown };
    if (typeof value.error === "string" && value.error.trim()) {
      return value.error;
    }
  } catch {
    // Fall through.
  }
  return `HTTP ${response.status}`;
}
