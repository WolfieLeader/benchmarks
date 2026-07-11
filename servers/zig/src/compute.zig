const std = @import("std");

const Sha256 = std.crypto.hash.sha2.Sha256;

// Compute canon (contract/web.json): SHA-256 applied `n` times to the seed
// bytes, lowercase hex. Mirrors the cross-server web contract.
pub const seed = "benchmark";
pub const max_rounds: u64 = 1_000_000;

pub const digest_len = Sha256.digest_length;

pub const Error = error{InvalidRounds};

/// Parses the `n` query value: an integer in [1, max_rounds]. Missing, empty,
/// non-numeric, zero, or negative -> error (400). A value above the cap is
/// clamped, not rejected (bounds per-request CPU work). Mirrors the go-stdlib
/// reference (strconv.Atoi then n<1 rejects; n>cap clamps); a value too large
/// for i64 fails to parse and is rejected, matching that reference.
pub fn parseRounds(raw: []const u8) Error!u64 {
    // std.fmt.parseInt accepts Zig-style underscore digit separators
    // ("1_000"); the cross-server canon (Go strconv.Atoi) rejects them.
    if (std.mem.indexOfScalar(u8, raw, '_') != null) return error.InvalidRounds;
    const n = std.fmt.parseInt(i64, raw, 10) catch return error.InvalidRounds;
    if (n < 1) return error.InvalidRounds;
    return @min(@as(u64, @intCast(n)), max_rounds);
}

/// Applies SHA-256 to the seed `n` times (caller guarantees n >= 1) and returns
/// the digest. Round 1 is sha256(seed); round k feeds the previous digest back in.
pub fn chain(n: u64) [digest_len]u8 {
    var state: [digest_len]u8 = undefined;
    Sha256.hash(seed, &state, .{});
    var i: u64 = 1;
    while (i < n) : (i += 1) {
        var next: [digest_len]u8 = undefined;
        Sha256.hash(&state, &next, .{});
        state = next;
    }
    return state;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const testing = std.testing;

test "chain(1) equals the known sha256('benchmark') vector" {
    const hex = std.fmt.bytesToHex(chain(1), .lower);
    try testing.expectEqualStrings(
        "0e89820860c342f2c7ec694d144023b10301c2accdd078cb5167a06d0c3d5bcc",
        &hex,
    );
}

test "chain(2) equals sha256(sha256('benchmark'))" {
    const hex = std.fmt.bytesToHex(chain(2), .lower);
    try testing.expectEqualStrings(
        "b061accddb3b47684b3bd36291c9b219cfbd43bb074f72251f6574b073425003",
        &hex,
    );
}

test "parseRounds enforces integer >= 1 and clamps above the cap" {
    try testing.expectEqual(@as(u64, 1), try parseRounds("1"));
    try testing.expectEqual(@as(u64, 1000), try parseRounds("1000"));
    try testing.expectEqual(max_rounds, try parseRounds("1000000"));
    try testing.expectEqual(max_rounds, try parseRounds("5000000"));
    try testing.expectError(error.InvalidRounds, parseRounds(""));
    try testing.expectError(error.InvalidRounds, parseRounds("abc"));
    try testing.expectError(error.InvalidRounds, parseRounds("0"));
    try testing.expectError(error.InvalidRounds, parseRounds("-5"));
    try testing.expectError(error.InvalidRounds, parseRounds("3.5"));
}

test "parseRounds rejects Zig-style underscore digit separators" {
    try testing.expectError(error.InvalidRounds, parseRounds("1_000"));
    try testing.expectError(error.InvalidRounds, parseRounds("_1"));
    try testing.expectError(error.InvalidRounds, parseRounds("1_"));
}
