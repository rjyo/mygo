const std = @import("std");

pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const module = b.createModule(.{
        .root_source_file = b.path("src/mygo.zig"),
        .target = target,
        .optimize = b.standardOptimizeOption(.{}),
        .link_libc = true,
    });
    // SQLite's amalgamation, pinned by its hash in build.zig.zon. Its
    // symbols stay hidden in the library, which exports only the module's:
    // a process that loaded another SQLite globally, as WebKit does on
    // Linux, can neither interpose on its calls nor see it.
    const sqlite = b.dependency("sqlite", .{});
    module.addIncludePath(sqlite.path(""));
    module.addCSourceFile(.{ .file = sqlite.path("sqlite3.c"), .flags = &.{ "-std=c11", "-fvisibility=hidden" } });
    module.addCMacro("SQLITE_THREADSAFE", "1");
    module.addCMacro("SQLITE_DQS", "0");
    module.addCMacro("SQLITE_DEFAULT_FOREIGN_KEYS", "1");
    module.addCMacro("SQLITE_ENABLE_FTS5", "1");
    module.addCMacro("SQLITE_ENABLE_RTREE", "1");
    module.addCMacro("SQLITE_OMIT_LOAD_EXTENSION", "1");
    module.addCMacro("SQLITE_OMIT_DEPRECATED", "1");
    module.addCMacro("SQLITE_OMIT_SHARED_CACHE", "1");
    if (target.result.os.tag != .windows) {
        module.linkSystemLibrary("m", .{});
        module.linkSystemLibrary("pthread", .{});
    }
    b.installArtifact(b.addLibrary(.{ .name = "Sqlitecore", .linkage = .dynamic, .root_module = module }));
}
