public func arrayOfSubstring() {
    let rows = "ab\ncé\n\nd".split(separator: "\n", omittingEmptySubsequences: false).map { Array($0) }
    for row in rows {
        print(row.count, row)
    }
}
