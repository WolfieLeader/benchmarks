const std = @import("std");

/// A stored user, as serialized on the wire. `favoriteNumber` is omitted when
/// null (serialize with `.emit_null_optional_fields = false`).
pub const User = struct {
    id: []const u8,
    name: []const u8,
    email: []const u8,
    favoriteNumber: ?i32 = null,
};

/// Create payload. `name`/`email` are required (no default) so a body missing
/// them — including PascalCase keys, which are unknown fields — fails to parse.
pub const CreateUser = struct {
    name: []const u8,
    email: []const u8,
    favoriteNumber: ?i32 = null,
};

/// Patch payload. All fields optional; absent fields leave the column untouched.
/// Unknown keys are ignored (see `update_opts`): a PATCH carrying only unknown
/// keys parses to all-null and applies as a no-op, matching cross-server canon.
pub const UpdateUser = struct {
    name: ?[]const u8 = null,
    email: ?[]const u8 = null,
    favoriteNumber: ?i32 = null,
};

pub const ValidationError = error{Invalid};

// Duplicate keys resolve last-wins on every decode (JS `JSON.parse` / Python
// semantics). CREATE keeps `ignore_unknown_fields = false` (the default): a body
// with only unknown/PascalCase keys is missing the required `name`/`email` and so
// fails to parse (400) — strictness that is the create contract.
const create_opts: std.json.ParseOptions = .{ .duplicate_field_behavior = .use_last };
// UPDATE ignores unknown fields (cross-server canon): every other server strips
// them, so a PATCH with a mismatched-case key becomes a no-op returning the
// existing row unchanged (200). Required-field parsing does not apply — all
// UpdateUser fields are optional.
const update_opts: std.json.ParseOptions = .{ .duplicate_field_behavior = .use_last, .ignore_unknown_fields = true };

pub fn parseCreate(arena: std.mem.Allocator, body: []const u8) !CreateUser {
    return std.json.parseFromSliceLeaky(CreateUser, arena, body, create_opts);
}

pub fn parseUpdate(arena: std.mem.Allocator, body: []const u8) !UpdateUser {
    return std.json.parseFromSliceLeaky(UpdateUser, arena, body, update_opts);
}

pub fn validateCreate(data: CreateUser) ValidationError!void {
    if (data.name.len < 1) return error.Invalid;
    if (!isEmail(data.email)) return error.Invalid;
    try validateFavorite(data.favoriteNumber);
}

pub fn validateUpdate(data: UpdateUser) ValidationError!void {
    if (data.name) |n| {
        if (n.len < 1) return error.Invalid;
    }
    if (data.email) |e| {
        if (!isEmail(e)) return error.Invalid;
    }
    try validateFavorite(data.favoriteNumber);
}

fn validateFavorite(n: ?i32) ValidationError!void {
    if (n) |v| {
        if (v < 0 or v > 100) return error.Invalid;
    }
}

/// Pragmatic email check covering the contract: rejects values with no `@`,
/// an empty local part, or a domain without an interior dot; accepts normal
/// `local@domain.tld` addresses. No whitespace allowed.
fn isEmail(email: []const u8) bool {
    if (email.len == 0) return false;
    if (std.mem.indexOfAny(u8, email, " \t\r\n") != null) return false;
    const at = std.mem.lastIndexOfScalar(u8, email, '@') orelse return false;
    const local = email[0..at];
    const domain = email[at + 1 ..];
    if (local.len == 0 or domain.len < 3) return false;
    const dot = std.mem.indexOfScalar(u8, domain, '.') orelse return false;
    if (dot == 0 or dot == domain.len - 1) return false;
    return true;
}
