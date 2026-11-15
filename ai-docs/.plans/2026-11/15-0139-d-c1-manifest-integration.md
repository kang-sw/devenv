# D-C1 manifest integration

## Authority and outcome

MAIN 2026-11-14 16:38:32 requests current develop integration into `fix/review794-whiteboard-takeover`, preservation of TASK5 and D-C1 content, normal resource-manifest hash regeneration, and exactly three offline wsrsrc tests. MAIN owns final develop merge and daemon swap after EXEC3's 777 slot.

## Fixed inputs and scope

The lead invoked ws/git.merge in this full checkout; it left the requested merge in progress. Source develop is `1223530580f90942524b613c949fa57bfa7eb335`; target D-C1 is `a4f19e114511feb995893c3061bda9edbd24c4d8`. Resolve only the three resource manifest conflicts under agents-plugin, agents-plugin-wsflow and agents-plugin-pi. Preserve the already-merged resource contents from both histories; do not replace one side wholesale or change workflow semantics. Use repository evidence for the normal generation method. Read all applicable manuals before editing.

## Execution and verification

Inspect Git status and merge diff. Regenerate the resource hashes normally, then run only `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local go test ./internal/wsrsrc -count=1 -run 'Test(ValidateRealTree|WsflowRsrcMirrorUpToDate|PiMirrorUpToDate)$' -v` from agents-plugin-tool. No outer kill or timeout wrapper. Keep complete output and owned-process quiescence evidence. No other test, native build, browser, live daemon or 29417 interaction, installation, network fetch, reset, rebase or branch movement.

Finish the existing merge with an English AI Context explaining preservation and regeneration. Commit only the intended resolution and this plan; do not squash or discard either history. If the workflow commit helper cannot finish the existing merge, use the normal merge-completion Git operation and preserve the required commit message fields. Do not begin another merge or push. Report exact new head, actual command/output paths, content preservation and remaining gaps. Leave release to the lead; stop on broader conflict or verification failure without repair loops.
