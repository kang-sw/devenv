use std::sync::Arc;

use block2::Block;
use objc2::{
    define_class, ffi, msg_send,
    rc::Weak,
    runtime::{NSObject, ProtocolObject},
    DefinedClass, MainThreadOnly,
};
use objc2_foundation::{MainThreadMarker, NSError, NSObjectProtocol};
use objc2_web_kit::{
    WKNavigation, WKNavigationAction, WKNavigationActionPolicy, WKNavigationDelegate,
    WKNavigationResponse, WKNavigationResponsePolicy, WKWebView,
};
use tauri::{AppHandle, Manager, WebviewWindow};
use url::Url;

use crate::{
    navigation::{self, Destination},
    shell::Connection,
};

struct NavigationState {
    original: Weak<ProtocolObject<dyn WKNavigationDelegate>>,
    connection: std::sync::Weak<Connection>,
    app: AppHandle,
}

define_class!(
    #[unsafe(super(NSObject))]
    #[thread_kind = MainThreadOnly]
    #[ivars = NavigationState]
    struct NavigationDelegate;

    unsafe impl NSObjectProtocol for NavigationDelegate {}

    unsafe impl WKNavigationDelegate for NavigationDelegate {
        #[unsafe(method(webView:decidePolicyForNavigationAction:decisionHandler:))]
        fn action(
            &self,
            webview: &WKWebView,
            action: &WKNavigationAction,
            reply: &Block<dyn Fn(WKNavigationActionPolicy)>,
        ) {
            let raw = unsafe { action.request().URL().and_then(|url| url.absoluteString()) };
            let allowed = raw
                .and_then(|url| Url::parse(&url.to_string()).ok())
                .is_some_and(|url| {
                    let target = unsafe { action.targetFrame().map(|frame| frame.isMainFrame()) };
                    self.allows(url, target)
                });
            if allowed {
                if let Some(original) = self.ivars().original.load() {
                    unsafe {
                        original.webView_decidePolicyForNavigationAction_decisionHandler(
                            webview, action, reply,
                        );
                    }
                    return;
                }
            }
            reply.call((WKNavigationActionPolicy::Cancel,));
        }

        #[unsafe(method(webView:decidePolicyForNavigationResponse:decisionHandler:))]
        fn response(
            &self,
            webview: &WKWebView,
            response: &WKNavigationResponse,
            reply: &Block<dyn Fn(WKNavigationResponsePolicy)>,
        ) {
            let raw = unsafe {
                response
                    .response()
                    .URL()
                    .and_then(|url| url.absoluteString())
            };
            let allowed = raw
                .and_then(|url| Url::parse(&url.to_string()).ok())
                .is_some_and(|url| self.allows(url, Some(unsafe { response.isForMainFrame() })));
            if allowed {
                if let Some(original) = self.ivars().original.load() {
                    unsafe {
                        original.webView_decidePolicyForNavigationResponse_decisionHandler(
                            webview, response, reply,
                        );
                    }
                    return;
                }
            }
            reply.call((WKNavigationResponsePolicy::Cancel,));
        }

        #[unsafe(method(webView:didCommitNavigation:))]
        fn committed(&self, webview: &WKWebView, navigation: Option<&WKNavigation>) {
            if let Some(original) = self.ivars().original.load() {
                unsafe {
                    original.webView_didCommitNavigation(webview, navigation);
                }
            }
        }

        #[unsafe(method(webView:didFinishNavigation:))]
        fn finished(&self, webview: &WKWebView, navigation: Option<&WKNavigation>) {
            if let Some(original) = self.ivars().original.load() {
                unsafe {
                    original.webView_didFinishNavigation(webview, navigation);
                }
            }
        }

        #[unsafe(method(webView:didFailProvisionalNavigation:withError:))]
        fn provisional_failed(&self, _: &WKWebView, _: Option<&WKNavigation>, error: &NSError) {
            self.load_failed(error);
        }

        #[unsafe(method(webView:didFailNavigation:withError:))]
        fn failed(&self, _: &WKWebView, _: Option<&WKNavigation>, error: &NSError) {
            self.load_failed(error);
        }

        #[unsafe(method(webViewWebContentProcessDidTerminate:))]
        fn renderer_failed(&self, _: &WKWebView) {
            self.recover("Dashboard renderer stopped");
        }
    }
);

impl NavigationDelegate {
    fn allows(&self, url: Url, target: Option<bool>) -> bool {
        let pin = self
            .ivars()
            .connection
            .upgrade()
            .and_then(|state| state.pinned_url());
        if target == Some(false) {
            return navigation::frame_allowed(&url, pin.as_ref());
        }
        let destination = navigation::destination(&url, pin.as_ref());
        if target.is_none() {
            // WKUIDelegate handles these through the shared popup policy.
            return matches!(destination, Destination::Dashboard | Destination::Browser);
        }
        match destination {
            Destination::Dashboard | Destination::Local => true,
            Destination::Browser => {
                navigation::open_browser(url);
                false
            }
            Destination::Block => false,
        }
    }

    fn load_failed(&self, error: &NSError) {
        // NSURLErrorCancelled and WebKit's policy-change interruption occur
        // when an otherwise healthy navigation was deliberately canceled.
        let code = error.code();
        if !matches!(code, -999 | 102) {
            self.recover("Dashboard navigation failed");
        }
    }

    fn recover(&self, reason: &'static str) {
        let Some(connection) = self.ivars().connection.upgrade() else {
            return;
        };
        let app = self.ivars().app.clone();
        // Wait until WebKit returns from its delegate callback before loading.
        std::thread::spawn(move || {
            let target = app.clone();
            let _ = app.run_on_main_thread(move || {
                if let Some(window) = target.get_webview_window("main") {
                    connection.recover(window, reason);
                }
            });
        });
    }
}

pub fn install(window: &WebviewWindow, connection: Arc<Connection>) -> tauri::Result<()> {
    static DELEGATE_KEY: u8 = 0;
    let target = window.clone();
    window.with_webview(move |platform| {
        let marker = MainThreadMarker::new().expect("with_webview runs on the main thread");
        // The pinned Tauri runtime supplies a WKWebView on the main thread.
        let webview = unsafe { &*platform.inner().cast::<WKWebView>() };
        let Some(original) = (unsafe { webview.navigationDelegate() }) else {
            connection.setup_failed(&target);
            return;
        };
        let delegate = marker
            .alloc::<NavigationDelegate>()
            .set_ivars(NavigationState {
                original: Weak::new(&original),
                connection: Arc::downgrade(&connection),
                app: target.app_handle().clone(),
            });
        let delegate: objc2::rc::Retained<NavigationDelegate> =
            unsafe { msg_send![super(delegate), init] };
        let protocol = ProtocolObject::from_ref(&*delegate);
        unsafe {
            // WKWebView's delegate property is weak. Associate the replacement
            // with its view; the original remains owned by Wry.
            ffi::objc_setAssociatedObject(
                (webview as *const WKWebView).cast_mut().cast(),
                std::ptr::from_ref(&DELEGATE_KEY).cast(),
                (protocol as *const ProtocolObject<dyn WKNavigationDelegate>)
                    .cast_mut()
                    .cast(),
                ffi::OBJC_ASSOCIATION_RETAIN_NONATOMIC,
            );
            webview.setNavigationDelegate(Some(protocol));
        }
        connection.start(target);
    })
}
