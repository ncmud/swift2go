// Swift values are independent: a copy does not follow later changes to the place it was read from.

struct VSGroup { var members: [Int]; var name: String }

private func valueForInRows(_ out: inout [String]) {
    var g = [[1, 2], [3, 4]]
    for var r in g { r[0] = 0; _ = r }
    out.append("\(g)")
    var seen: [[Int]] = []
    for r in g { g[1][1] = 99; seen.append(r) }
    out.append("\(seen) \(g)")
}

private func valueForVarStruct(_ out: inout [String]) {
    var groups = ["a": VSGroup(members: [1, 2, 3], name: "a")]
    for var g in groups.values { g.members.removeAll { $0 == 2 }; g.members[0] = 99; g.name = "z" }
    let list = [VSGroup(members: [1, 2, 3], name: "l")]
    for var g in list { g.members.remove(at: 0); g.members[0] = 77 }
    groups["b"] = groups["a"]
    groups["b"]!.members[1] = -1
    out.append("\(groups["a"]!.members) \(groups["b"]!.members) \(list[0].members)")
}

private func valueCapturedVar(_ out: inout [String]) {
    var xs = [1, 2, 3]
    let bump = { xs[0] += 1 }
    let before = xs
    bump()
    var seen: [Int] = []
    for x in xs { bump(); seen.append(x) }
    out.append("\(before) \(xs) \(seen)")
}

final class ITItem { let name: String; init(_ n: String) { name = n } }

final class ITRoom {
    private var _objects: [ITItem] = []
    var objects: [ITItem] { get { _objects } set { _objects = newValue } }
    func add(_ o: ITItem) { objects.append(o) }
    @discardableResult func removeObject(_ o: ITItem) -> Bool {
        guard let idx = objects.firstIndex(where: { $0 === o }) else { return false }
        objects.remove(at: idx)
        return true
    }
}

final class ITChar {
    var inventory: [ITItem] = []
    func removeFromInventory(_ o: ITItem) {
        if let idx = inventory.firstIndex(where: { $0 === o }) { inventory.remove(at: idx) }
    }
}

private func iterationDropAll(_ out: inout [String]) {
    let ch = ITChar(); let room = ITRoom()
    ch.inventory = ["sword", "shield", "potion", "torch", "bread"].map(ITItem.init)
    var log: [String] = []
    for obj in ch.inventory { ch.removeFromInventory(obj); room.add(obj); log.append(obj.name) }
    out.append("dropped \(log) left \(ch.inventory.map { $0.name })")
    log = []
    for obj in room.objects { if obj.name != "torch" { room.removeObject(obj) }; log.append(obj.name) }
    out.append("purged \(log) left \(room.objects.map { $0.name })")
}

final class ITBag { var items: [Int] = [1, 2, 3] }

private func iterationRemoveWhileLooping(_ out: inout [String]) {
    let b = ITBag()
    var seen: [Int] = []
    for x in b.items { b.items.removeAll { $0 == x }; seen.append(x) }
    out.append("\(seen) \(b.items)")
    b.items = [1, 2, 3, 4]
    seen = []
    for x in b.items {
        if let i = b.items.firstIndex(of: x) { b.items.remove(at: i) }
        seen.append(x)
    }
    out.append("\(seen) \(b.items)")
    b.items = [1, 2, 3]
    seen = []
    for x in b.items {
        if x < 3 { b.items.append(x + 10) }
        b.items[b.items.count - 1] = 0
        seen.append(x)
    }
    out.append("\(seen) \(b.items)")
}

private func iterationLocalCollections(_ out: inout [String]) {
    var d = ["a": 1, "b": 2, "c": 3]
    var n = 0
    for (k, _) in d { d.removeAll(); d["z\(k)"] = 1; n += 1 }
    out.append("\(n) \(d.count)")
    var s: Set<Int> = [1, 2, 3]
    n = 0
    for _ in s { s.removeAll(); n += 1 }
    out.append("\(n) \(s.count)")
    var xs = [1, 2, 3]
    var seen: [Int] = []
    for x in xs { if xs.count > 1 { xs[1] = 30 }; xs.removeFirst(); seen.append(x) }
    out.append("\(seen) \(xs)")
}

private func iterationForEach(_ out: inout [String]) {
    let b = ITBag()
    var seen: [Int] = []
    b.items.forEach { x in b.items.removeAll { $0 == x }; seen.append(x) }
    out.append("\(seen) \(b.items)")
    b.items = [3, 1, 2]
    let doubled = b.items.map { x -> Int in b.items[2] = 100; return x * 2 }
    out.append("\(doubled) \(b.items)")
}

private func iterationEnumerated(_ out: inout [String]) {
    let b = ITBag()
    var seen: [Int] = []
    for (i, x) in b.items.enumerated() { if i == 0 { b.items.removeAll() }; seen.append(x) }
    out.append("\(seen) \(b.items)")
    b.items = [5, 6, 7]
    seen = []
    for x in b.items.reversed() { b.items.removeLast(); b.items.append(0); seen.append(x) }
    out.append("\(seen) \(b.items)")
}

public func valueLoops() {
    var out: [String] = []
    valueForInRows(&out)
    valueForVarStruct(&out)
    valueCapturedVar(&out)
    iterationDropAll(&out)
    iterationRemoveWhileLooping(&out)
    iterationLocalCollections(&out)
    iterationForEach(&out)
    iterationEnumerated(&out)
    print(out.joined(separator: "\n"))
}
