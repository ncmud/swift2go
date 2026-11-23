// Copies of Optionals and protocol values share what they hold until one is written through. Each
// check writes through one copy and prints both.

struct CWOStats {
    var hits: [Int]
    var best: Int?
    mutating func record(_ n: Int) {
        hits.append(n)
        if best == nil || n > best! { best = n }
    }
}

enum CWOShape {
    case empty
    case points([Int])
    case named(String, [String])
}

protocol CWOTicker { var count: Int { get }; mutating func tick() }
struct CWOClock: CWOTicker { var count = 0; var log: [Int] = []; mutating func tick() { count += 1; log.append(count) } }
struct CWOFrozen: CWOTicker { let count: Int; let marks: [Int]; func tick() {} }

final class CWOCell { var value = 0 }

struct CWOView {
    let cell: CWOCell
    var value: Int {
        get { cell.value }
        nonmutating set { cell.value = newValue }
    }
}

final class CWOViewed {
    let cell = CWOCell()
    private var backing: CWOStats? = CWOStats(hits: [], best: nil)
    var view: CWOView? { CWOView(cell: cell) }
    var stats: CWOStats? {
        get { backing }
        set { backing = newValue }
    }
}

final class CWOHolder {
    var stats: CWOStats? = CWOStats(hits: [1], best: 1)
    var level: Int? = 1
}

private func cwoStep(_ n: inout Int) -> Int {
    n += 1
    return n
}

private func cwoBump(_ s: inout CWOStats?) {
    s?.hits[0] += 10
    s!.best! += 1
}

public func copyOnWriteOptionals() {
    // Optional structs in variables.
    var a: CWOStats? = CWOStats(hits: [1, 2], best: 2)
    var b = a
    b?.hits[0] = 10
    b!.hits.append(3)
    a?.record(7)
    print("optional", a!.hits, a!.best!, b!.hits, b!.best!)
    var c = b
    c!.best! += 5
    print("optional scalar", b!.best!, c!.best!)

    // Optionals of numbers.
    var n: Int? = 1
    let m = n
    n! += 1
    n! *= 3
    var total: Int? = 0
    for i in 1...4 { total! += i }
    print("numbers", n!, m!, total!)

    // Optionals held by a class, copied out and written on either side.
    let h = CWOHolder()
    var out = h.stats
    h.stats?.hits.append(2)
    out?.best = 99
    h.level! += 4
    let level = h.level
    h.level! -= 1
    print("class", h.stats!.hits, h.stats!.best!, out!.hits, out!.best!, level!, h.level!)

    // inout Optionals.
    var io: CWOStats? = CWOStats(hits: [0], best: 0)
    let ioCopy = io
    cwoBump(&io)
    print("inout", io!.hits, io!.best!, ioCopy!.hits, ioCopy!.best!)

    // A getter's Optional is new; a settable computed Optional is written back.
    let viewed = CWOViewed()
    viewed.view?.value = 4
    viewed.view!.value += 1
    let statsBefore = viewed.stats
    viewed.stats?.hits.append(1)
    viewed.stats!.best = 1
    print("computed", viewed.cell.value, viewed.stats!.hits, viewed.stats!.best!, statsBefore!.hits, statsBefore!.best ?? -1)

    // A write through an Optional in a loop condition or a short-circuit operand happens when it runs.
    var counter: Int? = 0
    var seen: [Int?] = []
    while cwoStep(&counter!) < 4 { seen.append(counter) }
    var slots: [Int?] = [1, 2]
    var i = 0
    var stepped: [Int] = []
    while i < 3 {
        if i < slots.count && cwoStep(&slots[i]!) > 0 { stepped.append(slots[i]!) }
        i += 1
    }
    let logged = [CWOStats(hits: [3], best: 3)]
    let note = "has 3"
    print("library getter", logged.first?.hits.contains(3) == true, logged.first.map { "\($0.hits)" }?.contains(note) == false)
    print("conditions", seen.map { $0! }, counter!, stepped, slots.map { $0! })

    // Optionals inside arrays and dictionaries.
    var arr: [CWOStats?] = [CWOStats(hits: [5], best: nil), nil]
    let arrCopy = arr
    arr[0]?.hits.append(6)
    arr[0]!.best = 6
    arr[1] = CWOStats(hits: [], best: 1)
    print("array", arr.map { $0?.hits ?? [] }, arrCopy.map { $0?.hits ?? [] }, arr[0]!.best ?? -1, arrCopy[0]!.best ?? -1)
    var dict: [String: CWOStats] = ["k": CWOStats(hits: [1], best: 1)]
    let dictCopy = dict
    dict["k"]?.hits.append(2)
    dict["k"]!.best! += 1
    print("dict", dict["k"]!.hits, dict["k"]!.best!, dictCopy["k"]!.hits, dictCopy["k"]!.best!)

    // Enums with payloads.
    var shapes: [CWOShape] = [.points([1, 2]), .named("n", ["a"]), .empty]
    let shapesCopy = shapes
    if case .points(var ps) = shapes[0] {
        ps.append(3)
        shapes[0] = .points(ps)
    }
    shapes[2] = .points([9])
    func show(_ s: [CWOShape]) -> [String] {
        s.map {
            switch $0 {
            case .empty: return "empty"
            case .points(let ps): return "\(ps)"
            case .named(let n, let xs): return "\(n)\(xs)"
            }
        }
    }
    print("enums", show(shapes), show(shapesCopy))

    // Protocol values: one held by pointer for its mutating method, and one that is not.
    var tickers: [CWOTicker] = [CWOClock(), CWOFrozen(count: 5, marks: [1])]
    let tickersCopy = tickers
    tickers[0].tick()
    tickers[0].tick()
    var single: CWOTicker = tickers[0]
    single.tick()
    print("protocols", tickers.map { $0.count }, tickersCopy.map { $0.count }, single.count)
    let boxed: Any = [1, 2, 3]
    var unboxed = boxed as! [Int]
    unboxed[0] = 100
    print("any", boxed, unboxed)
}
