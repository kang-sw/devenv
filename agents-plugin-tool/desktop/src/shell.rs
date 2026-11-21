use std::{
    sync::{
        atomic::{AtomicBool, AtomicUsize, Ordering},
        Arc, Mutex,
    },
    thread,
    time::{Duration, Instant},
};

use tauri::{
    webview::cookie::{time, Cookie, SameSite},
    AppHandle, Manager, WebviewWindow,
};
use url::Url;

use crate::{
    daemon::{self, Intent},
    endpoint::{BrowserCookie, Daemon},
    navigation::{self, destination, Destination, CONNECT_VIEW, PYTHON_VIEW},
};

#[derive(Default)]
pub struct Connection {
    daemon: Mutex<Option<Daemon>>,
    refusal: Mutex<Option<(usize, &'static str)>>,
    next_refusal: AtomicUsize,
    recovery: Mutex<Option<(Instant, u8)>>,
    connecting: AtomicBool,
    copying: AtomicBool,
    hooks: Mutex<Hooks>,
    window_epoch: AtomicUsize,
    pub closing: AtomicBool,
    pub rebuilding: AtomicBool,
}

#[derive(Default, PartialEq, Eq)]
enum Hooks {
    #[default]
    Installing,
    Ready,
    Failed,
}

impl Connection {
    pub fn window_epoch(&self) -> usize {
        self.window_epoch.load(Ordering::Acquire)
    }

    fn daemon(&self) -> Option<Daemon> {
        self.daemon.lock().unwrap().clone()
    }

    pub fn pinned_url(&self) -> Option<Url> {
        self.daemon.lock().unwrap().as_ref().map(|d| d.url.clone())
    }

    fn show_local(&self, window: &WebviewWindow, page: &str, reason: &str) {
        *self.daemon.lock().unwrap() = None;
        *self.refusal.lock().unwrap() = None;
        let _ = window.set_title(&format!("Werkschaft — {reason} — Connection → Retry"));
        if window.navigate(navigation::local_view(page)).is_err() {
            eprintln!("Werkschaft: Could not load the bundled recovery view");
        }
    }

    pub fn setup_failed(&self, window: &WebviewWindow) {
        *self.hooks.lock().unwrap() = Hooks::Failed;
        self.show_local(window, CONNECT_VIEW, "Could not initialize the webview");
    }

    pub fn retry_hooks(&self) -> bool {
        let mut hooks = self.hooks.lock().unwrap();
        if *hooks != Hooks::Failed {
            return false;
        }
        *hooks = Hooks::Installing;
        *self.recovery.lock().unwrap() = None;
        true
    }

    pub fn start(self: &Arc<Self>, window: WebviewWindow) {
        *self.hooks.lock().unwrap() = Hooks::Ready;
        self.connect(window, Intent::Connect);
    }

    pub fn connect(self: &Arc<Self>, window: WebviewWindow, intent: Intent) {
        if *self.hooks.lock().unwrap() != Hooks::Ready {
            return;
        }
        if self.closing.load(Ordering::Acquire) || self.connecting.swap(true, Ordering::AcqRel) {
            return;
        }
        if intent != Intent::Recover {
            *self.recovery.lock().unwrap() = None;
        }
        let retained = self.daemon();
        if retained.is_none() {
            let _ = window.set_title("Werkschaft — Connecting…");
        }
        let epoch = self.window_epoch();
        let state = self.clone();
        thread::spawn(move || {
            let paths = window.app_handle().path();
            let result = daemon::connect(
                paths
                    .resource_dir()
                    .map_err(|_| "Could not locate app resources"),
                paths
                    .app_local_data_dir()
                    .map_err(|_| "Could not locate user data"),
                intent,
            )
            .and_then(|answer| {
                let keep_document =
                    answer.refusal.is_some() && retained.as_ref() == Some(&answer.daemon);
                let cookie = if keep_document {
                    Ok(None)
                } else {
                    answer.daemon.browser_cookie()
                };
                let still_matches = if keep_document {
                    // A failed endpoint read does not evict an already-open
                    // document whose pinned listener still has this identity.
                    answer.daemon.is_healthy()
                } else {
                    answer.daemon.is_current()
                };
                if still_matches {
                    Ok((answer, keep_document, cookie))
                } else {
                    Err("Dashboard changed during connection".into())
                }
            });

            let target = window.clone();
            let ui_state = state.clone();
            if window
                .run_on_main_thread(move || {
                    if ui_state.window_epoch() != epoch {
                        return;
                    }
                    if !ui_state.closing.load(Ordering::Acquire) {
                        match result {
                            Ok((answer, keep_document, cookie)) => {
                                let reason = answer.refusal;
                                *ui_state.refusal.lock().unwrap() = reason.map(|reason| {
                                    (
                                        ui_state.next_refusal.fetch_add(1, Ordering::Relaxed),
                                        reason,
                                    )
                                });
                                if keep_document {
                                    ui_state.show_refusal(&target);
                                } else {
                                    ui_state.attach(&target, answer.daemon, cookie, reason);
                                }
                            }
                            Err(error) => {
                                let page = if error.python_required {
                                    PYTHON_VIEW
                                } else {
                                    CONNECT_VIEW
                                };
                                eprintln!("Werkschaft: {}", error.reason);
                                ui_state.show_local(&target, page, error.reason);
                            }
                        }
                    }
                    ui_state.connecting.store(false, Ordering::Release);
                })
                .is_err()
                && state.window_epoch() == epoch
            {
                state.connecting.store(false, Ordering::Release);
            }
        });
    }

    pub fn show_refusal(self: &Arc<Self>, window: &WebviewWindow) {
        let Some(pin) = self.pinned_url() else { return };
        let Some((sequence, reason)) = *self.refusal.lock().unwrap() else {
            return;
        };
        let _ = window.set_title(&format!("Werkschaft — {reason}"));
        if !window
            .url()
            .is_ok_and(|url| destination(&url, Some(&pin)) == Destination::Dashboard)
        {
            return;
        }
        let epoch = self.window_epoch();
        let script = refusal_script(&pin, reason);
        let state = self.clone();
        let target = window.clone();
        let _ = window.eval_with_callback(script, move |value| {
            if value != "1" {
                return;
            }
            let result = target.clone();
            let state = state.clone();
            let pin = pin.clone();
            let _ = target.run_on_main_thread(move || {
                if state.window_epoch() != epoch || state.pinned_url() != Some(pin) {
                    return;
                }
                let mut refusal = state.refusal.lock().unwrap();
                if *refusal == Some((sequence, reason)) {
                    *refusal = None;
                    let _ = result.set_title("Werkschaft");
                }
            });
        });
    }

    fn attach(
        &self,
        window: &WebviewWindow,
        daemon: Daemon,
        cookie: Result<Option<BrowserCookie>, &'static str>,
        refusal: Option<&'static str>,
    ) {
        let root = &daemon.url;
        let target = window
            .url()
            .ok()
            .filter(|current| destination(current, Some(root)) == Destination::Dashboard)
            .unwrap_or_else(|| root.clone());
        let cookie_ready = match cookie {
            Ok(Some(cookie)) => {
                let cookie = Cookie::build((cookie.name, cookie.value))
                    .domain("127.0.0.1")
                    .path("/")
                    .http_only(true)
                    .same_site(SameSite::Strict)
                    .expires(
                        time::OffsetDateTime::now_utc()
                            + time::Duration::seconds(cookie.max_age as i64),
                    )
                    .build();
                window.set_cookie(cookie).is_ok()
            }
            Ok(None) | Err(_) => false,
        };
        if !cookie_ready {
            eprintln!(
                "Werkschaft: Browser cookie unavailable; loading the daemon login gate if needed"
            );
        }
        if self.closing.load(Ordering::Acquire) {
            return;
        }
        *self.daemon.lock().unwrap() = Some(daemon);
        let title = refusal.map_or_else(
            || "Werkschaft".to_owned(),
            |reason| format!("Werkschaft — {reason}"),
        );
        let _ = window.set_title(&title);
        if window.navigate(target).is_err() {
            self.show_local(window, CONNECT_VIEW, "Could not open the dashboard");
        }
    }

    // A bad renderer or a healthy listener that fails document loads can
    // repeatedly fail across successful health checks. Bound the whole burst.
    fn allow_recovery(&self, now: Instant) -> bool {
        let mut recovery = self.recovery.lock().unwrap();
        let (start, count) = recovery.get_or_insert((now, 0));
        if now.duration_since(*start) >= Duration::from_secs(30) {
            (*start, *count) = (now, 0);
        }
        if *count >= 2 {
            return false;
        }
        *count += 1;
        true
    }

    pub fn recover(self: &Arc<Self>, window: WebviewWindow, reason: &'static str) {
        if self.closing.load(Ordering::Acquire) || self.connecting.load(Ordering::Acquire) {
            return;
        }
        eprintln!("Werkschaft: {reason}");
        if self.allow_recovery(Instant::now()) {
            self.connect(window, Intent::Recover);
        } else {
            self.show_local(&window, CONNECT_VIEW, "Lost connection to the dashboard");
        }
    }

    #[cfg(windows)]
    pub fn webview_failed(self: &Arc<Self>, window: WebviewWindow) {
        if self.closing.load(Ordering::Acquire) || self.rebuilding.swap(true, Ordering::AcqRel) {
            return;
        }
        if self.allow_recovery(Instant::now()) {
            // A dead WebView2 browser process invalidates its controllers.
            // RunEvent keeps the application alive until the replacement exists.
            *self.daemon.lock().unwrap() = None;
            *self.refusal.lock().unwrap() = None;
            *self.hooks.lock().unwrap() = Hooks::Installing;
            self.window_epoch.fetch_add(1, Ordering::AcqRel);
            self.connecting.store(false, Ordering::Release);
            if window.destroy().is_err() {
                self.rebuilding.store(false, Ordering::Release);
                self.setup_failed(&window);
            }
        } else {
            self.rebuilding.store(false, Ordering::Release);
            self.setup_failed(&window);
        }
    }

    pub fn watch(self: &Arc<Self>, app: AppHandle) {
        let state = Arc::downgrade(self);
        thread::spawn(move || loop {
            thread::sleep(Duration::from_secs(8));
            let Some(state) = state.upgrade() else { break };
            if state.connecting.load(Ordering::Acquire) || state.closing.load(Ordering::Acquire) {
                continue;
            }
            let Some(daemon) = state.daemon() else {
                continue;
            };
            if daemon.is_healthy() {
                continue;
            }
            let handle = app.clone();
            let _ = app.run_on_main_thread(move || {
                if state.daemon().as_ref() == Some(&daemon) {
                    if let Some(window) = handle.get_webview_window("main") {
                        state.recover(window, "Dashboard stopped responding");
                    }
                }
            });
        });
    }

    pub fn copy_login_link(self: &Arc<Self>, window: WebviewWindow) {
        let Some(daemon) = self.daemon() else { return };
        if self.connecting.load(Ordering::Acquire) || self.copying.swap(true, Ordering::AcqRel) {
            return;
        }
        let current = window
            .url()
            .ok()
            .filter(|url| destination(url, Some(&daemon.url)) == Destination::Dashboard);
        let path = current.map_or_else(
            || "/".to_owned(),
            |mut url| {
                // A consumed login code is never part of the next destination.
                if url.query_pairs().any(|(key, _)| key == "login") {
                    let query: Vec<_> = url
                        .query_pairs()
                        .into_owned()
                        .filter(|(key, _)| key != "login")
                        .collect();
                    url.set_query(None);
                    if !query.is_empty() {
                        url.query_pairs_mut().extend_pairs(query);
                    }
                }
                url[url::Position::BeforePath..].to_owned()
            },
        );
        let state = self.clone();
        thread::spawn(move || {
            let link = daemon.browser_login_link(&path).ok().flatten();
            let target = window.clone();
            let ui_state = state.clone();
            if window
                .run_on_main_thread(move || {
                    if !ui_state.connecting.load(Ordering::Acquire)
                        && !ui_state.closing.load(Ordering::Acquire)
                        && ui_state.daemon().as_ref() == Some(&daemon)
                    {
                        let copied = link.is_some_and(|link| {
                            arboard::Clipboard::new()
                                .and_then(|mut clipboard| clipboard.set_text(link.as_str()))
                                .is_ok()
                        });
                        let status = if copied {
                            "Browser login link copied"
                        } else {
                            "Could not copy browser login link"
                        };
                        let _ = target.set_title(&format!("Werkschaft — {status}"));
                    }
                    ui_state.copying.store(false, Ordering::Release);
                })
                .is_err()
            {
                state.copying.store(false, Ordering::Release);
            }
        });
    }
}

fn refusal_script(pin: &Url, reason: &str) -> String {
    let origin = serde_json::to_string(&pin.origin().ascii_serialization()).expect("string");
    let reason = serde_json::to_string(reason).expect("string");
    format!("({})({origin}, {reason})", include_str!("daemon-notice.js"))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn automatic_recovery_is_bounded_across_reconnections() {
        let connection = Connection::default();
        let now = Instant::now();
        assert!(connection.allow_recovery(now));
        assert!(connection.allow_recovery(now + Duration::from_secs(1)));
        assert!(!connection.allow_recovery(now + Duration::from_secs(29)));
        assert!(connection.allow_recovery(now + Duration::from_secs(30)));
    }

    #[test]
    fn failed_hooks_can_be_retried_without_duplicate_installation() {
        let connection = Connection::default();
        assert!(!connection.retry_hooks());
        *connection.hooks.lock().unwrap() = Hooks::Failed;
        assert!(connection.retry_hooks());
        assert!(!connection.retry_hooks());
        *connection.hooks.lock().unwrap() = Hooks::Ready;
        assert!(!connection.retry_hooks());
    }
}
