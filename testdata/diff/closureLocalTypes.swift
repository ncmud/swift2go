private func cltRun(_ body: () throws -> Void) rethrows { try body() }

private struct CltCounter {
    func pacing() throws -> [String] {
        var out: [String] = []
        try cltRun {
            enum Reach { case landed, missed, none }
            var reaches: [Reach] = []
            for i in 0..<5 { reaches.append(i % 3 == 0 ? .landed : i % 3 == 1 ? .missed : .none) }
            out.append("\(reaches.count) \(reaches.contains(.landed)) \(reaches.first == Reach.landed)")
            cltRun {
                struct Pair: Hashable { let a: Int; let b: Reach }
                let pairs = Set(reaches.enumerated().map { Pair(a: $0.offset % 2, b: $0.element) })
                out.append("\(pairs.count)")
            }
        }
        return out
    }
}

public func closureLocalTypes() {
    print((try? CltCounter().pacing()) ?? [])
}
