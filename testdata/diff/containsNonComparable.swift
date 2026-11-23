private enum CncCode: Equatable { case reach, time }

private struct CncFinding: Equatable {
    let code: CncCode
    let cause: String?
    let rooms: [Int]
}

private struct CncPlain: Equatable { let a: Int; let b: String }

public func containsNonComparable() {
    let found = [CncFinding(code: .reach, cause: "far", rooms: [7012]), CncFinding(code: .time, cause: nil, rooms: [])]
    print(found.contains(CncFinding(code: .reach, cause: "far", rooms: [7012])))
    print(found.contains(CncFinding(code: .reach, cause: "far", rooms: [7013])))
    print(found.contains(CncFinding(code: .time, cause: nil, rooms: [])))
    print([CncPlain(a: 1, b: "x")].contains(CncPlain(a: 1, b: "x")))
    var sinks: [String: (String) -> Void] = [:]
    let sink: ((String) -> Void)? = { print("sank", $0) }
    sinks["a"] = sink
    sinks["a"]?("hi")
    let none: ((String) -> Void)? = nil
    sinks["a"] = none
    print(sinks.count)
}
