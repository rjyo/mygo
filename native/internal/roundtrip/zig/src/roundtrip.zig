//! The implementation of package roundtrip in Zig: every value it is given,
//! back, as its Swift and Rust implementations do.

const std = @import("std");
const mygo = @import("mygo.zig");
const Allocator = std.mem.Allocator;

pub fn makeEcho() mygo.Error!mygo.Echo {
    const e = try mygo.allocator.create(Echoer);
    e.* = .{};
    return mygo.Echo.of(e);
}

const Echoer = struct {
    pub fn deinit(self: *Echoer) void {
        mygo.allocator.destroy(self);
    }

    pub fn scalars(_: *Echoer, _: Allocator, i: i64, @"i8": i8, @"u16": u16, @"i32": i32, @"u64": u64, @"f32": f32, f: f64, b: bool) mygo.Scalars {
        return .{
            .i = i + 1,
            .@"i8" = @"i8" +% 1,
            .@"u16" = @"u16" +% 1,
            .@"i32" = @"i32" + 1,
            .@"u64" = @"u64" +% 1,
            .@"f32" = @"f32" + 1,
            .f = f + 1,
            .b = !b,
        };
    }

    pub fn add(_: *Echoer, _: Allocator, a: i64, b: i64) i64 {
        return a + b;
    }

    pub fn text(_: *Echoer, arena: Allocator, s: []const u8, b: []const u8) mygo.Texts {
        const twice = std.mem.concat(arena, u8, &.{ s, s }) catch @panic("out of memory");
        const rev = arena.dupe(u8, b) catch @panic("out of memory");
        std.mem.reverse(u8, rev);
        return .{ .s = twice, .b = rev };
    }

    pub fn record(_: *Echoer, arena: Allocator, r: mygo.Record) mygo.Error!mygo.Record {
        var out = r;
        const counts = try arena.alloc(i64, r.counts.len);
        for (counts, r.counts) |*d, c| d.* = c * 2;
        out.counts = counts;
        return out;
    }

    pub fn lists(_: *Echoer, arena: Allocator, words: []const []const u8, nested: []const []const i64) mygo.Error![]const mygo.Entry(i64) {
        var m: std.ArrayList(mygo.Entry(i64)) = .empty;
        for (words) |w| try m.append(arena, .{ .key = w, .value = @intCast(w.len) });
        var sum: i64 = 0;
        for (nested) |ns| for (ns) |n| {
            sum += n;
        };
        try m.append(arena, .{ .key = "nested", .value = sum });
        return m.items;
    }

    pub fn maybe(_: *Echoer, _: Allocator, p: ?i64) ?i64 {
        return if (p) |v| v + 1 else null;
    }

    pub fn times(_: *Echoer, _: Allocator, t: mygo.Time, d: i64) mygo.Error!mygo.Moment {
        return .{ .at = t.add(d), .twice = d * 2 };
    }

    pub fn kind(_: *Echoer, _: Allocator, k: mygo.Kind, l: mygo.Level) mygo.Error!mygo.Kinds {
        const next: mygo.Kind = switch (k) {
            .kind_a => .kind_b,
            .kind_b => .kind_c,
            .kind_c => .kind_a,
        };
        return .{ .kind = next, .level = if (l == .low) .high else .low };
    }

    pub fn fail(_: *Echoer, _: Allocator, name: []const u8) mygo.Error!void {
        if (name.len == 0) return;
        if (std.mem.eql(u8, name, "named")) return error.Named;
        if (std.mem.eql(u8, name, "unsupported")) return error.Unsupported;
        return mygo.fail("RoundtripDomain", 7, name);
    }

    pub fn wait(_: *Echoer, _: Allocator, ctx: *mygo.Context, d: i64, s: []const u8) mygo.Error![]const u8 {
        try ctx.sleep(d);
        return s;
    }

    const Count = struct {
        i: i64 = 0,
        n: i64,
        fail: i64,

        pub fn next(self: *Count, _: Allocator) mygo.Error!?i64 {
            if (self.i >= self.n) return null;
            const i = self.i;
            self.i += 1;
            if (i == self.fail) return error.Named;
            return i;
        }

        pub fn deinit(self: *Count) void {
            mygo.allocator.destroy(self);
        }
    };

    pub fn count(_: *Echoer, _: Allocator, _: *mygo.Context, n: i64, fail_at: i64) mygo.Error!mygo.Iterator(i64) {
        const c = try mygo.allocator.create(Count);
        c.* = .{ .n = n, .fail = fail_at };
        return mygo.Iterator(i64).of(c);
    }

    const Records = struct {
        i: i64 = 0,
        n: i64,

        pub fn next(self: *Records, arena: Allocator) mygo.Error!?mygo.Record {
            if (self.i >= self.n) return null;
            const i = self.i;
            self.i += 1;
            const counts = try arena.alloc(i64, 1);
            counts[0] = i;
            const tags = try arena.alloc(mygo.Entry([]const u8), 1);
            tags[0] = .{ .key = "i", .value = try std.fmt.allocPrint(arena, "{d}", .{i}) };
            const data = try arena.alloc(u8, 1);
            data[0] = @intCast(i & 0xff);
            return .{
                .name = try std.fmt.allocPrint(arena, "r{d}", .{i}),
                .counts = counts,
                .tags = tags,
                .inner = .{ .x = @floatFromInt(i), .y = 0 },
                .next = null,
                .kind = .kind_a,
                .data = data,
            };
        }

        pub fn deinit(self: *Records) void {
            mygo.allocator.destroy(self);
        }
    };

    pub fn records(_: *Echoer, _: Allocator, _: *mygo.Context, n: i64) mygo.Error!mygo.Iterator(mygo.Record) {
        const r = try mygo.allocator.create(Records);
        r.* = .{ .n = n };
        return mygo.Iterator(mygo.Record).of(r);
    }

    pub fn counter(_: *Echoer, _: Allocator, n: i64) mygo.Error!mygo.Counter {
        return Counting.make(n);
    }

    pub fn each(_: *Echoer, _: Allocator, words: []const []const u8, f: mygo.EchoEachF) i64 {
        for (words, 0..) |w, i| f.call(w, @intCast(i));
        return @intCast(words.len);
    }

    pub fn ask(_: *Echoer, arena: Allocator, ns: []const i64, f: mygo.EchoAskF) mygo.Error!i64 {
        var sum: i64 = 0;
        for (ns) |n| sum += try f.call(arena, n);
        return sum;
    }

    const Ticker = struct {
        stop: std.atomic.Value(bool) = .init(false),
        done: std.atomic.Value(bool) = .init(false),
        f: mygo.EchoTicksF,
        interval: i64,

        fn main(t: *Ticker) void {
            var n: i64 = 0;
            while (!t.stop.load(.acquire)) {
                mygo.io().sleep(.fromNanoseconds(t.interval), .awake) catch {};
                if (t.stop.load(.acquire)) break;
                n += 1;
                t.f.call(n);
            }
            t.f.release();
            // The last of the ticker and its Closer frees it.
            if (t.done.swap(true, .acq_rel)) mygo.allocator.destroy(t);
        }

        pub fn close(t: *Ticker) mygo.Error!void {
            t.stop.store(true, .release);
        }

        pub fn deinit(t: *Ticker) void {
            t.stop.store(true, .release);
            if (t.done.swap(true, .acq_rel)) mygo.allocator.destroy(t);
        }
    };

    pub fn ticks(_: *Echoer, _: Allocator, interval: i64, f: mygo.EchoTicksF) mygo.Error!mygo.Closer {
        const t = try mygo.allocator.create(Ticker);
        t.* = .{ .f = f.keep(), .interval = interval };
        const th = std.Thread.spawn(.{}, Ticker.main, .{t}) catch {
            f.release();
            mygo.allocator.destroy(t);
            return mygo.failGo("no thread for the ticker");
        };
        th.detach();
        return mygo.Closer.of(t);
    }

    pub fn use(_: *Echoer, arena: Allocator, ctx: *mygo.Context, s: mygo.Scorer, words: []const []const u8) mygo.Error!f64 {
        var sum: f64 = 0;
        for (words) |w| sum += s.score(arena, w);
        if (words.len > 0) {
            const why = try s.explain(arena, ctx, words[0]);
            sum += @floatFromInt(why.len);
        }
        return sum;
    }

    pub fn fill(_: *Echoer, _: Allocator, buf: mygo.Buffer, x: i64) mygo.Error!i64 {
        for (buf.bytes(), 0..) |*b, i| b.* = @truncate(@as(u64, @bitCast(@as(i64, @intCast(i)) ^ x)));
        return @intCast(buf.len);
    }

    pub fn mix(_: *Echoer, _: Allocator, f: f32, d: f64, b: bool, i: i8) f64 {
        return @as(f64, f) + d + @as(f64, if (b) 1 else 0) + @as(f64, @floatFromInt(i));
    }

    pub fn box(_: *Echoer, _: Allocator, r: mygo.Rect) mygo.Error!mygo.Rect {
        return .{ .x = r.x + r.w, .y = r.y + r.h, .w = r.w, .h = r.h };
    }

    pub fn nothing(_: *Echoer, _: Allocator, e: []const mygo.Empty) i64 {
        return @intCast(e.len);
    }

    pub fn onMain(_: *Echoer, _: Allocator, r: mygo.Record, f: mygo.EchoOnMainF) mygo.Error!mygo.Texts {
        f.call(r.inner);
        return .{ .s = r.name, .b = r.data };
    }
};

const Counting = struct {
    n: std.atomic.Value(i64),

    fn make(n: i64) mygo.Error!mygo.Counter {
        const c = try mygo.allocator.create(Counting);
        c.* = .{ .n = .init(n) };
        return mygo.Counter.of(c);
    }

    pub fn close(_: *Counting) mygo.Error!void {}

    pub fn next(self: *Counting, _: Allocator) i64 {
        return self.n.fetchAdd(1, .seq_cst);
    }

    pub fn same(self: *Counting, _: Allocator) mygo.Counter {
        return make(self.n.load(.seq_cst)) catch @panic("out of memory");
    }

    pub fn deinit(self: *Counting) void {
        mygo.allocator.destroy(self);
    }
};
