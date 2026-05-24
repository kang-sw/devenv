use std::collections::{HashMap, HashSet};
use std::sync::Arc;
use std::time::Duration;

use axum::extract::State;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use serde::{Deserialize, Serialize};
use serde_json::{json, Value};
use sha2::{Digest, Sha256};
use tokio::sync::Mutex;

use crate::router::AppState;

pub const BLOCK_MODEL_VERSION: &str = "ws-dashboard-document-block-v1";
pub const PROMPT_VERSION: &str = "ws-dashboard-translation-prompt-v1";
const PROVIDER_KIND: &str = "llmOpenAICompatible";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TranslationProviderConfig {
    pub id: String,
    pub label: String,
    pub base_url: String,
    pub api_key: Option<String>,
    pub default_model: Option<String>,
    pub timeout_ms: u64,
}

impl TranslationProviderConfig {
    pub fn config_version(&self) -> String {
        let mut hasher = Sha256::new();
        hasher.update(self.id.as_bytes());
        hasher.update(PROVIDER_KIND.as_bytes());
        hasher.update(self.base_url.as_bytes());
        hasher.update(self.default_model.as_deref().unwrap_or_default().as_bytes());
        format_sha256(hasher.finalize().as_slice())
    }
}

#[derive(Clone)]
pub struct DocumentTranslationService {
    provider: Option<TranslationProviderConfig>,
    client: reqwest::Client,
    cache: Arc<Mutex<HashMap<String, DocumentTranslationResponse>>>,
}

impl Default for DocumentTranslationService {
    fn default() -> Self {
        Self::new(None)
    }
}

impl DocumentTranslationService {
    pub fn new(provider: Option<TranslationProviderConfig>) -> Self {
        Self {
            provider,
            client: reqwest::Client::new(),
            cache: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    pub fn from_env() -> Self {
        let base_url = std::env::var("WS_DASHBOARD_TRANSLATION_OPENAI_BASE_URL").ok();
        let provider = base_url.map(|base_url| TranslationProviderConfig {
            id: std::env::var("WS_DASHBOARD_TRANSLATION_PROVIDER_ID")
                .unwrap_or_else(|_| "local-ollama".to_owned()),
            label: std::env::var("WS_DASHBOARD_TRANSLATION_PROVIDER_LABEL")
                .unwrap_or_else(|_| "Local Ollama".to_owned()),
            base_url,
            api_key: std::env::var("WS_DASHBOARD_TRANSLATION_OPENAI_API_KEY").ok(),
            default_model: std::env::var("WS_DASHBOARD_TRANSLATION_OPENAI_MODEL").ok(),
            timeout_ms: std::env::var("WS_DASHBOARD_TRANSLATION_TIMEOUT_MS")
                .ok()
                .and_then(|value| value.parse().ok())
                .unwrap_or(30_000),
        });
        Self::new(provider)
    }
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TranslationProvidersResponse {
    pub providers: Vec<TranslationProviderStatus>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct TranslationProviderStatus {
    pub id: String,
    pub kind: String,
    pub label: String,
    pub configured: bool,
    pub reachable: bool,
    pub models: Vec<TranslationModelView>,
    pub default_model: Option<String>,
    pub error: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct TranslationModelView {
    pub id: String,
    pub label: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslationRequest {
    pub source: DocumentTranslationSource,
    pub provider: Option<DocumentTranslationProviderChoice>,
    pub locale: DocumentTranslationLocale,
    pub blocks: Vec<DocumentBlock>,
    pub requested_block_ids: Option<Vec<String>>,
    pub cache_policy: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslationSource {
    pub kind: String,
    pub work_root_id: Option<String>,
    pub path: Option<String>,
    pub content_hash: String,
    pub format: String,
    pub title: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct DocumentTranslationProviderChoice {
    pub id: String,
    pub model: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
pub struct DocumentTranslationLocale {
    pub source: Option<String>,
    pub target: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentBlock {
    pub block_id: String,
    pub ordinal: usize,
    pub kind: String,
    pub markdown: String,
    pub plain_text: String,
    pub line_start: Option<usize>,
    pub line_end: Option<usize>,
    pub pathref: Option<String>,
    pub translatable: bool,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslationResponse {
    pub source_content_hash: String,
    pub target_locale: String,
    pub status: String,
    pub cache: DocumentTranslationCacheView,
    pub blocks: Vec<DocumentTranslatedBlock>,
    pub unmatched: Vec<DocumentTranslationUnmatched>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslationCacheView {
    pub hit: bool,
    pub provider_id: String,
    pub provider_kind: String,
    pub model: String,
    pub provider_config_version: String,
    pub block_model_version: String,
    pub prompt_version: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslatedBlock {
    pub block_id: String,
    pub translated_markdown: Option<String>,
    pub translated_plain_text: Option<String>,
    pub status: String,
    pub note: Option<String>,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DocumentTranslationUnmatched {
    pub ordinal: usize,
    pub text: String,
    pub reason: String,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
struct TranslationErrorBody {
    error: String,
}

pub async fn translation_providers(State(state): State<AppState>) -> Response {
    let Some(provider) = &state.document_translation.provider else {
        return Json(TranslationProvidersResponse { providers: vec![] }).into_response();
    };
    let (reachable, models, error) = match fetch_models(&state.document_translation, provider).await
    {
        Ok(models) => (true, models, None),
        Err(error) => (false, vec![], Some(error)),
    };
    Json(TranslationProvidersResponse {
        providers: vec![TranslationProviderStatus {
            id: provider.id.clone(),
            kind: PROVIDER_KIND.to_owned(),
            label: provider.label.clone(),
            configured: true,
            reachable,
            models,
            default_model: provider.default_model.clone(),
            error,
        }],
    })
    .into_response()
}

pub async fn translate_document(
    State(state): State<AppState>,
    Json(request): Json<DocumentTranslationRequest>,
) -> Response {
    match translate(&state.document_translation, request).await {
        Ok(response) => Json(response).into_response(),
        Err((status, message)) => translation_error(status, message),
    }
}

async fn translate(
    service: &DocumentTranslationService,
    request: DocumentTranslationRequest,
) -> Result<DocumentTranslationResponse, (StatusCode, &'static str)> {
    let Some(provider) = &service.provider else {
        return Err((
            StatusCode::SERVICE_UNAVAILABLE,
            "translation provider not configured",
        ));
    };
    validate_request(&request)?;
    if let Some(choice) = &request.provider {
        if choice.id != provider.id {
            return Err((StatusCode::BAD_REQUEST, "unknown translation provider"));
        }
    }
    let model = match request
        .provider
        .as_ref()
        .and_then(|provider| provider.model.clone())
        .or_else(|| provider.default_model.clone())
    {
        Some(model) => model,
        None => fetch_models(service, provider)
            .await
            .ok()
            .and_then(|models| models.into_iter().next().map(|model| model.id))
            .ok_or((
                StatusCode::SERVICE_UNAVAILABLE,
                "translation model unavailable",
            ))?,
    };
    let cache_key = cache_key(provider, &model, &request);
    if request.cache_policy.as_deref() != Some("refresh") {
        if let Some(cached) = service.cache.lock().await.get(&cache_key).cloned() {
            let mut cached = cached;
            cached.cache.hit = true;
            return Ok(cached);
        }
    }

    let prompt = build_prompt(&request);
    let content = call_openai_compatible(service, provider, &model, &prompt)
        .await
        .map_err(|_| (StatusCode::BAD_GATEWAY, "translation provider failed"))?;
    let mut response = parse_translation_content(&request, provider, &model, &content);
    response.cache.hit = false;
    service
        .cache
        .lock()
        .await
        .insert(cache_key, response.clone());
    Ok(response)
}

fn validate_request(
    request: &DocumentTranslationRequest,
) -> Result<(), (StatusCode, &'static str)> {
    if request.source.kind != "workRootFile"
        || request
            .source
            .work_root_id
            .as_deref()
            .unwrap_or_default()
            .trim()
            .is_empty()
        || !is_safe_work_root_relative_path(request.source.path.as_deref().unwrap_or_default())
    {
        return Err((StatusCode::BAD_REQUEST, "unsupported translation source"));
    }
    if request.source.content_hash.trim().is_empty() || request.locale.target.trim().is_empty() {
        return Err((StatusCode::BAD_REQUEST, "invalid translation request"));
    }
    if request.source.format != "markdown" && request.source.format != "text" {
        return Err((StatusCode::BAD_REQUEST, "unsupported document format"));
    }
    let mut seen = HashSet::new();
    for block in &request.blocks {
        if block.block_id.trim().is_empty() || !seen.insert(block.block_id.clone()) {
            return Err((StatusCode::BAD_REQUEST, "invalid document block ids"));
        }
    }
    Ok(())
}

fn cache_key(
    provider: &TranslationProviderConfig,
    model: &str,
    request: &DocumentTranslationRequest,
) -> String {
    let mut hasher = Sha256::new();
    hasher.update(request.source.content_hash.as_bytes());
    hasher.update(request.locale.target.as_bytes());
    hasher.update(provider.id.as_bytes());
    hasher.update(PROVIDER_KIND.as_bytes());
    hasher.update(model.as_bytes());
    hasher.update(provider.config_version().as_bytes());
    hasher.update(BLOCK_MODEL_VERSION.as_bytes());
    hasher.update(PROMPT_VERSION.as_bytes());
    format_sha256(hasher.finalize().as_slice())
}

fn build_prompt(request: &DocumentTranslationRequest) -> String {
    let blocks: Vec<_> = request
        .blocks
        .iter()
        .filter(|block| block.translatable)
        .map(|block| json!({ "blockId": block.block_id, "content": block.plain_text }))
        .collect();
    json!({
        "instruction": "Translate each block to the target locale. Return only JSON with blocks array containing blockId and translatedContent strings.",
        "targetLocale": request.locale.target,
        "sourceLocale": request.locale.source,
        "documentTitle": request.source.title,
        "blocks": blocks,
    })
    .to_string()
}

async fn fetch_models(
    service: &DocumentTranslationService,
    provider: &TranslationProviderConfig,
) -> Result<Vec<TranslationModelView>, String> {
    let url = format!("{}/models", provider.base_url.trim_end_matches('/'));
    let mut request = service
        .client
        .get(url)
        .timeout(Duration::from_millis(provider.timeout_ms));
    if let Some(api_key) = &provider.api_key {
        request = request.bearer_auth(api_key);
    }
    let response = request
        .send()
        .await
        .map_err(|_| "model probe failed".to_owned())?;
    if !response.status().is_success() {
        return Err("model probe failed".to_owned());
    }
    let value: Value = response
        .json()
        .await
        .map_err(|_| "model probe invalid".to_owned())?;
    Ok(value
        .get("data")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .filter_map(|item| item.get("id").and_then(Value::as_str))
        .map(|id| TranslationModelView {
            id: id.to_owned(),
            label: None,
        })
        .collect())
}

async fn call_openai_compatible(
    service: &DocumentTranslationService,
    provider: &TranslationProviderConfig,
    model: &str,
    prompt: &str,
) -> Result<String, reqwest::Error> {
    let url = format!(
        "{}/chat/completions",
        provider.base_url.trim_end_matches('/')
    );
    let body = json!({
        "model": model,
        "messages": [
            {"role": "system", "content": "You are a document translation engine. Return JSON only."},
            {"role": "user", "content": prompt}
        ],
        "temperature": 0
    });
    let mut request = service
        .client
        .post(url)
        .timeout(Duration::from_millis(provider.timeout_ms))
        .json(&body);
    if let Some(api_key) = &provider.api_key {
        request = request.bearer_auth(api_key);
    }
    let response: Value = request.send().await?.error_for_status()?.json().await?;
    Ok(response
        .get("choices")
        .and_then(Value::as_array)
        .and_then(|choices| choices.first())
        .and_then(|choice| choice.get("message"))
        .and_then(|message| message.get("content"))
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned())
}

fn parse_translation_content(
    request: &DocumentTranslationRequest,
    provider: &TranslationProviderConfig,
    model: &str,
    content: &str,
) -> DocumentTranslationResponse {
    let mut by_block: HashMap<String, String> = HashMap::new();
    let mut unmatched = Vec::new();
    if let Ok(value) = serde_json::from_str::<Value>(content) {
        let entries = value
            .get("blocks")
            .and_then(Value::as_array)
            .cloned()
            .or_else(|| value.as_array().cloned())
            .unwrap_or_default();
        let known: HashSet<_> = request
            .blocks
            .iter()
            .map(|block| block.block_id.as_str())
            .collect();
        for (ordinal, entry) in entries.into_iter().enumerate() {
            let Some(block_id) = entry.get("blockId").and_then(Value::as_str) else {
                unmatched.push(DocumentTranslationUnmatched {
                    ordinal,
                    text: "omitted".to_owned(),
                    reason: "missing block id".to_owned(),
                });
                continue;
            };
            let Some(text) = entry
                .get("translatedContent")
                .or_else(|| entry.get("translatedMarkdown"))
                .and_then(Value::as_str)
            else {
                unmatched.push(DocumentTranslationUnmatched {
                    ordinal,
                    text: block_id.to_owned(),
                    reason: "missing translation".to_owned(),
                });
                continue;
            };
            if !known.contains(block_id) {
                unmatched.push(DocumentTranslationUnmatched {
                    ordinal,
                    text: block_id.to_owned(),
                    reason: "unknown block id".to_owned(),
                });
                continue;
            }
            if by_block.contains_key(block_id) {
                unmatched.push(DocumentTranslationUnmatched {
                    ordinal,
                    text: block_id.to_owned(),
                    reason: "duplicate block id".to_owned(),
                });
                continue;
            }
            by_block.insert(block_id.to_owned(), text.to_owned());
        }
    } else {
        unmatched.push(DocumentTranslationUnmatched {
            ordinal: 0,
            text: "omitted".to_owned(),
            reason: "unparseable provider response".to_owned(),
        });
    }

    let blocks: Vec<_> = request
        .blocks
        .iter()
        .map(|block| {
            if !block.translatable {
                return DocumentTranslatedBlock {
                    block_id: block.block_id.clone(),
                    translated_markdown: None,
                    translated_plain_text: None,
                    status: "omitted".to_owned(),
                    note: Some("non-translatable block".to_owned()),
                };
            }
            match by_block.get(&block.block_id) {
                Some(text) => DocumentTranslatedBlock {
                    block_id: block.block_id.clone(),
                    translated_markdown: Some(text.clone()),
                    translated_plain_text: Some(text.clone()),
                    status: "ok".to_owned(),
                    note: None,
                },
                None => DocumentTranslatedBlock {
                    block_id: block.block_id.clone(),
                    translated_markdown: None,
                    translated_plain_text: None,
                    status: "failed".to_owned(),
                    note: Some("translation missing".to_owned()),
                },
            }
        })
        .collect();
    let ok = blocks.iter().filter(|block| block.status == "ok").count();
    let failed = blocks
        .iter()
        .filter(|block| block.status == "failed")
        .count();
    let status = if failed == 0 && unmatched.is_empty() {
        "completed"
    } else if ok > 0 {
        "partial"
    } else {
        "failed"
    };
    DocumentTranslationResponse {
        source_content_hash: request.source.content_hash.clone(),
        target_locale: request.locale.target.clone(),
        status: status.to_owned(),
        cache: DocumentTranslationCacheView {
            hit: false,
            provider_id: provider.id.clone(),
            provider_kind: PROVIDER_KIND.to_owned(),
            model: model.to_owned(),
            provider_config_version: provider.config_version(),
            block_model_version: BLOCK_MODEL_VERSION.to_owned(),
            prompt_version: PROMPT_VERSION.to_owned(),
        },
        blocks,
        unmatched,
    }
}

fn is_safe_work_root_relative_path(path: &str) -> bool {
    !path.is_empty()
        && !path.starts_with('/')
        && !path.starts_with('~')
        && !path.contains('\\')
        && !path.split('/').any(|segment| segment == "..")
        && !path
            .get(..2)
            .map(|prefix| {
                prefix.ends_with(':') && prefix[..1].chars().all(|ch| ch.is_ascii_alphabetic())
            })
            .unwrap_or(false)
}

fn translation_error(status: StatusCode, message: &str) -> Response {
    (
        status,
        Json(TranslationErrorBody {
            error: message.to_owned(),
        }),
    )
        .into_response()
}

fn format_sha256(bytes: &[u8]) -> String {
    bytes.iter().map(|byte| format!("{byte:02x}")).collect()
}
