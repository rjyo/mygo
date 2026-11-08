const std = @import("std");

pub fn build(b: *std.Build) void {
    const module = b.createModule(.{
        .root_source_file = b.path("src/mygo.zig"),
        .target = b.standardTargetOptions(.{}),
        .optimize = b.standardOptimizeOption(.{}),
        .link_libc = true,
    });
    b.installArtifact(b.addLibrary(.{ .name = "Roundtrip", .linkage = .dynamic, .root_module = module }));
}
