# Contract conformance cases

Language-neutral JSON that defines the **observable HTTP contract** every server in
this repo must honor. Each server is idiomatic internally, but its behavior on the
wire — status codes, response bodies, error strings, security properties — may not
differ. These cases are the referee.

They are consumed by the Go benchmark client's conformance mode:

```sh
just benchmark --conformance --base-url=http://localhost:5001
# or, from benchmark/:
go run ./cmd/main.go --conformance --base-url=http://localhost:5001
```

Every case runs **once, sequentially, with strict full-body assertions**, against a
plain base URL (no docker/orchestrator/metrics). The command exits non-zero on any
failure — including when zero cases execute (wrong dir, empty suites). The
`scripts/contract.mts` harness (later slice) wraps this by starting a server
container first, then invoking the command against its port. Both lookup dirs
default relative to `benchmark/` and can be overridden for other working
directories: `--contract-dir=` (cases) and `--test-files-dir=` (upload fixtures).

## File layout

- One `.json` file per suite. Files are loaded alphabetically.
- Each file is a **Suite**: `{ "name": string, "cases": Case[] }`.
- Current suites: `basic`, `params`, `form`, `file`, `web`, `db`.

### Suite gating

Some suites only apply to servers that implement them. A suite is **loaded and
structurally validated for every run**, but only **executed** for a server when
that server opts in — the runner takes `--skip-suite=<name>[,<name>]` (loaded but
not run) and the `scripts/contract.mts` harness derives it per server from the
manifest. Today:

- `db` runs for every server (all declare a non-empty `databases` array).
- `web` runs only when a server's `bench.json` sets `"web": true`; otherwise the
  harness passes `--skip-suite=web`. Denylist by design: if the skip ever fails to
  match (typo, renamed suite), the web cases **run and fail loudly** rather than
  silently passing.

## Case format

A **Case** is either a single request+assertion, or (when `flow` is set) an ordered
group of steps that share captured variables.

```jsonc
{
  "name": "search_with_query",          // required, unique-ish label
  "note": "why this case exists",        // optional documentation

  // --- request (omitted for flow groups) ---
  "method": "GET",                        // default GET
  "path": "/params/search",               // required for a single case
  "query": { "q": "hi", "limit": "5" },   // query params
  "headers": { "X-Custom-Header": "v" },  // request headers (Cookie goes here too)
  "body": { "key": "value" },             // JSON body, marshaled as-is (any JSON value)
  "rawBody": "{\"bad\": }",               // raw string body; overrides `body`
                                          //   use for malformed JSON and null smuggling
  "contentType": "application/json",      // override the request Content-Type
  "form": { "name": "John" },             // application/x-www-form-urlencoded body
  "multipart": {                          // multipart/form-data body
    "fields": { "name": "John" },
    "file": {
      "field": "file",                    // form field name, default "file"
      "filename": "test.txt",
      "contentType": "text/plain",        // the *part* Content-Type; omitted if empty
      "source": "test.txt",               // read from contract/test-files/ (a committed fixture)
      "text": "inline content",           // OR inline literal content
      "sizeBytes": 1100000                // OR synthesize N bytes (oversized-payload cases)
    }
  },

  // --- response assertion ---
  "expect": {
    "status": 200,
    "statusAnyOf": [200, 404],            // any listed status passes (overrides status);
                                          //   cannot be combined with body/text — use only when
                                          //   routers legitimately differ (e.g. traversal safety)
    "headers": { "Content-Type": "application/json" }, // substring ("contains") match
    "text": "OK",                         // exact text body (trimmed); mutually exclusive with body
    "htmlContains": ["Hello, Alice"],     // each substring must appear in the raw body;
                                          //   whitespace/markup-tolerant HTML match (see below)
    "body": { "hello": "world" },         // JSON body assertion (see matchers below)
    "match": "exact"                      // "exact" (default) | "subset"
  },

  // --- sequencing ---
  "capture": { "id": "id" },              // after success, capture response.id into {id}
                                          //   only valid on steps inside a flow (load error otherwise)
  "flow": [ Case, ... ]                   // ordered steps sharing one capture map
}
```

### Body matching

Body assertions are compared against parsed JSON (so trailing newlines, key order,
and HTML escaping in the wire bytes are irrelevant). By default matching is **strict**:
the actual object must contain exactly the expected keys — no more, no less. Set
`"match": "subset"` to allow extra keys.

String values in `expect.body` may be **matcher tokens** instead of literals:

| Token              | Passes when the value is…                                                                                                                                                                                                                                                                                 |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `$present`         | present (any value)                                                                                                                                                                                                                                                                                       |
| `$string`          | a JSON string                                                                                                                                                                                                                                                                                             |
| `$number`          | a JSON number                                                                                                                                                                                                                                                                                             |
| `$bool`            | a JSON boolean                                                                                                                                                                                                                                                                                            |
| `$uuid`            | a canonical UUID string                                                                                                                                                                                                                                                                                   |
| `$objectid`        | a 24-char hex Mongo ObjectId                                                                                                                                                                                                                                                                              |
| `$id`              | a UUID **or** an ObjectId (use for `id` fields)                                                                                                                                                                                                                                                           |
| `$absent`          | **as an object key**: that key must NOT be present                                                                                                                                                                                                                                                        |
| `$optional`        | **as an object key**: the key MAY be absent; if present, any non-null value passes. Never an unexpected key under strict matching — use for contract-optional fields like `details`.                                                                                                                      |
| `$jwt`             | a string that is a valid **HS256 JWT**: signature verifies against the shared secret (`--jwt-secret`, default the dev secret) **and** an `exp` claim is present and unexpired. Structural + cryptographic check via `golang-jwt` — claim _values_ are asserted separately (e.g. by a `/jwt/verify` step). |
| `$sha256chain:<n>` | a lowercase-hex string equal to SHA-256 applied **n** times to the canon seed bytes `benchmark`. The rounds count rides in the token, so the runner recomputes the expected digest itself (no request context needed).                                                                                    |

Any other string is compared literally.

### HTML body matching (`htmlContains`)

`expect.htmlContains` is an array of substrings that must **all** appear in the
raw response body. It is the referee for server-rendered HTML (`GET /html`):
template engines differ in whitespace and tag layout across frameworks, so the
contract asserts `Content-Type: text/html` plus the presence of each interpolated
value rather than a byte-exact body. It is a body-assertion mode of its own —
`text`, `htmlContains`, and `body` are mutually exclusive, enforced at load time
(a case setting more than one is a suite load error).

### Variable substitution

`{name}` tokens in a case's `path`, `headers`, `query`, `body`, `rawBody`, and
`expect` are replaced with values captured by earlier steps in the same `flow`.
Example: a `create` step does `"capture": { "id": "id" }`, and the following `read`
step uses `"path": "/db/postgres/users/{id}"` and asserts `"id": "{id}"`.

### Flows

When `flow` is set, the case is a group: each step runs in order sharing one capture
map. If a step fails, the remaining steps are reported as skipped (they depend on it).
Used for CRUD lifecycles (reset → create → read → update → delete → verify-gone).

## Fixtures

Upload fixtures live in `contract/test-files/`:

- `test.txt`, `multi.txt` — valid small text uploads.
- `binary.bin` — a ~100-byte binary blob (contains null bytes) for the anti-sniffing
  security case: sent with `Content-Type: text/plain` it must still be rejected `415`.

Oversized payloads (>1 MiB) are **synthesized at run time** via `file.sizeBytes` and
are never committed.

## Coverage

All 16 routes with meaningful variations, plus the negative and security cases:

- **400** — malformed JSON; non-object JSON bodies (array/string/number/bool/null smuggling);
  wrong content-type on form/file; invalid email; out-of-range / fractional / negative /
  malformed `favoriteNumber`; empty name; case-mismatched (PascalCase) field names;
  malformed JSON on `PATCH`.
- **404** — unknown user id; unknown database name; nonexistent-but-well-formed id on
  `GET` / `PATCH` / `DELETE`.
- **413** — oversized file upload (synthesized).
- **415** — wrong declared content-type; sniffed binary; the anti-sniffing case
  (binary content lying as `text/plain`); and the **full-body NUL scan**
  (`file_rejects_nul_after_sniff_window`): a body whose first 512 bytes are clean
  text but which carries a NUL past that window is still rejected. Canon per
  go-stdlib (`servers/go-stdlib/internal/routes/params.go`): the 512-byte window
  is used **only** for MIME sniffing; the NUL / UTF-8 validity check scans the
  entire body, so a late NUL cannot slip through.
- **Content-Type** — every error response asserts its `Content-Type`: JSON error bodies are
  `application/json` (asserted via the substring/"contains" header match, so both bare and
  `; charset=...` forms pass), and the 503 unknown-db health is `text/plain`. Success bodies
  assert it on `GET /` (JSON) and the `/health` routes (text/plain).
- **boundary values** — `favoriteNumber` at the inclusive edges: `0` and `100` accepted and
  echoed (`0` distinct from absent); `-1` and `3.5` rejected. (Integral floats like `7.0`
  and numeric strings are intentionally not asserted — servers diverge there.)
- **Unicode** — a multi-byte value (Latin-accented + CJK + emoji + RTL) round-trips
  byte-for-byte through the `/params/body` echo and through DB create → store → retrieve.
- **path safety** — encoded traversal input returns a normal response, never a file read.
- **JSON parse semantics** — duplicate keys resolve last-wins, proven on both `/params/body`
  and the DB create path; field names are case-sensitive so PascalCase keys fail
  required-field validation (400) on create. On `PATCH` (canon), where every field is
  optional, the same mismatched-case key is instead ignored — an empty no-op update that
  returns the unchanged row (`200`).
- **lifecycle** — `reset` provably clears prior rows (create → reset → read is 404).

### Web suite (`web.json`)

The five web endpoints (`GET /html`, `GET /jwt/sign`, `GET /jwt/verify`,
`POST /validate`, `GET /compute`). Gated per server (`"web": true` in the
manifest); no server implements it yet, so it is skipped everywhere for now.
Canon (lead-ruled):

- **`/html`** — `200 text/html` rendering a template with a fixed greeting
  (`Hello, Alice`), list (`apple`,`banana`,`cherry`), and the **labeled** number
  `Total: 42` (the label makes the substring distinctive — a bare `42` could
  collide with an incidental value elsewhere in the markup). Asserted via
  `htmlContains` (whitespace/markup-tolerant), not a byte-exact body.
- **`/jwt/sign`** — `200 {"token": "$jwt"}`. HS256, shared `JWT_SECRET`. Fixed
  claims `sub=1234567890`, `name=John Doe`, `admin=true` plus dynamic `iat`/`exp`.
  **Canon TTL = 1 hour** (`exp = iat + 3600`): servers SHOULD sign with a 1-hour
  TTL; the contract asserts only that `exp` is present and unexpired (`$jwt`), so
  the exact TTL never flakes a run — but a too-short TTL risks expiring between
  the sign and verify steps, so don't shorten it.
- **`/jwt/verify`** — `Authorization: Bearer <t>`. Valid token → `200` echoing the
  payload (`iat`/`exp` matched as `$number`, the rest literally). Missing,
  malformed, **wrong-signature**, **expired**, or **`alg:none`** token →
  `401 {"error":"invalid token"}` (`details` `$optional`). The bad-signature case
  proves signature verification, not just parsing; the expired case (signed with
  the correct dev-default secret, `exp` in 2020) proves `exp` is validated, not
  just the signature; the `alg:none` case (an unsigned token with a far-future
  `exp` and an empty signature segment) proves the algorithm is pinned to HS256 at
  the library-config level — no stack trusts the header's `alg`.
- **`/validate`** — deep nested object (~4 levels; `user{id:uuid, email, profile{age:0..120, role:enum, preferences{theme:enum, notifications:bool}}}, items[]{sku, quantity:1..100, tags[]}, total:>=0`).
  Valid → `200 {"valid": true}`; invalid → `400 {"error":"validation failed", "details":"$present"}`.
  The **error count** from the endpoint sketch is intentionally _not_ asserted as an
  exact integer: the canonical error shape is `{"error", "details"?}`, and error
  counts diverge across Zod/Pydantic/validator/serde. `details` carries the
  per-framework summary (`$present`).
  **Zero-value canon** (pinned by `validate_omitted_{age,total,tags}_200`): fields
  Go only range-checks — `age` (`0..120`), `total` (`>= 0`), and `tags` (untagged) —
  are **not required**; when omitted they decode to their zero value (Go's
  `encoding/json` behavior) and validate. Every stack reproduces this at the schema
  layer (serde `#[serde(default)]`, zod `.default()`, pydantic field defaults, Zig
  struct defaults), so a missing optional field is a `200`, never a parse/validation
  error. Conversely the **presence/range** rules are pinned too: `items` (`min=1`)
  rejects an empty array and `sku` (`required`) rejects an empty string → `400`.
- **`/compute?n=`** — iterative SHA-256 chain, `200 {"result": "$sha256chain:<n>"}`
  (seed `benchmark`, lowercase hex). `n` must be an **integer ≥ 1**
  (validate-at-boundary): missing, non-numeric, zero, or negative `n` →
  `400 {"error":"invalid n","details":"$present"}` (house `invalid <thing>` error
  style; unlike `/params/search`'s `limit`, which falls back to a documented
  default, `/compute` has no meaningful default amount of work). Above the cap
  `1000000`, `n` is **clamped**, not rejected — the over-cap case asserts the
  cap-rounds digest.
  **Parse canon = Go `strconv.Atoi` (signed 64-bit, base-10, ASCII).** The
  `compute_*_n_*` cases pin the exact accept/reject set every stack must match:
  a leading `+` is accepted (`+5` → 5) and leading zeros are decimal (`007` → 7),
  but digit-group underscores (`1_000`), surrounding whitespace (` 5`, no trim),
  and non-ASCII digits (Arabic-Indic `٥`) are rejected. The overflow boundary is
  **`i64::MAX` (9223372036854775807), not `u64::MAX`**: `9300000000000000000`
  (above `i64::MAX`, below `u64::MAX`) is a `400`, while `9007199254740993`
  (`2^53+1`, a valid `i64` far above the cap) is a `200` clamped to the cap. The
  paired boundary cases stop a server from passing the overflow case by rejecting
  large-but-valid values too early. Stacks whose native parser diverges guard it:
  Python gates `int()` with an ASCII-digit regex + explicit `i64` range check
  (its `int()` accepts Unicode digits and is unbounded); TypeScript uses `BigInt`
  for the range check (`Number` loses precision past `2^53`); Rust parses as
  signed `i64`; Kotlin gates `toLongOrNull()` with an ASCII-digit regex (Java's
  `Long.parseLong` accepts Unicode digits via `Character.digit`); Zig guards the
  underscore its `parseInt` would otherwise accept.

**Env contract:** `JWT_SECRET` (shared HS256 secret) joins the server env-var
contract (defined in each `shared/*/env` module; dev default
`benchmarks-shared-jwt-secret-dev-default`). The contract harness passes the same
value to the container (`JWT_SECRET`) and the runner (`--jwt-secret`) so the
server and the `$jwt` matcher agree.

### Deliberately not asserted (servers diverge — canon rulings pending)

- **405 method-not-allowed** — a known path with the wrong method is not uniform: go-chi and
  ts-deno-oak return `405` + `Allow` (empty body); py-fastapi returns `405 {"error":"Method
Not Allowed"}` (no `Allow`); the other eight fall through to `404 {"error":"not found"}`.
- **error `details` on 415/413** — genuinely optional (`$optional`): go-chi/go-fiber/go-gin/zig
  omit it, the rest include a detail string. Left as `$optional`, not tightened to `$absent`.
- **name/email length maxima** — no server-level max; only the DB `varchar(255)` bounds them,
  so over-long names diverge (postgres errors, other stores accept).
- **JSON `null` into a non-required numeric field** (e.g. `"total": null`, `"age": null`,
  `"tags": null` on `/validate`) — intentionally **unpinned**. Go's `encoding/json` treats
  an explicit `null` for a scalar as a no-op (the field keeps its zero value → `200`), whereas
  zod, pydantic, and serde reject `null` for a typed field unless it is declared nullable
  (`400`). Pinning either way would force one side into unidiomatic null-handling (a bespoke
  null→zero coercion or a Go-side explicit-null rejection), which violates the idiom rule
  (PLAN §3). This is distinct from an **omitted** field (pinned `200` — zero-value canon
  above): only the explicit-`null` case diverges, and only for the non-required scalars.
