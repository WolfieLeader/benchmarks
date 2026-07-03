const std = @import("std");

const Map = std.process.Environ.Map;

/// Runtime configuration, read from the process environment. Mirrors the
/// env-var contract shared by every server in the suite. Returned slices are
/// owned by the process environment map (valid for the process lifetime).
pub const Env = struct {
    is_prod: bool,
    host: []const u8,
    port: u16,
    postgres_url: []const u8,
    mongodb_url: []const u8,
    mongodb_db: []const u8,
    redis_url: []const u8,
    cassandra_contact_points: []const u8,
    cassandra_local_dc: []const u8,
    cassandra_keyspace: []const u8,

    /// Reads a variable, falling back to `default` when unset or empty.
    fn get(map: *const Map, key: []const u8, default: []const u8) []const u8 {
        const value = map.get(key) orelse return default;
        return if (value.len == 0) default else value;
    }

    pub fn load(map: *const Map) Env {
        var port: u16 = 26001;
        if (map.get("PORT")) |raw| {
            port = std.fmt.parseInt(u16, std.mem.trim(u8, raw, " "), 10) catch 26001;
        }
        // Match the other servers: treat "localhost" as bind-all (0.0.0.0).
        var host = get(map, "HOST", "0.0.0.0");
        if (std.mem.eql(u8, host, "localhost")) host = "0.0.0.0";
        return .{
            .is_prod = std.mem.eql(u8, get(map, "ENV", "dev"), "prod"),
            .host = host,
            .port = port,
            .postgres_url = get(map, "POSTGRES_URL", "postgres://postgres:postgres@localhost:5432/benchmarks"),
            .mongodb_url = get(map, "MONGODB_URL", "mongodb://localhost:27017"),
            .mongodb_db = get(map, "MONGODB_DB", "benchmarks"),
            .redis_url = get(map, "REDIS_URL", "redis://localhost:6379"),
            .cassandra_contact_points = get(map, "CASSANDRA_CONTACT_POINTS", "localhost"),
            .cassandra_local_dc = get(map, "CASSANDRA_LOCAL_DATACENTER", "datacenter1"),
            .cassandra_keyspace = get(map, "CASSANDRA_KEYSPACE", "benchmarks"),
        };
    }
};
