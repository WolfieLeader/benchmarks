const std = @import("std");

// Kernel CSPRNG via libc `getentropy` (present on both macOS and Alpine musl;
// musl lacks arc4random_buf). Fills up to 256 bytes; we need 10. Avoids the
// removed std.crypto.random global and needs no seeding or locking.
extern "c" fn getentropy(buffer: ?*anyopaque, length: usize) c_int;

const hex = "0123456789abcdef";

/// Writes a canonical UUIDv7 (RFC 9562) string into `out`: 48-bit Unix-ms
/// timestamp + 74 random bits, version 7, variant 10. Used for postgres,
/// redis and cassandra ids (mongo uses a native ObjectId).
pub fn v7(out: *[36]u8) void {
    var b: [16]u8 = undefined;
    var ts: std.posix.timespec = undefined;
    _ = std.c.clock_gettime(std.posix.CLOCK.REALTIME, &ts);
    const ms: u64 = @as(u64, @intCast(ts.sec)) * 1000 + @as(u64, @intCast(ts.nsec)) / 1_000_000;
    b[0] = @truncate(ms >> 40);
    b[1] = @truncate(ms >> 32);
    b[2] = @truncate(ms >> 24);
    b[3] = @truncate(ms >> 16);
    b[4] = @truncate(ms >> 8);
    b[5] = @truncate(ms);
    _ = getentropy(b[6..].ptr, b[6..].len);
    b[6] = (b[6] & 0x0f) | 0x70; // version 7
    b[8] = (b[8] & 0x3f) | 0x80; // variant 10

    var i: usize = 0; // position in `out`
    for (b, 0..) |byte, j| {
        if (j == 4 or j == 6 or j == 8 or j == 10) {
            out[i] = '-';
            i += 1;
        }
        out[i] = hex[byte >> 4];
        out[i + 1] = hex[byte & 0x0f];
        i += 2;
    }
}

/// True if `s` is a canonical 36-char UUID string (used to short-circuit
/// lookups of malformed ids as "not found", matching the other servers).
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
