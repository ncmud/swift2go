public struct Catalog {
    public let names: [String]
    public let byID: [Int: String]
}

public final class Service {
    private var cachedCatalog: Catalog?
    private var table: [String] = []
    private var grown: [String] = []
    public var onScan: (() -> Void)?

    public init() {}

    public func catalog() -> Catalog {
        if let cachedCatalog { return cachedCatalog }
        let built = Catalog(names: ["a"], byID: [0: "a"])
        cachedCatalog = built
        return built
    }

    public func names() -> [String] {
        let c = catalog()
        return c.names
    }

    public func rebuild() { table = ["b", "a"].sorted() }
    public func grow() { grown.append("x") }

    public func scanTable() -> Int { Self.scan(table) { self.onScan?() } }
    public func scanGrown() -> Int { Self.scan(grown) { self.onScan?() } }

    static func scan(_ rows: [String], _ midway: () -> Void) -> Int {
        midway()
        return rows.count
    }
}
