// CONTRACT: Router smoke tests for Phase 1 live here.
// HINT: Use `tower::ServiceExt` against `router::build_router` rather than
// binding sockets.
//
// Required behavior targets:
// - `/pair` is reachable without an existing owner session.
// - `/healthz` rejects before pairing.
// - valid pairing installs an HTTP-only owner session cookie.
// - invalid, reused, missing, and expired pairing tokens do not install a
//   session.
// - `/healthz` and `/` succeed with the owner session cookie.
// - browser Host/Origin checks reject clearly invalid entrypoints without
//   weakening ordinary loopback usage.
// - narrow bearer auth supports CLI/smoke callers without replacing cookies.
// - future WebSocket upgrade paths reject unauthenticated requests before
//   endpoint behavior is considered.
// - health output stays minimal and does not expose token, host paths, cache
//   paths, Git roots, wsstate internals, or diagnostics.
// - `/api/dashboard/resources` is protected and returns the same deterministic
//   dashboard hierarchy contract that frontend work will consume.

use std::time::Duration;

use axum::body::Body;
use axum::http::{header, Request, StatusCode};
use tower::ServiceExt;
use ws_dashboard_daemon::auth::{OwnerAuthState, PairingTokenPolicy};
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::router::{build_router, AppState};

fn app_state() -> AppState {
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
    }
}

async fn pair_and_cookie(app: axum::Router, token: &str) -> String {
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");

    assert_eq!(response.status(), StatusCode::OK);
    response
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie")
        .to_str()
        .expect("cookie header is ASCII")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned()
}

async fn pair_response(app: axum::Router, token: Option<&str>) -> axum::response::Response {
    let uri = token
        .map(|token| format!("/pair?token={token}"))
        .unwrap_or_else(|| "/pair".to_owned());

    app.oneshot(
        Request::builder()
            .uri(uri)
            .body(Body::empty())
            .expect("pair request"),
    )
    .await
    .expect("pair response")
}

#[tokio::test]
async fn pair_is_the_only_unauthenticated_browser_route() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/pair")
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");

    assert_ne!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn non_pair_browser_routes_reject_before_pairing() {
    let app = build_router(app_state());

    for uri in ["/healthz", "/", "/favicon.ico"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .body(Body::empty())
                    .expect("unauthenticated request"),
            )
            .await
            .expect("unauthenticated response");

        assert_eq!(response.status(), StatusCode::UNAUTHORIZED, "{uri}");
    }
}

#[tokio::test]
async fn valid_pairing_installs_http_only_owner_session_cookie_once() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let response = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");

    let set_cookie = response
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie");
    let set_cookie = set_cookie.to_str().expect("cookie header is ASCII");
    assert!(set_cookie.contains("ws-dashboard-owner="));
    assert!(set_cookie.contains("HttpOnly"));

    let reused = app
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("reused pair request"),
        )
        .await
        .expect("reused pair response");

    assert_eq!(reused.status(), StatusCode::GONE);
    assert!(reused.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn invalid_missing_and_reused_pairing_tokens_do_not_install_sessions() {
    let missing = pair_response(build_router(app_state()), None).await;
    assert_eq!(missing.status(), StatusCode::BAD_REQUEST);
    assert!(missing.headers().get(header::SET_COOKIE).is_none());

    let invalid = pair_response(build_router(app_state()), Some("not-the-token")).await;
    assert_eq!(invalid.status(), StatusCode::UNAUTHORIZED);
    assert!(invalid.headers().get(header::SET_COOKIE).is_none());

    let reused_state = app_state();
    let token = reused_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let reused_app = build_router(reused_state);
    let first = pair_response(reused_app.clone(), Some(&token)).await;
    assert_eq!(first.status(), StatusCode::OK);
    let reused = pair_response(reused_app, Some(&token)).await;
    assert_eq!(reused.status(), StatusCode::GONE);
    assert!(reused.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn expired_pairing_tokens_do_not_install_sessions() {
    // CONTRACT: A zero TTL is the deterministic expired-token fixture.
    let expired_state = AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral_with_policy(PairingTokenPolicy::new(Duration::ZERO)),
    };
    let expired_token = expired_state
        .auth
        .pairing_token()
        .expose_for_owner_url()
        .to_owned();
    let expired = pair_response(build_router(expired_state), Some(&expired_token)).await;
    assert_eq!(expired.status(), StatusCode::UNAUTHORIZED);
    assert!(expired.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn health_and_static_ui_succeed_with_owner_session_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for uri in ["/healthz", "/"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(header::COOKIE, cookie.as_str())
                    .body(Body::empty())
                    .expect("authenticated request"),
            )
            .await
            .expect("authenticated response");

        assert_eq!(response.status(), StatusCode::OK);
    }
}

#[tokio::test]
async fn dashboard_resources_api_is_owner_authenticated() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/api/dashboard/resources")
                .body(Body::empty())
                .expect("unauthenticated dashboard resources request"),
        )
        .await
        .expect("unauthenticated dashboard resources response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn dashboard_resources_api_returns_mock_hierarchy_with_owner_cookie() {
    // CONTRACT: paired owners receive deterministic JSON with server,
    // workspaces, workRoots, mainInstances, subInstances, state, compactable,
    // and action hint fields. HINT: parse with serde_json and assert contract
    // field names rather than depending on private Rust structs.
    todo!("pair owner, call /api/dashboard/resources, and assert mock hierarchy JSON");
}

#[tokio::test]
async fn browser_auth_accepts_loopback_host_and_origin_with_owner_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let loopback_host = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::HOST, "127.0.0.1:3000")
                .body(Body::empty())
                .expect("loopback host request"),
        )
        .await
        .expect("loopback host response");
    assert_eq!(loopback_host.status(), StatusCode::OK);

    let loopback_origin = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::ORIGIN, "http://localhost:3000")
                .body(Body::empty())
                .expect("loopback origin request"),
        )
        .await
        .expect("loopback origin response");
    assert_eq!(loopback_origin.status(), StatusCode::OK);

    let loopback_host_and_origin = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .header(header::HOST, "localhost:3000")
                .header(header::ORIGIN, "http://127.0.0.1:3000")
                .body(Body::empty())
                .expect("loopback host and origin request"),
        )
        .await
        .expect("loopback host and origin response");
    assert_eq!(loopback_host_and_origin.status(), StatusCode::OK);
}

#[tokio::test]
async fn browser_auth_rejects_invalid_host_and_origin_with_owner_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    for (header_name, header_value) in [
        (header::HOST, "evil.example.test"),
        (header::ORIGIN, "https://evil.example.test"),
    ] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri("/healthz")
                    .header(header::COOKIE, cookie.as_str())
                    .header(&header_name, header_value)
                    .body(Body::empty())
                    .expect("invalid browser entrypoint request"),
            )
            .await
            .expect("invalid browser entrypoint response");

        assert_eq!(response.status(), StatusCode::FORBIDDEN, "{header_name}");
    }
}

#[tokio::test]
async fn bearer_auth_can_access_http_smoke_routes_without_cookie() {
    let state = app_state();
    let bearer = state.auth.issue_bearer_token().as_authorization_header();
    let app = build_router(state);

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::AUTHORIZATION, bearer)
                .body(Body::empty())
                .expect("bearer health request"),
        )
        .await
        .expect("bearer health response");

    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn invalid_bearer_auth_cannot_access_http_smoke_routes_without_cookie() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::AUTHORIZATION, "Bearer not-the-owner-token")
                .body(Body::empty())
                .expect("invalid bearer health request"),
        )
        .await
        .expect("invalid bearer health response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
    assert!(response.headers().get(header::SET_COOKIE).is_none());
}

#[tokio::test]
async fn websocket_upgrade_requests_are_auth_gated_before_upgrade_acceptance() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/ws")
                .header(header::UPGRADE, "websocket")
                .header(header::CONNECTION, "upgrade")
                .header(header::HOST, "127.0.0.1")
                .body(Body::empty())
                .expect("websocket upgrade request"),
        )
        .await
        .expect("websocket upgrade response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
async fn health_output_stays_minimal() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);
    let cookie = pair_and_cookie(app.clone(), &token).await;

    let session_value = cookie
        .split_once('=')
        .expect("cookie name and value")
        .1
        .to_owned();

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("health request"),
        )
        .await
        .expect("health response");

    assert_eq!(response.status(), StatusCode::OK);
    let body = axum::body::to_bytes(response.into_body(), 1024)
        .await
        .expect("health body bytes");
    let body = std::str::from_utf8(&body).expect("health body utf8");

    assert_eq!(body, "ok\n");
    for forbidden in [
        token.as_str(),
        session_value.as_str(),
        "wsstate",
        "target",
        "cache",
        "git",
        "diagnostic",
    ] {
        assert!(!body.contains(forbidden));
    }
}

#[tokio::test]
async fn daemon_security_smoke_covers_auth_and_health_boundary() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let unauthenticated_http = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .body(Body::empty())
                .expect("unauthenticated health request"),
        )
        .await
        .expect("unauthenticated health response");
    assert_eq!(unauthenticated_http.status(), StatusCode::UNAUTHORIZED);

    let unauthenticated_websocket = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/ws")
                .header(header::UPGRADE, "websocket")
                .header(header::CONNECTION, "upgrade")
                .header(header::HOST, "127.0.0.1")
                .body(Body::empty())
                .expect("unauthenticated websocket request"),
        )
        .await
        .expect("unauthenticated websocket response");
    assert_eq!(unauthenticated_websocket.status(), StatusCode::UNAUTHORIZED);

    let pair = app
        .clone()
        .oneshot(
            Request::builder()
                .uri(format!("/pair?token={token}"))
                .body(Body::empty())
                .expect("pair request"),
        )
        .await
        .expect("pair response");
    assert_eq!(pair.status(), StatusCode::OK);
    let cookie = pair
        .headers()
        .get(header::SET_COOKIE)
        .expect("owner session cookie")
        .to_str()
        .expect("cookie header is ASCII")
        .split(';')
        .next()
        .expect("cookie pair")
        .to_owned();

    let reused_pair = pair_response(app.clone(), Some(&token)).await;
    assert_eq!(reused_pair.status(), StatusCode::GONE);
    assert!(reused_pair.headers().get(header::SET_COOKIE).is_none());

    let health = app
        .clone()
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(header::COOKIE, cookie.as_str())
                .body(Body::empty())
                .expect("authenticated health request"),
        )
        .await
        .expect("authenticated health response");
    assert_eq!(health.status(), StatusCode::OK);
    let health_body = axum::body::to_bytes(health.into_body(), 1024)
        .await
        .expect("health body bytes");
    let health_body = std::str::from_utf8(&health_body).expect("health body utf8");
    assert_eq!(health_body, "ok\n");
    for forbidden in [
        token.as_str(),
        "wsstate",
        "target",
        "cache",
        "git",
        "diagnostic",
    ] {
        assert!(!health_body.contains(forbidden));
    }

    let ui = app
        .oneshot(
            Request::builder()
                .uri("/")
                .header(header::COOKIE, cookie)
                .body(Body::empty())
                .expect("authenticated UI request"),
        )
        .await
        .expect("authenticated UI response");
    assert_eq!(ui.status(), StatusCode::OK);
}
