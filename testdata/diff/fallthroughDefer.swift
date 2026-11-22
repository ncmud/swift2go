// A case whose body holds a defer runs the defer when it falls through, before the next case.

func ftSwitch(_ v: Int) -> [String] {
    var log: [String] = []
    switch v {
    case 0:
        defer { log.append("defer0") }
        log.append("case0")
        fallthrough
    case 1:
        defer { log.append("defer1") }
        if v == 1 {
            log.append("early1")
            break
        }
        log.append("case1")
        fallthrough
    case 2:
        log.append("case2")
    default:
        log.append("default")
    }
    return log
}

func ftReturn(_ v: Int) -> String {
    var log: [String] = []
    switch v {
    case 0:
        defer { log.append("d0") }
        if log.isEmpty { log.append("c0") }
        fallthrough
    case 1:
        return (log + ["c1"]).joined(separator: ",")
    default:
        break
    }
    return "none"
}

public func fallthroughDefer() {
    for v in 0...3 {
        print(v, ftSwitch(v), ftReturn(v))
    }
}
