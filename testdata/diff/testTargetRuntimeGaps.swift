import Foundation

public func testTargetRuntimeGaps() {
    let mobs = ["orc", "troll", "elf"]
    for (i, m) in mobs.enumerated().dropFirst() { print(i, m) }
    let spoke: [Int: [String]] = [1: ["a", "b"], 2: ["c"]]
    print(spoke.values.joined().count, UInt8(0b1011).nonzeroBitCount, (5 as Int).nonzeroBitCount)
    print(stride(from: 0, through: 30, by: 10).map { $0 * 2 }, stride(from: 0, to: 7, by: 3).map { Double($0) / 2 }, Array(stride(from: 2, to: 9, by: 2)))
    let outcomes: Set<[Int]> = [[1, 2], [3]]
    print(outcomes.allSatisfy { !$0.isEmpty }, outcomes.allSatisfy { $0.count == 2 })
    let a = Date(timeIntervalSince1970: 100), b = Date(timeIntervalSince1970: 160)
    print(a.distance(to: b), Array("hé".utf8))
    print(zip(["x", "y"], [1, 2]).first { $0.1 == 2 }?.0 ?? "-")
    var seen: [Int] = []
    (2..<5).forEach { seen.append($0) }
    print(seen)
}
