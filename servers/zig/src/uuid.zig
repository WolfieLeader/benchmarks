const std = @import("std");
const uuid = @import("uuid"); // r4gus/uuid-zig

/// Writes a canonical UUIDv7 (RFC 9562) string into `out`: 48-bit Unix-ms
/// timestamp + 74 random bits, version 7, variant 10. Used for postgres,
/// redis and cassandra ids (mongo uses a native ObjectId). Backed by the
/// r4gus/uuid-zig library (`io.random` supplies the entropy).
pub fn v7(io: std.Io, out: *[36]u8) void {
    out.* = uuid.urn.serialize(uuid.v7.new(io));
}

/// True if `s` is a canonical 36-char UUID string (used to short-circuit
/// lookups of malformed ids as "not found", matching the other servers). The
/// library exposes only a throwing `urn.deserialize`, not a boolean check.
pub fn isValid(s: []const u8) bool {
    if (s.len != 36) return false;
    for (s, 0..) |ch, i| {
        if (i == 8 or i == 13 or i == 18 or i == 23) {
            if (ch != '-') return false;
        } else if (!std.ascii.isHex(ch)) {
            return false;
        }
    }
    return true;
}
