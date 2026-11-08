//! The plugin's SQLite: the amalgamation build.zig compiles in, called
//! through its C API.

const std = @import("std");
const mygo = @import("mygo.zig");
const Allocator = std.mem.Allocator;

pub fn makeEngine() mygo.Error!mygo.Engine {
    if (sqlite3_threadsafe() == 0) return mygo.failGo("sqlite: the library is not threadsafe");
    const v = try mygo.allocator.create(Engine);
    v.* = .{};
    return mygo.Engine.of(v);
}

const Engine = struct {
    pub fn deinit(self: *Engine) void {
        mygo.allocator.destroy(self);
    }

    pub fn open(_: *Engine, arena: Allocator, path: []const u8, read_only: bool, busy_millis: i64) mygo.Error!mygo.Conn {
        const name = try arena.dupeSentinel(u8, path, 0);
        const flags: c_int = if (read_only) open_readonly | open_fullmutex else open_readwrite | open_create | open_fullmutex;
        var db: ?*Db = null;
        const rc = sqlite3_open_v2(name, &db, flags, null);
        if (rc != ok) {
            const err = failure(db, rc);
            if (db) |h| _ = sqlite3_close_v2(h);
            return err;
        }
        _ = sqlite3_extended_result_codes(db.?, 1);
        const c = mygo.allocator.create(Conn) catch |err| {
            _ = sqlite3_close_v2(db.?);
            return err;
        };
        c.* = .{ .db = db, .timeout_ms = @intCast(std.math.clamp(busy_millis, 0, std.math.maxInt(c_int))) };
        c.handle();
        return mygo.Conn.of(c);
    }

    pub fn version(_: *Engine, _: Allocator) []const u8 {
        return std.mem.span(sqlite3_libversion());
    }
};

const Conn = struct {
    db: ?*Db,
    timeout_ms: c_int,
    waited_ms: c_int = 0,
    allow_control: bool = true,

    /// Installs the connection's handlers: the wait for a lock, which
    /// Interrupt ends too, and the transaction control a call allows.
    fn handle(c: *Conn) void {
        _ = sqlite3_set_authorizer(c.db.?, authorize, c);
        _ = sqlite3_busy_handler(c.db.?, busy, c);
    }

    pub fn deinit(self: *Conn) void {
        if (self.db) |db| _ = sqlite3_close_v2(db);
        mygo.allocator.destroy(self);
    }

    pub fn close(self: *Conn) mygo.Error!void {
        const db = self.db orelse return;
        const rc = sqlite3_close_v2(db);
        if (rc != ok) return failure(db, rc);
        self.db = null;
    }

    pub fn autocommit(self: *Conn, _: Allocator) bool {
        const db = self.db orelse return true;
        return sqlite3_get_autocommit(db) != 0;
    }

    pub fn interrupt(self: *Conn, _: Allocator) void {
        if (self.db) |db| sqlite3_interrupt(db);
    }

    pub fn control(self: *Conn, _: Allocator, sql: []const u8) mygo.Error!void {
        const db = self.db orelse return error.Closed;
        const stmt = try prepare(self, sql, true);
        defer _ = sqlite3_finalize(stmt);
        const rc = sqlite3_step(stmt);
        if (rc != done) return failure(db, rc);
    }

    pub fn run(self: *Conn, arena: Allocator, sql: []const u8, args: []const mygo.Value, query: bool) mygo.Error!mygo.Outcome {
        const db = self.db orelse return error.Closed;
        const stmt = try prepare(self, sql, false);
        defer _ = sqlite3_finalize(stmt);

        const count = sqlite3_bind_parameter_count(stmt);
        if (count != args.len) {
            return mygo.failGo(try std.fmt.allocPrint(arena, "sqlite: statement needs {d} parameters, got {d}", .{ count, args.len }));
        }
        for (args, 1..) |arg, i| try bind(db, stmt, @intCast(i), arg);

        var out: mygo.Outcome = .{};
        const n: usize = if (query) @intCast(sqlite3_column_count(stmt)) else 0;
        if (query) {
            const columns = try arena.alloc([]const u8, n);
            for (columns, 0..) |*c, i| {
                c.* = if (sqlite3_column_name(stmt, @intCast(i))) |name| try arena.dupe(u8, std.mem.span(name)) else "";
            }
            out.columns = columns;
        }
        var values: std.ArrayList(mygo.Value) = .empty;
        const before = sqlite3_total_changes64(db);
        while (true) {
            const rc = sqlite3_step(stmt);
            if (rc == row) {
                if (!query) continue;
                for (0..n) |i| try values.append(arena, try column(arena, stmt, @intCast(i)));
            } else if (rc == done) {
                break;
            } else {
                return failure(db, rc);
            }
        }
        out.values = values.items;
        out.last_insert_id = sqlite3_last_insert_rowid(db);
        if (sqlite3_total_changes64(db) != before) out.changes = sqlite3_changes64(db);
        return out;
    }
};

fn busy(data: ?*anyopaque, previous: c_int) callconv(.c) c_int {
    const c: *Conn = @ptrCast(@alignCast(data.?));
    if (previous == 0) c.waited_ms = 0;
    if (sqlite3_is_interrupted(c.db.?) != 0 or c.waited_ms >= c.timeout_ms) return 0;
    var delay: c_int = if (previous < 10) previous + 1 else 10;
    delay = @min(delay, c.timeout_ms - c.waited_ms);
    c.waited_ms += sqlite3_sleep(delay);
    return @intFromBool(sqlite3_is_interrupted(c.db.?) == 0);
}

// Transactions belong to the batch API. ATTACH (including VACUUM INTO)
// could escape the plugin's database directory. Extension loading is
// omitted at compile time, and directory-changing pragmas are deprecated.
fn authorize(data: ?*anyopaque, action: c_int, a: ?[*:0]const u8, _: ?[*:0]const u8, _: ?[*:0]const u8, _: ?[*:0]const u8) callconv(.c) c_int {
    const c: *Conn = @ptrCast(@alignCast(data.?));
    return switch (action) {
        action_transaction, action_savepoint, action_detach => if (c.allow_control) ok else deny,
        // Ordinary VACUUM attaches an unnamed temporary database while
        // stepping. User ATTACH is denied while preparing, and VACUUM INTO
        // a named output file is denied here too.
        action_attach => if (c.allow_control and a != null and a.?[0] == 0) ok else deny,
        else => ok,
    };
}

/// Prepares sql, which must be exactly one statement: it compiles what
/// follows the first before anything runs, so that extra SQL, even invalid,
/// never runs a part of an operation.
fn prepare(c: *Conn, sql: []const u8, control: bool) mygo.Error!*Stmt {
    const db = c.db.?;
    c.allow_control = control;
    defer c.allow_control = true;
    var stmt: ?*Stmt = null;
    var tail: ?[*]const u8 = null;
    var rc = sqlite3_prepare_v2(db, sql.ptr, @intCast(sql.len), &stmt, &tail);
    if (rc != ok) {
        const err = failure(db, rc);
        if (stmt) |s| _ = sqlite3_finalize(s);
        return err;
    }
    const first = stmt orelse return mygo.failGo("sqlite: SQL contains no statement");
    var offset = if (tail) |t| @intFromPtr(t) - @intFromPtr(sql.ptr) else sql.len;
    while (offset < sql.len) {
        var extra: ?*Stmt = null;
        rc = sqlite3_prepare_v2(db, sql.ptr + offset, @intCast(sql.len - offset), &extra, &tail);
        if (rc != ok or extra != null) {
            const err = if (rc != ok) failure(db, rc) else mygo.failGo("sqlite: expected exactly one statement");
            if (extra) |s| _ = sqlite3_finalize(s);
            _ = sqlite3_finalize(first);
            return err;
        }
        const next = if (tail) |t| @intFromPtr(t) - @intFromPtr(sql.ptr) else sql.len;
        if (next <= offset) break;
        offset = next;
    }
    return first;
}

fn bind(db: *Db, stmt: *Stmt, index: c_int, v: mygo.Value) mygo.Error!void {
    const rc = switch (v.type) {
        .null => sqlite3_bind_null(stmt, index),
        .integer => sqlite3_bind_int64(stmt, index, v.int),
        .real => sqlite3_bind_double(stmt, index, v.real),
        .text => sqlite3_bind_text(stmt, index, v.text.ptr, @intCast(v.text.len), transient),
        // A pointer that is not null, even for no bytes, keeps an empty
        // BLOB from being NULL.
        .blob => sqlite3_bind_blob(stmt, index, if (v.blob.len == 0) "".ptr else v.blob.ptr, @intCast(v.blob.len), transient),
    };
    if (rc != ok) return failure(db, rc);
}

/// The value of column i of the current row, copied into arena.
fn column(arena: Allocator, stmt: *Stmt, i: c_int) mygo.Error!mygo.Value {
    return switch (sqlite3_column_type(stmt, i)) {
        1 => .{ .type = .integer, .int = sqlite3_column_int64(stmt, i) },
        2 => .{ .type = .real, .real = sqlite3_column_double(stmt, i) },
        3 => .{ .type = .text, .text = try bytes(arena, sqlite3_column_text(stmt, i), sqlite3_column_bytes(stmt, i)) },
        4 => .{ .type = .blob, .blob = try bytes(arena, @ptrCast(sqlite3_column_blob(stmt, i)), sqlite3_column_bytes(stmt, i)) },
        else => .{ .type = .null },
    };
}

fn bytes(arena: Allocator, p: ?[*]const u8, n: c_int) mygo.Error![]const u8 {
    if (n <= 0) return &.{};
    return arena.dupe(u8, p.?[0..@intCast(n)]);
}

/// The error of the last call on db that failed with rc, which Go reports
/// as an sqlite.Error: SQLite's extended code and message.
fn failure(db: ?*Db, rc: c_int) mygo.Error {
    const h = db orelse return mygo.fail("SQLite", rc, "could not open database");
    return mygo.fail("SQLite", sqlite3_extended_errcode(h), std.mem.span(sqlite3_errmsg(h)));
}

// SQLite's C API, as much as the module calls.

const Db = opaque {};
const Stmt = opaque {};
/// A destructor of a bound value, as an integer: SQLITE_TRANSIENT, -1, is
/// no aligned function pointer.
const Destructor = isize;

const ok = 0;
const deny = 1;
const row = 100;
const done = 101;
const open_readonly = 0x00000001;
const open_readwrite = 0x00000002;
const open_create = 0x00000004;
const open_fullmutex = 0x00010000;
const action_attach = 24;
const action_detach = 25;
const action_transaction = 22;
const action_savepoint = 32;
/// SQLITE_TRANSIENT: SQLite copies the value before the bind returns.
const transient: Destructor = -1;

extern fn sqlite3_threadsafe() c_int;
extern fn sqlite3_libversion() [*:0]const u8;
extern fn sqlite3_open_v2(filename: [*:0]const u8, db: *?*Db, flags: c_int, vfs: ?[*:0]const u8) c_int;
extern fn sqlite3_close_v2(db: *Db) c_int;
extern fn sqlite3_extended_result_codes(db: *Db, on: c_int) c_int;
extern fn sqlite3_errmsg(db: *Db) [*:0]const u8;
extern fn sqlite3_extended_errcode(db: *Db) c_int;
extern fn sqlite3_get_autocommit(db: *Db) c_int;
extern fn sqlite3_changes64(db: *Db) i64;
extern fn sqlite3_total_changes64(db: *Db) i64;
extern fn sqlite3_last_insert_rowid(db: *Db) i64;
extern fn sqlite3_sleep(ms: c_int) c_int;
extern fn sqlite3_set_authorizer(db: *Db, f: ?*const fn (?*anyopaque, c_int, ?[*:0]const u8, ?[*:0]const u8, ?[*:0]const u8, ?[*:0]const u8) callconv(.c) c_int, data: ?*anyopaque) c_int;
extern fn sqlite3_interrupt(db: *Db) void;
extern fn sqlite3_is_interrupted(db: *Db) c_int;
extern fn sqlite3_busy_handler(db: *Db, f: ?*const fn (?*anyopaque, c_int) callconv(.c) c_int, data: ?*anyopaque) c_int;
extern fn sqlite3_prepare_v2(db: *Db, sql: [*]const u8, n: c_int, stmt: *?*Stmt, tail: *?[*]const u8) c_int;
extern fn sqlite3_finalize(stmt: *Stmt) c_int;
extern fn sqlite3_step(stmt: *Stmt) c_int;
extern fn sqlite3_bind_parameter_count(stmt: *Stmt) c_int;
extern fn sqlite3_bind_null(stmt: *Stmt, i: c_int) c_int;
extern fn sqlite3_bind_int64(stmt: *Stmt, i: c_int, v: i64) c_int;
extern fn sqlite3_bind_double(stmt: *Stmt, i: c_int, v: f64) c_int;
extern fn sqlite3_bind_text(stmt: *Stmt, i: c_int, v: [*]const u8, n: c_int, d: Destructor) c_int;
extern fn sqlite3_bind_blob(stmt: *Stmt, i: c_int, v: [*]const u8, n: c_int, d: Destructor) c_int;
extern fn sqlite3_column_count(stmt: *Stmt) c_int;
extern fn sqlite3_column_name(stmt: *Stmt, i: c_int) ?[*:0]const u8;
extern fn sqlite3_column_type(stmt: *Stmt, i: c_int) c_int;
extern fn sqlite3_column_int64(stmt: *Stmt, i: c_int) i64;
extern fn sqlite3_column_double(stmt: *Stmt, i: c_int) f64;
extern fn sqlite3_column_text(stmt: *Stmt, i: c_int) ?[*]const u8;
extern fn sqlite3_column_blob(stmt: *Stmt, i: c_int) ?*const anyopaque;
extern fn sqlite3_column_bytes(stmt: *Stmt, i: c_int) c_int;
