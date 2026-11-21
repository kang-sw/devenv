use serde::Deserialize;
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
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

// These private types intentionally do not implement Debug: the endpoint can
// contain a bearer, and browser credentials must not appear in diagnostics.
#[derive(Clone, Deserialize, PartialEq, Eq)]
struct Endpoint {
    port: u16,
    pid: u32,
    #[serde(default)]
    token: Option<Value>,
}

#[derive(Deserialize)]
struct Health {
    service: String,
    pid: u32,
    #[serde(default)]
    port: Option<u16>,
    #[serde(default)]
    started_at: Option<Value>,
    #[serde(default)]
    version: String,
}

#[derive(Clone, PartialEq, Eq)]
pub struct Daemon {
    pub url: Url,
    pub version: String,
    root: PathBuf,
    endpoint: Endpoint,
    started_at: Option<Value>,
}

pub struct BrowserCookie {
    pub name: String,
    pub value: String,
    pub max_age: u64,
}

pub fn cache_root(
    override_home: Option<&OsStr>,
    home: Option<&OsStr>,
    cwd: &Path,
) -> Result<PathBuf> {
    let root = match override_home.filter(|value| !value.is_empty()) {
        Some(value) => PathBuf::from(value),
        None => PathBuf::from(
            home.filter(|value| !value.is_empty())
                .ok_or("Home directory is unavailable")?,
        )
        .join(".cache/ws@kang-sw-devenv"),
    };
    let absolute = if root.is_absolute() {
        root
    } else {
        cwd.join(root)
    };
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
    let ep: Endpoint = serde_json::from_slice(&raw).map_err(|_| "Invalid dashboard endpoint")?;
    if ep.port == 0 || ep.pid == 0 {
        return Err("Invalid dashboard endpoint port or PID");
    }
    Ok(ep)
}

fn agent(timeout: u64) -> ureq::Agent {
    ureq::Agent::config_builder()
        .proxy(None)
        .max_redirects(0)
        .http_status_as_error(false)
        .timeout_global(Some(Duration::from_secs(timeout)))
        .build()
        .into()
}

fn check_health(ep: &Endpoint) -> Result<(Url, Health)> {
    let root = Url::parse(&format!("http://127.0.0.1:{}/", ep.port))
        .map_err(|_| "Invalid dashboard URL")?;
    let mut response = agent(2)
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
    if health.service != "ws-dashboard"
        || health.pid != ep.pid
        || health.port.is_some_and(|port| port != ep.port)
    {
        return Err("Health response is not from the recorded dashboard daemon");
    }
    Ok((root, health))
}

pub fn discover_at(root: &Path) -> Result<Daemon> {
    let ep = read_endpoint(root)?;
    let (url, health) = check_health(&ep)?;
    if read_endpoint(root)? != ep {
        return Err("Dashboard endpoint changed; retry discovery");
    }
    Ok(Daemon {
        url,
        version: health.version,
        root: root.to_path_buf(),
        endpoint: ep,
        started_at: health.started_at,
    })
}

pub fn active_cache_root() -> Result<PathBuf> {
    let cwd = std::env::current_dir().map_err(|_| "Cannot resolve working directory")?;
    let home = std::env::var_os("HOME")
        .filter(|value| !value.is_empty())
        .or_else(|| {
            if cfg!(windows) {
                std::env::var_os("USERPROFILE")
            } else {
                None
            }
        });
    cache_root(
        std::env::var_os("WS_CACHE_HOME").as_deref(),
        home.as_deref(),
        &cwd,
    )
}

impl Daemon {
    pub fn is_healthy(&self) -> bool {
        check_health(&self.endpoint).is_ok_and(|(_, health)| {
            health.started_at == self.started_at && health.version == self.version
        })
    }

    pub fn is_current(&self) -> bool {
        self.is_healthy()
            && read_endpoint(&self.root).is_ok_and(|endpoint| endpoint == self.endpoint)
    }

    fn token(&self) -> Result<&str> {
        self.endpoint
            .token
            .as_ref()
            .and_then(Value::as_str)
            .filter(|s| {
                s.len() == 64
                    && s.bytes()
                        .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
            })
            .ok_or("Dashboard identity or credential changed; retry")
    }

    pub fn predecessor(&self) -> Result<Value> {
        let hash = format!("{:x}", Sha256::digest(self.token()?.as_bytes()));
        Ok(json!({
            "port": self.endpoint.port,
            "pid": self.endpoint.pid,
            "started_at": self.started_at.as_ref().and_then(Value::as_str).unwrap_or_default(),
            "version": self.version,
            "token_sha256": hash,
        }))
    }

    fn auth_json(&self, route: &str, body: Value) -> Result<Option<Value>> {
        let initial = read_endpoint(&self.root)?;
        let token = self.token()?;
        if initial != self.endpoint {
            return Err("Dashboard identity or credential changed; retry");
        }
        let current = discover_at(&self.root)?;
        if current != *self || read_endpoint(&self.root)? != initial {
            return Err("Dashboard changed before authenticated request; retry");
        }
        let mut response = agent(3)
            .post(self.url.join(route).unwrap().as_str())
            .header("Authorization", &format!("Bearer {token}"))
            .send_json(body)
            .map_err(|_| "Dashboard browser login request failed")?;
        if response.status() == 404 {
            return Ok(None);
        }
        if response.status() != 200 {
            return Err("Dashboard browser login request failed");
        }
        let value = response
            .body_mut()
            .with_config()
            .limit(MAX_JSON)
            .read_json()
            .map_err(|_| "Dashboard browser login request failed")?;
        Ok(Some(value))
    }

    pub fn browser_cookie(&self) -> Result<Option<BrowserCookie>> {
        let Some(value) = self.auth_json("/auth/cookie", json!({}))? else {
            return Ok(None);
        };
        let invalid = "Invalid dashboard browser cookie response";
        let name = value
            .get("name")
            .and_then(Value::as_str)
            .filter(|s| {
                !s.is_empty()
                    && s.bytes()
                        .all(|b| b.is_ascii_alphanumeric() || b"!#$%&'*+.^_`|~-".contains(&b))
            })
            .ok_or(invalid)?;
        let cookie = value
            .get("value")
            .and_then(Value::as_str)
            .filter(|s| {
                !s.is_empty()
                    && s.bytes().all(|b| {
                        b == 0x21
                            || (0x23..=0x2b).contains(&b)
                            || (0x2d..=0x3a).contains(&b)
                            || (0x3c..=0x5b).contains(&b)
                            || (0x5d..=0x7e).contains(&b)
                    })
            })
            .ok_or(invalid)?;
        let max_age = value
            .get("maxAge")
            .and_then(Value::as_u64)
            .filter(|age| *age > 0 && *age <= 34_560_000)
            .ok_or(invalid)?;
        Ok(Some(BrowserCookie {
            name: name.to_owned(),
            value: cookie.to_owned(),
            max_age,
        }))
    }

    pub fn browser_login_link(&self, path: &str) -> Result<Option<Url>> {
        let Some(value) = self.auth_json("/auth/link", json!({"path": path}))? else {
            return Ok(None);
        };
        let invalid = "Invalid dashboard browser login link";
        let url = value
            .get("url")
            .and_then(Value::as_str)
            .and_then(|s| Url::parse(s).ok())
            .ok_or(invalid)?;
        if url.scheme() != self.url.scheme()
            || url.origin() != self.url.origin()
            || !url.username().is_empty()
            || url.password().is_some()
            || !url
                .query_pairs()
                .any(|(key, value)| key == "login" && !value.is_empty())
        {
            return Err(invalid);
        }
        Ok(Some(url))
    }
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
        let absolute = if cfg!(windows) {
            Path::new(r"Q:\werkschaft-test")
        } else {
            Path::new("/werkschaft-test")
        };
        let cwd = absolute.join("work");
        let home = absolute.join("home");
        let override_path = absolute.join("override");
        assert_eq!(
            cache_root(None, Some(home.as_os_str()), &cwd).unwrap(),
            home.join(".cache/ws@kang-sw-devenv")
        );
        assert_eq!(
            cache_root(Some(OsStr::new("")), Some(home.as_os_str()), &cwd).unwrap(),
            cache_root(None, Some(home.as_os_str()), &cwd).unwrap()
        );
        assert_eq!(
            cache_root(Some(OsStr::new("relative")), None, &cwd).unwrap(),
            cwd.join("relative")
        );
        assert_eq!(
            cache_root(Some(override_path.as_os_str()), None, &cwd).unwrap(),
            override_path
        );
        assert!(cache_root(None, None, &cwd).is_err());
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
            assert_eq!(
                check_health(&Endpoint {
                    port,
                    pid: 12,
                    token: None
                })
                .is_ok(),
                ok
            );
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
            discover_at(&cache.0).unwrap().url.as_str(),
            format!("http://127.0.0.1:{port}/")
        );
        server.join().unwrap();
    }

    #[test]
    fn unreachable_endpoint_is_an_error_not_a_start_request() {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        drop(listener);
        assert!(check_health(&Endpoint {
            port,
            pid: 12,
            token: None
        })
        .is_err());
    }
    #[test]
    fn auth_validates_responses_and_rechecks_identity() {
        let cache = Cache::new();
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let port = listener.local_addr().unwrap().port();
        let token = "a".repeat(64);
        cache.write(format!(r#"{{"port":{port},"pid":12,"token":"{token}"}}"#).as_bytes());
        let server_token = token.clone();
        let server = thread::spawn(move || {
            for index in 0..7 {
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
                let length = request
                    .lines()
                    .find_map(|line| {
                        let (name, value) = line.split_once(':')?;
                        if name.eq_ignore_ascii_case("content-length") {
                            value.trim().parse::<usize>().ok()
                        } else {
                            None
                        }
                    })
                    .unwrap_or(0);
                let mut payload = vec![0; length];
                reader.read_exact(&mut payload).unwrap();
                let health = index == 0 || index == 1 || index == 3 || index == 5;
                let body = if health {
                    assert!(request.starts_with("GET /health "));
                    assert!(!request.to_lowercase().contains("authorization:"));
                    r#"{"service":"ws-dashboard","pid":12,"started_at":"start"}"#.to_string()
                } else {
                    assert!(request
                        .to_lowercase()
                        .contains(&format!("authorization: bearer {server_token}")));
                    assert!(!request.to_lowercase().contains("origin:"));
                    match index {
                        2 => r#"{"name":"ws_local","value":"browser-cookie","maxAge":34560000}"#
                            .to_string(),
                        4 => format!(r#"{{"url":"http://127.0.0.1:{port}/s/test?login=once"}}"#),
                        _ => r#"{"name":"bad name","value":"secret","maxAge":1}"#.to_string(),
                    }
                };
                let response = format!(
                    "HTTP/1.1 200 OK\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    body.len(),
                    body
                );
                stream.write_all(response.as_bytes()).unwrap();
            }
        });
        let daemon = discover_at(&cache.0).unwrap();
        let cookie = daemon.browser_cookie().unwrap().unwrap();
        assert_eq!(
            (cookie.name.as_str(), cookie.value.as_str(), cookie.max_age),
            ("ws_local", "browser-cookie", 34560000)
        );
        assert_eq!(
            daemon
                .browser_login_link("/s/test")
                .unwrap()
                .unwrap()
                .as_str(),
            format!("http://127.0.0.1:{port}/s/test?login=once")
        );
        assert!(daemon.browser_cookie().is_err());
        server.join().unwrap();
        cache.write(
            format!(r#"{{"port":{port},"pid":12,"token":"{}"}}"#, "b".repeat(64)).as_bytes(),
        );
        assert!(daemon.browser_cookie().is_err());
    }

    #[test]
    fn auth_requires_a_valid_observed_bearer() {
        let cache = Cache::new();
        let (port, server) = fixture("200 OK", r#"{"service":"ws-dashboard","pid":12}"#, "");
        cache.write(format!(r#"{{"port":{port},"pid":12,"token":"invalid-secret"}}"#).as_bytes());
        let daemon = discover_at(&cache.0).unwrap();
        server.join().unwrap();
        let error = match daemon.browser_cookie() {
            Err(error) => error,
            Ok(_) => panic!("accepted invalid bearer"),
        };
        assert!(!error.contains("invalid-secret"));
    }
}
