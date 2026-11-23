public func optionalElements() {
    let loads: [Int?] = [nil, 7042, nil, 2]
    let none: Int? = nil
    print(loads.contains(nil), loads.contains(7042), loads.contains(5), loads.contains(none))
    var targets: [Int64?] = []
    for i in 0..<6 { targets.append(i % 2 == 0 ? 2 : 7042) }
    print(Set(targets).count, Set(targets) == [2, 7042], Set(targets).contains(7042))
    let byFoe: [Int?: String] = [nil: "none", 2: "two"]
    print(byFoe[nil] ?? "-", byFoe[2] ?? "-", byFoe[3] ?? "-")
}
