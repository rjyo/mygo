import Foundation

// The implementation of package roundtrip: every value it is given, back,
// with Foundation only, so that it builds on Linux too.

func makeEcho() throws -> any Echo { Echoer() }

final class Echoer: Echo {
    func scalars(i: Int, i8: Int8, u16: UInt16, i32: Int32, u64: UInt64, f32: Float, f: Double, b: Bool) -> Scalars {
        Scalars(i: i + 1, i8: i8 &+ 1, u16: u16 &+ 1, i32: i32 + 1, u64: u64 &+ 1, f32: f32 + 1, f: f + 1, b: !b)
    }

    func add(a: Int, b: Int) -> Int { a + b }

    func text(s: String, b: Data) -> Texts { Texts(s: s + s, b: Data(b.reversed())) }

    func record(r: Record) throws -> Record {
        var r = r
        r.counts = r.counts.map { $0 * 2 }
        return r
    }

    func lists(words: [String], nested: [[Int]]) throws -> [String: Int] {
        var m: [String: Int] = [:]
        for w in words { m[w] = w.utf8.count }
        m["nested"] = nested.map { $0.reduce(0, +) }.reduce(0, +)
        return m
    }

    func maybe(p: Int?) -> Int? { p.map { $0 + 1 } }

    func times(t: Date, d: Duration) throws -> Moment {
        let c = d.components
        let seconds = Double(c.seconds) + Double(c.attoseconds) / 1e18
        return Moment(at: t.addingTimeInterval(seconds), twice: d * 2)
    }

    func kind(k: Kind, l: Level) throws -> Kinds {
        let all = Kind.allCases
        let next = all[(all.firstIndex(of: k)! + 1) % all.count]
        return Kinds(kind: next, level: l == .low ? .high : .low)
    }

    func fail(name: String) throws {
        switch name {
        case "": return
        case "named": throw RoundtripError.named
        case "unsupported": throw NativeError.unsupported
        default: throw NSError(domain: "RoundtripDomain", code: 7, userInfo: [NSLocalizedDescriptionKey: name])
        }
    }

    func wait(d: Duration, s: String) async throws -> String {
        try await Task.sleep(for: d)
        return s
    }

    func count(n: Int, fail: Int) -> AsyncThrowingStream<Int, Error> {
        AsyncThrowingStream { continuation in
            for i in 0..<n {
                if i == fail {
                    continuation.finish(throwing: RoundtripError.named)
                    return
                }
                continuation.yield(i)
            }
            continuation.finish()
        }
    }

    func records(n: Int) -> AsyncThrowingStream<Record, Error> {
        AsyncThrowingStream { continuation in
            for i in 0..<n {
                continuation.yield(Record(name: "r\(i)", counts: [i], tags: ["i": "\(i)"], inner: Inner(x: Double(i), y: 0), next: nil, kind: .kindA, data: Data([UInt8(i & 0xff)])))
            }
            continuation.finish()
        }
    }

    func counter(n: Int) throws -> any Counter { Counting(n) }

    func each(words: [String], f: @escaping @Sendable (String, Int) -> Void) -> Int {
        for (i, w) in words.enumerated() { f(w, i) }
        return words.count
    }

    func ask(ns: [Int], f: @escaping @Sendable (Int) throws -> Int) throws -> Int {
        try ns.reduce(0) { $0 + (try f($1)) }
    }

    func ticks(interval: Duration, f: @escaping @Sendable (Int) -> Void) -> any Closer {
        let timer = DispatchSource.makeTimerSource(queue: .global())
        let c = interval.components
        let nanos = Int(c.seconds) * 1_000_000_000 + Int(c.attoseconds / 1_000_000_000)
        nonisolated(unsafe) var n = 0
        timer.schedule(deadline: .now() + .nanoseconds(nanos), repeating: .nanoseconds(nanos))
        timer.setEventHandler { n += 1; f(n) }
        timer.resume()
        return NativeCloser { timer.cancel() }
    }

    func use(s: any Scorer, words: [String]) async throws -> Double {
        var sum = 0.0
        for w in words { sum += s.score(word: w) }
        if let first = words.first {
            let why = try await s.explain(word: first)
            sum += Double(why.utf8.count)
        }
        return sum
    }

    func fill(buf: NativeBuffer, x: Int) throws -> Int {
        for i in 0..<buf.count { buf.bytes[i] = UInt8(truncatingIfNeeded: i ^ x) }
        return buf.count
    }

    func mix(f: Float, d: Double, b: Bool, i: Int8) -> Double { Double(f) + d + (b ? 1 : 0) + Double(i) }

    func box(r: CGRect) throws -> CGRect { r.offsetBy(dx: r.width, dy: r.height) }

    func nothing(e: [Empty]) -> Int { e.count }

    func onMain(r: Record, f: @escaping @Sendable (Inner) -> Void) throws -> Texts {
        f(r.inner)
        return Texts(s: r.name, b: r.data)
    }
}

final class Counting: Counter, @unchecked Sendable {
    let lock = NSLock()
    var n: Int
    init(_ n: Int) { self.n = n }
    func next() -> Int {
        lock.lock()
        defer { lock.unlock() }
        n += 1
        return n - 1
    }
    func same() -> any Counter {
        lock.lock()
        defer { lock.unlock() }
        return Counting(n)
    }
    func close() throws {}
}
