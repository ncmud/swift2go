public func substringEquality() {
    let words = "send mail now".split(separator: " ")
    let hit = words.contains { ["mail", "letter"].contains($0) }
    print(hit, words.contains("mail"), words.firstIndex(of: "now") ?? -1)
    var counts: [Substring: Int] = [:]
    for w in "a b a".split(separator: " ") { counts[w, default: 0] += 1 }
    print(counts.count, counts["a"] ?? 0)
    let unique = Set("x y x".split(separator: " "))
    print(unique.count, unique.contains("x"))
    let a = "ab ab".split(separator: " ")
    print(a[0] == a[1], a[0] != a[1], [a[0]] == [a[1]])
}
