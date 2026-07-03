const std = @import("std");
const pg = @import("pg");
const uuid = @import("../uuid.zig");
const user = @import("../user.zig");

const User = user.User;

/// Postgres repository backed by pg.zig's native connection pool. Pool size
/// (50) matches the other servers in the suite for fairness.
pub const Postgres = struct {
    pool: *pg.Pool,

    pub fn init(io: std.Io, allocator: std.mem.Allocator, url: []const u8) !Postgres {
        const uri = try std.Uri.parse(url);
        const pool = try pg.Pool.initUri(io, allocator, uri, .{ .size = 50, .timeout = 10_000 });
        return .{ .pool = pool };
    }

    pub fn deinit(self: *Postgres) void {
        self.pool.deinit();
    }

    pub fn health(self: *Postgres) bool {
        _ = self.pool.exec("SELECT 1", .{}) catch return false;
        return true;
    }

    pub fn create(self: *Postgres, arena: std.mem.Allocator, data: user.CreateUser) !User {
        var id_buf: [36]u8 = undefined;
        uuid.v7(&id_buf);
        const id = try arena.dupe(u8, &id_buf);
        _ = try self.pool.exec(
            "INSERT INTO users (id, name, email, favorite_number) VALUES ($1::uuid, $2, $3, $4)",
            .{ id, data.name, data.email, data.favoriteNumber },
        );
        return .{ .id = id, .name = data.name, .email = data.email, .favoriteNumber = data.favoriteNumber };
    }

    pub fn find(self: *Postgres, arena: std.mem.Allocator, id: []const u8) !?User {
        if (!uuid.isValid(id)) return null;
        var row = (try self.pool.row(
            "SELECT id::text, name, email, favorite_number FROM users WHERE id = $1::uuid",
            .{id},
        )) orelse return null;
        defer row.deinit() catch {};
        return try rowToUser(arena, &row);
    }

    pub fn update(self: *Postgres, arena: std.mem.Allocator, id: []const u8, data: user.UpdateUser) !?User {
        if (!uuid.isValid(id)) return null;
        var row = (try self.pool.row(
            "UPDATE users SET name = COALESCE($2, name), email = COALESCE($3, email), " ++
                "favorite_number = COALESCE($4, favorite_number) WHERE id = $1::uuid " ++
                "RETURNING id::text, name, email, favorite_number",
            .{ id, data.name, data.email, data.favoriteNumber },
        )) orelse return null;
        defer row.deinit() catch {};
        return try rowToUser(arena, &row);
    }

    pub fn delete(self: *Postgres, id: []const u8) !bool {
        if (!uuid.isValid(id)) return false;
        const affected = (try self.pool.exec("DELETE FROM users WHERE id = $1::uuid", .{id})) orelse 0;
        return affected > 0;
    }

    pub fn deleteAll(self: *Postgres) !void {
        _ = try self.pool.exec("DELETE FROM users", .{});
    }

    fn rowToUser(arena: std.mem.Allocator, row: *pg.QueryRow) !User {
        return .{
            .id = try arena.dupe(u8, try row.get([]const u8, 0)),
            .name = try arena.dupe(u8, try row.get([]const u8, 1)),
            .email = try arena.dupe(u8, try row.get([]const u8, 2)),
            .favoriteNumber = try row.get(?i32, 3),
        };
    }
};
