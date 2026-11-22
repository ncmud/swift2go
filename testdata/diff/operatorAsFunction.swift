public func operatorAsFunction() {
    let pairs: [(String, Int)] = [("fury", 30), ("endurance", 40)]
    print(pairs.elementsEqual([("fury", 30), ("endurance", 40)], by: ==))
    print(pairs.elementsEqual([("fury", 30), ("endurance", 41)], by: ==))
    print([3, 1, 2].sorted(by: >), ["b", "a"].sorted(by: <))
}
