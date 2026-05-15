use std::net::IpAddr;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use axum::http::{header, HeaderMap, StatusCode};
use rand::RngCore;

const OWNER_COOKIE_NAME: &str = "ws-dashboard-owner";
const TOKEN_BYTES: usize = 32;
const DEFAULT_PAIRING_TOKEN_TTL: Duration = Duration::from_secs(5 * 60);

#[derive(Clone, Debug)]
pub struct OwnerAuthState {
    // CONTRACT: Pairing token is startup-generated, one-time, and the only
    // unauthenticated browser path accepted by the daemon.
    pairing_token: PairingToken,
    inner: Arc<Mutex<AuthInner>>,
}

#[derive(Debug)]
struct AuthInner {
    pairing_consumed: bool,
    pairing_issued_at: Instant,
    pairing_ttl: Duration,
    session_token: String,
    bearer_token: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PairingToken(String);

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OwnerSessionCookie(String);

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BearerAuthToken(String);

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PairingTokenPolicy {
    // CONTRACT: Pairing token lifetime is explicit daemon policy, not an
    // incidental test timeout or caller-owned timer.
    pub ttl: Duration,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum PairingOutcome {
    Paired,
    Invalid,
    AlreadyUsed,
    // CONTRACT: Expired pairing tokens fail without installing a browser
    // session or consuming later auth state.
    Expired,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AuthRejection {
    Unauthorized,
    Forbidden,
}

impl OwnerAuthState {
    pub fn new_ephemeral() -> Self {
        Self::new_ephemeral_with_policy(PairingTokenPolicy::default())
    }

    pub fn new_ephemeral_with_policy(policy: PairingTokenPolicy) -> Self {
        // CONTRACT: Phase 2 construction accepts a token policy so tests and
        // daemon startup can verify expiry behavior without sleeping.
        // HOLE filled: this constructor is the single project-local startup
        // path for ephemeral secrets; TTL enforcement remains Phase 2 logic.
        Self {
            pairing_token: PairingToken(random_secret()),
            inner: Arc::new(Mutex::new(AuthInner {
                pairing_consumed: false,
                pairing_issued_at: Instant::now(),
                pairing_ttl: policy.ttl,
                session_token: random_secret(),
                bearer_token: random_secret(),
            })),
        }
    }

    pub fn pairing_token(&self) -> &PairingToken {
        &self.pairing_token
    }

    pub fn consume_pairing_token(&self, candidate: &str) -> PairingOutcome {
        if candidate != self.pairing_token.0 {
            return PairingOutcome::Invalid;
        }

        let mut inner = self.inner.lock().expect("owner auth mutex poisoned");
        if inner.pairing_consumed {
            return PairingOutcome::AlreadyUsed;
        }

        if inner.pairing_issued_at.elapsed() >= inner.pairing_ttl {
            return PairingOutcome::Expired;
        }

        inner.pairing_consumed = true;
        PairingOutcome::Paired
    }

    pub fn issue_session_cookie(&self) -> OwnerSessionCookie {
        let inner = self.inner.lock().expect("owner auth mutex poisoned");
        OwnerSessionCookie(inner.session_token.clone())
    }

    pub fn authenticate_headers(&self, headers: &HeaderMap) -> Result<(), StatusCode> {
        let (pairing_consumed, expected_cookie, expected_bearer) = {
            let inner = self.inner.lock().expect("owner auth mutex poisoned");
            (
                inner.pairing_consumed,
                inner.session_token.clone(),
                inner.bearer_token.clone(),
            )
        };

        if bearer_header_matches(headers, &expected_bearer) {
            return Ok(());
        }

        if !pairing_consumed {
            return Err(StatusCode::UNAUTHORIZED);
        }

        if cookie_header_matches(headers, &expected_cookie) {
            return Ok(());
        }

        Err(StatusCode::UNAUTHORIZED)
    }

    pub fn issue_bearer_token(&self) -> BearerAuthToken {
        // CONTRACT: Bearer auth is a narrow owner-auth path for CLI or smoke
        // callers; it supplements browser cookies and must not replace them.
        // HINT: Reuse the same high-entropy owner secret material or a
        // separately generated daemon-local token.
        let inner = self.inner.lock().expect("owner auth mutex poisoned");
        BearerAuthToken(inner.bearer_token.clone())
    }

    pub fn authenticate_browser_entrypoint(
        &self,
        headers: &HeaderMap,
    ) -> Result<(), AuthRejection> {
        // CONTRACT: Browser entrypoints enforce session authentication plus
        // conservative Host/Origin checks before route handlers run.
        // HOLE escalated: exact local Host/Origin allowance remains ambiguous
        // until Phase 2 implementation chooses the loopback parsing boundary.
        self.authenticate_headers(headers)
            .map_err(|_| AuthRejection::Unauthorized)?;
        entrypoint_headers_allowed(headers)
            .then_some(())
            .ok_or(AuthRejection::Forbidden)
    }

    pub fn authenticate_websocket_upgrade(&self, headers: &HeaderMap) -> Result<(), AuthRejection> {
        // CONTRACT: Future WebSocket routes use the owner-auth gate before any
        // upgrade acceptance, even while endpoint behavior is still absent.
        // HINT normalized: route middleware shares the owner auth entrypoint
        // with HTTP requests; upgrade-specific checks remain a Phase 2 stub.
        self.authenticate_headers(headers)
            .map_err(|_| AuthRejection::Unauthorized)?;
        entrypoint_headers_allowed(headers)
            .then_some(())
            .ok_or(AuthRejection::Forbidden)
    }
}

impl Default for PairingTokenPolicy {
    fn default() -> Self {
        Self {
            ttl: DEFAULT_PAIRING_TOKEN_TTL,
        }
    }
}

impl PairingTokenPolicy {
    pub fn new(ttl: Duration) -> Self {
        Self { ttl }
    }
}

impl PairingToken {
    pub fn expose_for_owner_url(&self) -> &str {
        &self.0
    }
}

impl OwnerSessionCookie {
    pub fn as_request_cookie_header(&self) -> String {
        format!("{OWNER_COOKIE_NAME}={}", self.0)
    }

    pub fn as_set_cookie_header(&self) -> String {
        format!(
            "{OWNER_COOKIE_NAME}={}; Path=/; HttpOnly; SameSite=Lax",
            self.0
        )
    }
}

impl BearerAuthToken {
    pub fn as_authorization_header(&self) -> String {
        format!("Bearer {}", self.0)
    }
}

impl AuthRejection {
    pub fn status_code(self) -> StatusCode {
        match self {
            AuthRejection::Unauthorized => StatusCode::UNAUTHORIZED,
            AuthRejection::Forbidden => StatusCode::FORBIDDEN,
        }
    }
}

fn cookie_header_matches(headers: &HeaderMap, expected: &str) -> bool {
    headers.get_all(header::COOKIE).iter().any(|value| {
        let Ok(cookie_header) = value.to_str() else {
            return false;
        };
        cookie_header.split(';').any(|cookie| {
            let cookie = cookie.trim();
            let Some((name, value)) = cookie.split_once('=') else {
                return false;
            };
            name == OWNER_COOKIE_NAME && value == expected
        })
    })
}

fn bearer_header_matches(headers: &HeaderMap, expected: &str) -> bool {
    headers.get_all(header::AUTHORIZATION).iter().any(|value| {
        let Ok(auth_header) = value.to_str() else {
            return false;
        };
        let Some((scheme, token)) = auth_header.split_once(' ') else {
            return false;
        };
        scheme.eq_ignore_ascii_case("Bearer") && token.trim() == expected
    })
}

fn entrypoint_headers_allowed(headers: &HeaderMap) -> bool {
    header_values_allowed(headers, header::HOST, is_allowed_host)
        && header_values_allowed(headers, header::ORIGIN, is_allowed_origin)
}

fn header_values_allowed(
    headers: &HeaderMap,
    name: header::HeaderName,
    allowed: fn(&str) -> bool,
) -> bool {
    headers.get_all(name).iter().all(|value| {
        let Ok(value) = value.to_str() else {
            return false;
        };
        allowed(value)
    })
}

fn is_allowed_origin(origin: &str) -> bool {
    let origin = origin.trim();
    let Some(authority_and_path) = origin
        .strip_prefix("http://")
        .or_else(|| origin.strip_prefix("https://"))
    else {
        return false;
    };

    let authority = authority_and_path
        .split(['/', '?', '#'])
        .next()
        .unwrap_or_default();
    is_allowed_host(authority)
}

fn is_allowed_host(host: &str) -> bool {
    let host = host.trim();
    if host.is_empty() {
        return false;
    }

    if let Some(ip_literal) = bracketed_ipv6_host(host) {
        return ip_literal
            .parse::<IpAddr>()
            .map(|ip| ip.is_loopback())
            .unwrap_or(false);
    }

    if host
        .parse::<IpAddr>()
        .map(|ip| ip.is_loopback())
        .unwrap_or(false)
    {
        return true;
    }

    let authority_host = host_without_port(host);
    authority_host.eq_ignore_ascii_case("localhost")
        || authority_host
            .parse::<IpAddr>()
            .map(|ip| ip.is_loopback())
            .unwrap_or(false)
}

fn bracketed_ipv6_host(host: &str) -> Option<&str> {
    let rest = host.strip_prefix('[')?;
    let (ip, suffix) = rest.split_once(']')?;
    if suffix.is_empty() || valid_port_suffix(suffix) {
        Some(ip)
    } else {
        None
    }
}

fn host_without_port(host: &str) -> &str {
    let Some((candidate_host, port)) = host.rsplit_once(':') else {
        return host;
    };

    if candidate_host.contains(':') || !port.chars().all(|ch| ch.is_ascii_digit()) {
        return host;
    }

    candidate_host
}

fn valid_port_suffix(suffix: &str) -> bool {
    let Some(port) = suffix.strip_prefix(':') else {
        return false;
    };
    !port.is_empty() && port.chars().all(|ch| ch.is_ascii_digit())
}

fn random_secret() -> String {
    let mut bytes = [0_u8; TOKEN_BYTES];
    rand::rngs::OsRng.fill_bytes(&mut bytes);
    hex_encode(&bytes)
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}
