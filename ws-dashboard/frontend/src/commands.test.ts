import {
  dashboardCommandLabel,
  dispatchDashboardCommand,
  type DashboardCommand,
} from "./commands.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(`${label}: expected ${String(expected)}, got ${String(actual)}`);
  }
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

const observed: string[] = [];
const executed: string[] = [];
const openFileCommand: DashboardCommand = {
  commandId: "fileExplorer.openFile",
  payload: {
    type: "fileExplorer.openFile",
    workRootId: "workRoot:local",
    path: "src/App.tsx",
    gesture: "singleClick",
  },
};

dispatchDashboardCommand(openFileCommand, {
  observer: (command) => observed.push(command.commandId),
  handlers: {
    "fileExplorer.openFile": (command) => {
      if (command.payload.type !== "fileExplorer.openFile") {
        throw new Error("open file handler received wrong payload");
      }
      executed.push(`${command.payload.workRootId}:${command.payload.path}:${command.payload.gesture}`);
    },
  },
});

assertDeepEqual(observed, ["fileExplorer.openFile"], "observer sees dispatched command id");
assertDeepEqual(
  executed,
  ["workRoot:local:src/App.tsx:singleClick"],
  "handler executes from the same dispatch path with logical file target",
);
assertEqual(dashboardCommandLabel(openFileCommand), "Open file", "open file label is stable");

const workRootOpenCommand: DashboardCommand = {
  commandId: "workRoot.open",
  payload: { type: "workRoot.open" },
};
assertEqual(
  Object.prototype.hasOwnProperty.call(workRootOpenCommand.payload, "path"),
  false,
  "workRoot.open command payload does not carry a host path",
);

let terminalCreates = 0;
const terminalCreateCommand: DashboardCommand = {
  commandId: "terminal.create",
  payload: { type: "terminal.create", workRootId: "workRoot:local" },
};
dispatchDashboardCommand(terminalCreateCommand, {
  handlers: {
    "terminal.create": (command) => {
      if (command.payload.type !== "terminal.create") {
        throw new Error("terminal handler received wrong payload");
      }
      terminalCreates += command.payload.workRootId === "workRoot:local" ? 1 : 0;
    },
  },
});
assertEqual(terminalCreates, 1, "programmatic terminal.create dispatch reaches executable handler");
assertEqual(
  dashboardCommandLabel({
    commandId: "workbench.openActivity",
    payload: { type: "workbench.openActivity", workRootId: "workRoot:local" },
  }),
  "Open WorkRoot Activity",
  "activity command label leaves an obvious later command surface",
);
