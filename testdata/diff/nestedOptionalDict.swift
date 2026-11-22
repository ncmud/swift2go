func noLookup(_ d: [String: Int?], _ k: String) -> String {
    switch d[k] {
    case .none: return "missing"
    case .some(.none): return "stored nil"
    case .some(.some(let v)): return "value \(v)"
    }
}

func noDouble(_ x: Int??) -> String {
    if let inner = x {
        if let v = inner { return "some(some(\(v)))" }
        return "some(none)"
    }
    return "none"
}

public func nestedOptionalDict() {
    var d: [String: Int?] = ["a": 1, "b": nil]
    print(noLookup(d, "a"), noLookup(d, "b"), noLookup(d, "c"))
    print(d.count, d.keys.sorted())
    d["c"] = .some(nil)
    print(d.count, noLookup(d, "c"))
    d["a"] = nil
    print(d.count, d.keys.sorted())
    d.updateValue(nil, forKey: "z")
    print(d.count, noLookup(d, "z"))
    if let v = d["b"] { print("b present, inner \(String(describing: v))") }
    if let v = d["q"] { print("q present \(String(describing: v))") } else { print("q absent") }
    print(d["b"] == .some(nil), d["q"] == nil)
    let flat = d["c"] ?? 5
    print(String(describing: flat))
    print(noDouble(nil), noDouble(.some(nil)), noDouble(3))
    let opt: Int?? = .some(nil)
    print(opt == nil, String(describing: opt))
    for (k, v) in d.sorted(by: { $0.key < $1.key }) { print(k, String(describing: v)) }
    noClasses()
}

final class NOBox { let n: Int; init(_ n: Int) { self.n = n } }

func noClasses() {
    var d: [String: NOBox?] = ["a": NOBox(1), "b": nil]
    d["c"] = .some(NOBox(3))
    d["e"] = .some(nil)
    let direct: NOBox? = .some(NOBox(4))
    print(direct?.n ?? -1)
    for k in ["a", "b", "c", "d", "e"] {
        switch d[k] {
        case .none: print(k, "missing")
        case .some(.none): print(k, "stored nil")
        case .some(.some(let b)): print(k, b.n)
        }
    }
}
