import {
  createRootPickerDirectory,
  fetchRootPicker,
  pinRootPickerDirectory,
  rootPickerHistoryBack,
  rootPickerHistoryForward,
  rootPickerHistoryInitial,
  rootPickerHistoryPush,
  rootPickerCreateDirectoryEndpoint,
  rootPickerEndpoint,
  rootPickerEntryLabel,
  rootPickerInsertEntry,
  rootPickerListEndpoint,
  rootPickerModifiedTimeLabel,
  rootPickerPinnedPathSet,
  serverRootPickerCreateDirectoryEndpoint,
  serverRootPickerEndpoint,
  serverRootPickerPinsEndpoint,
  rootPickerPinsEndpoint,
  rootPickerVisibleEntries,
  rootPickerVisiblePlaces,
  unpinRootPickerDirectory,
  type RootPickerEntry,
  type RootPickerView,
} from "./rootPicker.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
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
  rootPickerListEndpoint(null, "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/root-picker",
  "server-scoped picker endpoint encodes server id",
);
assertEqual(
  rootPickerListEndpoint("C:/Users/Test Root", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/root-picker?path=C%3A%2FUsers%2FTest+Root",
  "server-scoped picker endpoint keeps host path only as query data",
);
assertEqual(
  serverRootPickerCreateDirectoryEndpoint("server-remote-1"),
  "/api/dashboard/servers/server-remote-1/root-picker/directories",
  "server-scoped create-directory endpoint encodes server id",
);
assertEqual(
  serverRootPickerPinsEndpoint("server-remote-1"),
  "/api/dashboard/servers/server-remote-1/root-picker/pins",
  "server-scoped pins endpoint encodes server id",
);

function assertThrows(fn: () => unknown, label: string) {
  let threw = false;
  try {
    fn();
  } catch {
    threw = true;
  }
  if (!threw) {
    throw new Error(`${label}: expected a thrown error`);
  }
}

// Dot is reserved as a future hop separator, so a dotted route segment must be
// rejected by the canonical server route builder rather than URL-encoded.
assertThrows(
  () => rootPickerListEndpoint(null, "server.remote"),
  "dotted server route is rejected by the canonical route builder",
);
assertThrows(
  () => serverRootPickerPinsEndpoint("server remote/1"),
  "route segment with reserved characters is rejected",
);
assertEqual(
  serverRootPickerEndpoint("server-local"),
  rootPickerEndpoint,
  "server-local picker endpoint preserves local compatibility route",
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
      source: "builtIn",
      available: true,
    },
    {
      id: "mnt",
      label: "Mounts",
      path: "/mnt",
      kind: "mount",
      source: "builtIn",
      available: false,
    },
    {
      id: "pin-missing",
      label: "missing",
      path: "/missing",
      kind: "pin",
      source: "pin",
      available: false,
    },
  ],
};

assertDeepEqual(
  rootPickerVisibleEntries([{ ...alpha, entryType: "directory" }]).map(
    (entry) => entry.path,
  ),
  ["/tmp/root/alpha"],
  "folder-only picker filter keeps directory rows",
);
assertDeepEqual(
  rootPickerVisiblePlaces(pickerView).map((place) => place.label),
  ["Home", "missing"],
  "known places hide unavailable built-ins but keep unavailable pins",
);
assertDeepEqual(
  Array.from(rootPickerPinnedPathSet(pickerView)),
  ["/missing"],
  "pinned path set includes persisted pin places",
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
  rootPickerHistoryPush(
    rootPickerHistoryInitial("/tmp/root"),
    "/tmp/root/alpha",
  ),
  "/tmp/root/zeta",
);
const back = rootPickerHistoryBack(history);
assertEqual(
  back.targetPath,
  "/tmp/root/alpha",
  "back history targets previous folder",
);
const forward = rootPickerHistoryForward(back.history);
assertEqual(
  forward.targetPath,
  "/tmp/root/zeta",
  "forward history restores next folder",
);

const originalFetch = globalThis.fetch;
let capturedUrl = "";
let capturedBody = "";
let capturedMethod = "";
globalThis.fetch = (async (input, init) => {
  capturedUrl = String(input);
  capturedBody = String(init?.body ?? "");
  capturedMethod = String(init?.method ?? "GET");
  if (capturedUrl.endsWith("/root-picker/pins")) {
    return new Response(JSON.stringify({ places: pickerView.places }), {
      status: 200,
      headers: { "content-type": "application/json" },
    });
  }
  if (capturedUrl.endsWith("/root-picker/directories")) {
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
assertEqual(
  fetched.currentPath,
  "/tmp/root",
  "fetchRootPicker decodes picker view",
);

const remoteFetched = await fetchRootPicker("C:/Remote Root", "server-remote");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server-remote/root-picker?path=C%3A%2FRemote+Root",
  "remote fetchRootPicker targets server-scoped picker listing endpoint",
);
assertEqual(
  remoteFetched.currentPath,
  "/tmp/root",
  "remote fetchRootPicker decodes picker view through local gateway",
);

const created = await createRootPickerDirectory("/tmp/root", "alpha");
assertEqual(
  capturedUrl,
  rootPickerCreateDirectoryEndpoint,
  "create directory endpoint is stable",
);
assertDeepEqual(
  JSON.parse(capturedBody),
  { parentPath: "/tmp/root", name: "alpha" },
  "create directory request carries parent and single-segment name",
);
assertEqual(created.path, "/tmp/root/alpha", "create directory decodes entry");

await createRootPickerDirectory("C:/Remote Root", "alpha", "server-remote");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server-remote/root-picker/directories",
  "remote create directory targets server-scoped endpoint through local gateway",
);
assertDeepEqual(
  JSON.parse(capturedBody),
  { parentPath: "C:/Remote Root", name: "alpha" },
  "remote create directory keeps remote path in authenticated request body",
);

await pinRootPickerDirectory("/tmp/root/alpha");
assertEqual(
  capturedUrl,
  rootPickerPinsEndpoint,
  "pin directory endpoint is stable",
);
assertEqual(capturedMethod, "POST", "pin directory uses POST");
assertDeepEqual(
  JSON.parse(capturedBody),
  { path: "/tmp/root/alpha" },
  "pin directory request carries path as authenticated request data",
);

await pinRootPickerDirectory("C:/Remote Root/alpha", "server-remote");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server-remote/root-picker/pins",
  "remote pin directory targets server-scoped endpoint through local gateway",
);
assertEqual(capturedMethod, "POST", "remote pin directory uses POST");
assertDeepEqual(
  JSON.parse(capturedBody),
  { path: "C:/Remote Root/alpha" },
  "remote pin directory keeps remote path in authenticated request body",
);

await unpinRootPickerDirectory("/tmp/root/alpha");
assertEqual(
  capturedUrl,
  rootPickerPinsEndpoint,
  "unpin directory endpoint is stable",
);
assertEqual(capturedMethod, "DELETE", "unpin directory uses DELETE");

await unpinRootPickerDirectory("C:/Remote Root/alpha", "server-remote");
assertEqual(
  capturedUrl,
  "/api/dashboard/servers/server-remote/root-picker/pins",
  "remote unpin directory targets server-scoped endpoint through local gateway",
);
assertEqual(capturedMethod, "DELETE", "remote unpin directory uses DELETE");

globalThis.fetch = originalFetch;
