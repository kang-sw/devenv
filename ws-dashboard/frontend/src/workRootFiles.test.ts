import {
  applyReadOnlyFilePaneContent,
  applyReadOnlyFilePaneError,
  applyReadOnlyFilePaneSourceContent,
  applyReadOnlyFilePaneSourceError,
  createLoadingReadOnlyFilePane,
  documentDraftContentChangeDecision,
  documentSaveStateForError,
  fetchWorkRootFiles,
  fetchWorkRootTextFile,
  parseWorkRootDocumentEvent,
  flattenWorkRootFileTree,
  loadReadOnlyFilePaneRestoreSnapshot,
  readOnlyFilePaneRestoreSnapshot,
  saveReadOnlyFilePaneRestoreSnapshot,
  toggleExpandedPath,
  workRootExplorerInitialLoadPath,
  workRootExplorerRefreshPaths,
  workRootExplorerShouldLoadOnExpand,
  workRootDocumentEventsEndpoint,
  workRootFileReadEndpoint,
  workRootFileWriteEndpoint,
  writeWorkRootTextFile,
  workRootFilesEndpoint,
  readOnlyFilePaneId,
  readOnlyFilePaneLogicalKey,
  readOnlyFilePaneModeForOpenGesture,
  type DirectoryLoadState,
} from "./workRootFiles.js";

function assertEqual<T>(actual: T, expected: T, label: string) {
  if (actual !== expected) {
    throw new Error(
      `${label}: expected ${String(expected)}, got ${String(actual)}`,
    );
  }
}

async function assertRejects(
  action: () => Promise<unknown>,
  pattern: RegExp,
  label: string,
) {
  try {
    await action();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (!pattern.test(message)) {
      throw new Error(`${label}: error ${message} did not match ${pattern}`);
    }
    return;
  }

  throw new Error(`${label}: expected rejection`);
}

function assertDeepEqual(actual: unknown, expected: unknown, label: string) {
  const actualJson = JSON.stringify(actual);
  const expectedJson = JSON.stringify(expected);
  if (actualJson !== expectedJson) {
    throw new Error(`${label}: expected ${expectedJson}, got ${actualJson}`);
  }
}

assertEqual(
  workRootFilesEndpoint("root-local-abc", ""),
  "/api/dashboard/work-roots/root-local-abc/files",
  "root listing omits path query",
);
assertEqual(
  workRootFilesEndpoint("root/local abc", "src/main file.ts"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/files?path=src%2Fmain+file.ts",
  "endpoint encodes opaque workRootId and relative path only",
);
assertEqual(
  workRootFileReadEndpoint("root-local-abc", "src/main.rs"),
  "/api/dashboard/work-roots/root-local-abc/files/read?path=src%2Fmain.rs",
  "read endpoint encodes relative path query",
);
assertEqual(
  workRootFileReadEndpoint("root/local abc", "docs/read me.md"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/files/read?path=docs%2Fread+me.md",
  "read endpoint encodes opaque workRootId and spaced relative path",
);
assertEqual(
  workRootFileWriteEndpoint("root/local abc"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/files/write",
  "write endpoint encodes only the opaque workRootId",
);
assertEqual(
  workRootDocumentEventsEndpoint("root/local abc"),
  "/api/dashboard/work-roots/root%2Flocal%20abc/documents/events",
  "document events endpoint encodes only the opaque workRootId",
);

assertEqual(
  workRootFilesEndpoint("root/id", "src/main.ts", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/files?path=src%2Fmain.ts",
  "server-scoped file listing endpoint encodes server id and nested ids",
);
assertEqual(
  workRootFileReadEndpoint("root/id", "docs/read me.md", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/files/read?path=docs%2Fread+me.md",
  "server-scoped file read endpoint encodes server id and relative path",
);
assertEqual(
  workRootFileWriteEndpoint("root/id", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/files/write",
  "server-scoped file write endpoint encodes server id",
);
assertEqual(
  workRootDocumentEventsEndpoint("root/id", "server-remote-1"),
  "/api/dashboard/servers/server-remote-1/work-roots/root%2Fid/documents/events",
  "server-scoped document events endpoint encodes server id",
);

assertDeepEqual(
  parseWorkRootDocumentEvent({
    type: "document.contentChanged",
    source: { workRootId: "root-local-abc", path: "README.md" },
    contentHash: "sha256:abc",
    changedAtMs: 42,
  }),
  {
    type: "document.contentChanged",
    workRootId: "root-local-abc",
    path: "README.md",
    contentHash: "sha256:abc",
    source: "dashboard",
    savedAtMs: 42,
  },
  "document event parser accepts daemon contentChanged events",
);
assertEqual(
  parseWorkRootDocumentEvent({ type: "unknown" }),
  null,
  "document event parser rejects unrelated events",
);
assertEqual(
  readOnlyFilePaneModeForOpenGesture("singleClick"),
  "preview",
  "single-click file open selects replaceable preview mode",
);
assertEqual(
  readOnlyFilePaneModeForOpenGesture("doubleClick"),
  "pinned",
  "double-click file open selects stable pinned mode",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs"),
  "editor/server-local/root-local-abc/src/main.rs",
  "read-only logical key is scoped by workRootId and relative path",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs", "preview"),
  "editor-preview/server-local/root-local-abc",
  "preview logical key is a replaceable workRoot-scoped surface",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs") ===
    readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs"),
  true,
  "same file produces stable read-only logical key",
);
assertEqual(
  readOnlyFilePaneLogicalKey("root-local-abc", "src/main.rs") ===
    readOnlyFilePaneLogicalKey("root-local-abc", "src/lib.rs"),
  false,
  "different relative paths produce different logical keys",
);
assertEqual(
  readOnlyFilePaneId("root/local abc", "docs/read me.md"),
  "readonly:server-local%2Froot%2Flocal%20abc:docs%2Fread%20me.md",
  "pane id encodes scoped file identity without host paths",
);
assertEqual(
  readOnlyFilePaneId("root/local abc", "docs/read me.md", "preview"),
  "readonly-preview:server-local%2Froot%2Flocal%20abc",
  "preview pane id is one replaceable pane per workRoot",
);

assertEqual(
  readOnlyFilePaneLogicalKey(
    "root-same",
    "src/main.rs",
    "pinned",
    "server-a",
  ) ===
    readOnlyFilePaneLogicalKey(
      "root-same",
      "src/main.rs",
      "pinned",
      "server-b",
    ),
  false,
  "same bare file ids on different servers produce distinct read-only logical keys",
);
assertEqual(
  readOnlyFilePaneId("root-same", "src/main.rs", "pinned", "server-a") ===
    readOnlyFilePaneId("root-same", "src/main.rs", "pinned", "server-b"),
  false,
  "same bare file ids on different servers produce distinct pane ids",
);

const pinnedPane = applyReadOnlyFilePaneContent(
  createLoadingReadOnlyFilePane("root-local-abc", "src/main.ts", "pinned"),
  {
    workRootId: "root-local-abc",
    path: "src/main.ts",
    name: "main.ts",
    status: "ok",
    readOnly: true,
    editable: true,
    contentHash: "sha256:test",
    content: "secret live content",
    sizeBytes: 19,
    languageHint: "typescript",
    extension: "ts",
  },
);
const previewPane = applyReadOnlyFilePaneError(
  createLoadingReadOnlyFilePane("root-local-abc", "README.md", "preview"),
  "stale read failed",
);

const secondPane = applyReadOnlyFilePaneContent(
  createLoadingReadOnlyFilePane("root-local-abc", "src/main.ts", "preview"),
  {
    workRootId: "root-local-abc",
    path: "src/main.ts",
    name: "main.ts",
    status: "ok",
    readOnly: true,
    editable: true,
    contentHash: "sha256:old-preview",
    content: "old preview",
    sizeBytes: 11,
    languageHint: "typescript",
    extension: "ts",
  },
);
const unrelatedPane = applyReadOnlyFilePaneContent(
  createLoadingReadOnlyFilePane("root-local-abc", "README.md", "pinned"),
  {
    workRootId: "root-local-abc",
    path: "README.md",
    name: "README.md",
    status: "ok",
    readOnly: true,
    editable: true,
    contentHash: "sha256:readme",
    content: "readme",
    sizeBytes: 6,
    languageHint: "markdown",
    extension: "md",
  },
);
const fannedOut = applyReadOnlyFilePaneSourceContent(
  {
    [pinnedPane.logicalKey]: pinnedPane,
    [secondPane.logicalKey]: secondPane,
    [unrelatedPane.logicalKey]: unrelatedPane,
  },
  {
    workRootId: "root-local-abc",
    path: "src/main.ts",
    name: "main.ts",
    status: "ok",
    readOnly: true,
    editable: true,
    contentHash: "sha256:new",
    content: "new content",
    sizeBytes: 11,
    languageHint: "typescript",
    extension: "ts",
  },
);
assertEqual(
  fannedOut[pinnedPane.logicalKey].content,
  "new content",
  "source-key fan-out updates the pinned matching pane",
);
assertEqual(
  fannedOut[secondPane.logicalKey].contentHash,
  "sha256:new",
  "source-key fan-out updates the preview matching pane",
);
assertEqual(
  fannedOut[unrelatedPane.logicalKey].content,
  "readme",
  "source-key fan-out leaves unrelated panes untouched",
);
const erroredFanout = applyReadOnlyFilePaneSourceError(
  {
    [pinnedPane.logicalKey]: pinnedPane,
    [unrelatedPane.logicalKey]: unrelatedPane,
  },
  "root-local-abc",
  "src/main.ts",
  "refresh failed",
);
assertEqual(
  erroredFanout[pinnedPane.logicalKey].error,
  "refresh failed",
  "source-key refresh errors mark matching panes only",
);
assertEqual(
  erroredFanout[unrelatedPane.logicalKey].status,
  "loaded",
  "source-key refresh errors preserve unrelated panes",
);
assertDeepEqual(
  documentDraftContentChangeDecision("dirty"),
  {
    action: "preserveDraft",
    saveState: "stale",
    message: "File changed while this draft has unsaved edits",
  },
  "dirty drafts become stale and preserve local raw text when source content changes",
);
assertDeepEqual(
  documentDraftContentChangeDecision("saved"),
  { action: "syncDraft" },
  "clean or saved drafts sync to source content changes",
);
assertEqual(
  documentSaveStateForError("content hash mismatch"),
  "conflict",
  "optimistic write hash errors surface as conflicts",
);
assertEqual(
  documentSaveStateForError("file unavailable"),
  "error",
  "non-conflict save errors stay generic errors",
);
const restoreSnapshot = readOnlyFilePaneRestoreSnapshot(
  [pinnedPane, previewPane],
  { "group-2": [pinnedPane.id, previewPane.id, "missing-pane"] },
);
assertDeepEqual(
  Object.values(restoreSnapshot.panes).map((pane) => ({
    workRootId: pane.workRootId,
    path: pane.path,
    mode: pane.mode,
    status: pane.status,
    content: pane.content,
    error: pane.error,
  })),
  [
    {
      workRootId: "root-local-abc",
      path: "src/main.ts",
      mode: "pinned",
      status: "loading",
      content: "",
      error: null,
    },
    {
      workRootId: "root-local-abc",
      path: "README.md",
      mode: "preview",
      status: "loading",
      content: "",
      error: null,
    },
  ],
  "restore snapshot keeps descriptors but not file contents or stale errors",
);
assertDeepEqual(
  restoreSnapshot.orderByGroup,
  { "group-2": [pinnedPane.id, previewPane.id] },
  "restore snapshot stores only live pane-order hints",
);

const fakeStorage = new Map<string, string>();
const storage = {
  getItem: (key: string) => fakeStorage.get(key) ?? null,
  setItem: (key: string, value: string) => {
    fakeStorage.set(key, value);
  },
  removeItem: (key: string) => {
    fakeStorage.delete(key);
  },
};
saveReadOnlyFilePaneRestoreSnapshot(
  [pinnedPane, previewPane],
  { "group-2": [pinnedPane.id, previewPane.id] },
  storage,
);
const restored = loadReadOnlyFilePaneRestoreSnapshot(storage);
assertDeepEqual(
  Object.values(restored.panes).map((pane) => ({
    logicalKey: pane.logicalKey,
    id: pane.id,
    workRootId: pane.workRootId,
    path: pane.path,
    mode: pane.mode,
    title: pane.title,
    status: pane.status,
    content: pane.content,
  })),
  [
    {
      logicalKey: pinnedPane.logicalKey,
      id: pinnedPane.id,
      workRootId: "root-local-abc",
      path: "src/main.ts",
      mode: "pinned",
      title: "main.ts",
      status: "loading",
      content: "",
    },
    {
      logicalKey: previewPane.logicalKey,
      id: previewPane.id,
      workRootId: "root-local-abc",
      path: "README.md",
      mode: "preview",
      title: "README.md",
      status: "loading",
      content: "",
    },
  ],
  "read-only file pane descriptors round-trip through storage without contents",
);
assertDeepEqual(
  restored.orderByGroup,
  { "group-2": [pinnedPane.id, previewPane.id] },
  "read-only pane order hints round-trip through storage",
);
fakeStorage.set(
  "ws-dashboard.readOnlyFilePanes.v1",
  JSON.stringify({
    version: 1,
    panes: [
      {
        workRootId: "root-local-abc",
        path: "/abs/path",
        mode: "pinned",
        title: "bad",
      },
      {
        workRootId: "root-local-abc",
        path: "../secret",
        mode: "pinned",
        title: "bad",
      },
      {
        workRootId: "root-local-ok",
        path: "notes..md",
        mode: "pinned",
        title: "ok",
      },
    ],
    orderByGroup: {
      "group-1": ["readonly:root-local-ok:notes..md", "unknown"],
    },
  }),
);
const sanitized = loadReadOnlyFilePaneRestoreSnapshot(storage);
assertDeepEqual(
  Object.values(sanitized.panes).map((pane) => pane.path),
  ["notes..md"],
  "restore storage accepts relative paths but drops absolute or traversal descriptors",
);
fakeStorage.set("ws-dashboard.readOnlyFilePanes.v1", "not json");
assertDeepEqual(
  loadReadOnlyFilePaneRestoreSnapshot(storage),
  { panes: {}, orderByGroup: {} },
  "malformed read-only pane restore storage degrades to empty",
);

const expanded = toggleExpandedPath(new Set([""]), "src");
assertEqual(expanded.has(""), true, "toggle preserves existing expanded root");
assertEqual(expanded.has("src"), true, "toggle expands a collapsed path");
assertEqual(
  toggleExpandedPath(expanded, "src").has("src"),
  false,
  "toggle collapses an expanded path",
);

const loaded = (
  entries: DirectoryLoadState["entries"],
): DirectoryLoadState => ({
  status: "loaded",
  entries,
  error: null,
});

const rows = flattenWorkRootFileTree({
  expandedPaths: new Set(["", "src"]),
  selectedPath: "README.md",
  directories: {
    "": loaded([
      {
        name: "src",
        path: "src",
        kind: "directory",
        status: "ok",
        readable: true,
        previewEligible: false,
      },
      {
        name: "README.md",
        path: "README.md",
        kind: "file",
        status: "ok",
        readable: true,
        previewEligible: true,
      },
    ]),
    src: loaded([
      {
        name: "main.ts",
        path: "src/main.ts",
        kind: "file",
        status: "ok",
        readable: true,
        previewEligible: true,
      },
    ]),
  },
});

assertDeepEqual(
  rows.map((row) =>
    row.type === "entry"
      ? `${row.depth}:${row.entry.path}`
      : `${row.depth}:${row.status}`,
  ),
  ["0:src", "1:src/main.ts", "0:README.md"],
  "flattened tree includes expanded child rows in order",
);
assertEqual(
  rows[2].type === "entry" && rows[2].selected,
  true,
  "flattened tree marks selected row",
);

const emptyRows = flattenWorkRootFileTree({
  expandedPaths: new Set([""]),
  selectedPath: null,
  directories: { "": loaded([]) },
});
assertEqual(
  emptyRows[0]?.type === "state" && emptyRows[0].status,
  "empty",
  "empty root surfaces state row",
);

const errorFetch = globalThis.fetch;
try {
  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unknown workRoot" }), {
      status: 404,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootFiles("root-local-missing", ""),
    /unknown workRoot/,
    "fetch surfaces bounded backend JSON errors",
  );

  globalThis.fetch = (async () =>
    new Response("not json", { status: 418 })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootFiles("root-local-teapot", "src"),
    /HTTP 418/,
    "fetch falls back to bounded HTTP status when error JSON is unavailable",
  );

  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({
        workRootId: "root-local-abc",
        path: "README.md",
        name: "README.md",
        status: "ok",
        readOnly: true,
        editable: true,
        contentHash: "sha256:hello",
        content: "hello\n",
        sizeBytes: 6,
        languageHint: "markdown",
        extension: "md",
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    )) as typeof fetch;
  const textFile = await fetchWorkRootTextFile("root-local-abc", "README.md");
  assertEqual(textFile.content, "hello\n", "read helper returns text content");

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "unsupported text file" }), {
      status: 400,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootTextFile("root-local-abc", "binary.bin"),
    /unsupported text file/,
    "read helper surfaces bounded backend JSON errors",
  );

  globalThis.fetch = (async () =>
    new Response("not json", { status: 413 })) as typeof fetch;
  await assertRejects(
    () => fetchWorkRootTextFile("root-local-abc", "large.txt"),
    /HTTP 413/,
    "read helper falls back to bounded HTTP status when error JSON is unavailable",
  );

  globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
    assertEqual(init?.method, "POST", "write helper uses POST");
    assertEqual(
      init?.body,
      JSON.stringify({
        path: "README.md",
        baseContentHash: "sha256:old",
        content: "saved\n",
      }),
      "write helper sends optimistic hash and content",
    );
    return new Response(
      JSON.stringify({
        contentHash: "sha256:new",
        sizeBytes: 6,
        savedAtMs: 123,
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
  }) as typeof fetch;
  const writeResponse = await writeWorkRootTextFile("root-local-abc", {
    path: "README.md",
    baseContentHash: "sha256:old",
    content: "saved\n",
  });
  assertDeepEqual(
    writeResponse,
    { contentHash: "sha256:new", sizeBytes: 6, savedAtMs: 123 },
    "write helper returns optimistic save metadata",
  );

  globalThis.fetch = (async () =>
    new Response(JSON.stringify({ error: "file changed on disk" }), {
      status: 409,
      headers: { "content-type": "application/json" },
    })) as typeof fetch;
  await assertRejects(
    () =>
      writeWorkRootTextFile("root-local-abc", {
        path: "README.md",
        baseContentHash: "sha256:stale",
        content: "lost",
      }),
    /file changed on disk/,
    "write helper surfaces optimistic conflict errors",
  );
} finally {
  globalThis.fetch = errorFetch;
}

assertEqual(
  workRootExplorerInitialLoadPath(undefined),
  "",
  "missing selected workRoot snapshot requests initial root listing",
);
assertEqual(
  workRootExplorerInitialLoadPath({
    directories: { "": { status: "loading", entries: [], error: null } },
  }),
  null,
  "loading root listing is not requested again",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand(undefined, "src", false),
  true,
  "expanding an unloaded directory requests that relative path",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand(
    { directories: { src: loaded([]) } },
    "src",
    false,
  ),
  false,
  "expanding a loaded directory does not request it again",
);
assertEqual(
  workRootExplorerShouldLoadOnExpand(undefined, "src", true),
  false,
  "collapsing a directory does not request it",
);
assertDeepEqual(
  workRootExplorerRefreshPaths(new Set(["", "src"])),
  ["", "src"],
  "refresh reloads expanded root and child directories",
);
assertDeepEqual(
  workRootExplorerRefreshPaths(new Set()),
  [""],
  "refresh falls back to root when expansion state is empty",
);

const errorRows = flattenWorkRootFileTree({
  expandedPaths: new Set([""]),
  selectedPath: null,
  directories: {
    "": { status: "error", entries: [], error: "unknown workRoot" },
  },
});
assertEqual(
  errorRows[0]?.type === "state" && errorRows[0].status,
  "error",
  "error snapshot renders an error state row",
);

const loadingPane = createLoadingReadOnlyFilePane(
  "root-local-abc",
  "src/main.rs",
);
assertEqual(loadingPane.status, "loading", "new read-only pane starts loading");
assertEqual(
  loadingPane.title,
  "main.rs",
  "new read-only pane derives basename title",
);
assertEqual(loadingPane.mode, "pinned", "default read-only pane is pinned");
assertEqual(
  createLoadingReadOnlyFilePane("root-local-abc", "src/main.rs", "preview")
    .mode,
  "preview",
  "preview read-only pane mode is explicit",
);
const loadedPane = applyReadOnlyFilePaneContent(loadingPane, {
  workRootId: "root-local-abc",
  path: "src/main.rs",
  name: "main.rs",
  status: "ok",
  readOnly: true,
  editable: true,
  contentHash: "sha256:test",
  content: "fn main() {}\n",
  sizeBytes: 13,
  languageHint: "rust",
  extension: "rs",
});
assertEqual(loadedPane.status, "loaded", "pane content marks pane loaded");
assertEqual(loadedPane.content, "fn main() {}\n", "pane content is preserved");
assertEqual(
  applyReadOnlyFilePaneError(loadingPane, "unsupported text file").error,
  "unsupported text file",
  "pane error remains bounded",
);
