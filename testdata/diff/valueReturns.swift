// A function hands its caller a value of the caller's own, whatever it was read from.

struct VRStats { var values: [Int] = [1, 2, 3]; var tags: [String] = ["a"] }

enum VRConst { static let defaults = [7, 8, 9] }

final class VRHolder {
    static let empty: [Int] = [0]
    var items: [Int] = [1, 2, 3]
    var stats = VRStats()
    var table: [String: [Int]] = ["k": [1]]
    var lazyCount = 0
    lazy var cached: [Int] = { lazyCount += 1; return [4, 5] }()
    var computed: VRStats { stats }
    func all() -> [Int] { items }
    func first(_ n: Int) -> [Int] { Array(items.prefix(n)) }
    func lookup(_ k: String) -> [Int] { table[k] ?? [] }
    func lookupOptional(_ k: String) -> [Int]? { table[k] }
    func set(_ xs: [Int]) { items = xs }
    func pair() -> ([Int], VRStats) { (items, stats) }
    func defaults() -> [Int] { VRConst.defaults }
    func none() -> [Int] { VRHolder.empty }
    func same(_ xs: [Int]) -> [Int] { xs }
    func viaLet() -> [Int] { let xs = items; return xs }
    func viaVar() -> [Int] { var xs = items; xs.append(4); return xs }
    func choose(_ flag: Bool) -> [Int] { flag ? items : [] }
}

private func returnedFromClass(_ out: inout [String]) {
    let h = VRHolder()
    var a = h.all()
    a[0] = 10
    var f = h.first(2)
    f[0] = 20
    var l = h.lookup("k")
    l.append(2)
    var o = h.lookupOptional("k")
    o?.append(3)
    var c = h.computed
    c.values[0] = 30
    var (p, q) = h.pair()
    p[0] = 40
    q.values[1] = 50
    out.append("\(h.items) \(h.table["k"]!) \(h.stats.values) \(a) \(f) \(l) \(o!) \(c.values) \(p) \(q.values)")
}

private func returnedConstants(_ out: inout [String]) {
    let h = VRHolder()
    var d = h.defaults()
    d[0] = 0
    var n = h.none()
    n.append(1)
    var g = VRConst.defaults
    g[1] = 0
    out.append("\(VRConst.defaults) \(VRHolder.empty) \(d) \(n) \(g)")
}

private func returnedParameter(_ out: inout [String]) {
    let h = VRHolder()
    var mine = [1, 2]
    var back = h.same(mine)
    back[0] = 9
    mine[1] = 8
    h.set(mine)
    mine[0] = 7
    var viaLet = h.viaLet()
    viaLet[0] = 6
    var viaVar = h.viaVar()
    viaVar[0] = 5
    var chosen = h.choose(true)
    chosen[0] = 4
    out.append("\(mine) \(back) \(h.items) \(viaLet) \(viaVar) \(chosen)")
}

private func returnedLazy(_ out: inout [String]) {
    let h = VRHolder()
    var c = h.cached
    c[0] = 0
    h.cached.append(6)
    out.append("\(h.cached) \(c) \(h.lazyCount)")
}

private func returnedFromClosure(_ out: inout [String]) {
    let h = VRHolder()
    let get = { h.items }
    var a = get()
    a[0] = 100
    var rows = [[1], [2]]
    let row: (Int) -> [Int] = { rows[$0] }
    var r = row(0)
    r[0] = 200
    rows[1][0] = 300
    out.append("\(h.items) \(a) \(rows) \(r)")
}

func vrDup<T>(_ x: T) -> (T, T) { (x, x) }
func vrIdentity<T>(_ x: T) -> T { x }

private func returnedGeneric(_ out: inout [String]) {
    let h = VRHolder()
    var (a, b) = vrDup(h.items)
    a[0] = 11
    b[1] = 22
    var c = vrIdentity(h.stats)
    c.values[2] = 33
    out.append("\(h.items) \(a) \(b) \(h.stats.values) \(c.values)")
}

public func valueReturns() {
    var out: [String] = []
    returnedFromClass(&out)
    returnedConstants(&out)
    returnedParameter(&out)
    returnedLazy(&out)
    returnedFromClosure(&out)
    returnedGeneric(&out)
    print(out.joined(separator: "\n"))
}
