import {
  createRootPickerDirectory,
  fetchRootPicker,
  rootPickerHistoryBack,
  rootPickerHistoryForward,
  rootPickerHistoryInitial,
  rootPickerHistoryPush,
  rootPickerCreateDirectoryEndpoint,
  rootPickerEntryLabel,
  rootPickerInsertEntry,
  rootPickerListEndpoint,
  rootPickerModifiedTimeLabel,
  rootPickerVisibleEntries,
  rootPickerVisiblePlaces,
  type RootPickerEntry,
  type RootPickerView,
} from "./rootPicker.js";

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

const alpha: RootPickerEntry = {
  name: "alpha",
  path: "/tmp/root/alpha",
  entryType: "directory",
  selectable: true,
  kindLabel: "Folder",
};
const zeta: RootPickerEntry = {
  name: "zeta",
  path: "/tmp/root/zeta",
  entryType: "directory",
  selectable: true,
};

assertEqual(
  rootPickerListEndpoint(),
  "/api/dashboard/root-picker",
  "default picker endpoint omits path query",
);
assertEqual(
  rootPickerListEndpoint("/tmp/work root"),
  "/api/dashboard/root-picker?path=%2Ftmp%2Fwork+root",
  "picker endpoint encodes exact host path as authenticated request data",
);
assertEqual(
  rootPickerEntryLabel("/tmp/work-root"),
  "work-root",
  "picker entry label derives basename",
);
assertDeepEqual(
  rootPickerInsertEntry([zeta], alpha).map((entry) => entry.name),
  ["alpha", "zeta"],
  "inserted picker entries stay sorted by display name",
);
assertDeepEqual(
  rootPickerInsertEntry([zeta, alpha], { ...alpha, name: "alpha-renamed" }).map(
    (entry) => entry.name,
  ),
  ["alpha-renamed", "zeta"],
  "inserted picker entries replace same path",
);

const pickerView: RootPickerView = {
  currentPath: "/tmp/root",
  parentPath: "/tmp",
  entries: [alpha, zeta],
  places: [
    {
      id: "home",
      label: "Home",
      path: "/home/tester",
      kind: "home",
      available: true,
    },
    {
      id: "mnt",
      label: "Mounts",
      path: "/mnt",
      kind: "mount",
      available: false,
    },
  ],
};

assertDeepEqual(
  rootPickerVisibleEntries([{ ...alpha, entryType: "directory" }]).map((entry) => entry.path),
  ["/tmp/root/alpha"],
  "folder-only picker filter keeps directory rows",
);
assertDeepEqual(
  rootPickerVisiblePlaces(pickerView).map((place) => place.label),
  ["Home"],
  "known places hide unavailable daemon-derived locations",
);
assertEqual(
  rootPickerModifiedTimeLabel(null),
  "",
  "missing modified time renders as an empty metadata cell",
);
assertEqual(
  rootPickerModifiedTimeLabel("not-a-timestamp"),
  "not-a-timestamp",
  "preformatted modified time labels pass through",
);

const history = rootPickerHistoryPush(
  rootPickerHistoryPush(rootPickerHistoryInitial("/tmp/root"), "/tmp/root/alpha"),
  "/tmp/root/zeta",
);
const back = rootPickerHistoryBack(history);
assertEqual(back.targetPath, "/tmp/root/alpha", "back history targets previous folder");
const forward = rootPickerHistoryForward(back.history);
assertEqual(forward.targetPath, "/tmp/root/zeta", "forward history restores next folder");

const originalFetch = globalThis.fetch;
let capturedUrl = "";
let capturedBody = "";
globalThis.fetch = (async (input, init) => {
  capturedUrl = String(input);
  capturedBody = String(init?.body ?? "");
  if (capturedUrl === rootPickerCreateDirectoryEndpoint) {
    return new Response(JSON.stringify(alpha), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }
  return new Response(JSON.stringify(pickerView), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}) as typeof fetch;

const fetched = await fetchRootPicker("/tmp/root");
assertEqual(
  capturedUrl,
  "/api/dashboard/root-picker?path=%2Ftmp%2Froot",
  "fetchRootPicker targets encoded picker listing endpoint",
);
assertEqual(fetched.currentPath, "/tmp/root", "fetchRootPicker decodes picker view");

const created = await createRootPickerDirectory("/tmp/root", "alpha");
assertEqual(capturedUrl, rootPickerCreateDirectoryEndpoint, "create directory endpoint is stable");
assertDeepEqual(
  JSON.parse(capturedBody),
  { parentPath: "/tmp/root", name: "alpha" },
  "create directory request carries parent and single-segment name",
);
assertEqual(created.path, "/tmp/root/alpha", "create directory decodes entry");

globalThis.fetch = originalFetch;
