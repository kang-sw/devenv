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
//	<root>/stores.json      the registry: id, label, remote URL, directory, member id
//	<root>/profiles.json    machine-local saved profiles; never written to a ledger
//	<root>/keys/            the personal signing key, passed to wsledger as KeyDir
//	<root>/<store id>/      one store's wsledger directory (its bare repository is repo.git)
//	<root>/<store id>/host-sync.json  the last check's outcome, shared by every process
//	<root>/poll.lock        held by the process that runs the background poll
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
//   - every PollInterval in the process running StartPoll (the dashboard
//     daemon), for every registered store;
//   - at lead session start (CheckAll), throttled per store by CheckInterval;
//   - before a tracker read when no process holds the poll lock (PreRead),
//     throttled the same way;
//   - when a store's drawer opens (Check), unthrottled.
//
// An unreachable remote takes the same path as offline: the check is
// recorded as failed and reads serve the local state.
//
// A Host keeps every store it opened open until Close, so wsledger's
// in-process idle push still fires in a process that wrote, with or without
// the daemon. Every read and write first reloads the store directory
// (wsledger Reload), because the daemon and other sessions' processes share
// it.
package wstrackerhost
