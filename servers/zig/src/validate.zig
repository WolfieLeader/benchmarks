const std = @import("std");
const uuid = @import("uuid.zig");

// POST /validate schema (contract/web.json, ~4 levels). Mirrors the cross-server
// web contract (Go's shared/web.ValidatePayload). Every field is optional at the
// parse layer so that a structurally-valid body always reaches validation — a
// semantic failure is 400 "validation failed", never a JSON parse error — while
// presence itself is enforced as a validation rule in `check`.
pub const Payload = struct {
    user: ?User = null,
    items: ?[]Item = null,
    total: f64 = 0,

    pub const User = struct {
        id: []const u8 = "",
        email: []const u8 = "",
        profile: ?Profile = null,
    };
    pub const Profile = struct {
        age: i64 = -1,
        role: []const u8 = "",
        preferences: ?Preferences = null,
    };
    pub const Preferences = struct {
        theme: []const u8 = "",
        notifications: ?bool = null,
    };
    pub const Item = struct {
        sku: []const u8 = "",
        quantity: i64 = 0,
        tags: []const []const u8 = &.{},
    };
};

pub const Error = error{Invalid};

const parse_opts: std.json.ParseOptions = .{
    .duplicate_field_behavior = .use_last,
    .ignore_unknown_fields = true,
};

/// Parses the request body into the schema. A JSON syntax error or a type
/// mismatch (e.g. a string where a number is required) is a parse error the
/// caller surfaces as "invalid JSON body"; a well-formed object that breaks a
/// rule is caught by `check`.
pub fn parse(arena: std.mem.Allocator, body: []const u8) !Payload {
    return std.json.parseFromSliceLeaky(Payload, arena, body, parse_opts);
}

/// Validates a parsed payload against the canon rules, returning error.Invalid
/// on the first violation (the contract asserts only that validation failed, not
/// a specific count).
pub fn check(p: Payload) Error!void {
    const user = p.user orelse return error.Invalid;
    if (!uuid.isValid(user.id)) return error.Invalid;
    if (!isEmail(user.email)) return error.Invalid;

    const profile = user.profile orelse return error.Invalid;
    if (profile.age < 0 or profile.age > 120) return error.Invalid;
    if (!isRole(profile.role)) return error.Invalid;

    const prefs = profile.preferences orelse return error.Invalid;
    if (!isTheme(prefs.theme)) return error.Invalid;
    if (prefs.notifications == null) return error.Invalid;

    const items = p.items orelse return error.Invalid;
    if (items.len < 1) return error.Invalid;
    for (items) |item| {
        if (item.sku.len < 1) return error.Invalid;
        if (item.quantity < 1 or item.quantity > 100) return error.Invalid;
    }

    if (p.total < 0) return error.Invalid;
}

fn isRole(role: []const u8) bool {
    return std.mem.eql(u8, role, "admin") or
        std.mem.eql(u8, role, "user") or
        std.mem.eql(u8, role, "guest");
}

fn isTheme(theme: []const u8) bool {
    return std.mem.eql(u8, theme, "light") or std.mem.eql(u8, theme, "dark");
}

/// Pragmatic email check (mirrors user.zig's isEmail): rejects values with no
/// '@', an empty local part, any whitespace, or a domain without an interior
/// dot; accepts a normal local@domain.tld.
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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const testing = std.testing;

const valid_body =
    \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"alice@conformance-suite.com","profile":{"age":30,"role":"admin","preferences":{"theme":"dark","notifications":true}}},"items":[{"sku":"SKU-1","quantity":2,"tags":["new","featured"]},{"sku":"SKU-2","quantity":100,"tags":[]}],"total":42.5}
;

const invalid_body =
    \\{"user":{"id":"not-a-uuid","email":"not-an-email","profile":{"age":200,"role":"superuser","preferences":{"theme":"neon","notifications":true}}},"items":[{"sku":"SKU-1","quantity":0,"tags":["x"]}],"total":-5}
;

fn checkBody(a: std.mem.Allocator, body: []const u8) !void {
    return check(try parse(a, body));
}

test "validate accepts the canon valid object" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    try checkBody(arena.allocator(), valid_body);
}

test "validate rejects the canon invalid object" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    try testing.expectError(error.Invalid, checkBody(arena.allocator(), invalid_body));
}

test "validate rejects each single-field violation" {
    var arena = std.heap.ArenaAllocator.init(testing.allocator);
    defer arena.deinit();
    const a = arena.allocator();

    const cases = [_][]const u8{
        // missing user
        \\{"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // bad uuid
        \\{"user":{"id":"nope","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // bad email
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"nope","profile":{"age":1,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // age out of range
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":121,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // bad role enum
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"root","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // bad theme enum
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"neon","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // missing notifications
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"light"}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":0}
        ,
        // empty items
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[],"total":0}
        ,
        // quantity out of range
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":101,"tags":[]}],"total":0}
        ,
        // negative total
        \\{"user":{"id":"3f1a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8","email":"a@b.co","profile":{"age":1,"role":"user","preferences":{"theme":"light","notifications":false}}},"items":[{"sku":"S","quantity":1,"tags":[]}],"total":-0.01}
        ,
    };
    for (cases) |body| {
        try testing.expectError(error.Invalid, checkBody(a, body));
    }
}
