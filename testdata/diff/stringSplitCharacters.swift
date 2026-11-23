public func stringSplitCharacters() {
    let text = "ab\ncde\nf"
    print(text.split(separator: "\n").allSatisfy { $0.count <= 3 }, text.split(separator: "\n").allSatisfy { $0.count <= 2 })
    let rows: [ArraySlice<Character>] = text.split(separator: "\n")
    print(rows.map { String($0) })
}
