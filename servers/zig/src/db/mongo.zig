const std = @import("std");
const user = @import("../user.zig");

const c = @cImport({
    @cInclude("mongoc/mongoc.h");
});

const User = user.User;

/// MongoDB repository via the libmongoc C driver (no living pure-Zig driver
/// exists). A `mongoc_client_pool_t` is used because `mongoc_client_t` is not
/// thread-safe and http.zig dispatches on a worker thread pool.
pub const Mongo = struct {
    pool: ?*c.mongoc_client_pool_t,
    db_name: [:0]const u8,
    allocator: std.mem.Allocator,

    pub fn init(allocator: std.mem.Allocator, url: []const u8, db_name: []const u8) !Mongo {
        c.mongoc_init();

        const urlz = try allocator.dupeZ(u8, url);
        defer allocator.free(urlz);
        var err: c.bson_error_t = undefined;
        const uri = c.mongoc_uri_new_with_error(urlz.ptr, &err) orelse return error.MongoUri;
        // client_pool takes ownership of the uri.
        const pool = c.mongoc_client_pool_new(uri) orelse {
            c.mongoc_uri_destroy(uri);
            return error.MongoPool;
        };
        return .{ .pool = pool, .db_name = try allocator.dupeZ(u8, db_name), .allocator = allocator };
    }

    pub fn deinit(self: *Mongo) void {
        if (self.pool) |p| c.mongoc_client_pool_destroy(p);
        self.allocator.free(self.db_name);
        c.mongoc_cleanup();
    }

    fn collection(self: *Mongo, client: ?*c.mongoc_client_t) ?*c.mongoc_collection_t {
        return c.mongoc_client_get_collection(client, self.db_name.ptr, "users");
    }

    pub fn health(self: *Mongo) bool {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return false;
        defer c.mongoc_client_pool_push(self.pool, client);

        const cmd = c.bson_new();
        defer c.bson_destroy(cmd);
        _ = c.bson_append_int32(cmd, "ping", -1, 1);
        var err: c.bson_error_t = undefined;
        return c.mongoc_client_command_simple(client, self.db_name.ptr, cmd, null, null, &err);
    }

    pub fn create(self: *Mongo, arena: std.mem.Allocator, data: user.CreateUser) !User {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return error.MongoPool;
        defer c.mongoc_client_pool_push(self.pool, client);
        const coll = self.collection(client);
        defer c.mongoc_collection_destroy(coll);

        var oid: c.bson_oid_t = undefined;
        c.bson_oid_init(&oid, null);
        var oid_buf: [25]u8 = undefined;
        c.bson_oid_to_string(&oid, &oid_buf);
        const id = try arena.dupe(u8, oid_buf[0..24]);

        const doc = c.bson_new();
        defer c.bson_destroy(doc);
        _ = c.bson_append_oid(doc, "_id", -1, &oid);
        _ = c.bson_append_utf8(doc, "name", -1, data.name.ptr, @intCast(data.name.len));
        _ = c.bson_append_utf8(doc, "email", -1, data.email.ptr, @intCast(data.email.len));
        if (data.favoriteNumber) |n| _ = c.bson_append_int32(doc, "favoriteNumber", -1, n);

        var err: c.bson_error_t = undefined;
        if (!c.mongoc_collection_insert_one(coll, doc, null, null, &err)) return error.MongoInsert;

        return .{ .id = id, .name = data.name, .email = data.email, .favoriteNumber = data.favoriteNumber };
    }

    pub fn find(self: *Mongo, arena: std.mem.Allocator, id: []const u8) !?User {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return error.MongoPool;
        defer c.mongoc_client_pool_push(self.pool, client);
        const coll = self.collection(client);
        defer c.mongoc_collection_destroy(coll);
        return findWith(arena, coll, id);
    }

    fn findWith(arena: std.mem.Allocator, coll: ?*c.mongoc_collection_t, id: []const u8) !?User {
        var oid: c.bson_oid_t = undefined;
        if (!oidFromString(id, &oid)) return null;

        const query = c.bson_new();
        defer c.bson_destroy(query);
        _ = c.bson_append_oid(query, "_id", -1, &oid);

        const cursor = c.mongoc_collection_find_with_opts(coll, query, null, null);
        defer c.mongoc_cursor_destroy(cursor);

        var doc: [*c]const c.bson_t = undefined;
        if (!c.mongoc_cursor_next(cursor, &doc)) return null;
        return try docToUser(arena, doc, id);
    }

    pub fn update(self: *Mongo, arena: std.mem.Allocator, id: []const u8, data: user.UpdateUser) !?User {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return error.MongoPool;
        defer c.mongoc_client_pool_push(self.pool, client);
        const coll = self.collection(client);
        defer c.mongoc_collection_destroy(coll);

        if ((try findWith(arena, coll, id)) == null) return null;

        const has_update = data.name != null or data.email != null or data.favoriteNumber != null;
        if (has_update) {
            var oid: c.bson_oid_t = undefined;
            _ = oidFromString(id, &oid);

            const query = c.bson_new();
            defer c.bson_destroy(query);
            _ = c.bson_append_oid(query, "_id", -1, &oid);

            const update_doc = c.bson_new();
            defer c.bson_destroy(update_doc);
            var set_child: c.bson_t = undefined;
            _ = c.bson_append_document_begin(update_doc, "$set", -1, &set_child);
            if (data.name) |v| _ = c.bson_append_utf8(&set_child, "name", -1, v.ptr, @intCast(v.len));
            if (data.email) |v| _ = c.bson_append_utf8(&set_child, "email", -1, v.ptr, @intCast(v.len));
            if (data.favoriteNumber) |v| _ = c.bson_append_int32(&set_child, "favoriteNumber", -1, v);
            _ = c.bson_append_document_end(update_doc, &set_child);

            var err: c.bson_error_t = undefined;
            if (!c.mongoc_collection_update_one(coll, query, update_doc, null, null, &err)) return error.MongoUpdate;
        }
        return findWith(arena, coll, id);
    }

    pub fn delete(self: *Mongo, id: []const u8) !bool {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return error.MongoPool;
        defer c.mongoc_client_pool_push(self.pool, client);
        const coll = self.collection(client);
        defer c.mongoc_collection_destroy(coll);

        var oid: c.bson_oid_t = undefined;
        if (!oidFromString(id, &oid)) return false;

        const query = c.bson_new();
        defer c.bson_destroy(query);
        _ = c.bson_append_oid(query, "_id", -1, &oid);

        var reply: c.bson_t = undefined;
        var err: c.bson_error_t = undefined;
        if (!c.mongoc_collection_delete_one(coll, query, null, &reply, &err)) {
            c.bson_destroy(&reply);
            return error.MongoDelete;
        }
        defer c.bson_destroy(&reply);

        var iter: c.bson_iter_t = undefined;
        if (c.bson_iter_init_find(&iter, &reply, "deletedCount")) {
            return c.bson_iter_as_int64(&iter) > 0;
        }
        return false;
    }

    pub fn deleteAll(self: *Mongo) !void {
        const client = c.mongoc_client_pool_pop(self.pool) orelse return error.MongoPool;
        defer c.mongoc_client_pool_push(self.pool, client);
        const coll = self.collection(client);
        defer c.mongoc_collection_destroy(coll);

        const selector = c.bson_new(); // {} matches everything
        defer c.bson_destroy(selector);
        var err: c.bson_error_t = undefined;
        if (!c.mongoc_collection_delete_many(coll, selector, null, null, &err)) return error.MongoDelete;
    }

    fn oidFromString(id: []const u8, oid: *c.bson_oid_t) bool {
        if (id.len != 24) return false;
        var idz: [25]u8 = undefined;
        @memcpy(idz[0..24], id);
        idz[24] = 0;
        if (!c.bson_oid_is_valid(&idz, 24)) return false;
        c.bson_oid_init_from_string(oid, &idz);
        return true;
    }

    fn docToUser(arena: std.mem.Allocator, doc: [*c]const c.bson_t, id: []const u8) !User {
        var result: User = .{ .id = try arena.dupe(u8, id), .name = "", .email = "", .favoriteNumber = null };
        var iter: c.bson_iter_t = undefined;
        if (!c.bson_iter_init(&iter, doc)) return result;
        while (c.bson_iter_next(&iter)) {
            const key = std.mem.span(c.bson_iter_key(&iter));
            if (std.mem.eql(u8, key, "name") and c.bson_iter_type(&iter) == c.BSON_TYPE_UTF8) {
                var len: u32 = 0;
                const ptr = c.bson_iter_utf8(&iter, &len);
                result.name = try arena.dupe(u8, ptr[0..len]);
            } else if (std.mem.eql(u8, key, "email") and c.bson_iter_type(&iter) == c.BSON_TYPE_UTF8) {
                var len: u32 = 0;
                const ptr = c.bson_iter_utf8(&iter, &len);
                result.email = try arena.dupe(u8, ptr[0..len]);
            } else if (std.mem.eql(u8, key, "favoriteNumber") and c.bson_iter_type(&iter) == c.BSON_TYPE_INT32) {
                result.favoriteNumber = c.bson_iter_int32(&iter);
            }
        }
        return result;
    }
};
