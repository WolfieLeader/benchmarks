const std = @import("std");
const httpz = @import("httpz");

/// Error strings shared across handlers. Byte-identical to the other servers
/// in the suite (the contract asserts full error bodies).
pub const err = struct {
    pub const invalid_json = "invalid JSON body";
    pub const invalid_form = "invalid form data";
    pub const expected_form_content_type = "expected content-type: application/x-www-form-urlencoded or multipart/form-data";
    pub const invalid_multipart = "invalid multipart form data";
    pub const expected_multipart_content_type = "expected content-type: multipart/form-data";
    pub const file_not_found = "file not found in form data";
    pub const file_size_exceeded = "file size exceeds limit";
    pub const invalid_file_type = "only text/plain files are allowed";
    pub const not_plain_text = "file does not look like plain text";
    pub const not_found = "not found";
    pub const internal = "internal error";
};

/// Serialize `data` as a JSON response with the given status. `data` is any
/// value std.json can stringify; `opts` tunes stringification (e.g. omitting
/// null optional fields for the `favoriteNumber` contract).
pub fn writeJson(res: *httpz.Response, status: u16, data: anytype, opts: std.json.Stringify.Options) void {
    res.status = status;
    res.json(data, opts) catch {};
}

/// Write an error body of the shape `{"error": string, "details"?: string}`.
/// `details` is omitted entirely when null or empty (matches omitempty).
pub fn writeError(res: *httpz.Response, status: u16, message: []const u8, details: ?[]const u8) void {
    res.status = status;
    if (details) |d| {
        if (d.len > 0) {
            res.json(.{ .@"error" = message, .details = d }, .{}) catch {};
            return;
        }
    }
    res.json(.{ .@"error" = message }, .{}) catch {};
}

/// Write a plain-text body.
pub fn writeText(res: *httpz.Response, status: u16, body: []const u8) void {
    res.status = status;
    res.content_type = .TEXT;
    res.body = body;
}
