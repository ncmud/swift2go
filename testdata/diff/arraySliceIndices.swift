// An ArraySlice keeps the indices of the array it was cut from.

public func arraySliceIndices() {
    var out: [String] = []
    let args = ["cast", "fire", "ball", "at", "orc"]
    let rest = args[1...]
    let tail = args.dropFirst(2)
    out.append("\(rest.startIndex) \(rest.endIndex) \(rest[1]) \(tail.startIndex) \(tail[2]) \(rest.firstIndex(of: "at")!)")
    out.append("\(rest.count) \(rest.first!) \(rest.last!) \(Array(rest.indices)) \(rest[rest.startIndex]) \(rest.firstIndex { $0.count == 2 }!)")
    let head = args.prefix(3)
    let end = args.suffix(2)
    out.append("\(head.startIndex) \(head.endIndex) \(end.startIndex) \(end[3]) \(end.lastIndex(of: "orc")!)")
    let inner = rest[2..<4]
    let innerTail = rest.dropFirst()
    out.append("\(inner.startIndex) \(inner[3]) \(Array(inner)) \(innerTail.startIndex) \(innerTail[2]) \(innerTail.indices.count)")
    if let i = rest.firstIndex(of: "ball") {
        out.append("\(args[i]) \(rest[i]) \(i)")
    }
    var total = 0
    for i in tail.indices { total += i }
    out.append("\(total) \(tail.map { $0.count }) \(tail.joined(separator: "-"))")
    print(out.joined(separator: "\n"))
}
