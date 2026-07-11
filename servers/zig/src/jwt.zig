const std = @import("std");

const HmacSha256 = std.crypto.auth.hmac.sha2.HmacSha256;
const b64 = std.base64.url_safe_no_pad;

/// Length of an HS256 signature (a SHA-256 digest): 32 bytes.
const mac_len = HmacSha256.mac_length;

// Canon JWT claims (contract/web.json): fixed sub/name/admin, dynamic iat/exp.
// The Zig server has no shared web module, so the cross-server canon lives here
// (the same way http_util.zig carries the shared error strings).
pub const subject = "1234567890";
pub const name = "John Doe";
pub const admin = true;
/// Canon TTL = 1 hour (exp = iat + ttl_seconds).
pub const ttl_seconds: i64 = 3600;

/// Base64url of the fixed HS256 header {"alg":"HS256","typ":"JWT"}. Constant
/// because every token this server signs uses the same header; a test asserts
/// it equals a runtime encode of that header JSON.
const header_b64 = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9";

/// Claims echoed by /jwt/verify. Field order is irrelevant to verification
/// (matched by name) and to signing (the signature covers our exact bytes).
pub const Claims = struct {
    sub: []const u8,
    name: []const u8,
    admin: bool,
    iat: i64,
    exp: i64,
};

/// Every structural, cryptographic, algorithm, or expiry failure collapses to a
/// single error: /jwt/verify maps them all to 401 "invalid token" and never
/// reveals the reason on the wire.
pub const Error = error{InvalidToken};

/// Signs an HS256 JWT carrying the canon claims plus the given iat/exp. Returns
/// the compact token `header.payload.signature`, allocated in `arena`.
pub fn sign(arena: std.mem.Allocator, secret: []const u8, iat: i64, exp: i64) ![]const u8 {
    const claims = Claims{ .sub = subject, .name = name, .admin = admin, .iat = iat, .exp = exp };
    const payload_json = try std.json.Stringify.valueAlloc(arena, claims, .{});
    const payload_b64 = try encode(arena, payload_json);

    // Signing input is the exact ASCII `header_b64 "." payload_b64`.
    const signing_input = try std.fmt.allocPrint(arena, "{s}.{s}", .{ header_b64, payload_b64 });
    var mac: [mac_len]u8 = undefined;
    HmacSha256.create(&mac, signing_input, secret);
    const sig_b64 = try encode(arena, &mac);

    return std.fmt.allocPrint(arena, "{s}.{s}", .{ signing_input, sig_b64 });
}

/// Verifies an HS256 JWT against `secret` at time `now` (Unix seconds) and
/// returns its claims. Security ordering is contract-critical:
///   1. constant-time HMAC signature check over the raw header.payload bytes;
///   2. algorithm pinning — decode the header and require alg == "HS256";
///   3. exp validation, only AFTER the signature is proven.
/// A token whose signature does not verify never has its claims trusted, and an
/// expired-but-correctly-signed token is still rejected.
pub fn verify(arena: std.mem.Allocator, secret: []const u8, token: []const u8, now: i64) Error!Claims {
    var it = std.mem.splitScalar(u8, token, '.');
    const h_b64 = it.next() orelse return error.InvalidToken;
    const p_b64 = it.next() orelse return error.InvalidToken;
    const s_b64 = it.next() orelse return error.InvalidToken;
    if (it.next() != null) return error.InvalidToken; // exactly three segments
    if (h_b64.len == 0 or p_b64.len == 0 or s_b64.len == 0) return error.InvalidToken;

    // 1. Signature first. The signing input is the received header.payload
    // prefix verbatim (token up to the last dot), so re-encoding can't drift
    // from what was actually signed.
    const signing_input = token[0 .. h_b64.len + 1 + p_b64.len];
    var expected: [mac_len]u8 = undefined;
    HmacSha256.create(&expected, signing_input, secret);
    var got: [mac_len]u8 = undefined;
    decodeSignature(s_b64, &got) catch return error.InvalidToken;
    if (!std.crypto.timing_safe.eql([mac_len]u8, expected, got)) return error.InvalidToken;

    // 2. Algorithm pinning: reject anything but HS256, defeating alg-confusion
    // (alg=none, or RS256 verified as if it were an HMAC).
    const header_json = decodeAlloc(arena, h_b64) catch return error.InvalidToken;
    if (!headerIsHs256(arena, header_json)) return error.InvalidToken;

    // 3. Decode + parse the claims, then check exp now that the token is trusted.
    const payload_json = decodeAlloc(arena, p_b64) catch return error.InvalidToken;
    const claims = std.json.parseFromSliceLeaky(Claims, arena, payload_json, .{
        .ignore_unknown_fields = true,
        .duplicate_field_behavior = .use_last,
    }) catch return error.InvalidToken;
    if (now >= claims.exp) return error.InvalidToken;

    return claims;
}

fn encode(arena: std.mem.Allocator, data: []const u8) ![]const u8 {
    const dest = try arena.alloc(u8, b64.Encoder.calcSize(data.len));
    return b64.Encoder.encode(dest, data);
}

/// Decodes a base64url signature that MUST be exactly one SHA-256 digest.
fn decodeSignature(src: []const u8, out: *[mac_len]u8) !void {
    const n = try b64.Decoder.calcSizeForSlice(src);
    if (n != mac_len) return error.InvalidToken;
    try b64.Decoder.decode(out[0..], src);
}

fn decodeAlloc(arena: std.mem.Allocator, src: []const u8) ![]u8 {
    const n = try b64.Decoder.calcSizeForSlice(src);
    const dest = try arena.alloc(u8, n);
    try b64.Decoder.decode(dest, src);
    return dest;
}

fn headerIsHs256(arena: std.mem.Allocator, header_json: []const u8) bool {
    const Header = struct { alg: []const u8 = "" };
    const h = std.json.parseFromSliceLeaky(Header, arena, header_json, .{
        .ignore_unknown_fields = true,
    }) catch return false;
    return std.mem.eql(u8, h.alg, "HS256");
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const testing = std.testing;
const dev_secret = "benchmarks-shared-jwt-secret-dev-default";

// The two static tokens embedded in contract/web.json.
const expired_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjoxNTc3ODQwNDAwLCJpYXQiOjE1Nzc4MzY4MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.8XxPN0yJufkzy8TdEspyV-GqR1b1MF8aW_YVERdoRic";
const bad_sig_token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZSwiZXhwIjo0MTAyNDQ0ODAwLCJpYXQiOjE3MzU2ODk2MDAsIm5hbWUiOiJKb2huIERvZSIsInN1YiI6IjEyMzQ1Njc4OTAifQ.J75FiSXpAhQxN9jiUjBHADeu_su1WJnZjJqDXI4aOWw";

test "header constant equals base64url of the HS256 header" {
    const encoded = try encode(testing.allocator, "{\"alg\":\"HS256\",\"typ\":\"JWT\"}");
    defer testing.allocator.free(encoded);
    try testing.expectEqualStrings(header_b64, encoded);
}

test "sign then verify round-trips the canon claims" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    const iat: i64 = 1_700_000_000;
    const exp: i64 = iat + ttl_seconds;
    const token = try sign(a, dev_secret, iat, exp);

    const claims = try verify(a, dev_secret, token, iat + 10);
    try testing.expectEqualStrings(subject, claims.sub);
    try testing.expectEqualStrings(name, claims.name);
    try testing.expect(claims.admin);
    try testing.expectEqual(iat, claims.iat);
    try testing.expectEqual(exp, claims.exp);
}

test "verify rejects an expired but correctly-signed token" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    const iat: i64 = 1_700_000_000;
    const exp: i64 = iat + ttl_seconds;
    const token = try sign(a, dev_secret, iat, exp);
    // now == exp is already expired (exp is the first non-valid second).
    try testing.expectError(error.InvalidToken, verify(a, dev_secret, token, exp));
    try testing.expectError(error.InvalidToken, verify(a, dev_secret, token, exp + 1));
}

test "verify rejects a token signed with a different secret" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    const iat: i64 = 1_700_000_000;
    const token = try sign(a, "some-other-secret", iat, iat + ttl_seconds);
    try testing.expectError(error.InvalidToken, verify(a, dev_secret, token, iat + 10));
}

test "static expired token (dev-default secret, exp 2020) is rejected" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    // exp = 1577840400 (2020-01-01T01:00:00Z). Any present-day time rejects it.
    try testing.expectError(error.InvalidToken, verify(a, dev_secret, expired_token, 1_700_000_000));
    // Before exp it verifies -> proves it is validly signed with the dev-default
    // secret, so the rejection above is the exp check, not a signature failure.
    const claims = try verify(a, dev_secret, expired_token, 1_500_000_000);
    try testing.expectEqualStrings(subject, claims.sub);
    try testing.expectEqual(@as(i64, 1577840400), claims.exp);
}

test "static bad-signature token (wrong secret) is rejected" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    // exp = 4102444800 (year 2100) -> not expired, so the only reason to reject
    // is the signature mismatch. Proves signature verification, not just parsing.
    try testing.expectError(error.InvalidToken, verify(a, dev_secret, bad_sig_token, 1_700_000_000));
}

test "malformed tokens are rejected" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();
    for ([_][]const u8{ "not-a-jwt", "a.b", "a.b.c.d", "..", "a..c", "" }) |t| {
        try testing.expectError(error.InvalidToken, verify(a, dev_secret, t, 1_700_000_000));
    }
}

test "algorithm confusion is rejected (valid HMAC over a non-HS256 header)" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    // Forge tokens whose header claims a different alg but whose signature is a
    // correct HMAC-SHA256 over their own header.payload with the real secret.
    // The signature check passes; alg pinning must still reject each one.
    for ([_][]const u8{
        "{\"alg\":\"HS384\",\"typ\":\"JWT\"}",
        "{\"alg\":\"none\",\"typ\":\"JWT\"}",
        "{\"alg\":\"RS256\",\"typ\":\"JWT\"}",
    }) |header_json| {
        const h_b64 = try encode(a, header_json);
        const p_b64 = try encode(a, "{\"sub\":\"1234567890\",\"name\":\"John Doe\",\"admin\":true,\"iat\":1700000000,\"exp\":4102444800}");
        const signing_input = try std.fmt.allocPrint(a, "{s}.{s}", .{ h_b64, p_b64 });
        var mac: [mac_len]u8 = undefined;
        HmacSha256.create(&mac, signing_input, dev_secret);
        const s_b64 = try encode(a, &mac);
        const token = try std.fmt.allocPrint(a, "{s}.{s}", .{ signing_input, s_b64 });
        try testing.expectError(error.InvalidToken, verify(a, dev_secret, token, 1_700_000_000));
    }
}
