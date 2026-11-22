final class NDLog { nonisolated(unsafe) static var lines: [String] = [] }

func ndNote(_ s: String) { NDLog.lines.append(s) }

enum NDError: Error { case bad }

func ndIf(_ flag: Bool) -> Int {
    var x = 1
    if flag {
        defer { ndNote("if-defer x=\(x)"); x += 100 }
        x = 2
        ndNote("if-body")
    }
    ndNote("after-if x=\(x)")
    return x
}

func ndLoop() -> Int {
    var total = 0
    outer: for i in 0..<5 {
        defer { ndNote("loop-defer \(i)") }
        if i == 1 { continue }
        for j in 0..<3 {
            defer { ndNote("inner-defer \(i).\(j)") }
            if j == 1 { break }
            if i == 3 { continue outer }
            if i == 4 { break outer }
            total += j
        }
        total += i
    }
    return total
}

func ndReturn(_ n: Int) -> String {
    while true {
        defer { ndNote("while-defer") }
        if n > 0 {
            defer { ndNote("nested-if-defer") }
            return "ret \(n)"
        }
        break
    }
    return "fell"
}

func ndSwitch(_ v: Int) -> Int {
    var r = 0
    switch v {
    case 0:
        defer { r = 10; ndNote("case0-defer") }
        r = 1
    case 1:
        defer { ndNote("case1-defer") }
        if r == 0 { break }
        r = 5
    default:
        r = -1
    }
    return r
}

func ndThrow(_ fail: Bool) throws -> Int {
    do {
        defer { ndNote("do-defer") }
        if fail { throw NDError.bad }
        ndNote("do-body")
    }
    return 7
}

func ndOrder() {
    do {
        defer { ndNote("first") }
        defer { ndNote("second") }
        ndNote("body")
    }
    ndNote("after-do")
}

func ndReturnValueBeforeDefer() -> Int {
    var x = 1
    if true {
        defer { x = 99 }
        return x
    }
}

public func nestedDefer() {
    ndNote("ndIf(true)=\(ndIf(true))")
    ndNote("ndIf(false)=\(ndIf(false))")
    ndNote("ndLoop=\(ndLoop())")
    ndNote(ndReturn(3))
    ndNote(ndReturn(0))
    ndNote("switch0=\(ndSwitch(0)) switch1=\(ndSwitch(1)) switch2=\(ndSwitch(2))")
    ndNote("throw=\(String(describing: try? ndThrow(true)))")
    ndNote("nothrow=\(String(describing: try? ndThrow(false)))")
    ndOrder()
    ndNote("rv=\(ndReturnValueBeforeDefer())")
    print(NDLog.lines.joined(separator: "\n"))
}
