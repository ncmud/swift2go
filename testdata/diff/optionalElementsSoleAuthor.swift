public func optionalElementsSoleAuthor() {
    let authors: [Int64?] = [7, 7, 7]
    let accounts = Set(authors)
    var sole: Int64? = nil
    if accounts.count == 1, let only = accounts.first { sole = only }
    print(sole.map(String.init) ?? "none", Set([Int64?]()).first == nil)
}
