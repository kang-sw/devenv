# Historical schema-2 writer fixture

`source.tar.gz` contains the unmodified non-test Go sources and embedded assets
from `agents-plugin-tool/internal/wswhiteboard/` at
`fee98b018ba9821b2e703ae9f674bb0cfe459841`. `manifest.json` pins every file's
SHA-256 and the archive hash. The compatibility test verifies these before
building a small external driver around the actual historical package.

The driver uses this checkout's compatible internal helper packages and module
cache, offline. It exercises the historical command grammar, typed Entry model,
FolderStore prefix comparison, append publication and opaque intent recovery;
it is not a historical daemon, provider or host-delivery test. No current Entry
type is substituted into the old package. Retaining the source and assets makes
the test independent of Git object availability in a downstream source checkout.

Committed unknown log members and pending append bytes are separate preservation
boundaries. Old typed legacy conversion demonstrably drops unknown legacy
members; neither the fixture nor the feature promises to recover them. The old
close producer is the non-imported, unattributed system line beginning exactly
`Topic closed: `. Imported and ItemID are conventions, not authentication;
authoritative local-file editors can forge them.
