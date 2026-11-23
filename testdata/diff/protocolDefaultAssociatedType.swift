private struct PdaCursor {
    var words: [String]
}

private struct PdaMissing: Error {}

private protocol PdaArg {
    static func resolve(_ c: inout PdaCursor) throws -> Self
    static var label: String { get }
}

private struct PdaWord: PdaArg {
    let text: String
    static func resolve(_ c: inout PdaCursor) throws -> PdaWord {
        guard !c.words.isEmpty else { throw PdaMissing() }
        return PdaWord(text: c.words.removeFirst())
    }
    static var label: String { "word" }
}

private enum PdaSide: String, PdaArg {
    case left, right
    static func resolve(_ c: inout PdaCursor) throws -> PdaSide {
        guard !c.words.isEmpty, let s = PdaSide(rawValue: c.words.removeFirst()) else { throw PdaMissing() }
        return s
    }
    static var label: String { "side" }
}

extension Optional: PdaArg where Wrapped: PdaArg {
    fileprivate static func resolve(_ c: inout PdaCursor) throws -> Self {
        do { return try Wrapped.resolve(&c) } catch { return nil }
    }
    fileprivate static var label: String { "optional " + Wrapped.label }
}

private protocol PdaKeyword: PdaArg, CaseIterable {
    var keyword: String { get }
}

extension PdaKeyword {
    static func resolve(_ c: inout PdaCursor) throws -> Self {
        guard !c.words.isEmpty else { throw PdaMissing() }
        let word = c.words.removeFirst()
        guard let match = Self.allCases.first(where: { $0.keyword.hasPrefix(word) }) else { throw PdaMissing() }
        return match
    }
}

private enum PdaLimb: PdaKeyword {
    case arm, leg
    var keyword: String { self == .arm ? "arm" : "leg" }
    static var label: String { "limb" }
}

private func pdaTake<A: PdaArg>(_: A.Type, _ c: inout PdaCursor) throws -> A {
    try A.resolve(&c)
}

private func pdaLabel<A: PdaArg>(_: A.Type) -> String { A.label }

private protocol PdaCommand {
    associatedtype Arguments
    func parse(_ c: inout PdaCursor) throws -> Arguments
    func run(_ a: Arguments) -> String
}

extension PdaCommand where Arguments: PdaArg {
    func parse(_ c: inout PdaCursor) throws -> Arguments {
        try pdaTake(Arguments.self, &c)
    }

    func describe() -> String { pdaLabel(Arguments.self) }
}

private struct PdaSay: PdaCommand {
    typealias Arguments = PdaWord
    func run(_ a: PdaWord) -> String { "say " + a.text }
}

private struct PdaLook: PdaCommand {
    typealias Arguments = PdaSide?
    func run(_ a: PdaSide?) -> String { "look " + (a?.rawValue ?? "around") }
}

private struct PdaShout: PdaCommand {
    func run(_ a: PdaWord?) -> String { "shout " + (a?.text ?? "nothing") }
}

private struct PdaStretch: PdaCommand {
    typealias Arguments = PdaLimb?
    func run(_ a: PdaLimb?) -> String { "stretch " + (a?.keyword ?? "everything") }
}

private func pdaInvoke<C: PdaCommand>(_ cmd: C, _ words: [String]) -> String {
    var c = PdaCursor(words: words)
    do { return cmd.run(try cmd.parse(&c)) } catch { return "missing" }
}

private func pdaInvokeAny(_ cmd: any PdaCommand, _ words: [String]) -> String {
    pdaInvoke(cmd, words)
}

public func protocolDefaultAssociatedType() {
    print(pdaInvoke(PdaSay(), ["hi"]), pdaInvoke(PdaSay(), []))
    print(pdaInvoke(PdaLook(), ["left"]), pdaInvoke(PdaLook(), []), pdaInvoke(PdaLook(), ["up"]))
    print(pdaInvokeAny(PdaShout(), ["hey"]), pdaInvokeAny(PdaShout(), []))
    print(pdaInvoke(PdaStretch(), ["le"]), pdaInvoke(PdaStretch(), []), pdaInvoke(PdaStretch(), ["x"]))
    print(PdaSay().describe(), PdaLook().describe(), PdaShout().describe())
    print(pdaLabel(PdaWord?.self), pdaLabel(PdaSide.self))
    var c = PdaCursor(words: [])
    print((try? pdaTake(PdaSide?.self, &c)).map { $0?.rawValue ?? "nil" } ?? "threw")
}
