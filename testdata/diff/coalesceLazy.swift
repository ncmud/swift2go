final class CLCache {
    var hits = 0
    var store = ["a": 1]
    func compute(_ k: String) -> Int { hits += 1; store[k] = 99; return 99 }
    func get(_ k: String) -> Int { store[k] ?? compute(k) }
}

final class CLThing { let name: String; init(_ n: String) { name = n } }

final class CLLog { nonisolated(unsafe) static var built: [String] = [] }

final class CLScope {
    nonisolated(unsafe) static let shared = clMake("singleton")
    nonisolated(unsafe) static var current: CLThing?
    static var active: CLThing { current ?? shared }
}

func clMake(_ n: String) -> CLThing { CLLog.built.append(n); return CLThing(n) }
func clMaybe(_ n: String?) -> CLThing? { n.map(CLThing.init) }
func clOptInt(_ v: Int?) -> Int? { CLLog.built.append("optInt"); return v }

public func coalesceLazy() {
    var out: [String] = []
    let c = CLCache()
    out.append("\(c.get("a")) hits=\(c.hits)")
    out.append("\(c.get("b")) hits=\(c.hits)")
    let xs: [Int] = []
    let first: Int? = 7
    out.append("\(first ?? xs[0])")
    let none: Int? = nil
    let some: Int? = 3
    out.append("\(some ?? xs[0]) \(none ?? 4)")
    let current: CLThing? = CLThing("override")
    out.append((current ?? clMake("shared")).name)
    CLScope.current = CLThing("scoped")
    out.append(CLScope.active.name)
    let missing: CLThing? = nil
    out.append((missing ?? clMake("fallback")).name)
    out.append((clMaybe("x") ?? clMake("y")).name)
    out.append("\(String(describing: none ?? clOptInt(nil))) \(String(describing: some ?? clOptInt(5)))")
    out.append("\(none ?? some ?? xs[0])")
    var count = 0
    let d: [String: Int] = ["k": 2]
    let v = d["k"] ?? { count += 1; return 0 }()
    out.append("\(v) \(count)")
    let w: Int? = nil
    let total = (w ?? { count += 10; return 1 }()) + (d["z"] ?? xs.first ?? 8)
    out.append("\(total) \(count)")
    let name: String? = "set"
    out.append(name ?? String(repeating: "!", count: xs[0]))
    out.append("built=\(CLLog.built)")
    print(out.joined(separator: "\n"))
}
