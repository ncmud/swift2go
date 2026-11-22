// A closure that outlives its creator keeps the values its constants had when it was made.

struct VCConfig { var limits: [Int] = [1, 2, 3] }

final class VCHost {
    var items: [Int] = [1, 2, 3]
    var config = VCConfig()
    var callbacks: [() -> [Int]] = []
    func counter(_ xs: [Int]) -> () -> [Int] { return { xs } }
    func snapshotter() -> () -> [Int] { let xs = items; return { xs } }
    func configured() -> () -> [Int] { let c = config; return { c.limits } }
    func remember(_ xs: [Int]) { callbacks.append { xs + self.items } }
    func total() -> Int { let xs = items; return xs.reduce(0, +) + config.limits.count }
    func scaled(by k: Int) -> [Int] { let xs = items; return xs.map { $0 * k } }
}

struct VCBag {
    var items: [Int] = [1, 2]
    func reader() -> () -> [Int] { { items } }
    func counts() -> [Int] { items.map { $0 + items.count } }
}

private func closuresKeepConstants(_ out: inout [String]) {
    let h = VCHost()
    var mine = [7, 8]
    let read = h.counter(mine)
    mine[0] = 70
    let snap = h.snapshotter()
    let conf = h.configured()
    h.remember(mine)
    h.items[0] = 100
    h.items.append(4)
    h.config.limits[1] = 200
    mine.append(9)
    out.append("\(read()) \(snap()) \(conf()) \(h.callbacks[0]()) \(mine)")
    out.append("\(h.total()) \(h.scaled(by: 2))")
}

private func closuresOverStructs(_ out: inout [String]) {
    var bag = VCBag()
    let read = bag.reader()
    bag.items[0] = 10
    bag.items.append(3)
    out.append("\(read()) \(bag.items) \(bag.counts())")
}

private func closuresShareVariables(_ out: inout [String]) {
    var log: [Int] = []
    let add: (Int) -> Void = { log.append($0) }
    let peek = { log }
    add(1)
    let first = peek()
    add(2)
    log[0] = 10
    var made: [() -> Int] = []
    for i in 0..<3 {
        let row = [i, i * 2]
        made.append { row[1] }
    }
    out.append("\(first) \(peek()) \(log) \(made.map { $0() })")
}

public func valueClosures() {
    var out: [String] = []
    closuresKeepConstants(&out)
    closuresOverStructs(&out)
    closuresShareVariables(&out)
    print(out.joined(separator: "\n"))
}
