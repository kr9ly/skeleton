const std = @import("std");
const mem = @import("std").mem;
const Allocator = std.mem.Allocator;

pub const MAX_SIZE: usize = 1024;
const internal_limit: usize = 512;

pub const Error = error{
    OutOfMemory,
    InvalidInput,
    Overflow,
};

pub const Status = enum {
    idle,
    running,
    stopped,
};

pub const Config = struct {
    host: []const u8,
    port: u16 = 8080,
    debug: bool = false,

    pub fn default() Config {
        return .{ .host = "localhost", .port = 8080, .debug = false };
    }
};

pub const Server = struct {
    config: Config,
    allocator: Allocator,
    running: bool,

    pub fn init(allocator: Allocator, config: Config) Server {
        return .{ .config = config, .allocator = allocator, .running = false };
    }

    pub fn start(self: *Server) !void {
        self.running = true;
    }

    pub fn stop(self: *Server) void {
        self.running = false;
    }

    fn internalHelper(self: *Server) void {
        _ = self;
    }
};

pub const Handler = struct {
    vtable: *const VTable,

    pub const VTable = struct {
        handleFn: *const fn (*Handler) anyerror!void,
    };

    pub fn handle(self: *Handler) !void {
        return self.vtable.handleFn(self);
    }
};

pub fn formatStatus(status: Status) []const u8 {
    return switch (status) {
        .idle => "idle",
        .running => "running",
        .stopped => "stopped",
    };
}

fn helperFunction() void {}

pub const Vector = struct {
    data: []f32,
    len: usize,

    pub fn dot(self: Vector, other: Vector) f32 {
        var sum: f32 = 0;
        for (self.data, other.data) |a, b| {
            sum += a * b;
        }
        return sum;
    }
};

pub const LinkedList = struct {
    pub const Node = struct {
        data: i32,
        next: ?*Node,
    };

    head: ?*Node,
    len: usize,
};

pub const StringList = []const []const u8;

pub var global_instance: ?*Server = null;

test "server init" {
    const config = Config.default();
    var server = Server.init(std.testing.allocator, config);
    try server.start();
}
