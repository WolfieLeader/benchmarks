# Web-suite constants and error strings, kept IN-SERVER (not in shared/python)
# under the multi-consumer rule (docs/languages/python.md §10; PLAN §3). py-flask
# is the first and only web-suite implementer today, so these single-consumer
# canon strings live here until a second Python web server (py-fastapi/py-django
# gaining `web: true`) triggers extraction into shared bench_shared.errors.

# Web error strings — contract canon: the /validate, /jwt/verify and /compute
# referees assert these exact values (contract/web.json). Same house "invalid
# <thing>" style as the shared INVALID_JSON_BODY / INVALID_FORM_DATA.
VALIDATION_FAILED = "validation failed"
# S105: this is the /jwt/verify error-message string (contract canon), not a
# credential — the name just happens to contain "TOKEN".
INVALID_TOKEN = "invalid token"  # noqa: S105
INVALID_N = "invalid n"

# /jwt/sign canon TTL: 1 hour (exp = iat + 3600). Long enough that a token never
# expires between the contract's sign and verify steps.
JWT_TTL_SECONDS = 3600

# /compute canon: SHA-256 chain over the fixed seed bytes, rounds clamped to the
# cap so per-request CPU work is bounded.
SHA256_SEED = b"benchmark"
COMPUTE_CAP = 1_000_000
