const std = @import("std");
const httpz = @import("httpz");
const App = @import("app.zig").App;
const httputil = @import("http_util.zig");
const user = @import("user.zig");

const err = httputil.err;
const json_omit_null: std.json.Stringify.Options = .{ .emit_null_optional_fields = false };

pub fn health(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const name = req.param("database") orelse "";
    const backend = app.resolve(name) orelse {
        httputil.writeText(res, 503, "Service Unavailable");
        return;
    };
    if (backend.health()) {
        httputil.writeText(res, 200, "OK");
    } else {
        httputil.writeText(res, 503, "Service Unavailable");
    }
}

pub fn createUser(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;

    const data = user.parseCreate(res.arena, req.body() orelse "") catch |e| {
        httputil.writeError(res, 400, err.invalid_json, @errorName(e));
        return;
    };
    user.validateCreate(data) catch {
        httputil.writeError(res, 400, err.invalid_json, "validation failed");
        return;
    };

    const created = backend.create(res.arena, data) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    httputil.writeJson(res, 201, created, json_omit_null);
}

pub fn getUser(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;
    const id = req.param("id") orelse "";

    const found = backend.find(res.arena, id) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    if (found) |u| {
        httputil.writeJson(res, 200, u, json_omit_null);
    } else {
        try notFoundUser(res, id);
    }
}

pub fn updateUser(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;
    const id = req.param("id") orelse "";

    const data = user.parseUpdate(res.arena, req.body() orelse "") catch |e| {
        httputil.writeError(res, 400, err.invalid_json, @errorName(e));
        return;
    };
    user.validateUpdate(data) catch {
        httputil.writeError(res, 400, err.invalid_json, "validation failed");
        return;
    };

    const updated = backend.update(res.arena, id, data) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    if (updated) |u| {
        httputil.writeJson(res, 200, u, json_omit_null);
    } else {
        try notFoundUser(res, id);
    }
}

pub fn deleteUser(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;
    const id = req.param("id") orelse "";

    const deleted = backend.delete(res.arena, id) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    if (deleted) {
        httputil.writeJson(res, 200, .{ .success = true }, .{});
    } else {
        try notFoundUser(res, id);
    }
}

pub fn deleteAll(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;
    backend.deleteAll(res.arena) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    httputil.writeJson(res, 200, .{ .success = true }, .{});
}

pub fn reset(app: *App, req: *httpz.Request, res: *httpz.Response) !void {
    const backend = (try resolveOr404(app, req, res)) orelse return;
    backend.deleteAll(res.arena) catch {
        httputil.writeError(res, 500, err.internal, null);
        return;
    };
    httputil.writeJson(res, 200, .{ .status = "ok" }, .{});
}

fn resolveOr404(app: *App, req: *httpz.Request, res: *httpz.Response) !?@import("app.zig").Backend {
    const name = req.param("database") orelse "";
    if (app.resolve(name)) |b| return b;
    const details = try std.fmt.allocPrint(res.arena, "unknown database type: {s}", .{name});
    httputil.writeError(res, 404, err.not_found, details);
    return null;
}

fn notFoundUser(res: *httpz.Response, id: []const u8) !void {
    const details = try std.fmt.allocPrint(res.arena, "user with id {s} not found", .{id});
    httputil.writeError(res, 404, err.not_found, details);
}
