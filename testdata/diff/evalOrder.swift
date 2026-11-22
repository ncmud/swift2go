final class EOPurse {
    var gold = 100
    var items = [1, 2, 3]
    func sell() -> Int { gold += 50; return 5 }
    func pop() -> Int { items.removeLast() }
}

struct EOG {
    nonisolated(unsafe) static var gx = 1
    nonisolated(unsafe) static var log = ""
}

func eoBump() -> Int { EOG.gx += 10; return 1 }
func eoIdx(_ tag: String, _ v: Int) -> Int { EOG.log += tag; return v }
func eoThree(_ a: Int, _ b: Int, _ c: Int) -> Int { a * 100 + b * 10 + c }

final class EOBox {
    var n = 0
    var tags: [String: Int] = [:]
    nonisolated(unsafe) static var picks = 0
    nonisolated(unsafe) static let shared = EOBox()
    static func pick() -> EOBox { picks += 1; return shared }
}

public func evalOrder() {
    var out: [String] = []
    let p = EOPurse()
    p.gold = p.gold + p.sell()
    out.append("\(p.gold)")
    p.gold = 100
    p.gold += p.sell()
    out.append("\(p.gold)")
    p.gold = 100
    p.gold = p.sell() + p.gold
    out.append("\(p.gold)")

    var hp = 10
    func regen() -> Int { hp += 5; return hp }
    let sum = hp + regen()
    out.append("\(sum)")
    hp = 10
    out.append("\(max(hp, regen() - 20)) \(hp - regen() * 0)")

    out.append("\(p.items[0] + p.pop() + p.items[0]) \(p.items.count - p.pop())")

    EOG.gx = 1
    EOG.gx = EOG.gx + eoBump()
    out.append("\(EOG.gx)")
    EOG.gx = 1
    EOG.gx = EOG.gx * eoBump() + EOG.gx
    out.append("\(EOG.gx)")

    var idxCount = 0
    var arr = [0, 0, 0]
    func next() -> Int { idxCount += 1; return idxCount - 1 }
    arr[next()] += 7
    arr[next()] -= 2
    arr[eoIdx("i", 2)] = eoIdx("v", 4)
    arr[eoIdx("j", 2)] += eoIdx("w", 3)
    out.append("\(arr) idx=\(idxCount) \(EOG.log)")

    var grid = [[1, 2], [3, 4]]
    grid[next() - 2][next() - 3] *= 10
    out.append("\(grid) idx=\(idxCount)")

    EOG.log = ""
    _ = eoThree(eoIdx("1", 1), eoIdx("2", 2), eoIdx("3", 3))
    out.append(EOG.log)

    var x = 1
    func setX() -> Int { x = 50; return 2 }
    let t = (x, setX(), x)
    out.append("\(t.0) \(t.1) \(t.2)")
    x = 1
    let s = "\(x) \(setX()) \(x)"
    out.append(s)
    x = 1
    let list = [x, setX(), x]
    out.append("\(list)")
    x = 1
    let cmp = x < setX() + x
    out.append("\(cmp)")
    EOBox.pick().n += 5
    EOBox.pick().tags[eoIdx("k", 0) == 0 ? "a" : "b", default: 0] += 2
    out.append("\(EOBox.shared.n) \(EOBox.shared.tags["a"] ?? -1) \(EOBox.shared.tags.count) picks=\(EOBox.picks)")
    print(out.joined(separator: "\n"))
}
