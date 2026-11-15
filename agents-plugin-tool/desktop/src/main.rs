#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

#[cfg(not(target_os = "macos"))]
compile_error!("This first-window slice supports macOS only");

mod endpoint;
mod navigation;

use navigation::{destination, Destination, CONNECT_VIEW};
use std::sync::{
    atomic::{AtomicBool, Ordering},
    Arc, Mutex,
};
use tauri::{
    menu::{Menu, MenuItem, Submenu},
    webview::NewWindowResponse,
    AppHandle, Manager, WebviewUrl, WebviewWindowBuilder,
};
use url::Url;

// The dashboard keys its desktop-only link behavior (a plain click on a file
// link opens its menu, Cmd/Ctrl+click previews) on the "Werkschaft/" token.
// A custom agent replaces WKWebView's default rather than extending it, so
// the macOS default (frozen by WebKit at 10_15_7) stays ahead of the token.
// Only a name, never a credential or native capability.
const USER_AGENT: &str = concat!(
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Werkschaft/",
    env!("CARGO_PKG_VERSION")
);

#[derive(Default)]
struct Connection {
    dashboard: Mutex<Option<Url>>,
    connecting: AtomicBool,
}

fn open_browser(url: Url) {
    // Both callers classify first. No shell command string, custom scheme,
    // credentials, plugin command, or endpoint token reaches the OS opener.
    std::thread::spawn(move || {
        if open::that(url.as_str()).is_err() {
            eprintln!("Could not open the external link in the system browser");
        }
    });
}

fn connect(app: AppHandle, state: Arc<Connection>) {
    if state.connecting.swap(true, Ordering::SeqCst) {
        return;
    }
    if let Some(window) = app.get_webview_window("main") {
        let _ = window.set_title("Werkschaft — Connecting…");
    }
    // One bounded health request, off the UI thread. No executable daemon calls.
    std::thread::spawn(move || {
        let result = endpoint::discover();
        let ui_app = app.clone();
        let ui_state = state.clone();
        if app
            .run_on_main_thread(move || {
                if let Some(window) = ui_app.get_webview_window("main") {
                    let (url, title) = match result {
                        Ok(root) => {
                            eprintln!(
                                "Werkschaft attached to {}",
                                root.origin().ascii_serialization()
                            );
                            *ui_state.dashboard.lock().unwrap() = Some(root.clone());
                            (root, "Werkschaft".to_owned())
                        }
                        Err(message) => {
                            *ui_state.dashboard.lock().unwrap() = None;
                            (
                                Url::parse(CONNECT_VIEW).unwrap(),
                                format!("Werkschaft — {message} — ⌘R to retry"),
                            )
                        }
                    };
                    if window.navigate(url).is_err() {
                        *ui_state.dashboard.lock().unwrap() = None;
                        let _ = window.set_title("Werkschaft — Navigation failed — ⌘R to retry");
                    } else {
                        let _ = window.set_title(&title);
                    }
                }
                ui_state.connecting.store(false, Ordering::SeqCst);
            })
            .is_err()
        {
            state.connecting.store(false, Ordering::SeqCst);
        }
    });
}

fn main() {
    let connection = Arc::new(Connection::default());
    let menu_state = connection.clone();
    tauri::Builder::default()
        // No invoke_handler, plugins, remote capabilities, or JS initialization.
        .on_menu_event(move |app, event| {
            if event.id().as_ref() == "retry" {
                connect(app.clone(), menu_state.clone());
            }
        })
        .setup(move |app| {
            let menu = Menu::default(app.handle())?;
            let retry = MenuItem::with_id(app, "retry", "Retry", true, Some("CmdOrCtrl+R"))?;
            menu.append(&Submenu::with_items(app, "Connection", true, &[&retry])?)?;
            app.set_menu(menu)?;

            let navigation_state = connection.clone();
            let popup_state = connection.clone();
            let popup_app = app.handle().clone();
            WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
                .title("Werkschaft — Connecting…")
                .user_agent(USER_AGENT)
                .inner_size(1280.0, 860.0)
                .min_inner_size(760.0, 520.0)
                .on_navigation(move |url| {
                    let action =
                        destination(url, navigation_state.dashboard.lock().unwrap().as_ref());
                    match action {
                        Destination::Dashboard
                        | Destination::ConnectView
                        | Destination::OpaqueDocument => true,
                        Destination::Browser => {
                            open_browser(url.clone());
                            false
                        }
                        Destination::Block => false,
                    }
                })
                .on_new_window(move |url, _features| {
                    let action = destination(&url, popup_state.dashboard.lock().unwrap().as_ref());
                    match action {
                        Destination::Dashboard => {
                            // Keep same-origin target=_blank/window.open links usable,
                            // without creating a second ungoverned native window.
                            let app = popup_app.clone();
                            let _ = popup_app.run_on_main_thread(move || {
                                if let Some(window) = app.get_webview_window("main") {
                                    let _ = window.navigate(url);
                                }
                            });
                        }
                        Destination::Browser => open_browser(url),
                        Destination::ConnectView
                        | Destination::OpaqueDocument
                        | Destination::Block => (),
                    }
                    NewWindowResponse::Deny
                })
                .build()?;
            connect(app.handle().clone(), connection.clone());
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("Unable to run the Werkschaft window");
}

#[cfg(test)]
mod tests {
    #[test]
    fn user_agent_carries_the_dashboard_desktop_token() {
        // The dashboard tests /\bWerkschaft\// against navigator.userAgent.
        let token = format!(" Werkschaft/{}", env!("CARGO_PKG_VERSION"));
        assert!(super::USER_AGENT.ends_with(&token));
        assert!(super::USER_AGENT.starts_with("Mozilla/5.0 (Macintosh;"));
        assert!(super::USER_AGENT.contains("AppleWebKit/"));
    }

    #[test]
    fn configuration_grants_no_local_or_remote_native_permissions() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.conf.json")).unwrap();
        let app = &config["app"];
        assert_eq!(app["withGlobalTauri"], false);
        assert!(app["windows"].as_array().unwrap().is_empty());
        let capabilities = app["security"]["capabilities"].as_array().unwrap();
        assert_eq!(capabilities.len(), 1);
        let capability = &capabilities[0];
        assert_eq!(capability["local"], false);
        assert!(capability.get("remote").is_none());
        assert!(capability["permissions"].as_array().unwrap().is_empty());
        assert!(config["bundle"].get("externalBin").is_none());
    }
}
