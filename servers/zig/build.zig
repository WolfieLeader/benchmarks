const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});

    const httpz = b.dependency("httpz", .{ .target = target, .optimize = optimize });
    const pg = b.dependency("pg", .{ .target = target, .optimize = optimize });

    const mod = b.createModule(.{
        .root_source_file = b.path("src/main.zig"),
        .target = target,
        .optimize = optimize,
        // libc lets us use the C allocator and the MongoDB C driver; the
        // Cassandra cpp-driver exposes a C API but needs the C++ runtime.
        .link_libc = true,
        .link_libcpp = true,
        .imports = &.{
            .{ .name = "httpz", .module = httpz.module("httpz") },
            .{ .name = "pg", .module = pg.module("pg") },
        },
    });

    // MongoDB C driver. mongo-c-driver 2.x ships pkg-config module `mongoc2`
    // (which transitively provides libbson); the legacy 1.x line used
    // `libmongoc-1.0`/`libbson-1.0`. Pick whichever the host provides so the
    // build works on both Homebrew (macOS) and Alpine (Docker).
    if (pkgConfigExists(b, "mongoc2")) {
        mod.linkSystemLibrary("mongoc2", .{});
    } else {
        mod.linkSystemLibrary("libmongoc-1.0", .{});
        mod.linkSystemLibrary("libbson-1.0", .{});
    }

    // DataStax/Apache C/C++ driver for Cassandra. Ships a pkg-config file on
    // some distros (Alpine) but not Homebrew, where the lib/header live under
    // the brew prefix and must be added explicitly.
    if (!pkgConfigExists(b, "cassandra") and target.result.os.tag == .macos) {
        mod.addIncludePath(.{ .cwd_relative = "/opt/homebrew/include" });
        mod.addLibraryPath(.{ .cwd_relative = "/opt/homebrew/lib" });
    }
    mod.linkSystemLibrary("cassandra", .{});

    const exe = b.addExecutable(.{ .name = "server", .root_module = mod });

    b.installArtifact(exe);

    const run_cmd = b.addRunArtifact(exe);
    run_cmd.step.dependOn(b.getInstallStep());
    if (b.args) |args| run_cmd.addArgs(args);
    const run_step = b.step("run", "Run the server");
    run_step.dependOn(&run_cmd.step);
}

/// Best-effort probe: does `pkg-config` know this module? Used to choose
/// between library-naming schemes without failing the build when pkg-config
/// is absent entirely.
fn pkgConfigExists(b: *std.Build, name: []const u8) bool {
    var code: u8 = 1;
    const out = b.runAllowFail(&.{ "pkg-config", "--exists", name }, &code, .ignore) catch return false;
    b.allocator.free(out);
    return true;
}
