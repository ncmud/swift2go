struct Pick: Equatable {
    var id: Int
    var note: String
    static func == (l: Pick, r: Pick) -> Bool { l.id == r.id }
}
struct Key: Hashable {
    var id: Int
    var note: String
    static func == (l: Key, r: Key) -> Bool { l.id == r.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}
final class Node: Equatable {
    let id: Int
    init(_ id: Int) { self.id = id }
    static func == (l: Node, r: Node) -> Bool { l.id == r.id }
}
final class Tag: Hashable {
    let name: String
    init(_ name: String) { self.name = name }
    static func == (l: Tag, r: Tag) -> Bool { l.name == r.name }
    func hash(into hasher: inout Hasher) { hasher.combine(name) }
}

final class Batch: Hashable {
    var lines: [String] = []
    static func == (l: Batch, r: Batch) -> Bool { l === r }
    func hash(into hasher: inout Hasher) { hasher.combine(ObjectIdentifier(self)) }
}

public func customEquality() {
    let a = Pick(id: 1, note: "x"), b = Pick(id: 1, note: "y")
    print(a == b, a != b, [a].contains(b), [a] == [b], [a] != [b], [b].firstIndex(of: a) ?? -1)
    let k1 = Key(id: 1, note: "x"), k2 = Key(id: 1, note: "y")
    print(Set([k1, k2]).count, [k1: "one"][k2] ?? "missing", Set([k1]).contains(k2))
    let n1 = Node(1), n2 = Node(1)
    print(n1 == n2, n1 != n2, [n1].contains(n2), [n2].firstIndex(of: n1) ?? -1, [n1] == [n2])
    let t1 = Tag("t"), t2 = Tag("t")
    print(Set([t1, t2]).count, [t1: 1][t2] ?? 0)
    let opt: Pick? = b
    print(opt == a, opt != a)
    let b1 = Batch(), b2 = Batch()
    var pending: [Batch: Int] = [b1: 1]
    pending[b2, default: 0] += 5
    pending[b1, default: 0] += 1
    print(pending.count, pending[b1] ?? 0, pending[b2] ?? 0, ObjectIdentifier(b1) == ObjectIdentifier(b1), ObjectIdentifier(b1) == ObjectIdentifier(b2))
}
