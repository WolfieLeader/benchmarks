const std = @import("std");
const httpz = @import("httpz");
const App = @import("app.zig").App;
const httputil = @import("http_util.zig");
const multipart = @import("multipart.zig");

const err = httputil.err;
const max_file_bytes = 1 << 20; // 1 MiB
const sniff_len = 512;
const max_safe_int: i64 = 9007199254740991; // 2^53 - 1

pub fn search(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const query = try req.query();
    const q = trimOrNone(query.get("q") orelse "");
    const limit = safeInt(query.get("limit") orelse "", 10);
    httputil.writeJson(res, 200, .{ .search = q, .limit = limit }, .{});
}

pub fn url(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    httputil.writeJson(res, 200, .{ .dynamic = req.param("dynamic") orelse "" }, .{});
}

pub fn header(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const value = trimOrNone(req.header("x-custom-header") orelse "");
    httputil.writeJson(res, 200, .{ .header = value }, .{});
}

pub fn cookie(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    var cookies = req.cookies();
    const value = trimOrNone(cookies.get("foo") orelse "");
    try res.setCookie("bar", "12345", .{ .path = "/", .max_age = 10, .http_only = true });
    httputil.writeJson(res, 200, .{ .cookie = value }, .{});
}

pub fn body(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const raw = req.body() orelse "";
    const parsed = std.json.parseFromSliceLeaky(std.json.Value, res.arena, raw, .{
        .duplicate_field_behavior = .use_last,
    }) catch {
        httputil.writeError(res, 400, err.invalid_json, "expected a JSON object");
        return;
    };
    switch (parsed) {
        .object => {},
        else => {
            httputil.writeError(res, 400, err.invalid_json, "expected a JSON object");
            return;
        },
    }
    httputil.writeJson(res, 200, .{ .body = parsed }, .{});
}

pub fn form(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const ct = req.header("content-type") orelse "";
    const is_urlencoded = std.ascii.startsWithIgnoreCase(ct, "application/x-www-form-urlencoded");
    const is_multipart = std.ascii.startsWithIgnoreCase(ct, "multipart/form-data");
    if (!is_urlencoded and !is_multipart) {
        httputil.writeError(res, 400, err.invalid_form, err.expected_form_content_type);
        return;
    }

    var name_raw: []const u8 = "";
    var age_raw: []const u8 = "";
    if (is_urlencoded) {
        const fd = req.formData() catch {
            httputil.writeError(res, 400, err.invalid_form, "could not parse form");
            return;
        };
        name_raw = fd.get("name") orelse "";
        age_raw = fd.get("age") orelse "";
    } else {
        const md = req.multiFormData() catch {
            httputil.writeError(res, 400, err.invalid_form, "could not parse form");
            return;
        };
        if (md.get("name")) |v| name_raw = v.value;
        if (md.get("age")) |v| age_raw = v.value;
    }

    const name = trimOrNone(name_raw);
    const age = safeInt(age_raw, 0);
    httputil.writeJson(res, 200, .{ .name = name, .age = age }, .{});
}

pub fn file(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const ct = req.header("content-type") orelse "";
    if (!std.ascii.startsWithIgnoreCase(ct, "multipart/form-data")) {
        httputil.writeError(res, 400, err.invalid_multipart, err.expected_multipart_content_type);
        return;
    }
    const boundary = multipart.boundary(ct) orelse {
        httputil.writeError(res, 400, err.invalid_multipart, "missing boundary");
        return;
    };
    const raw = req.body() orelse "";
    const part = multipart.findField(raw, boundary, "file") orelse {
        httputil.writeError(res, 400, err.file_not_found, "no file field in form");
        return;
    };

    const head = part.content[0..@min(sniff_len, part.content.len)];

    // Content-type gate: trust the declared part type when present, otherwise
    // sniff the bytes. A non-text declaration or a binary sniff → 415.
    if (part.content_type) |declared| {
        if (!std.ascii.startsWithIgnoreCase(declared, "text/plain")) {
            httputil.writeError(res, 415, err.invalid_file_type, null);
            return;
        }
    } else if (!looksTextPlain(head)) {
        httputil.writeError(res, 415, err.invalid_file_type, null);
        return;
    }

    // Content inspection: reject sniffed-binary lying as text, oversized, or
    // non-UTF-8 payloads.
    if (containsNull(head)) {
        httputil.writeError(res, 415, err.not_plain_text, null);
        return;
    }
    if (part.content.len > max_file_bytes) {
        httputil.writeError(res, 413, err.file_size_exceeded, null);
        return;
    }
    if (containsNull(part.content) or !std.unicode.utf8ValidateSlice(part.content)) {
        httputil.writeError(res, 415, err.not_plain_text, null);
        return;
    }

    httputil.writeJson(res, 200, .{
        .filename = part.filename orelse "",
        .size = part.content.len,
        .content = part.content,
    }, .{});
}

fn trimOrNone(s: []const u8) []const u8 {
    const t = std.mem.trim(u8, s, &std.ascii.whitespace);
    return if (t.len == 0) "none" else t;
}

fn safeInt(s: []const u8, default: i64) i64 {
    const t = std.mem.trim(u8, s, &std.ascii.whitespace);
    if (t.len == 0) return default;
    if (std.mem.indexOfScalar(u8, t, '.') != null) return default;
    const n = std.fmt.parseInt(i64, t, 10) catch return default;
    if (n < -max_safe_int or n > max_safe_int) return default;
    return n;
}

fn containsNull(bytes: []const u8) bool {
    return std.mem.indexOfScalar(u8, bytes, 0) != null;
}

/// Cheap stand-in for content sniffing: treat a leading chunk as plain text
/// unless it carries NUL bytes (matches the contract's binary fixtures).
fn looksTextPlain(head: []const u8) bool {
    return !containsNull(head);
}
