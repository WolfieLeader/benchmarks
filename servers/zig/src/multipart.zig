const std = @import("std");

pub const Part = struct {
    filename: ?[]const u8,
    content_type: ?[]const u8,
    content: []const u8,
};

/// Extracts the multipart boundary token from a Content-Type header value.
pub fn boundary(content_type: []const u8) ?[]const u8 {
    const at = std.ascii.indexOfIgnoreCase(content_type, "boundary=") orelse return null;
    var b = content_type[at + "boundary=".len ..];
    if (std.mem.indexOfScalar(u8, b, ';')) |i| b = b[0..i];
    b = std.mem.trim(u8, b, " \t");
    if (b.len >= 2 and b[0] == '"' and b[b.len - 1] == '"') b = b[1 .. b.len - 1];
    return if (b.len == 0) null else b;
}

/// Finds the multipart part whose Content-Disposition name equals `field`.
/// Parses just enough to recover the declared Content-Type (which http.zig's
/// own multipart parser discards) alongside the filename and raw content.
pub fn findField(body: []const u8, boundary_token: []const u8, field: []const u8) ?Part {
    var delim_buf: [74]u8 = undefined;
    if (boundary_token.len + 2 > delim_buf.len) return null;
    delim_buf[0] = '-';
    delim_buf[1] = '-';
    @memcpy(delim_buf[2 .. 2 + boundary_token.len], boundary_token);
    const delimiter = delim_buf[0 .. 2 + boundary_token.len];

    var it = std.mem.splitSequence(u8, body, delimiter);
    _ = it.next(); // preamble before the first boundary
    while (it.next()) |segment| {
        // Closing boundary is "--\r\n" (or "--"); stop.
        if (segment.len >= 2 and segment[0] == '-' and segment[1] == '-') break;
        // Each part starts with the boundary's trailing CRLF.
        var s = segment;
        if (s.len >= 2 and s[0] == '\r' and s[1] == '\n') s = s[2..];

        const sep = std.mem.indexOf(u8, s, "\r\n\r\n") orelse continue;
        const headers = s[0..sep];
        var content = s[sep + 4 ..];
        // Strip the trailing CRLF that precedes the next boundary.
        if (content.len >= 2 and content[content.len - 2] == '\r' and content[content.len - 1] == '\n') {
            content = content[0 .. content.len - 2];
        }

        var name: ?[]const u8 = null;
        var filename: ?[]const u8 = null;
        var content_type: ?[]const u8 = null;

        var lines = std.mem.splitSequence(u8, headers, "\r\n");
        while (lines.next()) |line| {
            if (std.ascii.startsWithIgnoreCase(line, "content-disposition:")) {
                const value = line["content-disposition:".len..];
                name = param(value, "name");
                filename = param(value, "filename");
            } else if (std.ascii.startsWithIgnoreCase(line, "content-type:")) {
                content_type = std.mem.trim(u8, line["content-type:".len..], " \t");
            }
        }

        if (name) |n| {
            if (std.mem.eql(u8, n, field)) {
                return .{ .filename = filename, .content_type = content_type, .content = content };
            }
        }
    }
    return null;
}

/// Reads a `key=value` (optionally quoted) attribute out of a header value.
fn param(header_value: []const u8, key: []const u8) ?[]const u8 {
    var rest = header_value;
    while (std.ascii.indexOfIgnoreCase(rest, key)) |i| {
        const after = rest[i + key.len ..];
        // Require the match to be a full attribute name followed by '='.
        const boundary_before = i == 0 or rest[i - 1] == ' ' or rest[i - 1] == ';';
        if (boundary_before and after.len > 0 and after[0] == '=') {
            var v = after[1..];
            if (v.len > 0 and v[0] == '"') {
                v = v[1..];
                if (std.mem.indexOfScalar(u8, v, '"')) |end| return v[0..end];
                return null;
            }
            if (std.mem.indexOfAny(u8, v, ";")) |end| return std.mem.trim(u8, v[0..end], " \t");
            return std.mem.trim(u8, v, " \t");
        }
        rest = after;
    }
    return null;
}
