private struct GssCursor {
    var words: [String]
    init(_ w: [String]) { words = w }
}

private protocol GssArg {
    static func resolve(_ c: inout GssCursor) throws -> Self
}

private struct GssWord: GssArg {
    var text: String
    static func resolve(_ c: inout GssCursor) throws -> GssWord { GssWord(text: c.words.removeFirst()) }
    mutating func shout() { text = text.uppercased() }
}

private struct GssNum: GssArg {
    let n: Int
    static func resolve(_ c: inout GssCursor) throws -> GssNum { GssNum(n: Int(c.words.removeFirst()) ?? -1) }
}

private func gssTake<T: GssArg>(_: T.Type, _ c: inout GssCursor) throws -> T { try T.resolve(&c) }

public func genericStaticSelf() {
    var c = GssCursor(["hi", "42"])
    var w = try! gssTake(GssWord.self, &c)
    w.shout()
    let n = try! gssTake(GssNum.self, &c)
    print(w.text, n.n)
}
