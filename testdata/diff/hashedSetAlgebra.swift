struct HSATag: Hashable, Comparable {
    let name: String
    let parts: Set<String>
    var note = ""

    static func == (a: HSATag, b: HSATag) -> Bool { a.name == b.name && a.parts == b.parts }
    func hash(into h: inout Hasher) { h.combine(name) }
    static func < (a: HSATag, b: HSATag) -> Bool { a.name < b.name }
}

func hsaShow(_ s: Set<HSATag>) -> String {
    s.sorted().map { $0.note.isEmpty ? $0.name : "\($0.name)/\($0.note)" }.joined(separator: " ")
}

public func hashedSetAlgebra() {
    let a = HSATag(name: "a", parts: ["x"])
    let b = HSATag(name: "b", parts: ["x", "y"])
    let c = HSATag(name: "c", parts: [])
    let d = HSATag(name: "d", parts: ["z"])
    let left: Set<HSATag> = [a, b, c]
    let right: Set<HSATag> = [HSATag(name: "b", parts: ["y", "x"], note: "right"), d]

    print("union", hsaShow(left.union(right)))
    print("union array", hsaShow(left.union([d, HSATag(name: "a", parts: ["x"], note: "array")])))
    print("union chained", hsaShow(Set([c]).union([a]).union(right)))
    print("intersection", hsaShow(left.intersection(right)))
    print("subtracting", hsaShow(left.subtracting(right)))
    print("symmetric", hsaShow(left.symmetricDifference(right)))
    print("subset", Set([a, b]).isSubset(of: left), left.isSubset(of: right))
    print("superset", left.isSuperset(of: [a, c]), left.isSuperset(of: right))
    print("disjoint", left.isDisjoint(with: [d]), left.isDisjoint(with: right))
    print("intersection array", hsaShow(left.intersection([d, HSATag(name: "b", parts: ["x", "y"], note: "array")])))
    print("subtracting array", hsaShow(left.subtracting([b, d])))
    print("symmetric array", hsaShow(left.symmetricDifference([a, d, d])))
    print("strict subset", Set([a]).isStrictSubset(of: left), left.isStrictSubset(of: left), left.isStrictSubset(of: [a, b, c, d]))
    print("strict superset", left.isStrictSuperset(of: Set([a])), left.isStrictSuperset(of: left), left.isStrictSuperset(of: [a, b]))

    var grown = left
    grown.formUnion(right)
    print("formUnion", hsaShow(grown))
    grown.formUnion([HSATag(name: "e", parts: ["e"]), a])
    print("formUnion array", hsaShow(grown))
    var kept = left
    kept.formIntersection([HSATag(name: "b", parts: ["y", "x"], note: "array"), c, d])
    print("formIntersection", hsaShow(kept))
    kept.formIntersection(right)
    print("formIntersection set", hsaShow(kept))
    var cut = left
    cut.subtract([a])
    print("subtract", hsaShow(cut))
    cut.subtract(right)
    print("subtract set", hsaShow(cut))
    var flipped = left
    flipped.formSymmetricDifference(right)
    print("formSymmetricDifference", hsaShow(flipped))
    flipped.formSymmetricDifference([c, HSATag(name: "e", parts: [])])
    print("formSymmetricDifference array", hsaShow(flipped))

    print("map", left.map { $0.parts.count }.sorted())
    print("sorted", left.sorted().map(\.name), left.sorted { $0.name > $1.name }.map(\.name))
    print("unchanged", hsaShow(left), hsaShow(right))
}
