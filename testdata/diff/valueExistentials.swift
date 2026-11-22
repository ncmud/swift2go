// A struct stored in a protocol-typed variable is still a value.

protocol EXEffect { var turns: Int { get }; mutating func tick() }
struct EXBurn: EXEffect { var turns = 3; mutating func tick() { turns -= 1 } }

private func existentialCopy(_ out: inout [String]) {
    let a: EXEffect = EXBurn()
    var b = a
    b.tick(); b.tick()
    var list: [EXEffect] = [EXBurn()]
    let saved = list
    list[0].tick()
    out.append("\(a.turns) \(b.turns) \(saved[0].turns) \(list[0].turns)")
}

protocol EXCounter { var n: Int { get }; mutating func inc(); mutating func setN(_ v: Int) }
struct EXStructCounter: EXCounter {
    var n = 0
    var hist: [Int] = []
    mutating func setN(_ v: Int) { n = v }
    mutating func inc() { n += 1; hist.append(n) }
}
final class EXClassCounter: EXCounter {
    var n = 0
    func setN(_ v: Int) { n = v }
    func inc() { n += 1 }
}

final class EXHolder { var c: EXCounter = EXStructCounter() }

private func existentialStructAndClass(_ out: inout [String]) {
    var a: EXCounter = EXStructCounter()
    var b = a
    b.inc(); b.inc()
    a.setN(10)
    out.append("\(a.n) \(b.n)")
    var arr: [EXCounter] = [EXStructCounter(), EXClassCounter()]
    var arr2 = arr
    arr2[0].inc(); arr2[1].inc()
    arr[0].setN(arr[0].n + 5)
    out.append("\(arr[0].n) \(arr[1].n) \(arr2[0].n) \(arr2[1].n)")
    for var c in arr { c.inc() }
    out.append("\(arr[0].n) \(arr[1].n)")
    let h = EXHolder()
    var copy = h.c
    copy.inc()
    out.append("\(h.c.n) \(copy.n)")
}

public func valueExistentials() {
    var out: [String] = []
    existentialCopy(&out)
    existentialStructAndClass(&out)
    print(out.joined(separator: "\n"))
}
