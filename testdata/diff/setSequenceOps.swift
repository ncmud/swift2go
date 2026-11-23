struct SSOKey: Hashable, CustomStringConvertible {
    let zone: Int
    let name: String
    var description: String { "\(name)@\(zone)" }
}

func ssoTotal<S: Sequence>(_ s: S) -> Int64 where S.Element == Int64 {
    s.reduce(0, +)
}

public func setSequenceOps() {
    var ids: Set<Int64> = []
    for i in stride(from: Int64(1), through: 3000, by: 7) {
        ids.insert(i * 1_000_003 % 100_000)
    }
    let even: [Int64] = ids.filter { $0 % 2 == 0 }
    print("even", even.count, even.sorted().prefix(5).map { String($0) }.joined(separator: ","))
    print("min", ids.min() ?? -1, "max", ids.max() ?? -1)
    print("sorted", ids.sorted().suffix(3).map { String($0) }.joined(separator: ","))
    print("total", ssoTotal(ids))
    var joined: [Int64] = [1, 2]
    joined.append(contentsOf: ids)
    print("appended", joined.count, joined.prefix(2).map { String($0) }.joined(separator: ","))
    let plus = [Int64(-1)] + ids
    print("plus", plus.count, plus.first ?? 0, plus.dropFirst().reduce(0, +) == ssoTotal(ids))
    var small = 0
    for case let x in ids where x < 1000 {
        small += Int(x)
    }
    print("small", small)

    let words: Set<String> = ["delta", "alpha", "charlie", "bravo", "echo", "10", "9"]
    print("words", words.filter { $0.count > 1 }.sorted().joined(separator: " "))
    print("first word", words.min() ?? "", "last word", words.max() ?? "")

    let keys: Set<SSOKey> = [SSOKey(zone: 3, name: "gate"), SSOKey(zone: 1, name: "inn"), SSOKey(zone: 3, name: "bridge")]
    let inZone3 = keys.filter { $0.zone == 3 }.map(\.description).sorted()
    print("zone 3", inZone3.joined(separator: " "))
}
