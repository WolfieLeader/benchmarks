//! `/params/*` handlers: query/path/header/cookie/body echoes plus the form and
//! file upload routes. Behaviour is byte-identical to rs-axum (and the Go server)
//! — the contract asserts strict bodies for all of it. Written in actix style:
//! plain `async fn` handlers registered on a `web::scope`, extracting from
//! `HttpRequest`/`web::Path`/`web::Bytes`.

use std::collections::HashMap;

use actix_web::http::header::{CONTENT_TYPE, COOKIE};
use actix_web::web::Bytes;
use actix_web::{HttpRequest, HttpResponse, web};
use futures_util::stream;
use serde_json::{Value, json};
use shared::consts;

use crate::error::ApiError;

/// JS `Number.MAX_SAFE_INTEGER`. Integer query/form values outside `±SAFE_INT`
/// fall back to their default, matching the other servers' safe-int parsing.
const SAFE_INT: i64 = (1 << 53) - 1;

/// Register the `/params/*` routes (mounted under the `/params` scope in `main`).
pub fn config(cfg: &mut web::ServiceConfig) {
    cfg.service(
        web::scope("/params")
            .route("/search", web::get().to(search))
            .route("/url/{dynamic}", web::get().to(url))
            .route("/header", web::get().to(header))
            .route("/cookie", web::get().to(cookie))
            .route("/body", web::post().to(body))
            .route("/form", web::post().to(form))
            .route("/file", web::post().to(file)),
    );
}

fn parse_safe_int(s: &str) -> Option<i64> {
    let n: i64 = s.trim().parse().ok()?;
    (-SAFE_INT..=SAFE_INT).contains(&n).then_some(n)
}

async fn search(req: HttpRequest) -> HttpResponse {
    // First occurrence wins (matches Go's url.Values.Get).
    let mut params: HashMap<String, String> = HashMap::new();
    for (k, v) in form_urlencoded::parse(req.query_string().as_bytes()) {
        params
            .entry(k.into_owned())
            .or_insert_with(|| v.into_owned());
    }
    let search = params
        .get("q")
        .map(|s| s.trim())
        .filter(|s| !s.is_empty())
        .unwrap_or("none");
    let limit = params
        .get("limit")
        .and_then(|s| parse_safe_int(s))
        .unwrap_or(consts::DEFAULT_LIMIT);
    HttpResponse::Ok().json(json!({ "search": search, "limit": limit }))
}

async fn url(dynamic: web::Path<String>) -> HttpResponse {
    HttpResponse::Ok().json(json!({ "dynamic": dynamic.into_inner() }))
}

async fn header(req: HttpRequest) -> HttpResponse {
    let value = req
        .headers()
        .get("x-custom-header")
        .and_then(|v| v.to_str().ok())
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .unwrap_or("none");
    HttpResponse::Ok().json(json!({ "header": value }))
}

async fn cookie(req: HttpRequest) -> HttpResponse {
    let value = req
        .headers()
        .get(COOKIE)
        .and_then(|v| v.to_str().ok())
        .and_then(|raw| {
            raw.split(';').find_map(|pair| {
                let (name, val) = pair.split_once('=')?;
                (name.trim() == "foo").then(|| val.trim().to_string())
            })
        })
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| "none".to_string());
    HttpResponse::Ok()
        .insert_header(("Set-Cookie", "bar=12345; Max-Age=10; Path=/; HttpOnly"))
        .json(json!({ "cookie": value }))
}

async fn body(body: Bytes) -> Result<HttpResponse, ApiError> {
    // Deserialize into a JSON object: any top-level array/string/number/bool/null
    // fails here and becomes a 400, matching the "non-object body" cases.
    let object: serde_json::Map<String, Value> =
        serde_json::from_slice(&body).map_err(|e| ApiError::InvalidJson(e.to_string()))?;
    Ok(HttpResponse::Ok().json(json!({ "body": object })))
}

async fn form(req: HttpRequest, body: Bytes) -> Result<HttpResponse, ApiError> {
    let content_type = content_type_lower(&req);
    let fields = if content_type.starts_with("application/x-www-form-urlencoded") {
        form_urlencoded::parse(&body)
            .map(|(k, v)| (k.into_owned(), v.into_owned()))
            .collect()
    } else if content_type.starts_with("multipart/form-data") {
        multipart_text_fields(&req, body).await?
    } else {
        return Err(ApiError::InvalidForm);
    };

    let name = fields
        .get("name")
        .map(|s| s.trim())
        .filter(|s| !s.is_empty())
        .unwrap_or("none");
    let age = fields
        .get("age")
        .and_then(|s| parse_safe_int(s))
        .unwrap_or(0);
    Ok(HttpResponse::Ok().json(json!({ "name": name, "age": age })))
}

async fn file(req: HttpRequest, body: Bytes) -> Result<HttpResponse, ApiError> {
    let content_type = content_type_lower(&req);
    if !content_type.starts_with("multipart/form-data") {
        return Err(ApiError::InvalidMultipartContentType);
    }
    let mut multipart = new_multipart(&req, body).ok_or(ApiError::InvalidMultipartContentType)?;

    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|e| ApiError::FileNotFound(e.to_string()))?
    {
        if field.name() != Some("file") {
            continue;
        }
        let filename = field.file_name().map(str::to_string).unwrap_or_default();
        let declared = field.content_type().map(ToString::to_string);
        let data = field
            .bytes()
            .await
            .map_err(|e| ApiError::FileNotFound(e.to_string()))?;

        // 1 MiB per-file cap → 413.
        if data.len() > consts::MAX_FILE_BYTES {
            return Err(ApiError::FileTooLarge);
        }

        let head = &data[..data.len().min(consts::SNIFF_LEN)];
        // Gate on the declared part content type; when absent, sniff the bytes.
        match &declared {
            Some(ct) if !ct.to_ascii_lowercase().starts_with("text/plain") => {
                return Err(ApiError::InvalidFileType);
            }
            None if !looks_like_text(head) => return Err(ApiError::InvalidFileType),
            _ => {}
        }
        // Content inspection: declared-text bytes that are not plain text are
        // rejected (anti-sniffing) — null bytes or invalid UTF-8.
        if data.contains(&0) || std::str::from_utf8(&data).is_err() {
            return Err(ApiError::NotPlainText);
        }

        let content = String::from_utf8_lossy(&data).into_owned();
        return Ok(HttpResponse::Ok()
            .json(json!({ "filename": filename, "size": data.len(), "content": content })));
    }

    Err(ApiError::FileNotFound(
        "no file field in form data".to_string(),
    ))
}

fn content_type_lower(req: &HttpRequest) -> String {
    req.headers()
        .get(CONTENT_TYPE)
        .and_then(|v| v.to_str().ok())
        .unwrap_or_default()
        .to_ascii_lowercase()
}

/// `net/http`'s "binary data byte" set — the bytes that make `DetectContentType`
/// classify content as non-text. Mirrors the Go server's sniffing.
fn looks_like_text(head: &[u8]) -> bool {
    !head
        .iter()
        .any(|&b| matches!(b, 0x00..=0x08 | 0x0B | 0x0E..=0x1A | 0x1C..=0x1F))
}

/// Build a `multer` multipart reader over the already-buffered body. Returns
/// `None` when the content type carries no usable boundary. Buffered parsing (not
/// actix-multipart's streaming extractor) keeps behaviour byte-identical to
/// rs-axum; multipart parsing is infrastructure, so the two servers share it.
fn new_multipart(req: &HttpRequest, body: Bytes) -> Option<multer::Multipart<'static>> {
    let content_type = req.headers().get(CONTENT_TYPE)?.to_str().ok()?;
    let boundary = multer::parse_boundary(content_type).ok()?;
    let body_stream = stream::once(async move { Ok::<Bytes, std::convert::Infallible>(body) });
    Some(multer::Multipart::new(body_stream, boundary))
}

async fn multipart_text_fields(
    req: &HttpRequest,
    body: Bytes,
) -> Result<HashMap<String, String>, ApiError> {
    let mut multipart = new_multipart(req, body).ok_or(ApiError::InvalidForm)?;
    let mut fields = HashMap::new();
    while let Some(field) = multipart
        .next_field()
        .await
        .map_err(|_| ApiError::InvalidForm)?
    {
        if let Some(name) = field.name().map(str::to_string) {
            let text = field.text().await.map_err(|_| ApiError::InvalidForm)?;
            fields.insert(name, text);
        }
    }
    Ok(fields)
}
