const std = @import("std");
const httpz = @import("httpz");
const Env = @import("env.zig").Env;
const httputil = @import("http_util.zig");
const user = @import("user.zig");

const Postgres = @import("db/postgres.zig").Postgres;
const Redis = @import("db/redis.zig").Redis;
const Mongo = @import("db/mongo.zig").Mongo;
const Cassandra = @import("db/cassandra.zig").Cassandra;

const User = user.User;

/// A database backend, resolved from the `:database` path parameter. Dispatches
/// each repository operation to the concrete client. Keeps the db route
/// handlers backend-agnostic (the same handler serves all four engines).
pub const Backend = union(enum) {
    postgres: *Postgres,
    redis: *Redis,
    mongo: *Mongo,
    cassandra: *Cassandra,

    pub fn health(self: Backend) bool {
        return switch (self) {
            inline else => |repo| repo.health(),
        };
    }

    pub fn create(self: Backend, arena: std.mem.Allocator, data: user.CreateUser) !User {
        return switch (self) {
            inline else => |repo| repo.create(arena, data),
        };
    }

    pub fn find(self: Backend, arena: std.mem.Allocator, id: []const u8) !?User {
        return switch (self) {
            inline else => |repo| repo.find(arena, id),
        };
    }

    pub fn update(self: Backend, arena: std.mem.Allocator, id: []const u8, data: user.UpdateUser) !?User {
        return switch (self) {
            inline else => |repo| repo.update(arena, id, data),
        };
    }

    pub fn delete(self: Backend, arena: std.mem.Allocator, id: []const u8) !bool {
        return switch (self) {
            .postgres => |r| r.delete(id),
            .redis => |r| r.delete(arena, id),
            .mongo => |r| r.delete(id),
            .cassandra => |r| r.delete(arena, id),
        };
    }

    pub fn deleteAll(self: Backend, arena: std.mem.Allocator) !void {
        return switch (self) {
            .postgres => |r| r.deleteAll(),
            .redis => |r| r.deleteAll(arena),
            .mongo => |r| r.deleteAll(),
            .cassandra => |r| r.deleteAll(),
        };
    }
};

/// Per-process application state shared across all requests. Holds the four
/// database clients and the resolved environment.
pub const App = struct {
    allocator: std.mem.Allocator,
    // Process-wide Io (0.16 sources wall-clock time from it — the web suite's
    // JWT handlers need the current time for iat/exp).
    io: std.Io,
    env: Env,
    postgres: Postgres,
    redis: Redis,
    mongo: Mongo,
    cassandra: Cassandra,

    pub fn resolve(self: *App, name: []const u8) ?Backend {
        if (std.mem.eql(u8, name, "postgres")) return .{ .postgres = &self.postgres };
        if (std.mem.eql(u8, name, "mongodb")) return .{ .mongo = &self.mongo };
        if (std.mem.eql(u8, name, "redis")) return .{ .redis = &self.redis };
        if (std.mem.eql(u8, name, "cassandra")) return .{ .cassandra = &self.cassandra };
        return null;
    }

    /// httpz dispatch hook: request logging in dev, silent in prod (logger off
    /// when ENV=prod, matching every other server).
    pub fn dispatch(self: *App, action: httpz.Action(*App), req: *httpz.Request, res: *httpz.Response) !void {
        if (self.env.is_prod) return action(self, req, res);
        try action(self, req, res);
        std.log.info("{s} {s} {d}", .{ @tagName(req.method), req.url.path, res.status });
    }

    pub fn notFound(_: *App, _: *httpz.Request, res: *httpz.Response) !void {
        httputil.writeError(res, 404, httputil.err.not_found, null);
    }

    pub fn uncaughtError(_: *App, _: *httpz.Request, res: *httpz.Response, err: anyerror) void {
        httputil.writeError(res, 500, httputil.err.internal, @errorName(err));
    }
};
