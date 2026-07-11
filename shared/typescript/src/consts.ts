// consts/defaults + consts/errors: request/file size caps, sniff length, the
// canonical error strings the contract asserts, and the shared error-shaping
// helper. These have no dependencies on the rest of the package.

// ── consts/defaults ─────────────────────────────────────────────────────────
export const MAX_REQUEST_BYTES: number = 10 * 1024 * 1024; // 10 MB
export const MAX_FILE_BYTES: number = 1 << 20; // 1MB
export const SNIFF_LEN = 512;
export const NULL_BYTE = 0x00;
export const DEFAULT_LIMIT = 10;

// ── consts/errors ───────────────────────────────────────────────────────────
export const INVALID_JSON_BODY = "invalid JSON body";
export const INVALID_FORM_DATA = "invalid form data";
export const EXPECTED_FORM_CONTENT_TYPE =
  "expected content-type: application/x-www-form-urlencoded or multipart/form-data";
export const INVALID_MULTIPART = "invalid multipart form data";
export const EXPECTED_MULTIPART_CONTENT_TYPE = "expected content-type: multipart/form-data";
export const FILE_NOT_FOUND = "file not found in form data";
export const NOT_FOUND = "not found";
export const FILE_SIZE_EXCEEDS = "file size exceeds limit";
export const ONLY_TEXT_PLAIN = "only text/plain files are allowed";
export const FILE_NOT_TEXT = "file does not look like plain text";
export const INTERNAL_ERROR = "internal error";
// Web-suite error strings (contract/web.json). INVALID_TOKEN covers every
// /jwt/verify rejection (missing/malformed/bad-signature/expired); VALIDATION_FAILED
// is the /validate 400; INVALID_N is the /compute boundary rejection (house
// "invalid <thing>" style).
export const INVALID_TOKEN = "invalid token";
export const VALIDATION_FAILED = "validation failed";
export const INVALID_N = "invalid n";

export type ErrorResponse = { error: string; details?: string };

export function makeError(error: string, detail?: unknown): ErrorResponse {
  if (detail instanceof Error) {
    return detail.message ? { error, details: detail.message } : { error };
  }
  if (typeof detail === "string" && detail) {
    return { error, details: detail };
  }
  return { error };
}
