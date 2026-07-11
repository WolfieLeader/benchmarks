// Test aggregator for the web suite's pure logic (JWT, /validate, /compute).
// Kept as a standalone test root so `zig build test` compiles only std +
// the pure uuid library — no httpz, libc, or the C database drivers — which
// makes the unit run fast and independent of the DB toolchain.
comptime {
    _ = @import("jwt.zig");
    _ = @import("validate.zig");
    _ = @import("compute.zig");
}
