// CONTRACT: Router smoke tests for Phase 1 live here.
// HINT: Use `tower::ServiceExt` against `router::build_router` rather than
// binding sockets.
//
// Required behavior targets:
// - `/pair` is reachable without an existing owner session.
// - `/healthz` rejects before pairing.
// - valid pairing installs an HTTP-only owner session cookie.
// - `/healthz` and `/` succeed with the owner session cookie.
// - health output stays minimal and does not expose token, host paths, cache
//   paths, Git roots, wsstate internals, or diagnostics.

use axum::body::Body;
use axum::http::{Request, StatusCode};
use tower::ServiceExt;
use ws_dashboard_daemon::auth::OwnerAuthState;
use ws_dashboard_daemon::config::ServeConfig;
use ws_dashboard_daemon::router::{build_router, AppState};

fn app_state() -> AppState {
    AppState {
        config: ServeConfig::default_loopback(),
        auth: OwnerAuthState::new_ephemeral(),
    }
}

fn owner_cookie_header() -> &'static str {
    "ws-dashboard-owner=placeholder"
}

#[tokio::test]
#[ignore = "Phase 1 skeleton scaffold: router behavior is implemented by the next pass"]
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
#[ignore = "Phase 1 skeleton scaffold: router behavior is implemented by the next pass"]
async fn health_rejects_before_pairing() {
    let app = build_router(app_state());

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .body(Body::empty())
                .expect("health request"),
        )
        .await
        .expect("health response");

    assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
}

#[tokio::test]
#[ignore = "Phase 1 skeleton scaffold: pairing/session behavior is implemented by the next pass"]
async fn valid_pairing_installs_http_only_owner_session_cookie() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let app = build_router(state);

    let response = app
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
        .get(axum::http::header::SET_COOKIE)
        .expect("owner session cookie");
    assert!(set_cookie
        .to_str()
        .expect("cookie header is ASCII")
        .contains("HttpOnly"));
}

#[tokio::test]
#[ignore = "Phase 1 skeleton scaffold: authenticated route behavior is implemented by the next pass"]
async fn health_and_static_ui_succeed_with_owner_session_cookie() {
    let state = app_state();
    let _cookie = state.auth.issue_session_cookie();
    let app = build_router(state);

    for uri in ["/healthz", "/"] {
        let response = app
            .clone()
            .oneshot(
                Request::builder()
                    .uri(uri)
                    .header(axum::http::header::COOKIE, owner_cookie_header())
                    .body(Body::empty())
                    .expect("authenticated request"),
            )
            .await
            .expect("authenticated response");

        assert_eq!(response.status(), StatusCode::OK);
    }
}

#[tokio::test]
#[ignore = "Phase 1 skeleton scaffold: minimal health behavior is implemented by the next pass"]
async fn health_output_stays_minimal() {
    let state = app_state();
    let token = state.auth.pairing_token().expose_for_owner_url().to_owned();
    let _cookie = state.auth.issue_session_cookie();
    let app = build_router(state);

    let response = app
        .oneshot(
            Request::builder()
                .uri("/healthz")
                .header(axum::http::header::COOKIE, owner_cookie_header())
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

    for forbidden in [token.as_str(), "wsstate", "target", "cache", "git"] {
        assert!(!body.contains(forbidden));
    }
}
