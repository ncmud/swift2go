private struct ScpBox: Equatable {
    var tags: [String]
}

public func sliceContainsEqual() {
    let rolls: [Int?] = [nil, 3, nil, 7, nil, 3]
    print(rolls.suffix(2).contains(3), rolls.suffix(2).contains(7), rolls.prefix(2).contains(nil), rolls[1...3].contains(7))
    print(rolls.dropFirst(4).contains(3), rolls.dropLast(3).contains(7))
    let boxes = [ScpBox(tags: ["a"]), ScpBox(tags: ["b", "c"])]
    print(boxes.suffix(1).contains(ScpBox(tags: ["b", "c"])), boxes.prefix(1).contains(ScpBox(tags: ["b"])))
}
