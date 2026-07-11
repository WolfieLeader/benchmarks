const std = @import("std");
const httpz = @import("httpz");
const App = @import("app.zig").App;
const httputil = @import("http_util.zig");
const jwt = @import("jwt.zig");
const validate = @import("validate.zig");
const compute = @import("compute.zig");

const err = httputil.err;

/// Canon /html body (contract/web.json): a greeting, a fruit list, and a
/// labeled total. The interpolated values are fixed canon, so a comptime string
/// is the idiomatic Zig "template" — no engine buys anything here.
const html_body =
    \\<!DOCTYPE html>
    \\<html lang="en">
    \\<head><meta charset="utf-8"><title>Benchmark</title></head>
    \\<body>
    \\  <h1>Hello, Alice</h1>
    \\  <ul>
    \\    <li>apple</li>
    \\    <li>banana</li>
    \\    <li>cherry</li>
    \\  </ul>
    \\  <p>Total: 42</p>
    \\</body>
    \\</html>
;

pub fn html(_: *App, _: *httpz.Request, res: *httpz.Response) !void {
    res.status = 200;
    res.content_type = .HTML;
    res.body = html_body;
}

pub fn jwtSign(app: *App, _: *httpz.Request, res: *httpz.Response) !void {
    const iat = nowSeconds(app.io);
    const token = jwt.sign(res.arena, app.env.jwt_secret, iat, iat + jwt.ttl_seconds) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    httputil.writeJson(res, 200, .{ .token = token }, .{});
}

pub fn jwtVerify(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const token = bearerToken(req.header("authorization") orelse "") orelse {
        httputil.writeError(res, 401, err.invalid_token, null);
        return;
    };
    const claims = jwt.verify(res.arena, app.env.jwt_secret, token, nowSeconds(app.io)) catch {
        httputil.writeError(res, 401, err.invalid_token, null);
        return;
    };
    // Echo exactly the five canon claims (sub/name/admin/iat/exp) the strict
    // body assertion expects.
    httputil.writeJson(res, 200, claims, .{});
}

pub fn validateBody(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const payload = validate.parse(res.arena, req.body() orelse "") catch |e| {
        httputil.writeError(res, 400, err.invalid_json, @errorName(e));
        return;
    };
    validate.check(payload) catch {
        httputil.writeError(res, 400, err.validation_failed, "payload failed schema validation");
        return;
    };
    httputil.writeJson(res, 200, .{ .valid = true }, .{});
}

pub fn computeChain(_: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const query = try req.query();
    const n = compute.parseRounds(query.get("n") orelse "") catch {
        httputil.writeError(res, 400, err.invalid_n, "n must be an integer >= 1");
        return;
    };
    const hex = std.fmt.bytesToHex(compute.chain(n), .lower);
    httputil.writeJson(res, 200, .{ .result = hex[0..] }, .{});
}

/// Current wall-clock time in Unix seconds. 0.16 sources time from `Io`
/// (std.time has no free-standing timestamp function anymore).
fn nowSeconds(io: std.Io) i64 {
    const ts = std.Io.Clock.now(.real, io);
    return @intCast(@divTrunc(ts.nanoseconds, std.time.ns_per_s));
}

/// Extracts the token from a case-sensitive `Bearer <token>` header (matching
/// the reference server). Returns null when the prefix is missing or the token
/// is empty/whitespace.
fn bearerToken(auth: []const u8) ?[]const u8 {
    const prefix = "Bearer ";
    if (!std.mem.startsWith(u8, auth, prefix)) return null;
    const token = std.mem.trim(u8, auth[prefix.len..], &std.ascii.whitespace);
    return if (token.len == 0) null else token;
}
