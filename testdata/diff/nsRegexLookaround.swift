import Foundation

private func nrlLinks(_ text: String, key: String) -> String {
    let escaped = NSRegularExpression.escapedPattern(for: key)
    let pattern = "(?i)\\bquest (accept|complete|journal|abandon) " + escaped + "(?![a-z0-9_]|\\.[a-z0-9_])"
    guard let regex = try? NSRegularExpression(pattern: pattern) else { return "no regex" }
    var result = text
    for match in regex.matches(in: result, range: NSRange(result.startIndex..., in: result)).reversed() {
        guard let range = Range(match.range, in: result), let verb = Range(match.range(at: 1), in: result) else { continue }
        result.replaceSubrange(range, with: "{SEND quest \(result[verb].lowercased()) \(key)|\(result[range])}")
    }
    return result
}

private func nrlAll(_ pattern: String, _ groups: Int, _ s: String) -> [String] {
    let re = try! NSRegularExpression(pattern: pattern)
    return re.matches(in: s, range: NSRange(s.startIndex..., in: s)).map { m in
        (0...groups).map { i in
            Range(m.range(at: i), in: s).map { String(s[$0]) } ?? "-"
        }.joined(separator: "/")
    }
}

public func nsRegexLookaround() {
    print(nrlLinks("Type QUEST Journal lore.frag or quest abandon lore.frag.two, quest accept lore.frag.", key: "lore.frag"))
    print(nrlLinks("😀 quest accept lore.frag_x then quest complete lore.frag", key: "lore.frag"))
    print(nrlAll("a(?![a-z])", 0, "ab a1 a"))
    print(nrlAll("(?<=x)(y)(z)?", 2, "xy xyz zy"))
    print(nrlAll("(?<!x)y", 0, "xy zy"))
    print(nrlAll("(\\w)\\1", 1, "a😀😀bbcc"))
    let re = try! NSRegularExpression(pattern: "(\\w+)(?=!)")
    let s = "hi! 😀 yo! no"
    if let m = re.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)), let r = Range(m.range, in: s) {
        print(s[r])
    }
    print(re.stringByReplacingMatches(in: s, options: [], range: NSRange(s.startIndex..., in: s), withTemplate: "<$1>\\$"))
}
