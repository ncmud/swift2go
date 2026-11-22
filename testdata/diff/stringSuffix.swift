public func stringSuffix() {
    let line = "orc [3001]"
    let rest = line.dropFirst(4)
    print(rest[rest.index(after: rest.startIndex)..<rest.index(before: rest.endIndex)])
    print("hello world".suffix(3), String("hello world".suffix(5)), "ab".suffix(5), "ab".suffix(0))
    print(rest.suffix(3), rest.suffix(10))
    let e = "caf\u{e9}s!"
    print(e.suffix(2), e[e.index(before: e.endIndex)], e.dropLast(2))
    let sub = e.dropFirst()
    print(sub[sub.index(before: sub.endIndex)], sub.suffix(3), sub.dropLast())
}
