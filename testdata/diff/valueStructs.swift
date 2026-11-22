// Structs copy with everything they hold; classes inside them keep their identity.

final class VTCounter { var n = 0 }

struct VTInner { var list: [Int] = [1] }
struct VTOuter {
    var inner = VTInner()
    var counter = VTCounter()
    var history: [VTInner] = []
    var total: Int { inner.list.reduce(0, +) }
    func snapshot() -> VTOuter { self }
    func lists() -> [Int] { inner.list }
    mutating func push(_ v: Int) { history.append(inner); inner.list.append(v) }
    mutating func reset(to other: VTOuter) { self = other }
    mutating func drain() -> [Int] { let all = inner.list; inner.list.removeAll(); return all }
}

final class VTOwner {
    var outer = VTOuter()
    var log: [VTOuter] = []
    func remember() { log.append(outer) }
    func mutateDuring(_ o: VTOuter) -> Int { outer.inner.list.append(100); return o.total }
}

private func structCopies(_ out: inout [String]) {
    var a = VTOuter()
    var b = a
    b.inner.list.append(2)
    b.counter.n = 5
    a.push(3)
    let s = a.snapshot()
    a.push(4)
    var l = a.lists()
    l[0] = 0
    out.append("\(a.inner.list) \(b.inner.list) \(a.counter.n) \(s.inner.list) \(s.history.count) \(a.history.map { $0.list }) \(l)")
    var c = VTOuter()
    c.reset(to: a)
    c.inner.list[0] = 50
    let drained = c.drain()
    out.append("\(a.inner.list) \(c.inner.list) \(drained) \(c.history.count)")
}

private func structInClass(_ out: inout [String]) {
    let o = VTOwner()
    let before = o.outer
    o.outer.push(2)
    o.remember()
    o.outer.inner.list[0] = 9
    o.remember()
    var held = o.outer.inner
    held.list.append(7)
    let seen = o.mutateDuring(o.outer)
    out.append("\(before.inner.list) \(o.log.map { $0.inner.list }) \(held.list) \(o.outer.inner.list) \(seen)")
}

struct VTStack<T> {
    var items: [T] = []
    mutating func push(_ x: T) { items.append(x) }
    mutating func pop() -> T? { items.popLast() }
    func peekAll() -> [T] { items }
}

private func structGeneric(_ out: inout [String]) {
    var s = VTStack<[Int]>()
    var row = [1]
    s.push(row)
    row.append(2)
    s.push(row)
    var t = s
    var top = t.pop()!
    top.append(3)
    var all = s.peekAll()
    all[0][0] = 9
    out.append("\(s.items) \(t.items) \(top) \(all) \(row)")
}

private func structObservers(_ out: inout [String]) {
    final class Watched {
        var log: [String] = []
        var list: [Int] = [1] { didSet { log.append("\(oldValue)->\(list)") } }
    }
    let w = Watched()
    w.list.append(2)
    w.list[0] = 5
    let kept = w.list
    w.list = [9]
    out.append("\(w.log) \(kept)")
}

struct VTBacking {
    private var _values: [Int] = [1, 2]
    var values: [Int] { _values }
    mutating func set(_ i: Int, _ v: Int) { _values[i] = v }
}

private func structUnderscoreStorage(_ out: inout [String]) {
    let a = VTBacking()
    var b = a
    b.set(0, 9)
    out.append("\(a.values) \(b.values)")
}

public func valueStructs() {
    var out: [String] = []
    structCopies(&out)
    structInClass(&out)
    structGeneric(&out)
    structObservers(&out)
    structUnderscoreStorage(&out)
    print(out.joined(separator: "\n"))
}
