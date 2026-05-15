use axum::http::{HeaderMap, StatusCode};

#[derive(Clone, Debug)]
pub struct OwnerAuthState {
    // CONTRACT: Pairing token is startup-generated, one-time, and the only
    // unauthenticated browser path accepted by the daemon.
    pairing_token: PairingToken,
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
        // HOLE: Generate high-entropy token material without logging it through
        // request traces. Phase 2 can add TTL/persistence without changing the
        // route contract.
        todo!("create startup owner auth state")
    }

    pub fn pairing_token(&self) -> &PairingToken {
        &self.pairing_token
    }

    pub fn consume_pairing_token(&self, candidate: &str) -> PairingOutcome {
        // CONTRACT: A valid `/pair` exchange consumes the startup token and
        // enables issuing an owner session cookie.
        let _ = candidate;
        todo!("consume one-time pairing token")
    }

    pub fn issue_session_cookie(&self) -> OwnerSessionCookie {
        // CONTRACT: Browser auth is represented as a normal HTTP-only session
        // cookie, not as bearer-only navigation.
        todo!("issue owner session cookie")
    }

    pub fn authenticate_headers(&self, headers: &HeaderMap) -> Result<(), StatusCode> {
        // CONTRACT: `/healthz`, static UI, and future WebSocket upgrades all
        // reject unauthenticated requests before reaching handlers.
        let _ = headers;
        todo!("validate owner session cookie or narrow CLI bearer path")
    }
}

impl PairingToken {
    pub fn expose_for_owner_url(&self) -> &str {
        &self.0
    }
}
