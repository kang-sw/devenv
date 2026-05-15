// CONTRACT: Server smoke tests for Phase 1 live here.
//
// Required behavior targets:
// - default serving config binds to `127.0.0.1`.
// - startup info builds a local owner pairing URL after the listener address is
//   known.
// - shutdown hooks can terminate the server without leaving a background task.
