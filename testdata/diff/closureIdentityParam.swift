private final class CipNode { let n: Int; init(_ n: Int) { self.n = n } }
private struct CipMissing: Error {}
private func cipNeed<T>(_ v: T?) throws -> T { guard let v else { throw CipMissing() }; return v }

public func closureIdentityParam() {
    let a = CipNode(1), b = CipNode(2), c = CipNode(3)
    do {
        let first = try cipNeed([a, b, c].first { $0.n == 2 })
        let other: CipNode = try cipNeed([a, b, c].first { $0 !== first })
        print(other.n)
        print([a, b, c].filter { $0 !== first }.map(\.n))
        print([a, b, c].contains { $0 === c })
    } catch {
        print("missing")
    }
}
