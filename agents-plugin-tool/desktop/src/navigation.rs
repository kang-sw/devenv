use url::Url;

#[cfg(target_os = "macos")]
pub const LOCAL_ORIGIN: &str = "tauri://localhost";
#[cfg(target_os = "windows")]
pub const LOCAL_ORIGIN: &str = "http://tauri.localhost";

pub const CONNECT_VIEW: &str = "/index.html";
pub const PYTHON_VIEW: &str = "/python-missing.html";

#[derive(Debug, PartialEq, Eq)]
pub enum Destination {
    Dashboard,
    Local,
    Browser,
    Block,
}

pub fn local_view(path: &str) -> Url {
    Url::parse(&format!("{LOCAL_ORIGIN}{path}")).expect("valid bundled page")
}

pub fn destination(url: &Url, dashboard: Option<&Url>) -> Destination {
    classify(url, dashboard, LOCAL_ORIGIN)
}

fn classify(url: &Url, dashboard: Option<&Url>, local_origin: &str) -> Destination {
    if !url.username().is_empty() || url.password().is_some() {
        return Destination::Block;
    }

    // Reserve both app hosts, including their alternate paths and ports.
    if url.scheme() == "tauri" || url.host_str() == Some("tauri.localhost") {
        return if [CONNECT_VIEW, PYTHON_VIEW]
            .iter()
            .any(|path| url.as_str() == format!("{local_origin}{path}"))
        {
            Destination::Local
        } else {
            Destination::Block
        };
    }

    // Blob URLs can report the underlying HTTP origin, so check scheme first.
    if !matches!(url.scheme(), "http" | "https") || url.host_str().is_none() {
        return Destination::Block;
    }
    if dashboard.is_some_and(|root| url.origin() == root.origin()) {
        Destination::Dashboard
    } else {
        Destination::Browser
    }
}

pub fn frame_allowed(url: &Url, dashboard: Option<&Url>) -> bool {
    matches!(url.as_str(), "about:blank" | "about:srcdoc")
        || destination(url, dashboard) == Destination::Dashboard
}

pub fn open_browser(url: Url) {
    std::thread::spawn(move || {
        if open::that(url.as_str()).is_err() {
            eprintln!("Werkschaft: Could not open the external link");
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exact_origin_only_for_dashboard_and_frames() {
        let pin = Url::parse("http://127.0.0.1:39417/").unwrap();
        for path in ["/", "/s/session?tab=terminal#tail", "/file-preview/a"] {
            let url = pin.join(path).unwrap();
            assert_eq!(destination(&url, Some(&pin)), Destination::Dashboard);
            assert!(frame_allowed(&url, Some(&pin)));
        }
        for raw in [
            "http://127.0.0.1:39418/",
            "http://localhost:39417/",
            "http://[::1]:39417/",
            "https://127.0.0.1:39417/",
            "http://127.0.0.1.evil.example:39417/",
            "https://example.com/",
        ] {
            let url = Url::parse(raw).unwrap();
            assert_eq!(destination(&url, Some(&pin)), Destination::Browser);
            assert!(!frame_allowed(&url, Some(&pin)));
        }
        assert_eq!(destination(&pin, None), Destination::Browser);
        assert!(!frame_allowed(&pin, None));
    }

    #[test]
    fn approved_local_pages_are_top_level_only_on_both_origins() {
        for origin in ["tauri://localhost", "http://tauri.localhost"] {
            for path in [CONNECT_VIEW, PYTHON_VIEW] {
                let url = Url::parse(&format!("{origin}{path}")).unwrap();
                assert_eq!(classify(&url, None, origin), Destination::Local);
                assert!(!frame_allowed(&url, None));
            }
            for path in ["/other", "/index.html?cmd=retry", "/index.html#retry"] {
                let url = Url::parse(&format!("{origin}{path}")).unwrap();
                assert_eq!(classify(&url, None, origin), Destination::Block);
            }
        }
    }

    #[test]
    fn only_frames_allow_blank_or_srcdoc_and_script_urls_are_blocked() {
        let pin = Url::parse("http://127.0.0.1:39417/").unwrap();
        for raw in ["about:blank", "about:srcdoc"] {
            let url = Url::parse(raw).unwrap();
            assert_eq!(destination(&url, Some(&pin)), Destination::Block);
            assert!(frame_allowed(&url, Some(&pin)));
        }
        for raw in [
            "javascript:alert(1)",
            "data:text/html,hello",
            "file:///etc/passwd",
            "mailto:person@example.com",
            "ws://127.0.0.1:39417/",
            "blob:http://127.0.0.1:39417/resource",
            "https://user:secret@example.com/",
            "http://user@127.0.0.1:39417/",
            "https://tauri.localhost/index.html",
        ] {
            let url = Url::parse(raw).unwrap();
            assert_eq!(destination(&url, Some(&pin)), Destination::Block);
            assert!(!frame_allowed(&url, Some(&pin)));
        }
    }
}
