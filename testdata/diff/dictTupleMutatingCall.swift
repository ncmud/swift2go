private struct DtmCounter {
    var hits = 0
    mutating func bump(by n: Int) -> Int {
        hits += n
        return hits
    }
}

private final class DtmBox {
    let label: String
    init(_ label: String) { self.label = label }
}

private struct DtmHolder {
    var boards: [Int: (DtmBox, DtmCounter)] = [:]
    var plain: [Int: DtmCounter] = [:]

    mutating func hit(_ key: Int) -> String {
        let box = boards[key]!.0
        let total = boards[key]!.1.bump(by: key)
        _ = plain[key]!.bump(by: 1)
        return "\(box.label) \(total)"
    }
}

public func dictTupleMutatingCall() {
    var holder = DtmHolder()
    holder.boards[2] = (DtmBox("two"), DtmCounter())
    holder.boards[5] = (DtmBox("five"), DtmCounter())
    holder.plain[2] = DtmCounter()
    holder.plain[5] = DtmCounter()
    print(holder.hit(2))
    print(holder.hit(2))
    print(holder.hit(5))
    print(holder.boards[2]!.1.hits, holder.boards[5]!.1.hits)
    print(holder.plain[2]!.hits, holder.plain[5]!.hits)
    var local: [String: (Int, DtmCounter)] = ["a": (1, DtmCounter())]
    _ = local["a"]!.1.bump(by: 7)
    local["a"]?.1.hits += 1
    print(local["a"]!.0, local["a"]!.1.hits)
}
