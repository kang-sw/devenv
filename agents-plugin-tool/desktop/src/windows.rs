use std::sync::Arc;

use tauri::WebviewWindow;
use webview2_com::{
    take_pwstr, Microsoft::Web::WebView2::Win32::*, NavigationCompletedEventHandler,
    NavigationStartingEventHandler, PermissionRequestedEventHandler, ProcessFailedEventHandler,
};
use windows::core::{Interface, BOOL, HSTRING, PWSTR};

use crate::{navigation, shell::Connection};

pub fn install(window: &WebviewWindow, connection: Arc<Connection>) -> tauri::Result<()> {
    let window = window.clone();
    window.clone().with_webview(move |platform| {
        let result =
            unsafe { install_hooks(&platform.controller().CoreWebView2(), &window, &connection) };
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
    webview: &windows::core::Result<ICoreWebView2>,
    window: &WebviewWindow,
    connection: &Arc<Connection>,
) -> windows::core::Result<()> {
    let webview = webview.as_ref().map_err(Clone::clone)?;
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
                    eprintln!("Dashboard navigation failed");
                    let connection = failed.clone();
                    queue(&failed_window, move |target| {
                        connection.recover(target, "navigation failed")
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
                        connection.webview_failed(target)
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
    let agent = format!("{default_agent} Werkschaft/{}", env!("CARGO_PKG_VERSION"));
    settings.SetUserAgent(&HSTRING::from(agent))?;
    Ok(())
}
