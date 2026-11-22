// Appending to a String leaves every earlier copy of it unchanged, wherever the String lives.

struct Line {
    var text = ""
}

final class Page {
    var body = ""
}

func appendTwice(_ s: inout String, _ t: String) {
    s += t
    s += t
}

public func stringAppendInPlace() {
    var s = String(repeating: "-", count: 70)
    s += "a"
    let before = s
    s += "b"
    var copy = s
    s += "c"
    copy += "Y"
    var older = before
    older += "Z"
    let prefix = String(s.prefix(71))
    s += "d"
    print(s.suffix(5), before.suffix(3), copy.suffix(3), older.suffix(3), prefix.suffix(2))

    var line = Line(text: String(repeating: "=", count: 66))
    let snapshot = line
    line.text += "!"
    appendTwice(&line.text, "?")
    print(line.text.suffix(4), snapshot.text.count, line.text.count)

    let page = Page()
    for i in 0..<200 { page.body += String(i % 10) }
    let half = page.body
    page.body += "end"
    print(page.body.count, half.count, page.body.suffix(6), half.suffix(3))

    var built = ""
    built.reserveCapacity(500)
    let empty = built
    var add: (String) -> Void = { built += $0 }
    for c in "héllo, wörld 🇺🇸 " { add(String(c)) }
    for _ in 0..<10 { built.append("xyz") }
    built.append(contentsOf: "!")
    add = { _ in }
    print(built, built.count, empty.isEmpty)
}
