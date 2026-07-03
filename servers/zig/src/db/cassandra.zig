const std = @import("std");
const uuid = @import("../uuid.zig");
const user = @import("../user.zig");

const c = @cImport({
    @cInclude("cassandra.h");
});

const User = user.User;

/// Cassandra repository backed by the DataStax/Apache C/C++ driver. The
/// CassSession is internally thread-safe with its own connection pool, so a
/// single shared session serves all http.zig worker threads. Connection is
/// lazy + retried, so a keyspace that is still being created at startup (the
/// compose `cassandra-init` step) resolves on a later health poll rather than
/// crashing the process.
pub const Cassandra = struct {
    cluster: ?*c.CassCluster,
    session: ?*c.CassSession = null,
    keyspace: [:0]const u8,
    allocator: std.mem.Allocator,
    io: std.Io,
    mutex: std.Io.Mutex = .init,

    pub fn init(io: std.Io, allocator: std.mem.Allocator, contact_points: []const u8, local_dc: []const u8, keyspace: []const u8) !Cassandra {
        const cluster = c.cass_cluster_new();
        const cpz = try allocator.dupeZ(u8, contact_points);
        defer allocator.free(cpz);
        _ = c.cass_cluster_set_contact_points(cluster, cpz.ptr);
        // DC-aware routing pinned to the local datacenter (mirrors the other
        // servers, e.g. gocql's DCAwareRoundRobinPolicy); remote-DC fallback
        // stays off (0 / cass_false).
        if (local_dc.len > 0) {
            _ = c.cass_cluster_set_load_balance_dc_aware_n(cluster, local_dc.ptr, local_dc.len, 0, c.cass_false);
        }
        return .{ .cluster = cluster, .keyspace = try allocator.dupeZ(u8, keyspace), .allocator = allocator, .io = io };
    }

    pub fn deinit(self: *Cassandra) void {
        if (self.session) |s| {
            const f = c.cass_session_close(s);
            c.cass_future_free(f);
            c.cass_session_free(s);
        }
        if (self.cluster) |cl| c.cass_cluster_free(cl);
        self.allocator.free(self.keyspace);
    }

    fn ensureConnected(self: *Cassandra) !void {
        self.mutex.lockUncancelable(self.io);
        defer self.mutex.unlock(self.io);
        if (self.session != null) return;

        const session = c.cass_session_new();
        const future = c.cass_session_connect_keyspace(session, self.cluster, self.keyspace.ptr);
        defer c.cass_future_free(future);
        if (c.cass_future_error_code(future) != c.CASS_OK) {
            c.cass_session_free(session);
            return error.CassandraConnect;
        }
        self.session = session;
    }

    pub fn health(self: *Cassandra) bool {
        self.ensureConnected() catch return false;
        const stmt = c.cass_statement_new("SELECT now() FROM system.local", 0);
        defer c.cass_statement_free(stmt);
        const future = c.cass_session_execute(self.session, stmt);
        defer c.cass_future_free(future);
        return c.cass_future_error_code(future) == c.CASS_OK;
    }

    /// Executes a non-SELECT statement, returning an error on CASS failure.
    fn exec(self: *Cassandra, stmt: ?*c.CassStatement) !void {
        const future = c.cass_session_execute(self.session, stmt);
        defer c.cass_future_free(future);
        if (c.cass_future_error_code(future) != c.CASS_OK) return error.CassandraQuery;
    }

    pub fn create(self: *Cassandra, arena: std.mem.Allocator, data: user.CreateUser) !User {
        try self.ensureConnected();
        var id_buf: [36]u8 = undefined;
        uuid.v7(self.io, &id_buf);
        const id = try arena.dupe(u8, &id_buf);

        var cass_uuid: c.CassUuid = undefined;
        const idz = zterm(id);
        _ = c.cass_uuid_from_string(&idz, &cass_uuid);

        const stmt = c.cass_statement_new("INSERT INTO users (id, name, email, favorite_number) VALUES (?, ?, ?, ?)", 4);
        defer c.cass_statement_free(stmt);
        _ = c.cass_statement_bind_uuid(stmt, 0, cass_uuid);
        _ = c.cass_statement_bind_string_n(stmt, 1, data.name.ptr, data.name.len);
        _ = c.cass_statement_bind_string_n(stmt, 2, data.email.ptr, data.email.len);
        if (data.favoriteNumber) |n| {
            _ = c.cass_statement_bind_int32(stmt, 3, n);
        } else {
            _ = c.cass_statement_bind_null(stmt, 3);
        }
        try self.exec(stmt);
        return .{ .id = id, .name = data.name, .email = data.email, .favoriteNumber = data.favoriteNumber };
    }

    pub fn find(self: *Cassandra, arena: std.mem.Allocator, id: []const u8) !?User {
        try self.ensureConnected();
        if (!uuid.isValid(id)) return null;
        var cass_uuid: c.CassUuid = undefined;
        const idz = zterm(id);
        if (c.cass_uuid_from_string(&idz, &cass_uuid) != c.CASS_OK) return null;

        const stmt = c.cass_statement_new("SELECT id, name, email, favorite_number FROM users WHERE id = ?", 1);
        defer c.cass_statement_free(stmt);
        _ = c.cass_statement_bind_uuid(stmt, 0, cass_uuid);

        const future = c.cass_session_execute(self.session, stmt);
        defer c.cass_future_free(future);
        if (c.cass_future_error_code(future) != c.CASS_OK) return error.CassandraQuery;

        const result = c.cass_future_get_result(future);
        defer c.cass_result_free(result);
        const row = c.cass_result_first_row(result) orelse return null;
        return try rowToUser(arena, row);
    }

    pub fn update(self: *Cassandra, arena: std.mem.Allocator, id: []const u8, data: user.UpdateUser) !?User {
        // Cassandra UPDATE is an upsert; mirror the other servers by confirming
        // the row exists first, then patching only the provided fields.
        const existing = (try self.find(arena, id)) orelse return null;

        var cass_uuid: c.CassUuid = undefined;
        const idz = zterm(id);
        _ = c.cass_uuid_from_string(&idz, &cass_uuid);

        var result = existing;
        if (data.name) |v| {
            try self.setColumn("name", cass_uuid, .{ .str = v });
            result.name = v;
        }
        if (data.email) |v| {
            try self.setColumn("email", cass_uuid, .{ .str = v });
            result.email = v;
        }
        if (data.favoriteNumber) |v| {
            try self.setColumn("favorite_number", cass_uuid, .{ .int = v });
            result.favoriteNumber = v;
        }
        return result;
    }

    const ColValue = union(enum) { str: []const u8, int: i32 };

    fn setColumn(self: *Cassandra, comptime column: []const u8, id: c.CassUuid, value: ColValue) !void {
        const stmt = c.cass_statement_new("UPDATE users SET " ++ column ++ " = ? WHERE id = ?", 2);
        defer c.cass_statement_free(stmt);
        switch (value) {
            .str => |s| _ = c.cass_statement_bind_string_n(stmt, 0, s.ptr, s.len),
            .int => |n| _ = c.cass_statement_bind_int32(stmt, 0, n),
        }
        _ = c.cass_statement_bind_uuid(stmt, 1, id);
        try self.exec(stmt);
    }

    pub fn delete(self: *Cassandra, arena: std.mem.Allocator, id: []const u8) !bool {
        if ((try self.find(arena, id)) == null) return false;
        var cass_uuid: c.CassUuid = undefined;
        const idz = zterm(id);
        _ = c.cass_uuid_from_string(&idz, &cass_uuid);

        const stmt = c.cass_statement_new("DELETE FROM users WHERE id = ?", 1);
        defer c.cass_statement_free(stmt);
        _ = c.cass_statement_bind_uuid(stmt, 0, cass_uuid);
        try self.exec(stmt);
        return true;
    }

    pub fn deleteAll(self: *Cassandra) !void {
        try self.ensureConnected();
        const stmt = c.cass_statement_new("TRUNCATE users", 0);
        defer c.cass_statement_free(stmt);
        try self.exec(stmt);
    }

    fn rowToUser(arena: std.mem.Allocator, row: ?*const c.CassRow) !User {
        var cass_uuid: c.CassUuid = undefined;
        _ = c.cass_value_get_uuid(c.cass_row_get_column(row, 0), &cass_uuid);
        var uuid_buf: [c.CASS_UUID_STRING_LENGTH]u8 = undefined;
        c.cass_uuid_string(cass_uuid, &uuid_buf);
        const id = try arena.dupe(u8, uuid_buf[0..36]);

        return .{
            .id = id,
            .name = try getString(arena, row, 1),
            .email = try getString(arena, row, 2),
            .favoriteNumber = getInt(row, 3),
        };
    }

    fn getString(arena: std.mem.Allocator, row: ?*const c.CassRow, index: usize) ![]const u8 {
        var ptr: [*c]const u8 = undefined;
        var len: usize = 0;
        _ = c.cass_value_get_string(c.cass_row_get_column(row, index), &ptr, &len);
        return arena.dupe(u8, ptr[0..len]);
    }

    fn getInt(row: ?*const c.CassRow, index: usize) ?i32 {
        const value = c.cass_row_get_column(row, index);
        if (c.cass_value_is_null(value) != 0) return null;
        var out: i32 = 0;
        if (c.cass_value_get_int32(value, &out) != c.CASS_OK) return null;
        return out;
    }
};

/// Null-terminate a 36-char UUID slice for the C API (callers guard length).
fn zterm(id: []const u8) [37]u8 {
    var out: [37]u8 = undefined;
    @memcpy(out[0..36], id[0..36]);
    out[36] = 0;
    return out;
}
