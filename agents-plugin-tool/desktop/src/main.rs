#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

#[cfg(not(any(target_os = "macos", target_os = "windows")))]
compile_error!("Werkschaft supports macOS and Windows");

mod close;
mod endpoint;
#[cfg(target_os = "macos")]
mod macos;
mod navigation;
mod python;
mod shell;
#[cfg(windows)]
mod windows;
mod zoom;

use std::sync::{atomic::Ordering, Arc, Mutex};

use tauri::{
    menu::{Menu, MenuItem, PredefinedMenuItem, Submenu},
    webview::NewWindowResponse,
    AppHandle, Manager, RunEvent, WebviewUrl, WebviewWindowBuilder, WindowEvent,
};

use navigation::{destination, Destination};
use shell::Connection;

#[cfg(target_os = "macos")]
const USER_AGENT: &str = concat!(
    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Werkschaft/",
    env!("CARGO_PKG_VERSION")
);

fn build_window(app: &AppHandle, connection: Arc<Connection>) -> tauri::Result<()> {
    let state = connection.clone();
    let popup_app = app.clone();
    let builder = WebviewWindowBuilder::new(app, "main", WebviewUrl::App("index.html".into()))
        .title("Werkschaft — Connecting…")
        .inner_size(1280.0, 860.0)
        .min_inner_size(760.0, 520.0)
        .disable_drag_drop_handler()
        .devtools(true)
        .on_new_window(move |url, _features| {
            let pin = state.pinned_url();
            match destination(&url, pin.as_ref()) {
                Destination::Dashboard => {
                    let app = popup_app.clone();
                    let state = state.clone();
                    std::thread::spawn(move || {
                        let target = app.clone();
                        let _ = app.run_on_main_thread(move || {
                            if state.pinned_url() == pin {
                                if let Some(window) = target.get_webview_window("main") {
                                    let _ = window.navigate(url);
                                }
                            }
                        });
                    });
                }
                Destination::Browser => navigation::open_browser(url),
                Destination::Local | Destination::Block => (),
            }
            NewWindowResponse::Deny
        });

    #[cfg(target_os = "macos")]
    let window = builder.user_agent(USER_AGENT).build()?;

    #[cfg(windows)]
    let window = {
        let state = connection.clone();
        builder
            .on_navigation(
                move |url| match destination(url, state.pinned_url().as_ref()) {
                    Destination::Dashboard | Destination::Local => true,
                    Destination::Browser => {
                        navigation::open_browser(url.clone());
                        false
                    }
                    Destination::Block => false,
                },
            )
            .build()?
    };

    let close_window = window.clone();
    let close_state = connection.clone();
    window.on_window_event(move |event| {
        if let WindowEvent::CloseRequested { api, .. } = event {
            api.prevent_close();
            close::request(close_window.clone(), close_state.clone());
        }
    });

    #[cfg(target_os = "macos")]
    macos::install(&window, connection)?;
    #[cfg(windows)]
    windows::install(&window, connection)?;

    Ok(())
}

fn install_menu(app: &AppHandle) -> tauri::Result<()> {
    let menu = Menu::default(app)?;
    let retry = MenuItem::with_id(app, "retry", "Retry", true, Some("CmdOrCtrl+R"))?;
    let copy = MenuItem::with_id(
        app,
        "copy_login",
        "Copy Browser Login Link to Clipboard",
        true,
        None::<&str>,
    )?;
    menu.append(&Submenu::with_items(
        app,
        "Connection",
        true,
        &[&retry, &copy],
    )?)?;

    #[cfg(target_os = "macos")]
    let (devtools, accelerator) = ("Toggle Developer Tools", "Cmd+Alt+I");
    #[cfg(windows)]
    let (devtools, accelerator) = ("Open Developer Tools", "Ctrl+Shift+I");
    let view = menu.items()?.iter().find_map(|item| {
        item.as_submenu()
            .filter(|submenu| submenu.text().ok().as_deref() == Some("View"))
            .cloned()
    });
    let view = match view {
        Some(view) => view,
        None => {
            let view = Submenu::new(app, "View", true)?;
            menu.append(&view)?;
            view
        }
    };
    view.append_items(&[
        &MenuItem::with_id(app, "devtools", devtools, true, Some(accelerator))?,
        &MenuItem::with_id(app, "devtools_f12", devtools, true, Some("F12"))?,
        &PredefinedMenuItem::separator(app)?,
        &MenuItem::with_id(app, "zoom_reset", "Actual Size", true, Some("CmdOrCtrl+0"))?,
        &MenuItem::with_id(app, "zoom_in", "Zoom In", true, Some("CmdOrCtrl+="))?,
        &MenuItem::with_id(
            app,
            "zoom_in_shifted",
            "Zoom In",
            true,
            Some("CmdOrCtrl+Shift+="),
        )?,
        &MenuItem::with_id(app, "zoom_out", "Zoom Out", true, Some("CmdOrCtrl+-"))?,
    ])?;
    app.set_menu(menu)?;
    Ok(())
}

fn main() {
    let state = Arc::new(Connection::default());
    let menu_state = state.clone();
    let setup_state = state.clone();
    let zoom = Mutex::new(1.0_f64);
    tauri::Builder::default()
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            if let Some(window) = app.get_webview_window("main") {
                let _ = window.show();
                let _ = window.unminimize();
                let _ = window.set_focus();
            }
        }))
        .plugin(tauri_plugin_dialog::init())
        .on_menu_event(move |app, event| {
            let Some(window) = app.get_webview_window("main") else {
                return;
            };
            let id = event.id().as_ref();
            match id {
                "retry" => menu_state.connect(window, true),
                "copy_login" => menu_state.copy_login_link(window),
                "devtools" | "devtools_f12" => {
                    #[cfg(target_os = "macos")]
                    if window.is_devtools_open() {
                        window.close_devtools();
                        return;
                    }
                    window.open_devtools();
                }
                _ => {
                    if let Some(step) = zoom::Step::from_menu_id(id) {
                        let mut level = zoom.lock().unwrap();
                        let target = zoom::next(*level, step);
                        if window.set_zoom(target).is_ok() {
                            *level = target;
                        }
                    }
                }
            }
        })
        .setup(move |app| {
            install_menu(app.handle())?;
            build_window(app.handle(), setup_state.clone())?;
            setup_state.watch(app.handle().clone());
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("Unable to initialize Werkschaft")
        .run(move |_app, event| match event {
            RunEvent::ExitRequested { api, .. } if state.rebuilding.load(Ordering::Acquire) => {
                api.prevent_exit();
            }
            #[cfg(windows)]
            RunEvent::WindowEvent {
                label,
                event: WindowEvent::Destroyed,
                ..
            } if label == "main" && state.rebuilding.load(Ordering::Acquire) => {
                let app = _app.clone();
                let state = state.clone();
                // Defer until Tauri removes the destroyed window from its manager.
                std::thread::spawn(move || {
                    let target = app.clone();
                    let _ = app.run_on_main_thread(move || {
                        state.rebuilding.store(false, Ordering::Release);
                        if build_window(&target, state).is_err() {
                            eprintln!("Werkschaft: Could not recreate the webview");
                            target.exit(1);
                        }
                    });
                });
            }
            _ => (),
        });
}

#[cfg(test)]
mod tests {
    #[test]
    fn pages_receive_no_native_command_permissions() {
        let config: serde_json::Value =
            serde_json::from_str(include_str!("../tauri.conf.json")).unwrap();
        let app = &config["app"];
        assert_eq!(app["withGlobalTauri"], false);
        let capabilities = app["security"]["capabilities"].as_array().unwrap();
        assert_eq!(capabilities.len(), 1);
        assert_eq!(capabilities[0]["local"], false);
        assert!(capabilities[0].get("remote").is_none());
        assert!(capabilities[0]["permissions"]
            .as_array()
            .unwrap()
            .is_empty());
    }
}
