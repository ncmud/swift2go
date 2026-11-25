public func substringFirstMatch() {
    let s = "xx[12] y"
    let sub = s.dropFirst(2).prefix(4)
    print(sub, sub.firstMatch(of: #/\[\d+\]\s*$/#) != nil, sub.firstMatch(of: #/^\[/#) != nil, sub.firstMatch(of: #/y/#) == nil)
    if let m = sub.firstMatch(of: #/(\d)(\d)/#) {
        print(m.output.0, m.output.1, m.output.2, s.distance(from: s.startIndex, to: m.range.lowerBound), s.distance(from: s.startIndex, to: m.range.upperBound))
    }
    if let m = s.dropFirst(3).firstMatch(of: #/(\d)(x)?/#) {
        print(m.output.0, m.output.1, m.output.2 ?? "none")
    }
    let output = "You hit the dummy. [12]\nThe dummy staggers. [3]  \nNothing here."
    for line in output.split(separator: "\n") {
        print(line.firstMatch(of: #/\[\d+\]\s*$/#) == nil)
    }
    let lastLine = output.split(separator: "\n").dropLast().last
    print(lastLine.map { $0.firstMatch(of: #/\[\d+\]\s*$/#) == nil } ?? false)
    print(""[...].firstMatch(of: #/^$/#) != nil)
    // A trailing \b sees the base string's text after the substring's end; the end itself is a boundary.
    print("a.b".prefix(2).firstMatch(of: #/a\b/#) == nil, "can't".prefix(4).firstMatch(of: #/n\b/#) == nil,
          "3.14".prefix(2).firstMatch(of: #/3\b/#) == nil, "ab".prefix(1).firstMatch(of: #/a\b/#) != nil,
          "x a.b".dropFirst(2).prefix(2).firstMatch(of: #/\w\b/#) == nil, "ab cd".dropFirst(3).firstMatch(of: #/c\w\b/#) != nil)
}
