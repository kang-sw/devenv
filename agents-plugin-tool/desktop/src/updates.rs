mod cache;
mod feed;

use std::{
    path::PathBuf,
    sync::{atomic::Ordering, Arc, Mutex},
    time::{Duration, Instant},
};

use serde_json::{json, Value};
use tauri::{async_runtime, AppHandle, Manager, WebviewWindow};
use url::Url;

use crate::{
    close,
    navigation::{destination, Destination},
    shell::Connection,
};

use self::feed::{Action, Feed, Offer, Platform};

const SCRIPT: &str = include_str!("update-bridge.js");
const FIRST_CHECK: Duration = Duration::from_secs(8);
const INTERVAL: Duration = Duration::from_secs(6 * 60 * 60);
const MIN_CHECK_INTERVAL: Duration = Duration::from_secs(2);

pub struct Controller {
    connection: Arc<Connection>,
    app: AppHandle,
    feed: Feed,
    state: Mutex<State>,
    cache: Mutex<Option<cache::Cache>>,
}

struct Selection {
    offer: Arc<Offer>,
    bytes: Option<Arc<Vec<u8>>>,
}

#[derive(Default)]
struct State {
    checked: bool,
    busy: bool,
    last_check: Option<Instant>,
    skipped: Option<String>,
    selected: Option<Selection>,
}

impl State {
    fn phase(&self) -> Value {
        let Some(selected) = &self.selected else {
            return json!({ "phase": if self.checked { "none" } else { "unchecked" } });
        };
        let offer = &selected.offer;
        json!({
            "phase": if selected.bytes.is_some() { "ready" } else { "available" },
            "version": offer.update.version,
            "action": offer.action.label(),
            "skipped": self.skipped.as_deref() == Some(&offer.update.version)
        })
    }
}

#[derive(Debug, PartialEq, Eq)]
enum Command {
    Check(Action),
    Skip,
    Download,
    Restart,
}

impl Command {
    fn parse(message: &str) -> Option<Self> {
        Some(match message {
            "werkschaft:update:check" => Self::Check(Action::Stable),
            "werkschaft:update:canary" => Self::Check(Action::Canary),
            "werkschaft:update:stable-return" => Self::Check(Action::StableReturn),
            "werkschaft:update:skip" => Self::Skip,
            "werkschaft:update:download" => Self::Download,
            "werkschaft:update:restart" => Self::Restart,
            _ => return None,
        })
    }
}

impl Controller {
    pub fn start(app: &AppHandle, connection: Arc<Connection>) {
        let configured = app
            .config()
            .plugins
            .0
            .get("updater")
            .and_then(|value| value["pubkey"].as_str())
            .is_some_and(|key| !key.trim().is_empty());
        let Some(platform) = Platform::installed().filter(|_| configured) else {
            return;
        };
        let Ok(feed) = Feed::new(platform) else {
            eprintln!("Werkschaft: Could not initialize shell updates");
            return;
        };
        let controller = Arc::new(Self {
            connection,
            app: app.clone(),
            feed,
            state: Mutex::new(State {
                busy: true,
                ..State::default()
            }),
            cache: Mutex::new(None),
        });
        app.manage(controller.clone());
        let directory = app
            .path()
            .app_local_data_dir()
            .map(|path| path.join("shell-updates"));
        async_runtime::spawn(async move {
            if let Ok(directory) = directory {
                let restoring = controller.clone();
                let _ = async_runtime::spawn_blocking(move || restoring.restore(directory)).await;
            }
            controller.state.lock().unwrap().busy = false;
            controller.publish(json!({"state": "phase"}));
            tokio::time::sleep(FIRST_CHECK).await;
            loop {
                if controller.begin_check(false) {
                    controller.check(Action::Stable, false).await;
                }
                tokio::time::sleep(INTERVAL).await;
            }
        });
    }

    fn restore(&self, directory: PathBuf) {
        let Ok(cache) = cache::Cache::open(directory) else {
            eprintln!("Werkschaft: Could not open shell update storage");
            return;
        };
        let skipped = cache.skipped().unwrap_or_default();
        let staged = cache::load_header(&cache.stage);
        let staged_exists = !matches!(&staged, Ok(None));
        let restored = staged.ok().flatten().and_then(|(metadata, input)| {
            let offer = self.feed.restore(&self.app, metadata).ok()??;
            let bytes = cache::load_bytes(input, offer.size()).ok()?;
            offer.verify(&bytes).ok()?;
            Some(Selection {
                offer: Arc::new(offer),
                bytes: Some(Arc::new(bytes)),
            })
        });
        if staged_exists && restored.is_none() {
            // This also clears the former stage after the updated app relaunches.
            let _ = std::fs::remove_file(&cache.stage);
        }
        *self.cache.lock().unwrap() = Some(cache);
        let mut state = self.state.lock().unwrap();
        state.skipped = skipped;
        state.selected = restored;
    }

    fn begin_check(&self, manual: bool) -> bool {
        let mut state = self.state.lock().unwrap();
        if state.busy
            || (!manual
                && state
                    .selected
                    .as_ref()
                    .is_some_and(|s| s.bytes.is_some() || s.offer.action != Action::Stable))
            || state
                .last_check
                .is_some_and(|last| last.elapsed() < MIN_CHECK_INTERVAL)
        {
            return false;
        }
        state.last_check = Some(Instant::now());
        state.busy = true;
        true
    }

    async fn check(&self, action: Action, manual: bool) {
        let result = self.feed.check(&self.app, action).await;
        let current = self.app.package_info().version.to_string();
        let event = {
            let mut state = self.state.lock().unwrap();
            state.busy = false;
            match result {
                Ok((Some(offer), _)) => {
                    state.checked = true;
                    let version = offer.update.version.clone();
                    let same_ready = state.selected.as_ref().is_some_and(|s| {
                        s.bytes.is_some()
                            && s.offer.update.version == version
                            && s.offer.action == action
                    });
                    if !same_ready {
                        state.selected = Some(Selection {
                            offer: Arc::new(offer),
                            bytes: None,
                        });
                    }
                    let skipped = state.skipped.as_deref() == Some(&version)
                        && action != Action::StableReturn;
                    json!({
                        "state": if same_ready { "ready" } else if skipped && !manual { "phase" } else { "available" },
                        "version": version, "current": current, "skipped": skipped, "action": action.label()
                    })
                }
                Ok((None, limited)) => {
                    state.checked = true;
                    if state
                        .selected
                        .as_ref()
                        .is_some_and(|s| s.bytes.is_none() && s.offer.action == action)
                    {
                        state.selected = None;
                    }
                    json!({
                        "state": if manual { "current" } else { "phase" },
                        "version": current, "action": action.label(), "limited": limited
                    })
                }
                Err(_) => {
                    json!({ "state": if manual { "error" } else { "phase" }, "action": action.label() })
                }
            }
        };
        self.publish(event);
    }

    fn begin_selection(&self, command: &Command) -> Option<(Arc<Offer>, Option<Arc<Vec<u8>>>)> {
        let mut state = self.state.lock().unwrap();
        let selected = state.selected.as_ref()?;
        let valid = match command {
            Command::Download => selected.bytes.is_none(),
            Command::Restart => selected.bytes.is_some(),
            Command::Skip => {
                selected.bytes.is_none()
                    && selected.offer.action != Action::StableReturn
                    && state.skipped.as_deref() != Some(&selected.offer.update.version)
            }
            _ => false,
        };
        if state.busy || !valid {
            return None;
        }
        let selection = (selected.offer.clone(), selected.bytes.clone());
        state.busy = true;
        Some(selection)
    }

    async fn download(self: &Arc<Self>, offer: Arc<Offer>) {
        let version = &offer.update.version;
        self.publish(json!({"state": "downloading", "version": version}));
        let staged = match self.feed.download(&offer).await {
            Ok(bytes) => {
                let updater = self.clone();
                let selected = offer.clone();
                async_runtime::spawn_blocking(move || {
                    selected.verify(&bytes)?;
                    if let Some(cache) = updater.cache.lock().unwrap().as_ref() {
                        cache::stage(&cache.stage, &selected.metadata, &bytes)
                            .map_err(|_| "Could not save the verified shell update.")?;
                    }
                    Ok(bytes)
                })
                .await
                .unwrap_or(Err("Could not verify the shell update."))
            }
            Err(message) => Err(message),
        };
        let event = {
            let mut state = self.state.lock().unwrap();
            state.busy = false;
            match staged {
                Ok(bytes) => {
                    state
                        .selected
                        .as_mut()
                        .expect("selection stays while busy")
                        .bytes = Some(Arc::new(bytes));
                    json!({"state": "ready", "version": version, "action": offer.action.label()})
                }
                Err(message) => failure("download-error", version, message),
            }
        };
        self.publish(event);
    }

    fn skip(&self, version: &str) {
        if let Some(cache) = self.cache.lock().unwrap().as_ref() {
            if cache.skip(version).is_err() {
                eprintln!("Werkschaft: Could not save the skipped shell update");
            }
        }
        let mut state = self.state.lock().unwrap();
        state.skipped = Some(version.to_owned());
        state.busy = false;
        drop(state);
        self.publish(json!({"state": "phase"}));
    }

    fn install(
        &self,
        offer: &Offer,
        bytes: &[u8],
        window: &WebviewWindow,
        origin: &Url,
        epoch: usize,
    ) {
        let owns_close = self.connection.window_epoch() == epoch
            && self.connection.pinned_url().as_ref() == Some(origin)
            && !self.connection.closing.swap(true, Ordering::AcqRel);
        let message = if !owns_close {
            "The dashboard is closing or has changed. Try again from Settings."
        } else if !close::draft_ready(window, &origin.origin().ascii_serialization()) {
            "Could not save the dashboard draft. Send or discard it, then try again."
        } else if offer.update.install(bytes).is_err() {
            "Could not install the shell update. Retry or reinstall Werkschaft."
        } else {
            // On Windows the updater exits after starting the installer,
            // which relaunches the installed app. macOS has swapped the bundle.
            self.app.request_restart();
            return;
        };
        if owns_close {
            close::release_document(window);
            self.connection.closing.store(false, Ordering::Release);
        }
        self.state.lock().unwrap().busy = false;
        self.publish(failure("apply-error", &offer.update.version, message));
    }

    fn publish(&self, mut event: Value) {
        event["phase"] = self.state.lock().unwrap().phase();
        let Some(pin) = self.connection.pinned_url() else {
            return;
        };
        let epoch = self.connection.window_epoch();
        let connection = self.connection.clone();
        let app = self.app.clone();
        let _ = self.app.run_on_main_thread(move || {
            if connection.window_epoch() != epoch || connection.pinned_url().as_ref() != Some(&pin) {
                return;
            }
            let Some(window) = app.get_webview_window("main") else { return };
            if window.url().is_ok_and(|source| authorized(true, &source, Some(&pin))) {
                let origin = json!(pin.origin().ascii_serialization());
                let _ = window.eval(format!(
                    "if (window === window.top && location.origin === {origin}) window.__werkschaftUpdateStatus?.({event});"
                ));
            }
        });
    }
}

fn failure(state: &str, version: &str, message: &str) -> Value {
    json!({ "state": state, "version": version, "message": message, "downloadPage": feed::release_page(version) })
}

fn authorized(main_frame: bool, source: &Url, pin: Option<&Url>) -> bool {
    main_frame && destination(source, pin) == Destination::Dashboard
}

pub fn inject(window: &WebviewWindow, source: &Url) {
    let Some(controller) = window.try_state::<Arc<Controller>>() else {
        return;
    };
    let pin = controller.connection.pinned_url();
    if !authorized(true, source, pin.as_ref()) {
        return;
    }
    let version = &controller.app.package_info().version;
    let origin = json!(source.origin().ascii_serialization());
    let channel = json!({ "version": version.to_string(), "canary": feed::canary(version) });
    let phase = controller.state.lock().unwrap().phase();
    let _ = window.eval(format!(
        "(() => {{ {SCRIPT}\ninstallWerkschaftUpdateBridge({origin}, {channel}, {phase}); }})()"
    ));
}

pub fn receive(window: WebviewWindow, epoch: usize, main_frame: bool, source: Url, message: &str) {
    let Some(command) = Command::parse(message) else {
        return;
    };
    let Some(controller) = window
        .try_state::<Arc<Controller>>()
        .map(|state| state.inner().clone())
    else {
        return;
    };
    let Some(pin) = controller.connection.pinned_url() else {
        return;
    };
    if epoch != controller.connection.window_epoch()
        || !authorized(main_frame, &source, Some(&pin))
        || controller.connection.closing.load(Ordering::Acquire)
        || !window
            .url()
            .is_ok_and(|url| authorized(true, &url, Some(&pin)))
    {
        return;
    }
    match command {
        Command::Check(action) => {
            if (action == Action::StableReturn
                && !feed::canary(&controller.app.package_info().version))
                || !controller.begin_check(true)
            {
                return;
            }
            async_runtime::spawn(async move {
                controller.check(action, true).await;
            });
        }
        other => {
            let Some((offer, bytes)) = controller.begin_selection(&other) else {
                return;
            };
            match other {
                Command::Download => {
                    async_runtime::spawn(async move {
                        controller.download(offer).await;
                    });
                }
                Command::Skip => {
                    async_runtime::spawn_blocking(move || controller.skip(&offer.update.version));
                }
                Command::Restart => {
                    async_runtime::spawn_blocking(move || {
                        controller.install(
                            &offer,
                            bytes.expect("verified staged bytes").as_slice(),
                            &window,
                            &pin,
                            epoch,
                        );
                    });
                }
                _ => unreachable!("selection command"),
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fixed_commands_and_verified_top_level_origin() {
        let pinned = Url::parse("http://127.0.0.1:52315/").unwrap();
        assert!(authorized(
            true,
            &pinned.join("/sessions").unwrap(),
            Some(&pinned)
        ));
        assert!(!authorized(false, &pinned, Some(&pinned)));
        assert!(!authorized(true, &pinned, None));
        for source in [
            "http://127.0.0.1:52316/",
            "http://localhost:52315/",
            "tauri://localhost/index.html",
            "http://tauri.localhost/index.html",
            "https://example.com/",
            "blob:http://127.0.0.1:52315/object",
        ] {
            assert!(!authorized(
                true,
                &Url::parse(source).unwrap(),
                Some(&pinned)
            ));
        }
        assert_eq!(
            Command::parse("werkschaft:update:download"),
            Some(Command::Download)
        );
        assert_eq!(
            Command::parse("werkschaft:update:download:https://example.com"),
            None
        );
        assert_eq!(Command::parse("werkschaft:update:skip:1.2.3"), None);
    }
}
