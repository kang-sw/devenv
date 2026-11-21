use std::sync::Arc;

use tauri::{Manager, WebviewWindow};
use webview2_com::{
    take_pwstr, AcceleratorKeyPressedEventHandler, Microsoft::Web::WebView2::Win32::*,
    NavigationCompletedEventHandler, NavigationStartingEventHandler,
    PermissionRequestedEventHandler, ProcessFailedEventHandler, WebMessageReceivedEventHandler,
};
use windows::{
    core::{Interface, BOOL, HSTRING, PWSTR},
    Win32::UI::Input::KeyboardAndMouse::{
        GetKeyState, VIRTUAL_KEY, VK_0, VK_ADD, VK_CONTROL, VK_F12, VK_I, VK_LWIN, VK_MENU,
        VK_OEM_MINUS, VK_OEM_PLUS, VK_R, VK_RWIN, VK_SHIFT, VK_SUBTRACT,
    },
};

use crate::{accelerator, menu_action, navigation, shell::Connection, updates};

pub fn install(window: &WebviewWindow, connection: Arc<Connection>) -> tauri::Result<()> {
    let window = window.clone();
    window.clone().with_webview(move |platform| {
        let result = unsafe { install_hooks(&platform.controller(), &window, &connection) };
        match result {
            Ok(()) => queue(&window, move |target| connection.start(target)),
            Err(_) => {
                eprintln!("Could not install WebView2 hooks");
                queue(&window, move |target| connection.setup_failed(&target));
            }
        }
    })
}

// Tauri runs main-thread dispatch inline on the UI thread. Dispatch from a
// worker so a WebView2 event has returned before shell code can rebuild it.
fn queue(window: &WebviewWindow, action: impl FnOnce(WebviewWindow) + Send + 'static) {
    let window = window.clone();
    std::thread::spawn(move || {
        let target = window.clone();
        if window.run_on_main_thread(move || action(target)).is_err() {
            eprintln!("Could not queue WebView2 shell action");
        }
    });
}

unsafe fn install_hooks(
    controller: &ICoreWebView2Controller,
    window: &WebviewWindow,
    connection: &Arc<Connection>,
) -> windows::core::Result<()> {
    let webview = controller.CoreWebView2()?;
    let mut token = 0;
    let frames = connection.clone();
    webview.add_FrameNavigationStarting(
        &NavigationStartingEventHandler::create(Box::new(move |_, args| {
            if let Some(args) = args {
                // Cancel first so malformed URLs and failed property reads fail closed.
                args.SetCancel(true)?;
                let mut raw = PWSTR::null();
                args.Uri(&mut raw)?;
                let uri = take_pwstr(raw);
                if url::Url::parse(&uri).ok().is_some_and(|url| {
                    navigation::frame_allowed(&url, frames.pinned_url().as_ref())
                }) {
                    args.SetCancel(false)?;
                }
            }
            Ok(())
        })),
        &mut token,
    )?;

    let permissions = connection.clone();
    webview.add_PermissionRequested(
        &PermissionRequestedEventHandler::create(Box::new(move |_, args| {
            if let Some(args) = args {
                let mut kind = COREWEBVIEW2_PERMISSION_KIND::default();
                args.PermissionKind(&mut kind)?;
                if kind == COREWEBVIEW2_PERMISSION_KIND_CLIPBOARD_READ {
                    let transient = args
                        .cast::<ICoreWebView2PermissionRequestedEventArgs3>()
                        .ok();
                    if let Some(args3) = &transient {
                        args3.SetSavesInProfile(false)?;
                    }
                    args.SetState(COREWEBVIEW2_PERMISSION_STATE_DENY)?;
                    let mut initiated = BOOL::default();
                    args.IsUserInitiated(&mut initiated)?;
                    let mut raw = PWSTR::null();
                    args.Uri(&mut raw)?;
                    let uri = take_pwstr(raw);
                    let allowed = url::Url::parse(&uri).ok().is_some_and(|url| {
                        navigation::destination(&url, permissions.pinned_url().as_ref())
                            == navigation::Destination::Dashboard
                    });
                    // A saved grant would bypass future user-gesture and pin checks.
                    if initiated.as_bool() && allowed {
                        if transient.is_some() {
                            args.SetState(COREWEBVIEW2_PERMISSION_STATE_ALLOW)?;
                        }
                    }
                }
            }
            Ok(())
        })),
        &mut token,
    )?;

    let failed = connection.clone();
    let epoch = failed.window_epoch();
    if window.try_state::<Arc<updates::Controller>>().is_some() {
        let target = window.clone();
        // The core event receives the top-level document only. Frame messages
        // have a separate WebView2 event, which the shell does not register.
        webview.add_WebMessageReceived(
            &WebMessageReceivedEventHandler::create(Box::new(move |_, args| {
                if let Some(args) = args {
                    let mut raw = PWSTR::null();
                    if args.TryGetWebMessageAsString(&mut raw).is_err() {
                        return Ok(());
                    }
                    let message = take_pwstr(raw);
                    if !message.starts_with("werkschaft:update:") || message.len() > 32 {
                        return Ok(());
                    }
                    let mut raw = PWSTR::null();
                    args.Source(&mut raw)?;
                    if let Ok(source) = url::Url::parse(&take_pwstr(raw)) {
                        updates::receive(target.clone(), epoch, true, source, &message);
                    }
                }
                Ok(())
            })),
            &mut token,
        )?;
    }
    let failed_window = window.clone();
    webview.add_NavigationCompleted(
        &NavigationCompletedEventHandler::create(Box::new(move |_, args| {
            if let Some(args) = args {
                let mut success = BOOL::default();
                args.IsSuccess(&mut success)?;
                let mut status = COREWEBVIEW2_WEB_ERROR_STATUS::default();
                args.WebErrorStatus(&mut status)?;
                if !success.as_bool()
                    && status != COREWEBVIEW2_WEB_ERROR_STATUS_OPERATION_CANCELED
                    && failed.pinned_url().is_some()
                {
                    let connection = failed.clone();
                    queue(&failed_window, move |target| {
                        if connection.window_epoch() == epoch {
                            connection.recover(target, "Dashboard navigation failed");
                        }
                    });
                }
            }
            Ok(())
        })),
        &mut token,
    )?;

    let failed = connection.clone();
    let failed_window = window.clone();
    webview.add_ProcessFailed(
        &ProcessFailedEventHandler::create(Box::new(move |_, args| {
            if let Some(args) = args {
                let mut kind = COREWEBVIEW2_PROCESS_FAILED_KIND::default();
                args.ProcessFailedKind(&mut kind)?;
                if kind == COREWEBVIEW2_PROCESS_FAILED_KIND_BROWSER_PROCESS_EXITED
                    || kind == COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_EXITED
                    || kind == COREWEBVIEW2_PROCESS_FAILED_KIND_RENDER_PROCESS_UNRESPONSIVE
                {
                    eprintln!("WebView2 process failed");
                    let connection = failed.clone();
                    queue(&failed_window, move |target| {
                        if connection.window_epoch() == epoch {
                            connection.webview_failed(target);
                        }
                    });
                }
            }
            Ok(())
        })),
        &mut token,
    )?;

    let settings = webview.Settings()?;
    // Disable the control's built-in zoom keys and retain its Edit shortcuts.
    settings.SetIsZoomControlEnabled(false)?;
    let settings = settings.cast::<ICoreWebView2Settings2>()?;
    let mut raw = PWSTR::null();
    settings.UserAgent(&mut raw)?;
    let default_agent = take_pwstr(raw);
    let agent = format!(
        "{default_agent} Werkschaft/{}",
        window.app_handle().package_info().version
    );
    settings.SetUserAgent(&HSTRING::from(agent))?;
    install_accelerators(controller, window.clone(), connection.clone())
}

unsafe fn install_accelerators(
    controller: &ICoreWebView2Controller,
    window: WebviewWindow,
    connection: Arc<Connection>,
) -> windows::core::Result<()> {
    let epoch = connection.window_epoch();
    let mut token = 0;
    controller.add_AcceleratorKeyPressed(
        &AcceleratorKeyPressedEventHandler::create(Box::new(move |_, args| {
            let Some(args) = args else { return Ok(()) };
            let mut kind = COREWEBVIEW2_KEY_EVENT_KIND::default();
            args.KeyEventKind(&mut kind)?;
            if kind != COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN {
                return Ok(());
            }
            let mut raw = 0;
            args.VirtualKey(&mut raw)?;
            let key = match VIRTUAL_KEY(raw as u16) {
                VK_R => accelerator::Key::R,
                VK_I => accelerator::Key::I,
                VK_F12 => accelerator::Key::F12,
                VK_0 => accelerator::Key::Zero,
                VK_OEM_PLUS | VK_ADD => accelerator::Key::Plus,
                VK_OEM_MINUS | VK_SUBTRACT => accelerator::Key::Minus,
                _ => return Ok(()),
            };
            if [VK_MENU, VK_LWIN, VK_RWIN].into_iter().any(key_down) {
                return Ok(());
            }
            if let Some(id) = accelerator::menu_id(key, key_down(VK_CONTROL), key_down(VK_SHIFT)) {
                args.SetHandled(true)?;
                let mut status = COREWEBVIEW2_PHYSICAL_KEY_STATUS::default();
                args.PhysicalKeyStatus(&mut status)?;
                if !status.WasKeyDown.as_bool() {
                    // WebView2 blocks the browser process during this callback.
                    // Dispatch after returning so Retry can safely navigate.
                    let connection = connection.clone();
                    queue(&window, move |target| {
                        if connection.window_epoch() == epoch {
                            menu_action(target, &connection, id);
                        }
                    });
                }
            }
            Ok(())
        })),
        &mut token,
    )
}

fn key_down(key: VIRTUAL_KEY) -> bool {
    unsafe { GetKeyState(i32::from(key.0)) < 0 }
}
