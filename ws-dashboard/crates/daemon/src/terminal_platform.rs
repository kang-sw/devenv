// Platform-specific detach-spawn / identity-verify / kill-through-handle
// operations, extending `terminal::TerminalPlatform`'s existing cfg-gated-
// module-behind-an-enum shape (see `terminal.rs`'s `default_shell`, which
// already dispatches Unix vs. Windows behind `#[cfg(not(windows))]`/
// `#[cfg(windows)]`). Every syscall-touching leaf lives in a `unix`/
// `windows` submodule; the daemon's cfg-independent call sites (`terminal.rs`
// spawn/kill paths, boot reconcile) call the top-level re-exported names.
//
// Identity model (ticket-pinned): PID + OS-reported start-time is the ONLY
// kill precondition. A nonce/token is explicitly rejected as a kill-gate
// because it is unverifiable once IPC is dead - exactly the moment the kill
// decision has to be made. `verify_process_identity` re-derives start-time
// from the OS and compares against the registry-recorded value; `kill_
// verified` captures a *stable* OS handle (Linux pidfd / Windows process
// handle) at verification time and signals through that handle rather than
// re-resolving the PID, closing the verify-to-kill TOCTOU window a plain
// "check PID, then `kill(pid)`" sequence would leave open.

use std::io;

#[cfg(unix)]
pub mod unix {
    use super::*;
    use std::os::unix::process::CommandExt;
    use std::process::Command;

    /// Detach `command` from the daemon via `setsid()` + a double fork, so
    /// the process `Command::spawn()` hands back is a short-lived middle
    /// process, never the long-lived helper (the daemon must not trust
    /// `Child::id()` as final identity - the helper reports its own real PID
    /// + start-time over the IPC handshake instead). The middle process is
    /// reaped (`wait()`) before returning so it never becomes a zombie.
    ///
    /// # Safety of the `pre_exec` closure
    /// `pre_exec` runs in the forked child between `fork()` and `exec()`,
    /// where only async-signal-safe calls are sound. `setsid()`, `fork()`,
    /// and `_exit()` are async-signal-safe; nothing else runs in this
    /// closure.
    pub fn spawn_detached(mut command: Command) -> io::Result<()> {
        unsafe {
            command.pre_exec(|| {
                if libc::setsid() == -1 {
                    return Err(io::Error::last_os_error());
                }
                match libc::fork() {
                    -1 => Err(io::Error::last_os_error()),
                    0 => Ok(()),
                    _ => libc::_exit(0),
                }
            });
        }
        let mut middle = command.spawn()?;
        let _ = middle.wait();
        Ok(())
    }

    /// Process start time in clock ticks since boot, read from
    /// `/proc/<pid>/stat` field 22 (`starttime`). `None` if the process does
    /// not exist or `/proc` is unreadable. The process name field (`comm`)
    /// may itself contain spaces or parentheses, so this splits on the
    /// *last* `)` the same way `ps`/`procps` do, rather than the first.
    pub fn process_start_time(pid: u32) -> Option<u64> {
        let stat = std::fs::read_to_string(format!("/proc/{pid}/stat")).ok()?;
        let after_comm = stat.rsplit_once(')')?.1;
        after_comm
            .split_whitespace()
            .nth(19) // fields after `)`: state(0) ppid(1) ... starttime is index 19
            .and_then(|value| value.parse().ok())
    }

    pub fn verify_process_identity(pid: u32, expected_start_time: u64) -> bool {
        process_start_time(pid) == Some(expected_start_time)
    }

    /// Verified kill: opens a `pidfd` for `pid`, re-checks identity (closing
    /// the TOCTOU window between an earlier check and this call), then
    /// signals SIGKILL through the pidfd rather than by re-resolving the raw
    /// PID. Returns `Ok(true)` if a verified kill was delivered, `Ok(false)`
    /// if the process was already gone or identity did not verify (the
    /// caller must treat `Ok(false)` as "do not report success", not as an
    /// error - an already-dead process is the common, expected case).
    pub fn kill_verified(pid: u32, expected_start_time: u64) -> io::Result<bool> {
        let pidfd = unsafe { libc::syscall(libc::SYS_pidfd_open, pid as libc::pid_t, 0) };
        if pidfd < 0 {
            // ESRCH (no such process) is the overwhelmingly common case here
            // (the process already exited); treat any pidfd_open failure as
            // "nothing to kill" rather than propagating an I/O error for
            // what is really just a race against process exit.
            return Ok(false);
        }
        let pidfd = pidfd as i32;
        let verified = verify_process_identity(pid, expected_start_time);
        let killed = if verified {
            let rc = unsafe {
                libc::syscall(
                    libc::SYS_pidfd_send_signal,
                    pidfd,
                    libc::SIGKILL,
                    std::ptr::null::<()>(),
                    0,
                )
            };
            rc == 0
        } else {
            false
        };
        unsafe {
            libc::close(pidfd);
        }
        Ok(killed)
    }
}

#[cfg(windows)]
pub mod windows {
    use super::*;
    use std::os::windows::io::{AsRawHandle, FromRawHandle, OwnedHandle, RawHandle};
    use std::os::windows::process::CommandExt;
    use std::process::Command;
    use windows_sys::Win32::Foundation::{
        CloseHandle, SetHandleInformation, FALSE, HANDLE, HANDLE_FLAG_INHERIT,
    };
    use windows_sys::Win32::System::JobObjects::{
        AssignProcessToJobObject, CreateJobObjectW, JobObjectExtendedLimitInformation,
        SetInformationJobObject, JOBOBJECT_EXTENDED_LIMIT_INFORMATION,
        JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
    };
    use windows_sys::Win32::System::Threading::{
        GetCurrentProcess, GetProcessTimes, OpenProcess, TerminateProcess,
        CREATE_BREAKAWAY_FROM_JOB, CREATE_NEW_PROCESS_GROUP, CREATE_NO_WINDOW,
        PROCESS_QUERY_LIMITED_INFORMATION, PROCESS_TERMINATE,
    };

    /// Daemon-side counterpart to `unix::spawn_detached` - same call-site
    /// contract (`terminal.rs`'s `TerminalSession::spawn` calls this
    /// uniformly across platforms via `crate::terminal_platform::
    /// spawn_detached`), different mechanism. Unix needs `setsid()` + a
    /// double fork because a child normally dies with its parent's session;
    /// Windows children already outlive a dead parent by default, so the
    /// only thing this needs to defeat is the daemon *itself* possibly being
    /// inside a Job Object (common under service managers/IDEs) that would
    /// otherwise take the helper down with it - `CREATE_BREAKAWAY_FROM_JOB`
    /// does that. `CREATE_NEW_PROCESS_GROUP` detaches the helper from the
    /// daemon's console Ctrl+C signal group (the Windows analogue of Unix's
    /// `setsid()` detaching from the controlling terminal), and
    /// `CREATE_NO_WINDOW` keeps the headless IPC-only helper from flashing a
    /// console window. The returned `Child` is deliberately dropped without
    /// waiting - like the Unix leg, the daemon never trusts this as the
    /// helper's real identity; that comes from the IPC handshake instead.
    pub fn spawn_detached(mut command: Command) -> io::Result<()> {
        command.creation_flags(CREATE_BREAKAWAY_FROM_JOB | CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW);
        let child = command.spawn()?;
        drop(child);
        Ok(())
    }

    /// Marks a socket/handle non-inheritable so children spawned later with
    /// `bInheritHandles=TRUE` (which is what `std::process::Command` always does
    /// on Windows) do NOT inherit it. The daemon's listening TCP socket must be
    /// marked this way right after bind: otherwise every detached terminal-helper
    /// spawned afterwards inherits the listen socket and keeps the port bound
    /// after the daemon exits, so a same-port daemon restart fails with
    /// WSAEADDRINUSE (`os error 10048`). Unix needs no equivalent because file
    /// descriptors are CLOEXEC by default there.
    pub fn mark_socket_non_inheritable<S: std::os::windows::io::AsRawSocket>(
        socket: &S,
    ) -> io::Result<()> {
        let handle = socket.as_raw_socket() as HANDLE;
        let ok = unsafe { SetHandleInformation(handle, HANDLE_FLAG_INHERIT, 0) };
        if ok == FALSE {
            return Err(io::Error::last_os_error());
        }
        Ok(())
    }

    /// Creates a Job Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and
    /// assigns the *current* process (the helper itself) to it. Call once
    /// from inside the helper process before spawning the shell child. The
    /// returned handle must be kept alive for the helper's lifetime -
    /// dropping/closing it, or the helper process exiting, tears down every
    /// process still assigned to the job (this is what makes killing just
    /// the helper reliably take its shell subtree with it).
    pub fn create_kill_on_close_job() -> io::Result<OwnedHandle> {
        let job = unsafe { CreateJobObjectW(std::ptr::null(), std::ptr::null()) };
        if job.is_null() {
            return Err(io::Error::last_os_error());
        }
        let mut info: JOBOBJECT_EXTENDED_LIMIT_INFORMATION = unsafe { std::mem::zeroed() };
        info.BasicLimitInformation.LimitFlags = JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE;
        let ok = unsafe {
            SetInformationJobObject(
                job,
                JobObjectExtendedLimitInformation,
                std::ptr::addr_of!(info).cast(),
                std::mem::size_of::<JOBOBJECT_EXTENDED_LIMIT_INFORMATION>() as u32,
            )
        };
        if ok == FALSE {
            let error = io::Error::last_os_error();
            unsafe {
                CloseHandle(job);
            }
            return Err(error);
        }
        let ok = unsafe { AssignProcessToJobObject(job, GetCurrentProcess()) };
        if ok == FALSE {
            let error = io::Error::last_os_error();
            unsafe {
                CloseHandle(job);
            }
            return Err(error);
        }
        Ok(unsafe { OwnedHandle::from_raw_handle(job as *mut _) })
    }

    /// Assigns an already-spawned process into `job` (the helper's own
    /// kill-on-close job) via its raw handle, so that killing/closing the
    /// helper's Job Object handle reliably takes that process down too.
    ///
    /// CONTRACT (260723 Phase-1 review finding I2): this takes a raw handle
    /// rather than spawning a `std::process::Command` itself (the original
    /// shape this primitive was sketched with in the plan) because the
    /// helper spawns its PTY-owned shell through `portable_pty::SlavePty::
    /// spawn_command` (see `terminal_helper_process.rs::spawn_shell`), not
    /// a bare `std::process::Command` - `portable_pty`'s Windows (ConPTY)
    /// backend does the actual `CreateProcessW` call internally and only
    /// exposes the resulting child via `portable_pty::Child::as_raw_handle`.
    /// Assigning post-spawn is the integration point that actually fits;
    /// `CREATE_BREAKAWAY_FROM_JOB` is not applicable here since
    /// `portable_pty::CommandBuilder` has no creation-flags hook to carry it
    /// (breakaway at daemon->helper spawn time is unrelated and already
    /// handled by `spawn_detached`, above).
    ///
    /// KNOWN SIMPLIFICATION: there is a narrow window between the shell's
    /// spawn and this assignment call during which a pathologically fast
    /// child could itself spawn a grandchild that escapes the job.
    /// `portable_pty` exposes neither the suspended-create + `ResumeThread`
    /// bracket nor `PROC_THREAD_ATTRIBUTE_JOB_LIST` needed to close this
    /// window without bypassing `portable_pty`'s `CreateProcessW` call
    /// entirely. This Stage-2 leg (cross-compile-checked only, no live
    /// Windows E2E in this session per Decision B) accepts that window;
    /// revisit if it matters in practice.
    pub fn assign_into_job(job: &OwnedHandle, handle: RawHandle) -> io::Result<()> {
        let ok = unsafe { AssignProcessToJobObject(job.as_raw_handle() as HANDLE, handle as HANDLE) };
        if ok == FALSE {
            return Err(io::Error::last_os_error());
        }
        Ok(())
    }

    pub fn process_start_time(pid: u32) -> Option<u64> {
        let handle = unsafe { OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, FALSE, pid) };
        if handle.is_null() {
            return None;
        }
        let mut creation = 0u64;
        let mut exit = 0u64;
        let mut kernel = 0u64;
        let mut user = 0u64;
        let ok = unsafe {
            GetProcessTimes(
                handle,
                std::ptr::addr_of_mut!(creation).cast(),
                std::ptr::addr_of_mut!(exit).cast(),
                std::ptr::addr_of_mut!(kernel).cast(),
                std::ptr::addr_of_mut!(user).cast(),
            )
        };
        unsafe {
            CloseHandle(handle);
        }
        if ok == FALSE {
            return None;
        }
        Some(creation)
    }

    pub fn verify_process_identity(pid: u32, expected_start_time: u64) -> bool {
        process_start_time(pid) == Some(expected_start_time)
    }

    /// Verified kill: opens a stable process handle for `pid`, re-checks
    /// identity through that same handle's reported creation time (closing
    /// the TOCTOU window the same way the Unix pidfd leg does), then
    /// `TerminateProcess`s through the handle.
    pub fn kill_verified(pid: u32, expected_start_time: u64) -> io::Result<bool> {
        let handle = unsafe {
            OpenProcess(
                PROCESS_TERMINATE | PROCESS_QUERY_LIMITED_INFORMATION,
                FALSE,
                pid,
            )
        };
        if handle.is_null() {
            return Ok(false);
        }
        let verified = verify_process_identity(pid, expected_start_time);
        let killed = if verified {
            unsafe { TerminateProcess(handle, 1) != FALSE }
        } else {
            false
        };
        unsafe {
            CloseHandle(handle);
        }
        Ok(killed)
    }
}

#[cfg(unix)]
pub use unix::{kill_verified, process_start_time, spawn_detached, verify_process_identity};
#[cfg(windows)]
pub use windows::{kill_verified, process_start_time, spawn_detached, verify_process_identity};

#[cfg(all(test, unix))]
mod unix_tests {
    use super::unix::*;
    use std::process::Command;

    #[test]
    fn process_start_time_is_stable_for_the_same_live_process() {
        let pid = std::process::id();
        let first = process_start_time(pid).expect("start time for self");
        let second = process_start_time(pid).expect("start time for self again");
        assert_eq!(first, second);
    }

    #[test]
    fn process_start_time_is_none_for_an_implausible_pid() {
        // PID 1 always exists (init/systemd) with a start time far in the
        // past; instead probe a PID that is exceedingly unlikely to be
        // assigned inside a short-lived test run: reap a child immediately
        // so its PID is available for reuse detection tests, then assume - a
        // few iterations out - that no other process claimed it during the
        // assertion window.
        let mut child = Command::new("true")
            .spawn()
            .or_else(|_| Command::new("/bin/true").spawn())
            .expect("spawn short-lived child");
        let pid = child.id();
        child.wait().expect("reap short-lived child");
        // Give the OS a moment to finish teardown bookkeeping; a small sleep
        // is acceptable here since this asserts an *absence*, not a race-
        // prone presence.
        std::thread::sleep(std::time::Duration::from_millis(50));
        assert!(process_start_time(pid).is_none() || verify_process_identity(pid, 0));
    }

    #[test]
    fn kill_verified_refuses_to_kill_on_start_time_mismatch() {
        let mut child = Command::new("sleep")
            .arg("5")
            .spawn()
            .expect("spawn long-lived child");
        let pid = child.id();
        let real_start_time = process_start_time(pid).expect("start time for live child");
        let bogus_start_time = real_start_time.wrapping_add(999_999);

        let killed = kill_verified(pid, bogus_start_time).expect("kill_verified call succeeds");
        assert!(!killed, "mismatched identity must never be killed");
        assert!(
            child.try_wait().expect("try_wait after refused kill").is_none(),
            "child must still be alive after a refused kill"
        );

        let _ = child.kill();
        let _ = child.wait();
    }

    #[test]
    fn kill_verified_kills_on_matching_identity() {
        let mut child = Command::new("sleep")
            .arg("5")
            .spawn()
            .expect("spawn long-lived child");
        let pid = child.id();
        let real_start_time = process_start_time(pid).expect("start time for live child");

        let killed = kill_verified(pid, real_start_time).expect("kill_verified call succeeds");
        assert!(killed, "matching identity must be killed");

        let status = child.wait().expect("reap killed child");
        assert!(!status.success());
    }
}
