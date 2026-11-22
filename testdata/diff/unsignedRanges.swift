public func unsignedRanges() {
    let seeds: ClosedRange<UInt64> = 1...20
    print(seeds.first { $0 % 7 == 0 } ?? 0, seeds.allSatisfy { $0 > 0 }, seeds.contains { $0 == 21 })
    print((UInt64(0)..<5).map { $0 * 2 }, seeds.map { Int($0) }.reduce(0, +), (UInt64(1)...3).reduce(UInt64(10), +))
    print((UInt64(0)..<10).filter { $0 % 3 == 0 }, (UInt64(0)..<4).compactMap { $0 == 2 ? nil : $0 }, (UInt64(0)..<4).first { $0 > 1 } ?? 9)
}
