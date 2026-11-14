use serde::Deserialize;
use std::{
    ffi::OsStr,
    fs::File,
    io::Read,
    path::{Path, PathBuf},
    time::Duration,
};
use url::Url;

type Result<T> = std::result::Result<T, &'static str>;
const MAX_JSON: u64 = 64 * 1024;

// Deliberately omit token: serde discards unknown fields, so lifecycle credentials
// cannot become part of a URL, diagnostic, window state, or JavaScript value.
#[derive(Deserialize, PartialEq, Eq)]
struct Endpoint {
    port: u16,
    pid: u32,
}

#[derive(Deserialize)]
struct Health {
    service: String,
    pid: u32,
}

pub fn cache_root(
    override_home: Option<&OsStr>,
    home: Option<&OsStr>,
    cwd: &Path,
) -> Result<PathBuf> {
    let root = match override_home.filter(|value| !value.is_empty()) {
        Some(value) => PathBuf::from(value),
        None => PathBuf::from(home.ok_or("Home directory is unavailable")?)
            .join(".cache/ws@kang-sw-devenv"),
    };
    let absolute = if root.is_absolute() {
        root
    } else {
        cwd.join(root)
    };
    // Match wsstate.CacheRoot: absolute path, resolving symlinks when possible.
    Ok(absolute.canonicalize().unwrap_or(absolute))
}

fn read_endpoint(root: &Path) -> Result<Endpoint> {
    let file = File::open(root.join("dashboard/endpoint.json"))
        .map_err(|_| "No readable dashboard endpoint; start the dashboard normally")?;
    let mut raw = Vec::new();
    file.take(MAX_JSON + 1)
        .read_to_end(&mut raw)
        .map_err(|_| "Cannot read dashboard endpoint")?;
    if raw.len() as u64 > MAX_JSON {
        return Err("Dashboard endpoint is too large");
    }
    // Never include serde errors or raw file contents in diagnostics.
    let ep: Endpoint = serde_json::from_slice(&raw).map_err(|_| "Invalid dashboard endpoint")?;
    if ep.port == 0 || ep.pid == 0 {
        return Err("Invalid dashboard endpoint port or PID");
    }
    Ok(ep)
}

fn check_health(ep: &Endpoint) -> Result<Url> {
    let root = Url::parse(&format!("http://127.0.0.1:{}/", ep.port))
        .map_err(|_| "Invalid dashboard URL")?;
    let agent: ureq::Agent = ureq::Agent::config_builder()
        .proxy(None)
        .max_redirects(0)
        .timeout_global(Some(Duration::from_secs(2)))
        .build()
        .into();
    let mut response = agent
        .get(root.join("health").unwrap().as_str())
        .call()
        .map_err(|_| "Dashboard health check failed; start the dashboard normally")?;
    if response.status() != 200 {
        return Err("Dashboard health check did not return HTTP 200");
    }
    let health: Health = response
        .body_mut()
        .with_config()
        .limit(MAX_JSON)
        .read_json()
        .map_err(|_| "Invalid dashboard health response")?;
    // This is the existing Go client.Health identity check, not a new auth scheme.
    if health.service != "ws-dashboard" || health.pid != ep.pid {
        return Err("Health response is not from the recorded dashboard daemon");
    }
    Ok(root)
}

pub fn discover_at(root: &Path) -> Result<Url> {
    let ep = read_endpoint(root)?;
    let url = check_health(&ep)?;
    if read_endpoint(root)? != ep {
        return Err("Dashboard endpoint changed; retry discovery");
    }
    Ok(url)
}

pub fn discover() -> Result<Url> {
    let cwd = std::env::current_dir().map_err(|_| "Cannot resolve working directory")?;
    let root = cache_root(
        std::env::var_os("WS_CACHE_HOME").as_deref(),
        std::env::var_os("HOME").as_deref(),
        &cwd,
    )?;
    discover_at(&root)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::{
        fs,
        io::{BufRead, BufReader, Write},
        net::TcpListener,
        sync::atomic::{AtomicUsize, Ordering},
        thread,
    };

    struct Cache(PathBuf);
    impl Cache {
        fn new() -> Self {
            static NEXT: AtomicUsize = AtomicUsize::new(0);
            let root = std::env::temp_dir().join(format!(
                "werkschaft-unit-{}-{}",
                std::process::id(),
                NEXT.fetch_add(1, Ordering::Relaxed)
            ));
            fs::create_dir_all(root.join("dashboard")).unwrap();
            Self(root)
        }
        fn write(&self, value: &[u8]) {
            fs::write(self.0.join("dashboard/endpoint.json"), value).unwrap();
        }
    }
    impl Drop for Cache {
        fn drop(&mut self) {
            let _ = fs::remove_dir_all(&self.0);
        }
    }

    #[test]
    fn cache_override_default_empty_and_relative() {
        let cwd = Path::new("/work");
        let home = Some(OsStr::new("/home/example"));
        assert_eq!(
            cache_root(None, home, cwd).unwrap(),
            Path::new("/home/example/.cache/ws@kang-sw-devenv")
        );
        assert_eq!(
            cache_root(Some(OsStr::new("")), home, cwd).unwrap(),
            cache_root(None, home, cwd).unwrap()
        );
        assert_eq!(
            cache_root(Some(OsStr::new("relative")), None, cwd).unwrap(),
            Path::new("/work/relative")
        );
        assert_eq!(
            cache_root(Some(OsStr::new("/override")), None, cwd).unwrap(),
            Path::new("/override")
        );
        assert!(cache_root(None, None, cwd).is_err());
    }

    #[test]
    fn missing_malformed_and_out_of_range_endpoints_fail_without_disclosing_contents() {
        let cache = Cache::new();
        assert!(read_endpoint(&cache.0).is_err());
        for raw in [
            r#"{"port":0,"pid":12}"#,
            r#"{"port":65536,"pid":12}"#,
            r#"{"port":-1,"pid":12}"#,
            r#"{"port":29417,"pid":0}"#,
            r#"{"port":29417}"#,
            r#"{"token":"never-echo-this", broken}"#,
        ] {
            cache.write(raw.as_bytes());
            let error = read_endpoint(&cache.0).err().unwrap();
            assert!(!error.contains("never-echo-this"));
        }
        cache.write(&vec![b' '; MAX_JSON as usize + 1]);
        assert!(read_endpoint(&cache.0).is_err());
        cache.write(br#"{"port":29417,"pid":12,"token":"never-echo-this","future":true}"#);
        let ep = read_endpoint(&cache.0).unwrap();
        assert_eq!((ep.port, ep.pid), (29417, 12));
    }

    fn fixture(status: &str, body: &str, extra_headers: &str) -> (u16, thread::JoinHandle<()>) {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let response = format!("HTTP/1.1 {status}\r\nContent-Length: {}\r\nConnection: close\r\n{extra_headers}\r\n{body}", body.len());
        let handle = thread::spawn(move || {
            let (mut stream, _) = listener.accept().unwrap();
            stream
                .set_read_timeout(Some(Duration::from_secs(3)))
                .unwrap();
            let mut reader = BufReader::new(stream.try_clone().unwrap());
            let mut request = String::new();
            loop {
                let mut line = String::new();
                reader.read_line(&mut line).unwrap();
                request.push_str(&line);
                if line == "\r\n" || line.is_empty() {
                    break;
                }
            }
            assert!(request.starts_with("GET /health HTTP/1.1\r\n"));
            assert!(request
                .to_lowercase()
                .contains(&format!("host: 127.0.0.1:{port}")));
            assert!(!request.to_lowercase().contains("authorization:"));
            assert!(!request.contains("never-echo-this"));
            // A bounded client may close early on an oversized body.
            let _ = stream.write_all(response.as_bytes());
        });
        (port, handle)
    }

    #[test]
    fn health_identity_status_redirect_and_body_limits() {
        for (status, body, headers, ok) in [
            (
                "200 OK",
                r#"{"service":"ws-dashboard","pid":12,"extra":true}"#.to_string(),
                "",
                true,
            ),
            (
                "200 OK",
                r#"{"service":"other","pid":12}"#.to_string(),
                "",
                false,
            ),
            (
                "200 OK",
                r#"{"service":"ws-dashboard","pid":13}"#.to_string(),
                "",
                false,
            ),
            ("503 Unavailable", "{}".to_string(), "", false),
            (
                "302 Found",
                "{}".to_string(),
                "Location: http://127.0.0.1:1/\r\n",
                false,
            ),
            ("200 OK", "not-json".to_string(), "", false),
            ("200 OK", " ".repeat(MAX_JSON as usize + 1), "", false),
        ] {
            let (port, server) = fixture(status, &body, headers);
            assert_eq!(check_health(&Endpoint { port, pid: 12 }).is_ok(), ok);
            server.join().unwrap();
        }
    }

    #[test]
    fn missing_endpoint_then_successful_retry_uses_only_discovered_port() {
        let cache = Cache::new();
        assert!(discover_at(&cache.0).is_err());
        let (port, server) = fixture("200 OK", r#"{"service":"ws-dashboard","pid":12}"#, "");
        cache.write(format!(r#"{{"port":{port},"pid":12,"token":"never-echo-this"}}"#).as_bytes());
        assert_eq!(
            discover_at(&cache.0).unwrap().as_str(),
            format!("http://127.0.0.1:{port}/")
        );
        server.join().unwrap();
    }

    #[test]
    fn unreachable_endpoint_is_an_error_not_a_start_request() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        drop(listener);
        assert!(check_health(&Endpoint { port, pid: 12 }).is_err());
    }
}
