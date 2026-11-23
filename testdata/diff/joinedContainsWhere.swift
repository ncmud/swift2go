public func joinedContainsWhere() {
    let rolls: [String: [Int]] = ["orc": [1, 4], "elf": [9]]
    print(rolls.values.joined().contains { $0 > 8 }, rolls.values.joined().contains { $0 > 9 })
    let empty: [[Int]] = [[], []]
    print(empty.joined().contains { _ in true })
}
