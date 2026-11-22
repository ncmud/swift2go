// Swift values are independent: a copy does not follow later changes to the place it was read from.

private func valueLetSnapshot(_ out: inout [String]) {
    let b = VSBox()
    let snap = b.items
    b.items[0] = 99
    b.items.append(4)
    out.append("\(snap) \(b.items)")
}

private func valueCallSnapshot(_ out: inout [String]) {
    let b = VSBox()
    let snap = b.snapshot()
    let snap2 = b.computedItems
    b.items[0] = 99
    out.append("\(snap) \(snap2) \(b.items)")
}

private func valueSubscriptRow(_ out: inout [String]) {
    let b = VSBox()
    var row = b.grid[0]
    row[0] = 100
    out.append("\(b.grid) \(row)")
    var g = [[1, 2], [3, 4]]
    var r = g[1]
    r[1] = 50
    g[0][0] = 9
    out.append("\(g) \(r)")
    let fixed = g[1]
    g[1][0] = 60
    out.append("\(g) \(fixed)")
}

private func valueOptionalBinding(_ out: inout [String]) {
    let b = VSBox()
    if var xs = b.dd["a"] { xs[0] = 42; out.append("\(xs)") }
    out.append("\(b.dd["a"]!)")
    guard let ys = b.dd["a"] else { return }
    b.dd["a"]![1] = 43
    out.append("\(ys) \(b.dd["a"]!)")
}

private func valueTernaryAndCoalesce(_ out: inout [String]) {
    let b = VSBox(), c = VSBox()
    var x = b.items.count > 0 ? b.items : c.items
    x[0] = 1000
    out.append("\(b.items) \(x)")
    var o: [Int]? = b.items
    o?[1] = 2000
    var y = o ?? c.items
    y[2] = 3000
    out.append("\(b.items) \(o!) \(y)")
}

private func valueGrid(_ out: inout [String]) {
    var grid = Array(repeating: Array(repeating: ".", count: 3), count: 2)
    grid[0][0] = "#"
    var row = grid[1]
    row[2] = "@"
    let snapshot = grid.map { $0 }
    grid[1][1] = "!"
    out.append("\(grid) \(row) \(snapshot)")
}

private func valueFilterElements(_ out: inout [String]) {
    var rows = [[1, 2], [3], [4, 5]]
    var big = rows.filter { $0.count > 1 }
    big[0][0] = 9
    var first = rows.first!
    first[1] = 8
    var sorted = rows.sorted { $0.count < $1.count }
    sorted[0][0] = 7
    rows[2][1] = 6
    out.append("\(rows) \(big) \(first) \(sorted)")
}

private func valueDictionaryOfArrays(_ out: inout [String]) {
    var d = ["a": [1, 2]]
    var e = d
    e["a"]![0] = 9
    d["b"] = d["a"]
    d["b"]!.append(3)
    var vals = d["a", default: []]
    vals.append(4)
    out.append("\(d["a"]!) \(d["b"]!) \(e["a"]!) \(vals)")
}

private func valueSetCopy(_ out: inout [String]) {
    var s: Set<Int> = [1, 2, 3]
    let t = s
    s.insert(4)
    s.remove(1)
    out.append("\(s.sorted()) \(t.sorted())")
}

private func valueTuple(_ out: inout [String]) {
    let b = VSBox()
    let t = (b.items, 1)
    b.items[0] = 9
    out.append("\(t.0)")
    var u = (a: [1, 2], b: 2)
    var v = u
    v.a[0] = 100
    u.b = 3
    out.append("\(u.a) \(u.b) \(v.a) \(v.b)")
}

public func valueBindings() {
    var out: [String] = []
    valueLetSnapshot(&out)
    valueCallSnapshot(&out)
    valueSubscriptRow(&out)
    valueOptionalBinding(&out)
    valueTernaryAndCoalesce(&out)
    valueGrid(&out)
    valueFilterElements(&out)
    valueDictionaryOfArrays(&out)
    valueSetCopy(&out)
    valueTuple(&out)
    print(out.joined(separator: "\n"))
}
