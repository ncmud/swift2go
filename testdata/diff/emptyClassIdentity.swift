private final class EciToken: Hashable {
    static func == (lhs: EciToken, rhs: EciToken) -> Bool { lhs === rhs }
    func hash(into hasher: inout Hasher) { hasher.combine(ObjectIdentifier(self)) }
}

private class EciBase {}
private final class EciDerived: EciBase {}

public func emptyClassIdentity() {
    let a = EciToken(), b = EciToken()
    print(a === b, a == b, ObjectIdentifier(a) == ObjectIdentifier(b), Set([a, b, a]).count)
    let x = EciDerived(), y = EciDerived()
    print(x === y, (x as EciBase) === (y as EciBase))
    var seen: [ObjectIdentifier: Int] = [:]
    for t in [a, b, EciToken()] { seen[ObjectIdentifier(t), default: 0] += 1 }
    print(seen.count)
}
