// Swift values are independent: a copy does not follow later changes to the place it was read from.

struct VSAffect { var dur: Int; var mods: [Int] }

final class VSMob {
    var affects: [VSAffect] = [VSAffect(dur: 3, mods: [1]), VSAffect(dur: 1, mods: [2])]
    func snapshot() -> [VSAffect] { affects }
    func tick() {
        for i in affects.indices { affects[i].dur -= 1; affects[i].mods[0] += 10 }
        affects.removeAll { $0.dur <= 0 }
    }
}

struct VSInventory {
    var items: [Int] = [1, 2, 3]
    func all() -> [Int] { items }
    mutating func take() -> [Int] { let out = items; items.removeAll(); return out }
}

final class VSRegistry {
    var byName: [String: [Int]] = [:]
    var log: [[Int]] = []
    func record(_ xs: [Int]) { log.append(xs); byName["last"] = xs }
}

private func valueArgument(_ out: inout [String]) {
    let b = VSBox()
    let seen = b.takes(b.items)
    out.append("\(seen) \(b.items)")
}

private func valueStoredArgument(_ out: inout [String]) {
    let r = VSRegistry()
    var xs = [1, 2]
    r.record(xs)
    xs[0] = 9
    r.record(xs)
    r.log[0][1] = 7
    out.append("\(r.log) \(r.byName["last"]!) \(xs)")
}

private func valueReturn(_ out: inout [String]) {
    func pick(_ g: [[Int]]) -> [Int] { return g[0] }
    var g = [[1, 2], [3]]
    var r = pick(g)
    r[0] = 7
    g[0][1] = 8
    out.append("\(g) \(r)")
}

private func valueClosureParameter(_ out: inout [String]) {
    let ps = [VSPoint(x: 1, arr: [1]), VSPoint(x: 2, arr: [2])]
    let qs = ps.map { p -> VSPoint in var c = p; c.arr[0] += 10; c.x += 1; return c }
    out.append("\(ps.map { $0.arr }) \(qs.map { $0.arr })")
    let g = [[1], [2]]
    let h = g.map { r -> [Int] in var c = r; c[0] = 0; return c }
    out.append("\(g) \(h)")
}

private func valueInout(_ out: inout [String]) {
    func fill(_ xs: inout [Int], from src: [Int]) { xs.append(contentsOf: src); xs[0] = -1 }
    var a = [1, 2]
    let b = a
    fill(&a, from: b)
    var c = a
    c[1] = 50
    out.append("\(a) \(b) \(c)")
}

private func valueStructMethods(_ out: inout [String]) {
    var inv = VSInventory()
    var a = inv.all()
    a[0] = 10
    let taken = inv.take()
    inv.items.append(4)
    out.append("\(a) \(taken) \(inv.items)")
    var copy = inv
    copy.items[0] = 5
    out.append("\(inv.items) \(copy.items)")
}

private func valueSnapshotThenTick(_ out: inout [String]) {
    let m = VSMob()
    let before = m.snapshot()
    let before2 = m.affects
    m.tick()
    out.append("\(before.map { $0.dur }) \(before.map { $0.mods }) \(before2.map { $0.dur }) \(before2.map { $0.mods }) \(m.affects.count)")
    func total(_ xs: [VSAffect]) -> Int { m.tick(); return xs.reduce(0) { $0 + $1.dur } }
    let m2 = VSMob()
    out.append("\(total(m2.affects))")
}

public func valueArguments() {
    var out: [String] = []
    valueArgument(&out)
    valueStoredArgument(&out)
    valueReturn(&out)
    valueClosureParameter(&out)
    valueInout(&out)
    valueStructMethods(&out)
    valueSnapshotThenTick(&out)
    print(out.joined(separator: "\n"))
}
