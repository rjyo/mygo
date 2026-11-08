import Foundation
import Roundtrip

// The implementation of package uses, with Foundation only.

func makeMixer() throws -> any Mixer { Mixing() }

final class Mixing: Mixer {
    func pair(p: Pair) throws -> Pair {
        var p = p
        p.n = p.r.counts.reduce(0, +)
        p.inner = p.r.inner
        return p
    }

    func kinds(rs: [Record]) -> [Kind] { rs.map(\.kind) }

    func next(c: any Counter) -> Int { c.next() }
}
