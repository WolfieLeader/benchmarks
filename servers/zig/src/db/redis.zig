const std = @import("std");
const uuid = @import("../uuid.zig");
const user = @import("../user.zig");

const Io = std.Io;
const User = user.User;

const pool_size = 50; // matches the postgres pool for cross-server fairness
const key_prefix = "user:";

/// A single Redis connection. Reader/writer are created per-operation from the
/// stream (each request/reply fully drains the socket, so nothing is buffered
/// across operations).
const Conn = struct {
    stream: Io.net.Stream,
    broken: bool = false,
};

/// Minimal hand-rolled RESP2 client with a fixed-size blocking connection pool.
/// okredis does not compile on Zig 0.16.0 stable (its bulk/array decode path
/// broke on the std.Io redesign), so we speak the wire protocol directly.
pub const Redis = struct {
    allocator: std.mem.Allocator,
    io: Io,
    host: []const u8,
    port: u16,
    mutex: Io.Mutex = .init,
    cond: Io.Condition = .init,
    idle: []*Conn,
    idle_count: usize = 0,

    pub fn init(io: Io, allocator: std.mem.Allocator, url: []const u8) !Redis {
        const parsed = parseUrl(url);
        const host = try allocator.dupe(u8, parsed.host);
        const idle = try allocator.alloc(*Conn, pool_size);
        var self: Redis = .{
            .allocator = allocator,
            .io = io,
            .host = host,
            .port = parsed.port,
            .idle = idle,
            .idle_count = 0,
        };
        // Eagerly open the pool; a failure here surfaces at startup.
        for (0..pool_size) |_| {
            const conn = try self.allocator.create(Conn);
            conn.* = .{ .stream = try self.dial() };
            idle[self.idle_count] = conn;
            self.idle_count += 1;
        }
        return self;
    }

    fn dial(self: *Redis) !Io.net.Stream {
        const hostname: Io.net.HostName = try .init(self.host);
        return hostname.connect(self.io, self.port, .{ .mode = .stream });
    }

    fn acquire(self: *Redis) *Conn {
        self.mutex.lockUncancelable(self.io);
        defer self.mutex.unlock(self.io);
        while (self.idle_count == 0) self.cond.waitUncancelable(self.io, &self.mutex);
        self.idle_count -= 1;
        return self.idle[self.idle_count];
    }

    fn release(self: *Redis, conn: *Conn) void {
        if (conn.broken) {
            // Best-effort reconnect so the pool slot stays usable.
            conn.stream.close(self.io);
            if (self.dial()) |s| {
                conn.stream = s;
                conn.broken = false;
            } else |_| {}
        }
        self.mutex.lockUncancelable(self.io);
        defer self.mutex.unlock(self.io);
        self.idle[self.idle_count] = conn;
        self.idle_count += 1;
        self.cond.signal(self.io);
    }

    pub fn health(self: *Redis) bool {
        const conn = self.acquire();
        defer self.release(conn);
        return self.ping(conn);
    }

    fn ping(self: *Redis, conn: *Conn) bool {
        var wbuf: [64]u8 = undefined;
        var rbuf: [64]u8 = undefined;
        var writer = conn.stream.writer(self.io, &wbuf);
        var reader = conn.stream.reader(self.io, &rbuf);
        sendCommand(&writer.interface, &.{"PING"}) catch {
            conn.broken = true;
            return false;
        };
        const t = reader.interface.takeByte() catch {
            conn.broken = true;
            return false;
        };
        _ = takeLine(&reader.interface) catch {
            conn.broken = true;
            return false;
        };
        return t == '+' or t == ':';
    }

    pub fn create(self: *Redis, arena: std.mem.Allocator, data: user.CreateUser) !User {
        var id_buf: [36]u8 = undefined;
        uuid.v7(&id_buf);
        const id = try arena.dupe(u8, &id_buf);
        const key = try std.fmt.allocPrint(arena, key_prefix ++ "{s}", .{id});

        const conn = self.acquire();
        defer self.release(conn);
        errdefer conn.broken = true;

        var wbuf: [512]u8 = undefined;
        var rbuf: [256]u8 = undefined;
        var writer = conn.stream.writer(self.io, &wbuf);
        var reader = conn.stream.reader(self.io, &rbuf);

        if (data.favoriteNumber) |n| {
            var num_buf: [16]u8 = undefined;
            const num = try std.fmt.bufPrint(&num_buf, "{d}", .{n});
            try sendCommand(&writer.interface, &.{ "HSET", key, "name", data.name, "email", data.email, "favoriteNumber", num });
        } else {
            try sendCommand(&writer.interface, &.{ "HSET", key, "name", data.name, "email", data.email });
        }
        _ = try readInteger(&reader.interface);

        return .{ .id = id, .name = data.name, .email = data.email, .favoriteNumber = data.favoriteNumber };
    }

    pub fn find(self: *Redis, arena: std.mem.Allocator, id: []const u8) !?User {
        const key = try std.fmt.allocPrint(arena, key_prefix ++ "{s}", .{id});
        const conn = self.acquire();
        defer self.release(conn);
        errdefer conn.broken = true;
        return self.hgetUser(arena, conn, id, key);
    }

    fn hgetUser(self: *Redis, arena: std.mem.Allocator, conn: *Conn, id: []const u8, key: []const u8) !?User {
        var wbuf: [256]u8 = undefined;
        var rbuf: [1024]u8 = undefined;
        var writer = conn.stream.writer(self.io, &wbuf);
        var reader = conn.stream.reader(self.io, &rbuf);

        try sendCommand(&writer.interface, &.{ "HMGET", key, "name", "email", "favoriteNumber" });
        const count = try readArrayHeader(&reader.interface);
        if (count != 3) return error.Protocol;
        const name = try readBulk(&reader.interface, arena);
        const email = try readBulk(&reader.interface, arena);
        const fav_str = try readBulk(&reader.interface, arena);

        if (name == null or email == null) return null;
        var favorite: ?i32 = null;
        if (fav_str) |fs| favorite = std.fmt.parseInt(i32, fs, 10) catch null;
        return .{ .id = try arena.dupe(u8, id), .name = name.?, .email = email.?, .favoriteNumber = favorite };
    }

    pub fn update(self: *Redis, arena: std.mem.Allocator, id: []const u8, data: user.UpdateUser) !?User {
        const key = try std.fmt.allocPrint(arena, key_prefix ++ "{s}", .{id});
        const conn = self.acquire();
        defer self.release(conn);
        errdefer conn.broken = true;

        {
            var wbuf: [256]u8 = undefined;
            var rbuf: [64]u8 = undefined;
            var writer = conn.stream.writer(self.io, &wbuf);
            var reader = conn.stream.reader(self.io, &rbuf);
            try sendCommand(&writer.interface, &.{ "EXISTS", key });
            if (try readInteger(&reader.interface) == 0) return null;
        }

        // Apply only the provided fields.
        var argv: [8][]const u8 = undefined;
        var n: usize = 0;
        argv[n] = "HSET";
        n += 1;
        argv[n] = key;
        n += 1;
        if (data.name) |v| {
            argv[n] = "name";
            argv[n + 1] = v;
            n += 2;
        }
        if (data.email) |v| {
            argv[n] = "email";
            argv[n + 1] = v;
            n += 2;
        }
        var num_buf: [16]u8 = undefined;
        if (data.favoriteNumber) |v| {
            argv[n] = "favoriteNumber";
            argv[n + 1] = try std.fmt.bufPrint(&num_buf, "{d}", .{v});
            n += 2;
        }
        if (n > 2) {
            var wbuf: [512]u8 = undefined;
            var rbuf: [64]u8 = undefined;
            var writer = conn.stream.writer(self.io, &wbuf);
            var reader = conn.stream.reader(self.io, &rbuf);
            try sendCommand(&writer.interface, argv[0..n]);
            _ = try readInteger(&reader.interface);
        }

        return self.hgetUser(arena, conn, id, key);
    }

    pub fn delete(self: *Redis, arena: std.mem.Allocator, id: []const u8) !bool {
        const key = try std.fmt.allocPrint(arena, key_prefix ++ "{s}", .{id});
        const conn = self.acquire();
        defer self.release(conn);
        errdefer conn.broken = true;

        var wbuf: [256]u8 = undefined;
        var rbuf: [64]u8 = undefined;
        var writer = conn.stream.writer(self.io, &wbuf);
        var reader = conn.stream.reader(self.io, &rbuf);
        try sendCommand(&writer.interface, &.{ "DEL", key });
        return try readInteger(&reader.interface) > 0;
    }

    pub fn deleteAll(self: *Redis, arena: std.mem.Allocator) !void {
        const conn = self.acquire();
        defer self.release(conn);
        errdefer conn.broken = true;

        var cursor: []const u8 = "0";
        while (true) {
            var wbuf: [128]u8 = undefined;
            var rbuf: [8192]u8 = undefined;
            var writer = conn.stream.writer(self.io, &wbuf);
            var reader = conn.stream.reader(self.io, &rbuf);
            try sendCommand(&writer.interface, &.{ "SCAN", cursor, "MATCH", key_prefix ++ "*", "COUNT", "100" });

            // SCAN reply: *2 [ bulk next-cursor, *N [ bulk keys... ] ]
            if (try readArrayHeader(&reader.interface) != 2) return error.Protocol;
            const next = (try readBulk(&reader.interface, arena)) orelse return error.Protocol;
            const key_count = try readArrayHeader(&reader.interface);

            // COUNT is only a hint, so a single SCAN batch can return more keys
            // than any fixed size; collect the whole batch in an arena-backed
            // list (argv[0] = "DEL") so no key is ever silently dropped.
            var del_argv: std.ArrayList([]const u8) = .empty;
            try del_argv.append(arena, "DEL");
            var i: i64 = 0;
            while (i < key_count) : (i += 1) {
                const k = (try readBulk(&reader.interface, arena)) orelse return error.Protocol;
                try del_argv.append(arena, k);
            }
            if (del_argv.items.len > 1) {
                var dwbuf: [8192]u8 = undefined;
                var drbuf: [64]u8 = undefined;
                var dwriter = conn.stream.writer(self.io, &dwbuf);
                var dreader = conn.stream.reader(self.io, &drbuf);
                try sendCommand(&dwriter.interface, del_argv.items);
                _ = try readInteger(&dreader.interface);
            }

            if (std.mem.eql(u8, next, "0")) break;
            cursor = next;
        }
    }
};

const UrlParts = struct { host: []const u8, port: u16 };

fn parseUrl(url: []const u8) UrlParts {
    var rest = url;
    if (std.mem.indexOf(u8, rest, "://")) |i| rest = rest[i + 3 ..];
    // strip any auth@ and trailing path
    if (std.mem.indexOfScalar(u8, rest, '@')) |i| rest = rest[i + 1 ..];
    if (std.mem.indexOfScalar(u8, rest, '/')) |i| rest = rest[0..i];
    var host: []const u8 = rest;
    var port: u16 = 6379;
    if (std.mem.lastIndexOfScalar(u8, rest, ':')) |i| {
        host = rest[0..i];
        port = std.fmt.parseInt(u16, rest[i + 1 ..], 10) catch 6379;
    }
    if (host.len == 0) host = "localhost";
    return .{ .host = host, .port = port };
}

fn sendCommand(w: *Io.Writer, args: []const []const u8) !void {
    try w.print("*{d}\r\n", .{args.len});
    for (args) |a| {
        try w.print("${d}\r\n", .{a.len});
        try w.writeAll(a);
        try w.writeAll("\r\n");
    }
    try w.flush();
}

/// Reads one RESP line (up to CRLF), returning it without the trailing CR.
/// Valid only until the next read from `r`.
fn takeLine(r: *Io.Reader) ![]const u8 {
    // Inclusive so the trailing '\n' is consumed from the stream; then strip
    // the CRLF. (Exclusive would leave '\n' and desync the next read.)
    const line = try r.takeDelimiterInclusive('\n');
    return std.mem.trimEnd(u8, line, "\r\n");
}

fn readInteger(r: *Io.Reader) !i64 {
    const t = try r.takeByte();
    const line = try takeLine(r);
    if (t == ':') return std.fmt.parseInt(i64, line, 10);
    if (t == '-') return error.RedisError;
    return error.Protocol;
}

fn readArrayHeader(r: *Io.Reader) !i64 {
    const t = try r.takeByte();
    const line = try takeLine(r);
    if (t != '*') return error.Protocol;
    return std.fmt.parseInt(i64, line, 10);
}

/// Reads a RESP bulk string into `arena`, or null for `$-1`.
fn readBulk(r: *Io.Reader, arena: std.mem.Allocator) !?[]const u8 {
    const t = try r.takeByte();
    if (t != '$') return error.Protocol;
    const line = try takeLine(r);
    const len = try std.fmt.parseInt(i64, line, 10);
    if (len < 0) return null;
    const buf = try arena.alloc(u8, @intCast(len));
    try r.readSliceAll(buf);
    _ = try r.takeArray(2); // trailing CRLF
    return buf;
}
