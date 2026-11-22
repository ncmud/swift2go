class OCA {
    init() {}
    func f() -> String { "OCA" }
    func g() -> String { "gA" }
    func call() -> String { f() + "/" + g() }
    var label: String { "lA" }
    func add(_ n: Int) -> Int { n + 1 }
}
class OCB: OCA {
    override init() { super.init() }
    override func f() -> String { "OCB<" + super.f() + ">" }
    override var label: String { "lB+" + super.label }
    override func add(_ n: Int) -> Int { super.add(n) * 10 }
}
class OCC: OCB {
    override func f() -> String { "OCC<" + super.f() + ">" }
    override func g() -> String { "gC" }
    override func add(_ n: Int) -> Int { super.add(n) + 5 }
}
class OCD: OCC {
    override func f() -> String { "OCD<" + super.f() + ">" }
    override var label: String { "lD+" + super.label }
}
class OCE: OCD {}
final class OCF: OCE {
    override func g() -> String { "gF<" + super.g() + ">" }
    override func add(_ n: Int) -> Int { super.add(n) - 1 }
}

public func classOverrideChain() {
    var out: [String] = []
    let all: [OCA] = [OCA(), OCB(), OCC(), OCD(), OCE(), OCF()]
    for x in all {
        out.append("\(x.f()) \(x.g()) \(x.call()) \(x.label) \(x.add(1))")
    }
    let b = OCB()
    out.append(b.f())
    let c: OCB = OCC()
    out.append(c.f() + " " + c.label)
    print(out.joined(separator: "\n"))
}
