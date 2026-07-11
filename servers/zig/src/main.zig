const std = @import("std");
const httpz = @import("httpz");

const Env = @import("env.zig").Env;
const App = @import("app.zig").App;
const httputil = @import("http_util.zig");
const params = @import("routes_params.zig");
const db = @import("routes_db.zig");
const web = @import("routes_web.zig");

const Postgres = @import("db/postgres.zig").Postgres;
const Redis = @import("db/redis.zig").Redis;
const Mongo = @import("db/mongo.zig").Mongo;
const Cassandra = @import("db/cassandra.zig").Cassandra;

const Server = httpz.Server(*App);

// Set once main has a live server, so the signal handler can stop it.
var server_instance: ?*Server = null;

pub fn main(init: std.process.Init) !void {
    const allocator = std.heap.c_allocator;
    const io = init.io;

    const env = Env.load(init.environ_map);

    const app = try allocator.create(App);
    app.* = .{
        .allocator = allocator,
        .io = io,
        .env = env,
        .postgres = try Postgres.init(io, allocator, env.postgres_url),
        .redis = try Redis.init(io, allocator, env.redis_url),
        .mongo = try Mongo.init(allocator, env.mongodb_url, env.mongodb_db),
        .cassandra = try Cassandra.init(io, allocator, env.cassandra_contact_points, env.cassandra_local_dc, env.cassandra_keyspace),
    };

    // Bind to the configured HOST (localhost already normalised to 0.0.0.0 in
    // env); fall back to bind-all if it is not a valid IP literal.
    const address: httpz.Config.Address = if (std.Io.net.IpAddress.parse(env.host, env.port)) |ip|
        .{ .ip = ip }
    else |_|
        .all(env.port);

    var server = try Server.init(io, allocator, .{
        .address = address,
        .request = .{
            // Global 10 MiB request-body cap, matching every other server. The
            // file route enforces its own smaller 1 MiB limit in the handler, so
            // uploads under this cap still reach that check and return 413.
            .max_body_size = 10 * 1024 * 1024,
            .max_form_count = 20,
            .max_multiform_count = 20,
        },
    }, app);
    defer server.deinit();

    var router = try server.router(.{});
    router.get("/", index, .{});
    router.get("/health", healthz, .{});

    router.get("/params/search", params.search, .{});
    router.get("/params/url/:dynamic", params.url, .{});
    router.get("/params/header", params.header, .{});
    router.post("/params/body", params.body, .{});
    router.get("/params/cookie", params.cookie, .{});
    router.post("/params/form", params.form, .{});
    router.post("/params/file", params.file, .{});

    router.get("/html", web.html, .{});
    router.get("/jwt/sign", web.jwtSign, .{});
    router.get("/jwt/verify", web.jwtVerify, .{});
    router.post("/validate", web.validateBody, .{});
    router.get("/compute", web.computeChain, .{});

    router.get("/db/:database/health", db.health, .{});
    router.post("/db/:database/users", db.createUser, .{});
    router.get("/db/:database/users/:id", db.getUser, .{});
    router.patch("/db/:database/users/:id", db.updateUser, .{});
    router.delete("/db/:database/users/:id", db.deleteUser, .{});
    router.delete("/db/:database/users", db.deleteAll, .{});
    router.delete("/db/:database/reset", db.reset, .{});

    installSignalHandlers();
    server_instance = &server;

    std.log.info("Zig server listening on {s}:{d}", .{ env.host, env.port });
    try server.listen();

    // Graceful teardown once listen() returns (SIGINT/SIGTERM -> stop()): the
    // http workers have stopped, so release each DB client in reverse init
    // order before exit.
    app.cassandra.deinit();
    app.mongo.deinit();
    app.redis.deinit();
    app.postgres.deinit();
    allocator.destroy(app);
}

fn index(_: *App, _: *httpz.Request, res: *httpz.Response) !void {
    res.content_type = .JSON;
    res.body = "{\"hello\":\"world\"}";
}

fn healthz(_: *App, _: *httpz.Request, res: *httpz.Response) !void {
    httputil.writeText(res, 200, "OK");
}

fn installSignalHandlers() void {
    if (comptime @import("builtin").os.tag == .windows) return;
    const action: std.posix.Sigaction = .{
        .handler = .{ .handler = shutdown },
        .mask = std.posix.sigemptyset(),
        .flags = 0,
    };
    std.posix.sigaction(std.posix.SIG.INT, &action, null);
    std.posix.sigaction(std.posix.SIG.TERM, &action, null);
}

fn shutdown(_: std.posix.SIG) callconv(.c) void {
    if (server_instance) |server| {
        server_instance = null;
        server.stop();
    }
}
