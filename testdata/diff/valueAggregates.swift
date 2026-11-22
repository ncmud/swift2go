// Swift values are independent: a copy does not follow later changes to the place it was read from.

struct VSPoint { var x = 0; var arr: [Int] = [] }

final class VSBox {
    var items: [Int] = [1, 2, 3]
    var grid: [[Int]] = [[1, 2], [3, 4]]
    var dd: [String: [Int]] = ["a": [1, 2]]
    var computedItems: [Int] { items }
    func snapshot() -> [Int] { return items }
    func takes(_ xs: [Int]) -> [Int] { items[0] = 77; return xs }
}

enum VSList { case list([Int]) }

private func valueAppend(_ out: inout [String]) {
    var row = [1, 2]
    var g: [[Int]] = []
    g.append(row)
    row[0] = 9
    g.append(row)
    out.append("\(g)")
    var p = VSPoint(x: 1, arr: [1, 2])
    var ps: [VSPoint] = []
    ps.append(p)
    p.arr[0] = 9
    out.append("\(ps[0].arr) \(p.arr)")
    var q = [p, p]
    q[0].arr[1] = 5
    p.arr[0] = 6
    out.append("\(q[0].arr) \(q[1].arr) \(p.arr)")
}

private func valueRepeating(_ out: inout [String]) {
    var g = Array(repeating: Array(repeating: 0, count: 2), count: 2)
    g[0][0] = 1
    out.append("\(g)")
    let row = [0, 0]
    var h = Array(repeating: row, count: 3)
    h[1][1] = 5
    out.append("\(h) \(row)")
    var ps = Array(repeating: VSPoint(x: 0, arr: [0]), count: 2)
    ps[0].arr[0] = 7
    out.append("\(ps[0].arr) \(ps[1].arr)")
}

private func valueStructField(_ out: inout [String]) {
    let b = VSBox()
    let p = VSPoint(x: 1, arr: b.items)
    b.items[1] = 55
    out.append("\(p.arr)")
    var q = p
    q.arr[0] = 3
    out.append("\(p.arr) \(q.arr)")
}

private func valueEnumPayload(_ out: inout [String]) {
    let b = VSBox()
    let e = VSList.list(b.items)
    b.items[0] = 9
    if case .list(var xs) = e { xs[1] = 0; out.append("\(xs)") }
    if case .list(let xs) = e { out.append("\(xs)") }
}

struct VSPair { var left: [Int]; var right: VSPoint }

final class VSLedger {
    var rows: [[Int]] = []
    var pairs: [VSPair] = []
    func keep(_ incoming: [[Int]]) { rows = incoming }
    func total(_ incoming: [[Int]]) -> Int { incoming.reduce(0) { $0 + $1.count } }
}

private func valueTransientAggregates(_ out: inout [String]) {
    let b = VSBox()
    let ledger = VSLedger()
    func touch(_ rows: [[Int]]) -> [[Int]] { b.items[0] = 50; return rows }
    let seen = touch([b.items, b.items])
    var held = [b.items]
    held[0][1] = 60
    let wrapped = VSPoint(x: 1, arr: b.items)
    let payload = VSList.list(b.items)
    let pair = VSPair(left: b.items, right: VSPoint(x: 2, arr: b.items))
    let table = ["k": b.items]
    ledger.keep([b.items])
    ledger.pairs.append(VSPair(left: b.items, right: wrapped))
    let count = ledger.total([b.items, [1]])
    b.items[2] = 70
    b.items.append(80)
    var payloadItems: [Int] = []
    if case .list(let xs) = payload { payloadItems = xs }
    out.append("\(seen) \(held) \(wrapped.arr) \(payloadItems) \(pair.left) \(pair.right.arr) \(table["k"]!)")
    out.append("\(ledger.rows) \(ledger.pairs[0].left) \(ledger.pairs[0].right.arr) \(count) \(b.items)")
}

public func valueAggregates() {
    var out: [String] = []
    valueAppend(&out)
    valueRepeating(&out)
    valueStructField(&out)
    valueEnumPayload(&out)
    valueTransientAggregates(&out)
    print(out.joined(separator: "\n"))
}
