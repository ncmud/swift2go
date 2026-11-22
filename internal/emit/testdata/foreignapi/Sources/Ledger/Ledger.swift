// Ledger stands in for a third-party library: the test maps it to the hand-written Go package in go/ledger.

public final class Book {
    public var entries: [String] = []

    public init() {}

    public var count: Int { entries.count }

    public func write<T>(_ body: (Book) throws -> T) rethrows -> T { try body(self) }
}

public protocol Expressible {
    var sql: String { get }
}

extension Int: Expressible {
    public var sql: String { String(self) }
}

extension String: Expressible {
    public var sql: String { "'\(self)'" }
}

public struct Expr: Expressible {
    public var sql: String
}

public protocol Specific: Expressible {}

public struct Column: Specific {
    public var name: String

    public init(_ name: String) { self.name = name }

    public var sql: String { name }
}

extension Expr: Specific {}

extension Specific {
    public static func == (lhs: Self, rhs: (any Expressible)?) -> Expr {
        Expr(sql: rhs.map { "\(lhs.sql) = \($0.sql)" } ?? "\(lhs.sql) IS NULL")
    }

    public static func == (lhs: Self, rhs: Bool) -> Expr { Expr(sql: "\(lhs.sql) = \(rhs ? 1 : 0)") }

    public static func && (lhs: Self, rhs: Expr) -> Expr { Expr(sql: "(\(lhs.sql) AND \(rhs.sql))") }

    public var desc: Expr { Expr(sql: "\(sql) DESC") }
}

public func || (lhs: Expr, rhs: Expr) -> Expr { Expr(sql: "(\(lhs.sql) OR \(rhs.sql))") }

public prefix func ! (e: Expr) -> Expr { Expr(sql: "NOT \(e.sql)") }

extension Sequence where Element: Expressible {
    public func contains(_ column: Column) -> Expr {
        Expr(sql: "\(column.sql) IN (\(map(\.sql).joined(separator: ", ")))")
    }
}

public struct Query<T> {
    public var sql: String

    public func limit(_ n: Int, offset: Int? = nil) -> Query<T> {
        Query(sql: sql + " LIMIT \(n)" + (offset.map { " OFFSET \($0)" } ?? ""))
    }
}

public protocol Table {
    static var tableName: String { get }
}

extension Table {
    public static var tableName: String { "\(Self.self)" }

    public static func filter(_ e: some Specific) -> Query<Self> {
        Query(sql: "SELECT * FROM \(tableName) WHERE \(e.sql)")
    }
}

public protocol Persistable: Table {
    mutating func didInsert(_ id: Int64)
}

extension Persistable {
    public mutating func didInsert(_ id: Int64) {}

    public mutating func insert(_ book: Book) {
        book.entries.append(Self.tableName)
        didInsert(Int64(book.entries.count))
    }
}

public struct Row {
    var values: [String: Any]

    public init(_ values: [String: Any]) { self.values = values }

    public subscript<V>(_ name: String) -> V { values[name] as! V }
}

public struct Arguments: ExpressibleByArrayLiteral, ExpressibleByDictionaryLiteral {
    public var text: String

    public init(arrayLiteral elements: Int...) { text = elements.map(String.init).joined(separator: ",") }

    public init(dictionaryLiteral elements: (String, Int)...) {
        text = elements.map { "\($0.0)=\($0.1)" }.joined(separator: ",")
    }
}

public struct Options: OptionSet, Sendable {
    public var rawValue: Int

    public init(rawValue: Int) { self.rawValue = rawValue }

    public static let strict = Options(rawValue: 1)
    public static let temporary = Options(rawValue: 2)
}

public struct Config {
    public var timeout: Int = 0

    public init() {}
}

public enum Mode {
    case fast
    case safe(retries: Int)

    public var retries: Int {
        switch self {
        case .fast: return 0
        case .safe(let n): return n
        }
    }
}

public struct Code: Equatable, Sendable {
    public var rawValue: Int

    public static let busy = Code(rawValue: 5)
}
