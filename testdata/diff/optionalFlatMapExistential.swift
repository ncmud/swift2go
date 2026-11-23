private protocol OfeCaller: AnyObject { func say() -> String }

private final class OfeSession: OfeCaller {
    let n: Int
    init?(_ n: Int) {
        if n < 0 { return nil }
        self.n = n
    }
    func say() -> String { "session \(n)" }
}

private protocol OfeNamed { func name() -> String }

private struct OfeTag: OfeNamed {
    let s: String
    init?(_ s: String) {
        if s.isEmpty { return nil }
        self.s = s
    }
    func name() -> String { "tag " + s }
}

private func ofeFind(_ n: Int?) -> (any OfeCaller)? {
    n.flatMap { OfeSession($0) }
}

private func ofeFindRef(_ s: OfeSession?) -> (any OfeCaller)? {
    s.flatMap { OfeSession($0.n - 5) }
}

private func ofeTag(_ s: String?) -> (any OfeNamed)? {
    s.flatMap { OfeTag($0) }
}

public func optionalFlatMapExistential() {
    for n in [3, -1, nil] as [Int?] {
        if let c = ofeFind(n) { print(c.say()) } else { print("none") }
    }
    for s in [OfeSession(9), OfeSession(2), nil] {
        print(ofeFindRef(s)?.say() ?? "none")
    }
    for s in ["a", "", nil] as [String?] {
        print(ofeTag(s)?.name() ?? "none")
    }
}
