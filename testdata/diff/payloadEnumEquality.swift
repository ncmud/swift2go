struct Quote: Equatable {
    var grade: Int
    var yield: [String]
}
struct Loose: Equatable {
    var id: Int
    var note: String
    static func == (l: Loose, r: Loose) -> Bool { l.id == r.id }
}
enum Step: Equatable {
    case learn(String)
    case disenchant(Int64, Quote)
    case loose(Loose)
    case plain
}
enum Flat: Equatable {
    case one(Int)
    case two(String, Bool)
}
struct Pending: Equatable {
    let step: Step
    func confirms(_ other: Step) -> Bool { step == other }
}

public func payloadEnumEquality() {
    let a = Step.disenchant(7, Quote(grade: 2, yield: ["dust", "crystal"]))
    let b = Step.disenchant(7, Quote(grade: 2, yield: ["dust", "crystal"]))
    let c = Step.disenchant(7, Quote(grade: 2, yield: ["dust"]))
    print(a == b, a != b, a == c, a != c, a == .plain, a == .learn("x"))
    print(Pending(step: a).confirms(b), Pending(step: a).confirms(c), Pending(step: a) == Pending(step: b))
    print(Step.loose(Loose(id: 1, note: "x")) == .loose(Loose(id: 1, note: "y")))
    print([a, .plain].contains(b), [a].firstIndex(of: c) ?? -1)
    let f: Flat = .two("x", true)
    print(f == .two("x", true), f == .one(1), f != .two("x", false))
}
