public func closedRangeLazy() {
    let firstEven = (UInt64(1)...9).lazy.compactMap { n -> UInt64? in n % 2 == 0 ? n * 10 : nil }.first
    print(firstEven as Any)
    let none = (3...5).lazy.compactMap { n -> Int? in n > 5 ? n : nil }.first
    print(none as Any)
    print(Array((1...4).lazy.map { $0 * $0 }))
    print(Array((Int8(126)...Int8.max).lazy.filter { $0 > 0 }))
    print((5...5).lazy.map { "n\($0)" }.first ?? "none")
}
