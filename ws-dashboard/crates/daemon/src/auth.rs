use std::sync::{Arc, Mutex};
use std::time::Duration;

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
    session_token: String,
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

    pub fn new_ephemeral_with_policy(_policy: PairingTokenPolicy) -> Self {
        // CONTRACT: Phase 2 construction accepts a token policy so tests and
        // daemon startup can verify expiry behavior without sleeping.
        // HOLE filled: this constructor is the single project-local startup
        // path for ephemeral secrets; TTL enforcement remains Phase 2 logic.
        Self {
            pairing_token: PairingToken(random_secret()),
            inner: Arc::new(Mutex::new(AuthInner {
                pairing_consumed: false,
                session_token: random_secret(),
            })),
        }
    }

    pub fn pairing_token(&self) -> &PairingToken {
        &self.pairing_token
    }

    pub fn consume_pairing_token(&self, candidate: &str) -> PairingOutcome {
        let mut inner = self.inner.lock().expect("owner auth mutex poisoned");
        if inner.pairing_consumed {
            return PairingOutcome::AlreadyUsed;
        }

        if candidate == self.pairing_token.0 {
            inner.pairing_consumed = true;
            PairingOutcome::Paired
        } else {
            PairingOutcome::Invalid
        }
    }

    pub fn issue_session_cookie(&self) -> OwnerSessionCookie {
        let inner = self.inner.lock().expect("owner auth mutex poisoned");
        OwnerSessionCookie(inner.session_token.clone())
    }

    pub fn authenticate_headers(&self, headers: &HeaderMap) -> Result<(), StatusCode> {
        let expected = {
            let inner = self.inner.lock().expect("owner auth mutex poisoned");
            if !inner.pairing_consumed {
                return Err(StatusCode::UNAUTHORIZED);
            }
            inner.session_token.clone()
        };

        for value in headers.get_all(header::COOKIE) {
            let Ok(cookie_header) = value.to_str() else {
                continue;
            };
            if cookie_header.split(';').any(|cookie| {
                let cookie = cookie.trim();
                let Some((name, value)) = cookie.split_once('=') else {
                    return false;
                };
                name == OWNER_COOKIE_NAME && value == expected
            }) {
                return Ok(());
            }
        }

        Err(StatusCode::UNAUTHORIZED)
    }

    pub fn issue_bearer_token(&self) -> BearerAuthToken {
        // CONTRACT: Bearer auth is a narrow owner-auth path for CLI or smoke
        // callers; it supplements browser cookies and must not replace them.
        // HINT: Reuse the same high-entropy owner secret material or a
        // separately generated daemon-local token.
        unimplemented!("Phase 2 skeleton: issue narrow bearer auth token")
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
            .map_err(|_| AuthRejection::Unauthorized)
    }

    pub fn authenticate_websocket_upgrade(&self, headers: &HeaderMap) -> Result<(), AuthRejection> {
        // CONTRACT: Future WebSocket routes use the owner-auth gate before any
        // upgrade acceptance, even while endpoint behavior is still absent.
        // HINT normalized: route middleware shares the owner auth entrypoint
        // with HTTP requests; upgrade-specific checks remain a Phase 2 stub.
        self.authenticate_headers(headers)
            .map_err(|_| AuthRejection::Unauthorized)
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
