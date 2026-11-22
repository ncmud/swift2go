// A `var` ArraySlice keeps its parent's indices through each reassignment.

struct ASWords {
    var words: [String]
    func abbreviates(_ name: ASWords) -> Bool {
        var remaining = name.words[...]
        for word in words {
            guard let index = remaining.firstIndex(where: { $0.hasPrefix(word) }) else { return false }
            remaining = remaining[(index + 1)...]
        }
        return true
    }
}

public func arraySliceVarIndices() {
    var out: [String] = []
    let name = ASWords(words: ["greater", "ring", "of", "fire", "ii"])
    for probe in [["gr", "fi"], ["ring", "gr"], ["of", "ii"], ["g", "r", "o", "f", "i"], ["fire", "fire"]] {
        out.append("\(probe) \(ASWords(words: probe).abbreviates(name))")
    }
    let all = ["a", "b", "c", "d", "e", "f"]
    var rest = all[1...]
    var seen: [String] = []
    while let i = rest.firstIndex(where: { $0 > "b" }) {
        seen.append("\(i):\(all[i]):\(rest[i])")
        rest = rest.dropFirst(2)
        if rest.isEmpty { break }
    }
    out.append("\(seen) \(rest.startIndex) \(rest.endIndex)")
    out.append("\(asBatches([1, 2, 3, 4, 5, 6, 7], size: { 3 }))")
    print(out.joined(separator: "\n"))
}

func asBatches(_ all: [Int], size: () -> Int) -> [[Int]] {
    var pending = all[...]
    var batches: [[Int]] = []
    while !pending.isEmpty {
        let batch = Array(pending.prefix(size()))
        pending = pending.dropFirst(batch.count)
        batches.append(batch)
    }
    return batches
}
