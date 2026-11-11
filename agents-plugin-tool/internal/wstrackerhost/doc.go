// Package wstrackerhost is the machine-side caller of the tracker: it owns
// what wsledger and wstracker leave to their caller (paths, the store
// registry, saved profiles, the git runner, and when to sync) and hands both
// the MCP server and the dashboard daemon one surface over the user's
// stores.
//
// # Layout
//
// Everything lives under one root, Root: the sibling "tracker" directory of
// the global config file, so it follows $WS_CONFIG_HOME like the machine
// note layer (default ~/.ws/tracker).
//
//	<root>/stores.json      the registry: id, label, remote URL, directory, member id, hidden flag, registration nonce
//	<root>/profiles.json    machine-local saved profiles; never written to a ledger
//	<root>/keys/            the personal signing key, passed to wsledger as KeyDir
//	<root>/<store id>/      one store's wsledger directory (its bare repository is repo.git)
//	<root>/<store id>/host-sync.json  the last check's outcome, shared by every process
//	<root>/<store id>/write.lock      held across one write's reload, build, and append
//	<root>/<store id>/net.lock        wsledger's: held by the one call, in any process, that is talking to the store's remote
//	<root>/<store id>/ui-seen.json    what the viewer has seen: badge and read marks, written by the daemon only
//	<root>/poll.lock        held by the process that runs the background poll
//	<root>/.removed-*       a store directory Unregister renamed away and is deleting
//
// All of a user's registered stores are shown in every session; the
// registry is per machine, not per project.
//
// # Sync triggers
//
// A check is a receive-only wsledger Fetch; a sync is the apply path
// (wsledger Sync: close the batch, push, fetch). Own writes are pushed by
// wsledger's idle push delay, by Sync, or when the drawer panel collapses
// (Sync). Checks run:
//
//   - when the process running StartPoll (the dashboard daemon) takes the
//     poll lock, and every PollInterval after, for every registered store;
//   - at lead session start (CheckAll), throttled per store by CheckInterval;
//   - before a tracker read when no process holds the poll lock (PreRead),
//     throttled the same way;
//   - when a store's drawer opens (Check), unthrottled.
//
// An unreachable remote takes the same path as offline: the check is
// recorded as failed and reads serve the local state.
//
// A check or sync keeps no local read or write of its store waiting. One
// call per store talks to the remote at a time, in this process (the
// store's network mutex) and across processes (net.lock); it takes the
// store's lock only for its local steps, the last of which applies what it
// fetched in one piece, so a read sees a fetch whole or not at all. Each
// of those steps reads the registry entry again first: a store
// unregistered while the call was on the network gets nothing applied.
// The calls that talk to the remote are Check, Sync, the checks above, the
// idle push, a read of an attachment this clone lacks, and a wiki save
// (which, unlike the others, keeps the store's lock across its own fetch
// and push).
//
// # Seeing other processes' writes
//
// The process running StartWatch (the dashboard daemon) stamps every
// registered store's journals and branch refs each DefaultWatchInterval and
// calls its change callback when a stamp moves, so a write by any other
// process reaches the page within seconds. The page reads through
// ViewWithin, which serves the last folded state instead of waiting behind
// whatever holds the store's lock (another process's write, a wiki save),
// and calls the same callback when the reload it left behind lands. View,
// the MCP tools' read, waits.
//
// A Host keeps every store it opened open until Close or until the store is
// unregistered, so wsledger's in-process idle push still fires in a process
// that wrote, with or without the daemon. Another process may unregister a
// store this one holds open (Unregister), so every call re-reads the
// store's registry entry before it reuses the open store (lockStore). Every read and write first reloads the store directory
// (wsledger Reload), because the daemon and other sessions' processes share
// it.
package wstrackerhost
