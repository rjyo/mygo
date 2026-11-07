import Foundation
import NaturalLanguage

// The implementation of package language on Apple's platforms, with the
// NaturalLanguage framework.

func makeAnalyzer() throws -> any Analyzer { NLAnalyzer() }

final class NLAnalyzer: Analyzer {
    func detectLanguage(text: String) throws -> String {
        guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
            throw LanguageError.undetermined
        }
        return lang.rawValue
    }

    func hypotheses(text: String, max: Int) throws -> [String: Double] {
        let r = NLLanguageRecognizer()
        r.processString(text)
        return Dictionary(uniqueKeysWithValues: r.languageHypotheses(withMaximum: max).map { ($0.key.rawValue, $0.value) })
    }

    func tokens(text: String, unit: Unit) -> AsyncThrowingStream<Token, Error> {
        AsyncThrowingStream { continuation in
            let tokenizer = NLTokenizer(unit: NLTokenUnit(unit))
            tokenizer.string = text
            tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
                continuation.yield(Token(range, in: text))
                return !Task.isCancelled
            }
            continuation.finish()
        }
    }

    func embedding(language: String) async throws -> any Embedding {
        guard let e = NLEmbedding.wordEmbedding(for: NLLanguage(language)) else {
            throw LanguageError.noEmbedding
        }
        return WordEmbedding(e)
    }

    func suggest(e: any Embedding, word: String, r: any Ranker) -> AsyncThrowingStream<String, Error> {
        AsyncThrowingStream { continuation in
            let task = Task {
                let best = e.neighbors(word: word, n: 10).sorted { r.score(word: $0) > r.score(word: $1) }
                for w in best { continuation.yield(w) }
                if let first = best.first {
                    do {
                        continuation.yield(try await r.explain(word: first))
                    } catch {
                        continuation.finish(throwing: error)
                        return
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    func count(text: String, progress: @escaping @Sendable (String, Int) -> Void) -> Int {
        let tokenizer = NLTokenizer(unit: .word)
        tokenizer.string = text
        var n = 0
        tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
            progress(String(text[range]), n)
            n += 1
            return true
        }
        return n
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

    func uppercase(text: String, buf: NativeBuffer) throws -> Int {
        let up = Array(text.uppercased().utf8)
        let n = min(up.count, buf.count)
        buf.bytes.copyBytes(from: up[..<n])
        return n
    }

    func chunks(text: String, size: Int) -> AsyncThrowingStream<NativeBuffer, Error> {
        let pool = NativeBufferPool(size: size)
        let bytes = Array(text.utf8)
        return AsyncThrowingStream { continuation in
            for start in stride(from: 0, to: bytes.count, by: size) {
                let b = pool.get()
                b.bytes.initializeMemory(as: UInt8.self, repeating: 0)
                let n = min(size, bytes.count - start)
                b.bytes.copyBytes(from: bytes[start..<start + n])
                continuation.yield(b)
            }
            continuation.finish()
        }
    }

    func sleep(d: Duration) async throws {
        try await Task.sleep(for: d)
    }
}

final class WordEmbedding: Embedding {
    let e: NLEmbedding
    init(_ e: NLEmbedding) { self.e = e }
    func distance(a: String, b: String) -> Double { e.distance(between: a, and: b) }
    func neighbors(word: String, n: Int) -> [String] { e.neighbors(for: word, maximumCount: n).map(\.0) }
    func close() throws {}
}

extension NLTokenUnit {
    init(_ unit: Unit) {
        switch unit {
        case .word: self = .word
        case .sentence: self = .sentence
        case .paragraph: self = .paragraph
        }
    }
}

extension Token {
    /// The token of a range of text, in UTF-8 bytes as Go counts.
    init(_ range: Range<String.Index>, in text: String) {
        let start = text.utf8.distance(from: text.startIndex, to: range.lowerBound)
        let end = text.utf8.distance(from: text.startIndex, to: range.upperBound)
        self.init(start: start, end: end, text: String(text[range]))
    }
}
