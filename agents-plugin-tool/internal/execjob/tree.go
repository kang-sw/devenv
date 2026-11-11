package execjob

// KillTree kills process pid and every process it started, best effort: on
// Unix the process groups of the tree and each process, on Windows each
// process of the tree. It returns the first error and still tries the
// rest. A command started with its own process group (Setpgid on Unix,
// CREATE_NEW_PROCESS_GROUP on Windows) is ended with all it started.
func KillTree(pid int) error { return cancelProcess(pid) }
