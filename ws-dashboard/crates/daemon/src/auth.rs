use std::sync::{Arc, Mutex};

use axum::http::{header, HeaderMap, StatusCode};
use rand::RngCore;

const OWNER_COOKIE_NAME: &str = "ws-dashboard-owner";
const TOKEN_BYTES: usize = 32;

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

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum PairingOutcome {
    Paired,
    Invalid,
    AlreadyUsed,
}

impl OwnerAuthState {
    pub fn new_ephemeral() -> Self {
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
