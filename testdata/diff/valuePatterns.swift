// Pattern bindings (if let, guard let, switch, tuples) are values of their own.

struct VPStats { var values: [Int] = [1, 2, 3] }
enum VPShape { case list([Int]); case pair([Int], VPStats); case empty }

final class VPBox {
    var shape: VPShape = .list([1, 2, 3])
    var maybe: [Int]? = [1, 2]
    var stats: VPStats? = VPStats()
    var pairs: [(String, [Int])] = [("a", [1]), ("b", [2])]
    var byName: [String: VPStats] = ["x": VPStats()]
}

private func patternSwitch(_ out: inout [String]) {
    let b = VPBox()
    switch b.shape {
    case .list(var xs):
        xs[0] = 9
        out.append("\(xs)")
    default: break
    }
    switch b.shape {
    case .list(let xs):
        b.shape = .list([0])
        out.append("\(xs)")
    default: break
    }
    b.shape = .pair([1, 2], VPStats())
    if case .pair(let xs, var st) = b.shape {
        st.values[0] = 7
        b.shape = .empty
        out.append("\(xs) \(st.values)")
    }
    if case .empty = b.shape { out.append("empty") }
}

private func patternOptional(_ out: inout [String]) {
    let b = VPBox()
    if let m = b.maybe {
        b.maybe![0] = 5
        b.maybe!.append(3)
        out.append("\(m) \(b.maybe!)")
    }
    guard var s = b.stats else { return }
    s.values[0] = 100
    b.stats?.values[1] = 200
    out.append("\(s.values) \(b.stats!.values)")
    if let st = b.byName["x"] {
        b.byName["x"]!.values[2] = 300
        out.append("\(st.values) \(b.byName["x"]!.values)")
    }
    while var top = b.maybe, top.count > 1 {
        top.removeLast()
        b.maybe = top
    }
    out.append("\(b.maybe!)")
}

private func patternTuple(_ out: inout [String]) {
    let b = VPBox()
    var (name, xs) = b.pairs[0]
    xs.append(9)
    name += "!"
    let (_, ys) = b.pairs[1]
    b.pairs[1].1[0] = 20
    for (n, var zs) in b.pairs {
        zs.append(0)
        b.pairs[0].1.append(1)
        out.append("\(n) \(zs)")
    }
    out.append("\(name) \(xs) \(ys) \(b.pairs[0].1) \(b.pairs[1].1)")
    var a = [1], c = [2]
    (a, c) = (c, a)
    a.append(3)
    swap(&a, &c)
    out.append("\(a) \(c)")
}

public func valuePatterns() {
    var out: [String] = []
    patternSwitch(&out)
    patternOptional(&out)
    patternTuple(&out)
    print(out.joined(separator: "\n"))
}
