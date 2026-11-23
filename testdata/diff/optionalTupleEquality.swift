public func optionalTupleEquality() {
    let xs: [(x: Int, y: Int)] = [(1, 2), (0, -1)]
    print((xs.last?.x, xs.last?.y) == (0, -1), (xs.first?.x, xs.first?.y) == (0, -1))
    let empty: [(x: Int, y: Int)] = []
    print((empty.last?.x, empty.last?.y) == (nil, nil))
}
