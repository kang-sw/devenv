use url::Url;

// Only the local, script-free connection view uses an app protocol. The real
// dashboard always uses its HTTP origin, preserving existing same-origin writes.
pub const CONNECT_VIEW: &str = "tauri://localhost/index.html";

#[derive(Debug, PartialEq, Eq)]
pub enum Destination {
    Dashboard,
    ConnectView,
    OpaqueDocument,
    Browser,
    Block,
}

pub fn destination(url: &Url, dashboard: Option<&Url>) -> Destination {
    if url.as_str() == CONNECT_VIEW {
        return Destination::ConnectView;
    }
    // WKWebView's public navigation callback also sees subframe loads but gives
    // no frame identifier. Preserve blank/srcdoc sandbox frames; neither URL
    // has native capabilities, even if navigated to as the main document.
    if matches!(url.as_str(), "about:blank" | "about:srcdoc") {
        return Destination::OpaqueDocument;
    }
    // Reject userinfo even though URL origin comparisons deliberately ignore it.
    if !url.username().is_empty() || url.password().is_some() {
        return Destination::Block;
    }
    if dashboard.is_some_and(|root| url.origin() == root.origin()) {
        return Destination::Dashboard;
    }
    if matches!(url.scheme(), "http" | "https") && url.host_str().is_some() {
        return Destination::Browser;
    }
    Destination::Block
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_exact_discovered_http_origin_stays_in_the_dashboard() {
        let root = Url::parse("http://127.0.0.1:39417/").unwrap();
        for path in [
            "/",
            "/s/session?tab=terminal#tail",
            "/tracker",
            "/file-preview/a",
        ] {
            assert_eq!(
                destination(&root.join(path).unwrap(), Some(&root)),
                Destination::Dashboard
            );
        }
        for other in [
            "http://127.0.0.1:39418/",
            "http://localhost:39417/",
            "http://[::1]:39417/",
            "https://127.0.0.1:39417/",
            "http://127.0.0.1.evil.example:39417/",
            "https://example.com/",
        ] {
            assert_eq!(
                destination(&Url::parse(other).unwrap(), Some(&root)),
                Destination::Browser
            );
        }
        assert_eq!(destination(&root, None), Destination::Browser);
    }

    #[test]
    fn scripts_files_custom_schemes_and_userinfo_never_reach_the_os_opener() {
        let root = Url::parse("http://127.0.0.1:39417/").unwrap();
        for bad in [
            "javascript:alert(1)",
            "data:text/html,hello",
            "file:///etc/passwd",
            "mailto:person@example.com",
            "ws://127.0.0.1:39417/",
            "tauri://localhost/other",
            "https://user:secret@example.com/",
            "http://user@127.0.0.1:39417/",
        ] {
            assert_eq!(
                destination(&Url::parse(bad).unwrap(), Some(&root)),
                Destination::Block
            );
        }
        assert_eq!(
            destination(&Url::parse(CONNECT_VIEW).unwrap(), None),
            Destination::ConnectView
        );
        for frame in ["about:blank", "about:srcdoc"] {
            assert_eq!(
                destination(&Url::parse(frame).unwrap(), Some(&root)),
                Destination::OpaqueDocument
            );
        }
    }

    #[test]
    fn rediscovery_replaces_not_widens_the_port_pin() {
        let first = Url::parse("http://127.0.0.1:39417/").unwrap();
        let next = Url::parse("http://127.0.0.1:39418/").unwrap();
        assert_eq!(destination(&first, Some(&next)), Destination::Browser);
        assert_eq!(destination(&next, Some(&next)), Destination::Dashboard);
    }
}
