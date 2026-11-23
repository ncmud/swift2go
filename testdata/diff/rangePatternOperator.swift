public func rangePatternOperator() {
    let hp = 42
    print(40...45 ~= hp, 0..<42 ~= hp, (1...3).map { 2...3 ~= $0 })
}
