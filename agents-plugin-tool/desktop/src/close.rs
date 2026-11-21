use std::{
    sync::{atomic::Ordering, mpsc, Arc},
    thread,
    time::{Duration, Instant},
};

use tauri::WebviewWindow;
use tauri_plugin_dialog::{
    DialogExt, MessageDialogButtons, MessageDialogKind, MessageDialogResult,
};

use crate::{
    navigation::{destination, Destination},
    shell::Connection,
};

pub fn request(window: WebviewWindow, connection: Arc<Connection>) {
    if connection.closing.swap(true, Ordering::AcqRel) {
        return;
    }
    let pin = connection.pinned_url();
    let current = window.url().ok();
    if current
        .as_ref()
        .is_some_and(|url| destination(url, pin.as_ref()) == Destination::Local)
    {
        let _ = window.destroy();
        return;
    }

    thread::spawn(move || {
        let ready = pin.is_some_and(|pin| {
            current.is_some_and(|url| destination(&url, Some(&pin)) == Destination::Dashboard)
                && draft_ready(&window, &pin.origin().ascii_serialization())
        });
        let target = window.clone();
        if window
            .run_on_main_thread(move || {
                if ready {
                    let _ = target.destroy();
                } else {
                    confirm_close(target, connection);
                }
            })
            .is_err()
        {
            eprintln!("Werkschaft: Could not complete the close request");
        }
    });
}

fn eval(window: &WebviewWindow, script: &str, deadline: Instant) -> Option<String> {
    let (sender, receiver) = mpsc::channel();
    window
        .eval_with_callback(script, move |value| {
            let _ = sender.send(value);
        })
        .ok()?;
    receiver
        .recv_timeout(deadline.saturating_duration_since(Instant::now()))
        .ok()
}

fn draft_ready(window: &WebviewWindow, origin: &str) -> bool {
    // Evaluation runs in the main frame. The IPC surface stays closed to every
    // page, and the renderer only reports whether its existing store is ready.
    let origin = serde_json::to_string(origin).expect("string");
    let start = format!(
        "(() => {{
            if (window !== window.top || location.origin !== {origin} ||
                typeof window.wsNativeCloseReady !== 'function') return -1;
            const attempt = {{ result: 0, wasInert: document.documentElement.inert }};
            window.__werkschaftClose = attempt;
            document.documentElement.inert = true;
            Promise.resolve().then(() => window.wsNativeCloseReady()).then(
                ready => {{ attempt.result = ready === true ? 1 : -1; }},
                () => {{ attempt.result = -1; }}
            );
            return 0;
        }})()"
    );
    let poll = format!(
        "window === window.top && location.origin === {origin} && window.__werkschaftClose ?
            window.__werkschaftClose.result : -1"
    );
    let deadline = Instant::now() + Duration::from_secs(2);
    let mut result = eval(window, &start, deadline);
    while result.as_deref() == Some("0") && Instant::now() < deadline {
        thread::sleep(Duration::from_millis(40));
        result = eval(window, &poll, deadline);
    }
    result.as_deref() == Some("1")
}

fn confirm_close(window: WebviewWindow, connection: Arc<Connection>) {
    window
        .dialog()
        .message("Some changes may not be saved. Close Werkschaft anyway?")
        .title("Close Werkschaft?")
        .kind(MessageDialogKind::Warning)
        .buttons(MessageDialogButtons::OkCancelCustom(
            "Stay".into(),
            "Close".into(),
        ))
        .parent(&window)
        .show_with_result(move |result| {
            if result == MessageDialogResult::Custom("Close".into()) {
                let _ = window.destroy();
            } else {
                let _ = window.eval(
                    "if (window.__werkschaftClose) {
                        document.documentElement.inert = window.__werkschaftClose.wasInert;
                        delete window.__werkschaftClose;
                    }",
                );
                connection.closing.store(false, Ordering::Release);
            }
        });
}
