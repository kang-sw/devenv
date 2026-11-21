#[cfg(target_os = "macos")]
pub fn available() -> bool {
    use std::{
        process::{Command, Stdio},
        thread,
        time::{Duration, Instant},
    };

    let interpreter = std::env::var_os("WS_MCP_HOLDER_PYTHON")
        .filter(|value| !value.is_empty())
        .unwrap_or_else(|| "python3".into());
    let Ok(mut child) = Command::new(interpreter)
        .args(["-c", "import sys"])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
    else {
        return false;
    };

    let deadline = Instant::now() + Duration::from_secs(2);
    while Instant::now() < deadline {
        match child.try_wait() {
            Ok(Some(status)) => return status.success(),
            Ok(None) => thread::sleep(Duration::from_millis(25)),
            Err(_) => break,
        }
    }
    let _ = child.kill();
    let _ = child.wait();
    false
}

#[cfg(target_os = "windows")]
pub fn available() -> bool {
    true
}
