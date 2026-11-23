struct SVInner {
    var tags: [String]
}

struct SVCatalog {
    let names: [String]
    let byID: [Int: String]
    let hubs: Set<Int>
    let inner: SVInner

    init(names: [String]) {
        self.names = names
        var by: [Int: String] = [:]
        for (i, n) in names.enumerated() { by[i] = n }
        byID = by
        hubs = Set(names.indices)
        inner = SVInner(tags: names.map { $0.uppercased() })
    }

    mutating func reset() { self = SVCatalog(names: ["reset"]) }

    func describe() -> String {
        "\(names) \(byID.sorted { $0.key < $1.key }.map { "\($0.key)=\($0.value)" }) \(hubs.sorted()) \(inner.tags)"
    }
}

struct SVHolder {
    var catalog: SVCatalog
    var count: Int
}

final class SVService {
    private var cached: SVCatalog?
    private var table: [String] = ["alpha", "beta"]
    private var grown: [String] = ["one", "two"]
    var onScan: (() -> Void)?

    func catalog() -> SVCatalog {
        if let cached { return cached }
        let c = SVCatalog(names: ["a", "b", "c"])
        cached = c
        return c
    }

    func names() -> [String] {
        let c = catalog()
        return c.names
    }

    func replaceTable() { table = table.map { $0 + "!" } + ["gamma"] }
    func growInPlace() { grown.append("three"); grown[0] = "ONE" }

    func scanTable() -> [String] { Self.scan(table) { self.onScan?() } }
    func scanGrown() -> [String] { Self.scan(grown) { self.onScan?() } }

    static func scan(_ rows: [String], _ midway: () -> Void) -> [String] {
        var seen: [String] = [rows[0]]
        midway()
        for r in rows.dropFirst() { seen.append(r) }
        seen.append("count=\(rows.count)")
        return seen
    }

    func cachedDescription() -> String { catalog().describe() }
}

func svMutateParts(_ s: SVService) {
    var c = s.catalog()
    var names = c.names
    names.append("d")
    names[0] = "A"
    var by = c.byID
    by[0] = "zero"
    by[9] = "nine"
    var hubs = c.hubs
    hubs.insert(99)
    var inner = c.inner
    inner.tags.append("EXTRA")
    inner.tags[0] = "FIRST"
    var viaKeyPath = c[keyPath: \SVCatalog.names]
    viaKeyPath.removeAll()
    print("local:", names, by.count, hubs.count, inner.tags, viaKeyPath)
    c.reset()
    print("reset:", c.describe())
}

func svMutateReturned(_ s: SVService) {
    var n = s.names()
    n.append("from names()")
    n[0] = "changed"
    var parts = s.catalog().names
    parts.sort(by: >)
    print("returned:", n, parts)
}

func svHolders(_ s: SVService) {
    var a = SVHolder(catalog: s.catalog(), count: 1)
    var b = a
    b.count = 2
    b.catalog = SVCatalog(names: ["x"])
    var list = [a.catalog, b.catalog]
    list[0] = SVCatalog(names: ["y"])
    a.count += 10
    print("holders:", a.count, a.catalog.names, b.count, b.catalog.names, list.map(\.names))
    var copies = [s.catalog(), s.catalog()].filter { !$0.names.isEmpty }
    copies[1] = SVCatalog(names: ["z"])
    var swapped = s.catalog()
    var other = SVCatalog(names: ["w"])
    swap(&swapped, &other)
    print("copies:", copies.map(\.names), swapped.names, other.names)
}

public func sharedValueMutation() {
    let s = SVService()
    print("cache:", s.cachedDescription())
    svMutateParts(s)
    print("cache:", s.cachedDescription())
    svMutateReturned(s)
    print("cache:", s.cachedDescription())
    svHolders(s)
    print("cache:", s.cachedDescription())

    // A callee keeps the rows it was lent while the property is replaced, or grown in place.
    s.onScan = { s.replaceTable() }
    print("table scan:", s.scanTable())
    print("table after:", s.scanTable())
    s.onScan = { s.growInPlace() }
    print("grown scan:", s.scanGrown())
    print("grown after:", s.scanGrown())
}
